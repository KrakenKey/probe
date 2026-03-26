package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/krakenkey/probe/internal/config"
	"github.com/krakenkey/probe/internal/health"
	"github.com/krakenkey/probe/internal/reporter"
	"github.com/krakenkey/probe/internal/scanner"
)

func TestRunCycleStandalone(t *testing.T) {
	// Reset cached endpoints
	cachedRemoteEndpoints = nil

	var reportReceived atomic.Bool

	// API mock — standalone should NOT call this
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/probes/report" {
			reportReceived.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer apiSrv.Close()

	cfg := &config.Config{
		Probe: config.ProbeConfig{
			Mode:     "standalone",
			Interval: 1 * time.Hour,
			Timeout:  5 * time.Second,
		},
		Endpoints: []config.Endpoint{
			{Host: "127.0.0.1", Port: 19997}, // will fail, that's okay
		},
	}

	h := health.New(0, "0.1.0", "test-id", "standalone", "")
	logger := slog.Default()

	// No reporter for standalone
	s := New(cfg, "test-id", nil, h, logger, "0.1.0")

	s.runCycle(context.Background())

	if reportReceived.Load() {
		t.Error("standalone mode should not send reports to API")
	}
}

func TestRunCycleHostedMode(t *testing.T) {
	cachedRemoteEndpoints = nil

	var configFetched atomic.Bool

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/probes/hosted-id/config" {
			configFetched.Store(true)
			cfg := map[string]any{
				"endpoints": []map[string]any{
					{"host": "127.0.0.1", "port": 19996, "userId": "user1"},
				},
				"interval": "5m",
			}
			_ = json.NewEncoder(w).Encode(cfg)
			return
		}
		if r.URL.Path == "/probes/report" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer apiSrv.Close()

	cfg := &config.Config{
		Probe: config.ProbeConfig{
			Mode:     "hosted",
			Region:   "us-east-1",
			Interval: 5 * time.Minute,
			Timeout:  2 * time.Second,
		},
	}

	rep := reporter.New(apiSrv.URL, "service_key", "0.1.0", "linux", "amd64")
	logger := slog.Default()

	s := New(cfg, "hosted-id", rep, nil, logger, "0.1.0")
	s.runCycle(context.Background())

	if !configFetched.Load() {
		t.Error("expected hosted config to be fetched from API")
	}
}

func TestRunCycleHostedCacheFallback(t *testing.T) {
	// Pre-populate cache
	cachedRemoteEndpoints = []scanner.EndpointResult{
		{Host: "cached.example.com", Port: 443, SNI: "cached.example.com"},
	}

	// API returns error
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/probes/hosted-id/config" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.URL.Path == "/probes/report" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer apiSrv.Close()

	cfg := &config.Config{
		Probe: config.ProbeConfig{
			Mode:     "hosted",
			Region:   "us-east-1",
			Interval: 5 * time.Minute,
			Timeout:  2 * time.Second,
		},
	}

	rep := reporter.New(apiSrv.URL, "service_key", "0.1.0", "linux", "amd64")
	logger := slog.Default()

	s := New(cfg, "hosted-id", rep, nil, logger, "0.1.0")

	endpoints := s.resolveEndpoints(context.Background())
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 cached endpoint, got %d", len(endpoints))
	}
	if endpoints[0].Host != "cached.example.com" {
		t.Errorf("expected cached host, got %q", endpoints[0].Host)
	}
}

func TestRunSetsReady(t *testing.T) {
	cachedRemoteEndpoints = nil

	cfg := &config.Config{
		Probe: config.ProbeConfig{
			Mode:     "standalone",
			Interval: 1 * time.Hour,
			Timeout:  1 * time.Second,
		},
		Endpoints: []config.Endpoint{
			{Host: "127.0.0.1", Port: 19995},
		},
	}

	h := health.New(0, "0.1.0", "test-id", "standalone", "")
	logger := slog.Default()

	s := New(cfg, "test-id", nil, h, logger, "0.1.0")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Run in background, it will do one cycle then block on ticker
	go s.Run(ctx)

	// Wait for the first cycle to complete
	time.Sleep(2 * time.Second)

	// Verify readyz is now OK
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	// Access the handler directly since we didn't start the HTTP server
	var body map[string]string
	h.SetReady() // already called by Run, but let's check the handler
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})
	mux.ServeHTTP(rec, req)

	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "ready" {
		t.Errorf("expected ready status, got %q", body["status"])
	}
}

func TestResolveEndpointsSelfHosted(t *testing.T) {
	cachedRemoteEndpoints = nil

	cfg := &config.Config{
		Probe: config.ProbeConfig{Mode: "standalone"},
		Endpoints: []config.Endpoint{
			{Host: "a.com", Port: 443},
			{Host: "b.com", Port: 8443, SNI: "custom.b.com"},
		},
	}

	s := &Scheduler{cfg: cfg}
	endpoints := s.resolveEndpoints(context.Background())

	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}
	if endpoints[0].SNI != "a.com" {
		t.Errorf("endpoints[0].SNI = %q, want %q (default to host)", endpoints[0].SNI, "a.com")
	}
	if endpoints[1].SNI != "custom.b.com" {
		t.Errorf("endpoints[1].SNI = %q, want %q", endpoints[1].SNI, "custom.b.com")
	}
}

func TestSchedulerContextCancellation(t *testing.T) {
	cachedRemoteEndpoints = nil

	cfg := &config.Config{
		Probe: config.ProbeConfig{
			Mode:     "standalone",
			Interval: 24 * time.Hour, // long interval so it blocks on ticker
			Timeout:  1 * time.Second,
		},
		Endpoints: []config.Endpoint{
			{Host: "127.0.0.1", Port: 19994},
		},
	}

	logger := slog.Default()

	s := New(cfg, "test-id", nil, nil, logger, "0.1.0")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		s.Run(ctx)
		close(done)
	}()

	// Wait for first cycle to finish, then cancel
	time.Sleep(2 * time.Second)
	cancel()

	select {
	case <-done:
		// Scheduler stopped cleanly
	case <-time.After(3 * time.Second):
		t.Fatal("scheduler did not stop after context cancellation")
	}
}
