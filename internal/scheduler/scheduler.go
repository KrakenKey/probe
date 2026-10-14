package scheduler

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/krakenkey/probe/internal/config"
	"github.com/krakenkey/probe/internal/health"
	"github.com/krakenkey/probe/internal/reporter"
	"github.com/krakenkey/probe/internal/scanner"
)

const (
	// configPollInterval is how often connected and hosted probes fetch
	// their endpoint list from the API.
	configPollInterval = 60 * time.Second

	// scanTick is how often connected and hosted probes check which
	// endpoints are due. It is half the shortest interval the API hands
	// out (1m), so a 1m endpoint is scanned on every second tick. A tick
	// with nothing due makes no network calls.
	scanTick = 30 * time.Second

	// dueSlack lets an endpoint count as due slightly before its interval
	// has fully elapsed. Ticks and scan start times jitter by a few
	// milliseconds; without slack an endpoint scanned at T would be
	// checked at T+interval-1ms, look not yet due, and wait a whole extra
	// tick. It is kept well below scanTick so scans are never a tick early.
	dueSlack = 5 * time.Second

	// minEndpointInterval and maxEndpointInterval are the bounds the probe
	// accepts for its own interval setting; API-provided intervals are
	// clamped to the same range.
	minEndpointInterval = time.Minute
	maxEndpointInterval = 24 * time.Hour

	defaultInterval = 60 * time.Minute
)

type Scheduler struct {
	cfg      *config.Config
	probeID  string
	reporter *reporter.Reporter
	health   *health.Server
	logger   *slog.Logger
	version  string

	// State for connected and hosted modes. Only the Run goroutine
	// touches it, so it needs no locking.

	// remote is the deduplicated endpoint list from the last successful
	// config fetch. It is kept when a fetch fails.
	remote []remoteEndpoint
	// configLoaded is true once a config fetch has succeeded.
	configLoaded bool
	// lastScan holds the start time of the last scan per endpoint key.
	// Endpoints that leave the config are removed.
	lastScan map[string]time.Time
	// lastCycle is the start time of the most recent scan cycle, for the
	// health endpoint.
	lastCycle time.Time
	// loggedIntervals remembers which interval problems were already
	// logged so a bad or missing value is reported once, not every poll.
	loggedIntervals map[string]bool
}

// remoteEndpoint is an endpoint from the API config together with the
// interval the probe scans it on.
type remoteEndpoint struct {
	key      string
	endpoint scanner.EndpointResult
	interval time.Duration
}

func New(cfg *config.Config, probeID string, rep *reporter.Reporter, h *health.Server, logger *slog.Logger, version string) *Scheduler {
	return &Scheduler{
		cfg:             cfg,
		probeID:         probeID,
		reporter:        rep,
		health:          h,
		logger:          logger,
		version:         version,
		lastScan:        make(map[string]time.Time),
		loggedIntervals: make(map[string]bool),
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	if s.usesRemoteConfig() {
		s.runRemote(ctx)
		return
	}
	s.runStandalone(ctx)
}

// runStandalone scans every local endpoint on the local interval.
func (s *Scheduler) runStandalone(ctx context.Context) {
	s.runCycle(ctx)

	if s.health != nil {
		s.health.SetReady()
	}

	scanTicker := time.NewTicker(s.localInterval())
	defer scanTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopping")
			return
		case <-scanTicker.C:
			s.runCycle(ctx)
		}
	}
}

// runRemote handles connected and hosted modes. Each endpoint has its own
// interval (see effectiveInterval). Every scanTick the probe scans the
// endpoints that are due; every configPollInterval it refreshes the endpoint
// list and scans new endpoints right away.
func (s *Scheduler) runRemote(ctx context.Context) {
	s.refreshConfig(ctx)
	s.runDue(ctx, time.Now())

	if s.health != nil {
		s.health.SetReady()
	}

	scanTicker := time.NewTicker(scanTick)
	defer scanTicker.Stop()
	configTicker := time.NewTicker(configPollInterval)
	defer configTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopping")
			return
		case <-scanTicker.C:
			s.runDue(ctx, time.Now())
		case <-configTicker.C:
			if added := s.refreshConfig(ctx); added > 0 {
				s.logger.Info("endpoint config changed, scanning new endpoints", "new", added)
				s.runDue(ctx, time.Now())
			}
		}
	}
}

func (s *Scheduler) usesRemoteConfig() bool {
	return s.cfg.Probe.Mode == "connected" || s.cfg.Probe.Mode == "hosted"
}

