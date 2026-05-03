package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/krakenkey/probe/internal/config"
	"github.com/krakenkey/probe/internal/health"
	"github.com/krakenkey/probe/internal/reporter"
	"github.com/krakenkey/probe/internal/scheduler"
	"github.com/krakenkey/probe/internal/state"
)

var version = "dev"

func main() {
	cfgPath := flag.String("config", "", "path to probe.yaml config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	healthcheck := flag.Bool("healthcheck", false, "check health endpoint and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("krakenkey-probe %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logger := setupLogger(cfg.Logging.Level, cfg.Logging.Format)

	probeID, err := state.LoadOrCreate(cfg)
	if err != nil {
		logger.Error("failed to initialize probe ID", "error", err)
		os.Exit(1)
	}

	logger.Info("krakenkey-probe starting",
		"version", version,
		"probeId", probeID,
		"mode", cfg.Probe.Mode,
		"region", cfg.Probe.Region,
		"endpoints", len(cfg.Endpoints),
		"interval", cfg.Probe.Interval.String(),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var rep *reporter.Reporter
	if cfg.Probe.Mode != "standalone" {
		rep = reporter.New(cfg.API.URL, cfg.API.Key, version, runtime.GOOS, runtime.GOARCH)

		regInfo := reporter.ProbeInfo{
			ProbeID: probeID,
			Name:    cfg.Probe.Name,
			Version: version,
			Mode:    cfg.Probe.Mode,
			Region:  cfg.Probe.Region,
			OS:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		}
		if err := rep.Register(ctx, regInfo); err != nil {
			logger.Warn("failed to register with API (will retry on next cycle)", "error", err)
		} else {
			logger.Info("registered with API")
		}
	} else {
		logger.Info("standalone mode: no API communication")
	}

	// Start health server
	var healthSrv *health.Server
	if cfg.Health.Enabled {
		healthSrv = health.New(cfg.Health.Port, version, probeID, cfg.Probe.Mode, cfg.Probe.Region, cfg.ScanAPI)
		go func() {
			logger.Info("health server starting", "port", cfg.Health.Port)
			if err := healthSrv.Start(); err != nil {
				logger.Error("health server error", "error", err)
			}
		}()
	}

	// Run scan scheduler
	sched := scheduler.New(cfg, probeID, rep, healthSrv, logger, version)
	sched.Run(ctx)

	// Graceful shutdown
	if healthSrv != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*1e9) // 5s
		defer shutdownCancel()
		if err := healthSrv.Shutdown(shutdownCtx); err != nil {
			logger.Error("health server shutdown error", "error", err)
		}
	}

	logger.Info("krakenkey-probe stopped")
}

func runHealthcheck() int {
	resp, err := http.Get("http://localhost:8080/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return 0
	}
	return 1
}

func setupLogger(level, format string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
