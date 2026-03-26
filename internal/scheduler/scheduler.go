package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/krakenkey/probe/internal/config"
	"github.com/krakenkey/probe/internal/health"
	"github.com/krakenkey/probe/internal/reporter"
	"github.com/krakenkey/probe/internal/scanner"
)

type Scheduler struct {
	cfg      *config.Config
	probeID  string
	reporter *reporter.Reporter
	health   *health.Server
	logger   *slog.Logger
	version  string
}

func New(cfg *config.Config, probeID string, rep *reporter.Reporter, h *health.Server, logger *slog.Logger, version string) *Scheduler {
	return &Scheduler{
		cfg:      cfg,
		probeID:  probeID,
		reporter: rep,
		health:   h,
		logger:   logger,
		version:  version,
	}
}

const configPollInterval = 60 * time.Second

func (s *Scheduler) Run(ctx context.Context) {
	s.runCycle(ctx)

	if s.health != nil {
		s.health.SetReady()
	}

	interval := s.cfg.Probe.Interval
	if interval == 0 {
		interval = 60 * time.Minute
	}

	scanTicker := time.NewTicker(interval)
	defer scanTicker.Stop()

	// For connected/hosted modes, poll config every 60s and trigger
	// an immediate scan when the endpoint list changes.
	if s.usesRemoteConfig() {
		configTicker := time.NewTicker(configPollInterval)
		defer configTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.logger.Info("scheduler stopping")
				return
			case <-scanTicker.C:
				s.runCycle(ctx)
			case <-configTicker.C:
				if s.checkConfigChanged(ctx) {
					s.logger.Info("endpoint config changed, triggering immediate scan")
					s.runCycle(ctx)
					scanTicker.Reset(interval)
				}
			}
		}
	}

	// Standalone: just scan on interval
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

func (s *Scheduler) usesRemoteConfig() bool {
	return s.cfg.Probe.Mode == "connected" || s.cfg.Probe.Mode == "hosted"
}

// checkConfigChanged fetches the latest endpoint list and returns true if it
// differs from the cached list.
func (s *Scheduler) checkConfigChanged(ctx context.Context) bool {
	if s.reporter == nil {
		return false
	}

	cfg, err := s.reporter.FetchHostedConfig(ctx, s.probeID)
	if err != nil {
		s.logger.Debug("config poll failed", "error", err)
		return false
	}

	newEndpoints := make([]scanner.EndpointResult, len(cfg.Endpoints))
	for i, ep := range cfg.Endpoints {
		sni := ep.SNI
		if sni == "" {
			sni = ep.Host
		}
		newEndpoints[i] = scanner.EndpointResult{
			Host: ep.Host,
			Port: ep.Port,
			SNI:  sni,
		}
	}

	if endpointsEqual(cachedRemoteEndpoints, newEndpoints) {
		return false
	}

	cachedRemoteEndpoints = newEndpoints
	return true
}

func endpointsEqual(a, b []scanner.EndpointResult) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Host != b[i].Host || a[i].Port != b[i].Port || a[i].SNI != b[i].SNI {
			return false
		}
	}
	return true
}

func (s *Scheduler) runCycle(ctx context.Context) {
	now := time.Now().UTC()
	s.logger.Info("scan cycle starting", "time", now.Format(time.RFC3339))

	endpoints := s.resolveEndpoints(ctx)
	if len(endpoints) == 0 {
		s.logger.Warn("no endpoints to scan, skipping cycle")
		return
	}

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

	interval := s.cfg.Probe.Interval
	if interval == 0 {
		interval = 60 * time.Minute
	}
	if s.health != nil {
		s.health.UpdateScanTimes(now, now.Add(interval))
	}
}

func (s *Scheduler) resolveEndpoints(ctx context.Context) []scanner.EndpointResult {
	switch s.cfg.Probe.Mode {
	case "hosted", "connected":
		return s.fetchRemoteEndpoints(ctx)
	default:
		endpoints := make([]scanner.EndpointResult, len(s.cfg.Endpoints))
		for i, ep := range s.cfg.Endpoints {
			endpoints[i] = scanner.EndpointFromConfig(ep)
		}
		return endpoints
	}
}

var cachedRemoteEndpoints []scanner.EndpointResult

func (s *Scheduler) fetchRemoteEndpoints(ctx context.Context) []scanner.EndpointResult {
	cfg, err := s.reporter.FetchHostedConfig(ctx, s.probeID)
	if err != nil {
		s.logger.Error("failed to fetch config from API, using cached endpoints", "error", err, "mode", s.cfg.Probe.Mode)
		return cachedRemoteEndpoints
	}

	if len(cfg.Endpoints) == 0 {
		s.logger.Warn("API config returned empty endpoint list", "mode", s.cfg.Probe.Mode)
		cachedRemoteEndpoints = nil
		return nil
	}

	endpoints := make([]scanner.EndpointResult, len(cfg.Endpoints))
	for i, ep := range cfg.Endpoints {
		sni := ep.SNI
		if sni == "" {
			sni = ep.Host
		}
		endpoints[i] = scanner.EndpointResult{
			Host: ep.Host,
			Port: ep.Port,
			SNI:  sni,
		}
	}

	cachedRemoteEndpoints = endpoints
	return endpoints
}