func (s *Scheduler) localInterval() time.Duration {
	if s.cfg.Probe.Interval <= 0 {
		return defaultInterval
	}
	return s.cfg.Probe.Interval
}

func endpointKey(ep scanner.EndpointResult) string {
	return ep.Host + ":" + strconv.Itoa(ep.Port) + ":" + ep.SNI
}

// refreshConfig fetches the endpoint list from the API and replaces the
// cached list. On failure the cached list is kept. Scan state for endpoints
// that are no longer listed is dropped. It returns the number of endpoints
// that were not in the previous list; those have never been scanned and are
// due at once.
func (s *Scheduler) refreshConfig(ctx context.Context) int {
	if s.reporter == nil {
		return 0
	}

	cfg, err := s.reporter.FetchHostedConfig(ctx, s.probeID)
	if err != nil {
		if len(s.remote) > 0 {
			s.logger.Warn("failed to fetch config from API, using cached endpoints", "error", err, "mode", s.cfg.Probe.Mode)
		} else {
			s.logger.Error("failed to fetch config from API", "error", err, "mode", s.cfg.Probe.Mode)
		}
		return 0
	}

	// Warn about an empty list on the first fetch and whenever the list
	// becomes empty, not on every poll.
	if len(cfg.Endpoints) == 0 && (!s.configLoaded || len(s.remote) > 0) {
		s.logger.Warn("API config returned empty endpoint list", "mode", s.cfg.Probe.Mode)
	}
	s.configLoaded = true

	// The same host:port:sni can appear more than once in hosted mode
	// (different users monitoring the same endpoint). The API applies one
	// result to every matching endpoint, so scan it once, on the shortest
	// interval among the duplicates.
	endpoints := make([]remoteEndpoint, 0, len(cfg.Endpoints))
	index := make(map[string]int, len(cfg.Endpoints))
	for _, ep := range cfg.Endpoints {
		sni := ep.SNI
		if sni == "" {
			sni = ep.Host
		}
		res := scanner.EndpointResult{Host: ep.Host, Port: ep.Port, SNI: sni}
		key := endpointKey(res)
		interval := s.effectiveInterval(ep.Interval)

		if i, ok := index[key]; ok {
			if interval < endpoints[i].interval {
				endpoints[i].interval = interval
			}
			continue
		}
		index[key] = len(endpoints)
		endpoints = append(endpoints, remoteEndpoint{key: key, endpoint: res, interval: interval})
	}

	added := 0
	for key := range index {
		if _, ok := s.lastScan[key]; !ok {
			added++
		}
	}
	for key := range s.lastScan {
		if _, ok := index[key]; !ok {
			delete(s.lastScan, key)
		}
	}

	s.remote = endpoints
	return added
}

// effectiveInterval turns the interval the API sent for one endpoint into
// the interval the probe scans it on.
//
//   - hosted: the API value, since KrakenKey runs the probe and the value
//     reflects the endpoint owner's plan.
//   - connected: the API value is the shortest interval the owner's plan
//     allows (the API drops results that arrive sooner), so it is a floor:
//     the local interval applies when it is slower.
//
// A missing, unparseable or non-positive value falls back to the local
// interval. Values outside 1m..24h are clamped. The top-level "interval"
// in the config response is ignored, so a probe talking to an API without
// per-endpoint intervals behaves as before.
func (s *Scheduler) effectiveInterval(raw string) time.Duration {
	local := s.localInterval()

	if raw == "" {
		s.logIntervalOnce("missing", slog.LevelDebug,
			"API config has no per-endpoint interval, using the local interval",
			"interval", local.String())
		return local
	}

	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		s.logIntervalOnce("invalid:"+raw, slog.LevelWarn,
			"ignoring invalid endpoint interval from API, using the local interval",
			"value", raw, "interval", local.String())
		return local
	}

	if clamped := min(max(d, minEndpointInterval), maxEndpointInterval); clamped != d {
		s.logIntervalOnce("clamped:"+raw, slog.LevelDebug,
			"endpoint interval from API out of range, clamping",
			"value", raw, "interval", clamped.String())
		d = clamped
	}

	if s.cfg.Probe.Mode == "connected" && local > d {
		return local
	}
	return d
}

func (s *Scheduler) logIntervalOnce(key string, level slog.Level, msg string, args ...any) {
	if s.loggedIntervals[key] {
		return
	}
	s.loggedIntervals[key] = true
	s.logger.Log(context.Background(), level, msg, args...)
}

