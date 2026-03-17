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

func (s *Scheduler) Run(ctx context.Context) {
	s.runCycle(ctx)

	if s.health != nil {
		s.health.SetReady()
	}

	interval := s.cfg.Probe.Interval
	if interval == 0 {
		interval = 60 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopping")
			return
		case <-ticker.C:
			s.runCycle(ctx)
		}
	}
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

	interval := s.cfg.Probe.Interval
	if interval == 0 {
		interval = 60 * time.Minute
	}
	if s.health != nil {
		s.health.UpdateScanTimes(now, now.Add(interval))
	}
}

func (s *Scheduler) resolveEndpoints(ctx context.Context) []scanner.EndpointResult {
	if s.cfg.Probe.Mode == "hosted" {
		return s.fetchHostedEndpoints(ctx)
	}

	endpoints := make([]scanner.EndpointResult, len(s.cfg.Endpoints))
	for i, ep := range s.cfg.Endpoints {
		endpoints[i] = scanner.EndpointFromConfig(ep)
	}
	return endpoints
}

var cachedHostedEndpoints []scanner.EndpointResult

func (s *Scheduler) fetchHostedEndpoints(ctx context.Context) []scanner.EndpointResult {
	cfg, err := s.reporter.FetchHostedConfig(ctx, s.probeID)
	if err != nil {
		s.logger.Error("failed to fetch hosted config, using cached endpoints", "error", err)
		return cachedHostedEndpoints
	}

	if len(cfg.Endpoints) == 0 {
		s.logger.Warn("hosted config returned empty endpoint list")
		cachedHostedEndpoints = nil
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

	cachedHostedEndpoints = endpoints
	return endpoints
}