// isDue reports whether an endpoint should be scanned at now.
func (s *Scheduler) isDue(ep remoteEndpoint, now time.Time) bool {
	last, ok := s.lastScan[ep.key]
	if !ok {
		return true
	}
	return now.Sub(last) >= ep.interval-dueSlack
}

// dueEndpoints returns the cached endpoints that are due at now.
func (s *Scheduler) dueEndpoints(now time.Time) []remoteEndpoint {
	var due []remoteEndpoint
	for _, ep := range s.remote {
		if s.isDue(ep, now) {
			due = append(due, ep)
		}
	}
	return due
}

// nextDue returns the earliest time any cached endpoint becomes due. It
// returns the zero time when there are no endpoints.
func (s *Scheduler) nextDue(now time.Time) time.Time {
	var next time.Time
	for _, ep := range s.remote {
		at := now
		if last, ok := s.lastScan[ep.key]; ok {
			at = last.Add(ep.interval)
		}
		if next.IsZero() || at.Before(next) {
			next = at
		}
	}
	return next
}

// runDue scans and reports the endpoints that are due at now.
func (s *Scheduler) runDue(ctx context.Context, now time.Time) {
	now = now.UTC()
	due := s.dueEndpoints(now)
	if len(due) == 0 {
		s.updateHealth(now)
		return
	}

	s.logger.Info("scan cycle starting", "time", now.Format(time.RFC3339), "due", len(due), "total", len(s.remote))

	endpoints := make([]scanner.EndpointResult, len(due))
	for i, ep := range due {
		endpoints[i] = ep.endpoint
		s.lastScan[ep.key] = now
	}

	s.scanAndReport(ctx, now, endpoints)
	s.lastCycle = now
	s.updateHealth(now)
}

func (s *Scheduler) updateHealth(now time.Time) {
	if s.health == nil || s.lastCycle.IsZero() {
		return
	}
	next := s.nextDue(now)
	if next.IsZero() {
		next = now.Add(scanTick)
	}
	s.health.UpdateScanTimes(s.lastCycle, next)
}

// runCycle scans every local endpoint once (standalone mode).
func (s *Scheduler) runCycle(ctx context.Context) {
	now := time.Now().UTC()
	s.logger.Info("scan cycle starting", "time", now.Format(time.RFC3339))

	endpoints := s.resolveEndpoints()
	if len(endpoints) == 0 {
		s.logger.Warn("no endpoints to scan, skipping cycle")
		return
	}

	s.scanAndReport(ctx, now, endpoints)

	if s.health != nil {
		s.health.UpdateScanTimes(now, now.Add(s.localInterval()))
	}
}

func (s *Scheduler) scanAndReport(ctx context.Context, now time.Time, endpoints []scanner.EndpointResult) {
	results := scanner.ScanAll(ctx, endpoints, s.cfg.Probe.Timeout)

	for _, r := range results {
		if r.Connection.Success {
			days := 0
			if r.Certificate != nil {
				days = r.Certificate.DaysUntilExpiry
			}
			s.logger.Info("endpoint scanned",
				"host", r.Endpoint.Host,
				"port", r.Endpoint.Port,
				"tls", r.Connection.TLSVersion,
				"daysUntilExpiry", days,
			)
		} else {
			errMsg := ""
			if r.Connection.Error != nil {
				errMsg = *r.Connection.Error
			}
			s.logger.Warn("endpoint scan failed",
				"host", r.Endpoint.Host,
				"port", r.Endpoint.Port,
				"error", errMsg,
			)
		}
	}

	if s.cfg.Probe.Mode != "standalone" && s.reporter != nil {
		report := reporter.ScanReport{
			ProbeID:   s.probeID,
			Mode:      s.cfg.Probe.Mode,
			Region:    s.cfg.Probe.Region,
			Timestamp: now,
			Results:   results,
		}

		if err := s.reporter.Report(ctx, report); err != nil {
			s.logger.Error("failed to send report", "error", err)
		} else {
			s.logger.Info("report sent", "endpoints", len(results))
		}
	} else {
		s.logger.Info("scan cycle complete (standalone, no API report)", "endpoints", len(results))
	}
}

// resolveEndpoints returns the local endpoints (standalone mode).
func (s *Scheduler) resolveEndpoints() []scanner.EndpointResult {
	endpoints := make([]scanner.EndpointResult, len(s.cfg.Endpoints))
	for i, ep := range s.cfg.Endpoints {
		endpoints[i] = scanner.EndpointFromConfig(ep)
	}
	return endpoints
}
