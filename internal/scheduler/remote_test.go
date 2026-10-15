package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/krakenkey/probe/internal/config"
	"github.com/krakenkey/probe/internal/health"
	"github.com/krakenkey/probe/internal/reporter"
)

// fakeAPI serves GET /probes/:id/config from a mutable endpoint list and
// records the endpoints in each POST /probes/report.
type fakeAPI struct {
	t   *testing.T
	srv *httptest.Server

	mu        sync.Mutex
	endpoints []map[string]any
	failing   bool
	reports   [][]string

	configHits atomic.Int32
	otherHits  atomic.Int32
}

func newFakeAPI(t *testing.T, probeID string) *fakeAPI {
	t.Helper()
	f := &fakeAPI{t: t}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/probes/"+probeID+"/config":
			f.configHits.Add(1)
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.failing {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"endpoints": f.endpoints,
				"interval":  "60m",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/probes/report":
			var report struct {
				Results []struct {
					Endpoint struct {
						Host string `json:"host"`
						Port int    `json:"port"`
					} `json:"endpoint"`
				} `json:"results"`
			}
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				t.Errorf("decoding report: %v", err)
			}
			var got []string
			for _, res := range report.Results {
				got = append(got, res.Endpoint.Host+":"+strconv.Itoa(res.Endpoint.Port))
			}
			slices.Sort(got)
			f.mu.Lock()
			f.reports = append(f.reports, got)
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			f.otherHits.Add(1)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) setEndpoints(eps ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.endpoints = eps
}

func (f *fakeAPI) setFailing(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failing = v
}

func (f *fakeAPI) lastReport() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.reports) == 0 {
		return nil
	}
	return f.reports[len(f.reports)-1]
}

func (f *fakeAPI) reportCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reports)
}

// ep builds a config endpoint on 127.0.0.1. The ports are closed, so scans
// fail fast with "connection refused", which is enough to produce a report.
func ep(port int, interval string) map[string]any {
	m := map[string]any{"host": "127.0.0.1", "port": port, "userId": "user1"}
	if interval != "" {
		m["interval"] = interval
	}
	return m
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newRemoteScheduler(t *testing.T, mode string, local time.Duration, api *fakeAPI, probeID string) *Scheduler {
	t.Helper()
	cfg := &config.Config{
		Probe: config.ProbeConfig{
			Mode:     mode,
			Region:   "us-east-1",
			Interval: local,
			Timeout:  time.Second,
		},
	}
	var rep *reporter.Reporter
	if api != nil {
		rep = reporter.New(api.srv.URL, "kk_test", "0.1.0", "linux", "amd64")
	}
	return New(cfg, probeID, rep, nil, quietLogger(), "0.1.0")
}

func TestEffectiveIntervalHosted(t *testing.T) {
	tests := []struct {
		name  string
		local time.Duration
		raw   string
		want  time.Duration
	}{
		{"missing falls back to local", 60 * time.Minute, "", 60 * time.Minute},
		{"plan interval used", 60 * time.Minute, "5m", 5 * time.Minute},
		{"faster than local is honored", 60 * time.Minute, "1m", time.Minute},
		{"slower than local is honored", 5 * time.Minute, "30m", 30 * time.Minute},
		{"unparseable falls back", 60 * time.Minute, "soon", 60 * time.Minute},
		{"negative falls back", 60 * time.Minute, "-5m", 60 * time.Minute},
		{"zero falls back", 60 * time.Minute, "0s", 60 * time.Minute},
		{"below 1m clamped", 60 * time.Minute, "10s", time.Minute},
		{"above 24h clamped", 60 * time.Minute, "48h", 24 * time.Hour},
		{"unset local defaults to 60m", 0, "", 60 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newRemoteScheduler(t, "hosted", tt.local, nil, "p")
			if got := s.effectiveInterval(tt.raw); got != tt.want {
				t.Errorf("effectiveInterval(%q) = %s, want %s", tt.raw, got, tt.want)
			}
		})
	}
}

func TestEffectiveIntervalConnectedIsFloor(t *testing.T) {
	tests := []struct {
		name  string
		local time.Duration
		raw   string
		want  time.Duration
	}{
		{"plan floor slower than local wins", 30 * time.Minute, "60m", 60 * time.Minute},
		{"local slower than plan floor wins", 30 * time.Minute, "15m", 30 * time.Minute},
		{"local slower than 1m floor wins", 30 * time.Minute, "1m", 30 * time.Minute},
		{"equal", 15 * time.Minute, "15m", 15 * time.Minute},
		{"fast local raised to floor", time.Minute, "15m", 15 * time.Minute},
		{"missing falls back to local", 30 * time.Minute, "", 30 * time.Minute},
		{"unparseable falls back to local", 30 * time.Minute, "1 hour", 30 * time.Minute},
		{"clamped floor still a floor", time.Minute, "10s", time.Minute},
		{"clamped above 24h", time.Hour, "100h", 24 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newRemoteScheduler(t, "connected", tt.local, nil, "p")
			if got := s.effectiveInterval(tt.raw); got != tt.want {
				t.Errorf("effectiveInterval(%q) = %s, want %s", tt.raw, got, tt.want)
			}
		})
	}
}

func TestInvalidIntervalLoggedOnce(t *testing.T) {
	var buf bytes.Buffer
	s := newRemoteScheduler(t, "hosted", time.Hour, nil, "p")
	s.logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	for range 3 {
		s.effectiveInterval("soon")
		s.effectiveInterval("")
	}

	if n := strings.Count(buf.String(), "ignoring invalid endpoint interval"); n != 1 {
		t.Errorf("invalid interval logged %d times, want 1", n)
	}
	if n := strings.Count(buf.String(), "no per-endpoint interval"); n != 1 {
		t.Errorf("missing interval logged %d times, want 1", n)
	}
}

func TestDueComputationHosted(t *testing.T) {
	api := newFakeAPI(t, "hosted-id")
	api.setEndpoints(ep(19981, "5m"), ep(19982, "60m"), ep(19983, ""))
	s := newRemoteScheduler(t, "hosted", 30*time.Minute, api, "hosted-id")

	if added := s.refreshConfig(context.Background(), time.Now()); added != 3 {
		t.Fatalf("refreshConfig() added = %d, want 3", added)
	}

	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	s.runDue(context.Background(), t0)
	if got, want := api.lastReport(), []string{"127.0.0.1:19981", "127.0.0.1:19982", "127.0.0.1:19983"}; !slices.Equal(got, want) {
		t.Fatalf("first report = %v, want %v", got, want)
	}

	checks := []struct {
		after time.Duration
		want  []int
	}{
		{time.Minute, nil},
		{4 * time.Minute, nil},
		{5*time.Minute - dueSlack - time.Second, nil},
		{5*time.Minute - dueSlack, []int{19981}}, // slack absorbs tick jitter
		{5 * time.Minute, []int{19981}},
		{30 * time.Minute, []int{19981, 19983}}, // missing interval uses local 30m
		{60 * time.Minute, []int{19981, 19982, 19983}},
	}
	for _, c := range checks {
		var got []int
		for _, d := range s.dueEndpoints(t0.Add(c.after)) {
			got = append(got, d.endpoint.Port)
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("due after %s = %v, want %v", c.after, got, c.want)
		}
	}

	if got, want := s.nextDue(t0), t0.Add(5*time.Minute); !got.Equal(want) {
		t.Errorf("nextDue = %s, want %s", got, want)
	}

	// Scanning the due endpoint reports only that endpoint.
	s.runDue(context.Background(), t0.Add(5*time.Minute))
	if got, want := api.lastReport(), []string{"127.0.0.1:19981"}; !slices.Equal(got, want) {
		t.Errorf("second report = %v, want %v", got, want)
	}
	if got, want := s.nextDue(t0.Add(5*time.Minute)), t0.Add(10*time.Minute); !got.Equal(want) {
		t.Errorf("nextDue after second scan = %s, want %s", got, want)
	}
}

func TestDueComputationConnectedFloor(t *testing.T) {
	api := newFakeAPI(t, "conn-id")
	// Plan allows 1m, but the user set 30m locally: scan every 30m.
	api.setEndpoints(ep(19984, "1m"))
	s := newRemoteScheduler(t, "connected", 30*time.Minute, api, "conn-id")
	s.refreshConfig(context.Background(), time.Now())

	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	s.runDue(context.Background(), t0)
	if api.reportCount() != 1 {
		t.Fatalf("reports = %d, want 1", api.reportCount())
	}

	if due := s.dueEndpoints(t0.Add(5 * time.Minute)); len(due) != 0 {
		t.Errorf("endpoint due after 5m, want it held to the 30m local interval")
	}
	if due := s.dueEndpoints(t0.Add(30 * time.Minute)); len(due) != 1 {
		t.Errorf("endpoint not due after 30m")
	}

	// Plan floor of 60m is slower than the local 30m: scan every 60m.
	api.setEndpoints(ep(19984, "60m"))
	if added := s.refreshConfig(context.Background(), time.Now()); added != 0 {
		t.Errorf("interval change counted as %d new endpoints, want 0", added)
	}
	if due := s.dueEndpoints(t0.Add(30 * time.Minute)); len(due) != 0 {
		t.Errorf("endpoint due after 30m, want it held to the 60m plan floor")
	}
	if due := s.dueEndpoints(t0.Add(60 * time.Minute)); len(due) != 1 {
		t.Errorf("endpoint not due after 60m")
	}
}

func TestOlderAPIUsesLocalInterval(t *testing.T) {
	for _, mode := range []string{"hosted", "connected"} {
		t.Run(mode, func(t *testing.T) {
			api := newFakeAPI(t, "p")
			api.setEndpoints(ep(19985, ""), ep(19986, ""))
			s := newRemoteScheduler(t, mode, 10*time.Minute, api, "p")
			s.refreshConfig(context.Background(), time.Now())

			for _, r := range s.remote {
				if r.interval != 10*time.Minute {
					t.Errorf("%s interval = %s, want local 10m", r.key, r.interval)
				}
			}
		})
	}
}

func TestNewEndpointScannedImmediatelyAndRemovedForgotten(t *testing.T) {
	api := newFakeAPI(t, "hosted-id")
	api.setEndpoints(ep(19987, "60m"))
	s := newRemoteScheduler(t, "hosted", time.Hour, api, "hosted-id")
	ctx := context.Background()

	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	s.refreshConfig(ctx, time.Now())
	s.runDue(ctx, t0)

	// A second endpoint shows up: only it is scanned.
	api.setEndpoints(ep(19987, "60m"), ep(19988, "60m"))
	if added := s.refreshConfig(ctx, time.Now()); added != 1 {
		t.Fatalf("refreshConfig() added = %d, want 1", added)
	}
	s.runDue(ctx, t0.Add(time.Minute))
	if got, want := api.lastReport(), []string{"127.0.0.1:19988"}; !slices.Equal(got, want) {
		t.Errorf("report after adding endpoint = %v, want %v", got, want)
	}

	// The first endpoint is removed: its state is dropped.
	api.setEndpoints(ep(19988, "60m"))
	if added := s.refreshConfig(ctx, time.Now()); added != 0 {
		t.Errorf("refreshConfig() after removal added = %d, want 0", added)
	}
	if len(s.remote) != 1 || len(s.lastScan) != 1 {
		t.Fatalf("remote = %d, lastScan = %d, want 1 and 1", len(s.remote), len(s.lastScan))
	}
	if _, ok := s.lastScan["127.0.0.1:19987:127.0.0.1"]; ok {
		t.Error("removed endpoint still has scan state")
	}

	// If it comes back it counts as new and is scanned right away.
	api.setEndpoints(ep(19987, "60m"), ep(19988, "60m"))
	if added := s.refreshConfig(ctx, time.Now()); added != 1 {
		t.Errorf("refreshConfig() after re-adding added = %d, want 1", added)
	}
	s.runDue(ctx, t0.Add(2*time.Minute))
	if got, want := api.lastReport(), []string{"127.0.0.1:19987"}; !slices.Equal(got, want) {
		t.Errorf("report after re-adding endpoint = %v, want %v", got, want)
	}

	// A changed SNI is a different endpoint.
	api.setEndpoints(ep(19987, "60m"), map[string]any{"host": "127.0.0.1", "port": 19988, "sni": "other.example", "interval": "60m"})
	if added := s.refreshConfig(ctx, time.Now()); added != 1 {
		t.Errorf("refreshConfig() after SNI change added = %d, want 1", added)
	}
}

func TestDuplicateEndpointsScannedOnceAtShortestInterval(t *testing.T) {
	api := newFakeAPI(t, "hosted-id")
	api.setEndpoints(ep(19989, "60m"), ep(19989, "5m"), ep(19989, ""))
	s := newRemoteScheduler(t, "hosted", time.Hour, api, "hosted-id")

	if added := s.refreshConfig(context.Background(), time.Now()); added != 1 {
		t.Fatalf("refreshConfig() added = %d, want 1", added)
	}
	if len(s.remote) != 1 {
		t.Fatalf("remote = %d endpoints, want 1", len(s.remote))
	}
	if got := s.remote[0].interval; got != 5*time.Minute {
		t.Errorf("interval = %s, want 5m", got)
	}
}

func TestRefreshKeepsCacheOnError(t *testing.T) {
	api := newFakeAPI(t, "hosted-id")
	api.setEndpoints(ep(19990, "5m"))
	s := newRemoteScheduler(t, "hosted", time.Hour, api, "hosted-id")
	ctx := context.Background()

	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	s.refreshConfig(ctx, time.Now())
	s.runDue(ctx, t0)

	api.setFailing(true)
	if added := s.refreshConfig(ctx, time.Now()); added != 0 {
		t.Errorf("refreshConfig() on error added = %d, want 0", added)
	}
	if len(s.remote) != 1 || len(s.lastScan) != 1 {
		t.Fatalf("cache lost on error: remote = %d, lastScan = %d", len(s.remote), len(s.lastScan))
	}

	// Cached endpoints keep being scanned on schedule.
	s.runDue(ctx, t0.Add(5*time.Minute))
	if api.reportCount() != 2 {
		t.Errorf("reports = %d, want 2", api.reportCount())
	}
}

func TestEmptyConfigClearsState(t *testing.T) {
	api := newFakeAPI(t, "hosted-id")
	api.setEndpoints(ep(19991, "5m"))
	s := newRemoteScheduler(t, "hosted", time.Hour, api, "hosted-id")
	ctx := context.Background()

	s.refreshConfig(ctx, time.Now())
	s.runDue(ctx, time.Now())

	api.setEndpoints()
	s.refreshConfig(ctx, time.Now())
	if len(s.remote) != 0 || len(s.lastScan) != 0 {
		t.Errorf("remote = %d, lastScan = %d, want both empty", len(s.remote), len(s.lastScan))
	}
	if !s.nextDue(time.Now()).IsZero() {
		t.Error("nextDue with no endpoints should be zero")
	}

	before := api.reportCount()
	s.runDue(ctx, time.Now())
	if api.reportCount() != before {
		t.Error("runDue with no endpoints sent a report")
	}
}

func TestRunHostedScansOnStartAndSetsReady(t *testing.T) {
	api := newFakeAPI(t, "hosted-id")
	api.setEndpoints(ep(19992, "5m"))
	s := newRemoteScheduler(t, "hosted", time.Hour, api, "hosted-id")
	s.health = health.New(0, "0.1.0", "hosted-id", "hosted", "us-east-1", config.ScanAPIConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	deadline := time.After(5 * time.Second)
	for api.reportCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("no report sent after startup")
		case <-time.After(20 * time.Millisecond):
		}
	}
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("scheduler did not stop after context cancellation")
	}

	if api.configHits.Load() == 0 {
		t.Error("expected hosted config to be fetched from API")
	}
	if got, want := api.lastReport(), []string{"127.0.0.1:19992"}; !slices.Equal(got, want) {
		t.Errorf("report = %v, want %v", got, want)
	}
}

func TestStandaloneIgnoresRemoteConfig(t *testing.T) {
	api := newFakeAPI(t, "test-id")
	api.setEndpoints(ep(19993, "1m"))

	cfg := &config.Config{
		Probe: config.ProbeConfig{
			Mode:     "standalone",
			Interval: time.Hour,
			Timeout:  time.Second,
		},
		Endpoints: []config.Endpoint{{Host: "127.0.0.1", Port: 19994}},
	}
	rep := reporter.New(api.srv.URL, "kk_test", "0.1.0", "linux", "amd64")
	s := New(cfg, "test-id", rep, nil, quietLogger(), "0.1.0")

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	s.Run(ctx)

	if n := api.configHits.Load() + api.otherHits.Load(); n != 0 {
		t.Errorf("standalone mode made %d API requests, want 0", n)
	}
	if api.reportCount() != 0 {
		t.Error("standalone mode sent a report")
	}
	if len(s.remote) != 0 || len(s.lastScan) != 0 {
		t.Error("standalone mode populated per-endpoint state")
	}
}

func seeded(port int, interval string, lastScanAt string) map[string]any {
	m := ep(port, interval)
	if lastScanAt != "" {
		m["lastScanAt"] = lastScanAt
	}
	return m
}

func ports(eps []remoteEndpoint) []int {
	var out []int
	for _, e := range eps {
		out = append(out, e.endpoint.Port)
	}
	return out
}

func TestSeedFromLastScanAt(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	api := newFakeAPI(t, "hosted-id")
	api.setEndpoints(
		seeded(19961, "60m", t0.Add(-50*time.Minute).Format(time.RFC3339)),
		seeded(19962, "60m", ""),
		seeded(19963, "60m", "yesterday"),
	)
	var buf bytes.Buffer
	s := newRemoteScheduler(t, "hosted", time.Hour, api, "hosted-id")
	s.logger = slog.New(slog.NewTextHandler(&buf, nil))
	ctx := context.Background()

	if added := s.refreshConfig(ctx, t0); added != 3 {
		t.Fatalf("refreshConfig() added = %d, want 3", added)
	}
	if got := s.lastScan["127.0.0.1:19961:127.0.0.1"]; !got.Equal(t0.Add(-50 * time.Minute)) {
		t.Errorf("seeded last scan = %s, want %s", got, t0.Add(-50*time.Minute))
	}

	// Only the endpoints without a usable lastScanAt are scanned on startup.
	if got, want := ports(s.dueEndpoints(t0)), []int{19962, 19963}; !slices.Equal(got, want) {
		t.Errorf("due at startup = %v, want %v", got, want)
	}
	s.runDue(ctx, t0)
	if got, want := api.lastReport(), []string{"127.0.0.1:19962", "127.0.0.1:19963"}; !slices.Equal(got, want) {
		t.Errorf("startup report = %v, want %v", got, want)
	}

	// The seeded endpoint is due one interval after the API's lastScanAt.
	if got, want := ports(s.dueEndpoints(t0.Add(10*time.Minute))), []int{19961}; !slices.Equal(got, want) {
		t.Errorf("due after 10m = %v, want %v", got, want)
	}

	// A second poll with the same bad value does not log again.
	s.refreshConfig(ctx, t0.Add(time.Minute))
	if n := strings.Count(buf.String(), "ignoring invalid lastScanAt"); n != 1 {
		t.Errorf("invalid lastScanAt logged %d times, want 1", n)
	}
}

func TestSeedDuplicatesUseEarliest(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	recent := t0.Add(-10 * time.Minute).Format(time.RFC3339)
	older := t0.Add(-50 * time.Minute).Format(time.RFC3339)

	api := newFakeAPI(t, "hosted-id")
	api.setEndpoints(
		seeded(19964, "60m", recent),
		seeded(19964, "60m", older),
		seeded(19965, "60m", recent),
		seeded(19965, "60m", ""), // one duplicate never scanned by this probe
		seeded(19966, "60m", recent),
		seeded(19966, "60m", "not-a-time"),
	)
	s := newRemoteScheduler(t, "hosted", time.Hour, api, "hosted-id")
	s.refreshConfig(context.Background(), t0)

	if got := s.lastScan["127.0.0.1:19964:127.0.0.1"]; !got.Equal(t0.Add(-50 * time.Minute)) {
		t.Errorf("duplicate seed = %s, want the earliest %s", got, t0.Add(-50*time.Minute))
	}
	if got, want := ports(s.dueEndpoints(t0)), []int{19965, 19966}; !slices.Equal(got, want) {
		t.Errorf("due = %v, want %v (a duplicate without a usable lastScanAt is never scanned)", got, want)
	}
}

func TestSeedNeverOverridesLocalScan(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	api := newFakeAPI(t, "hosted-id")
	api.setEndpoints(seeded(19967, "60m", ""))
	s := newRemoteScheduler(t, "hosted", time.Hour, api, "hosted-id")
	ctx := context.Background()

	s.refreshConfig(ctx, t0)
	s.runDue(ctx, t0)

	for _, at := range []time.Time{t0.Add(-30 * time.Minute), t0.Add(2 * time.Minute)} {
		api.setEndpoints(seeded(19967, "60m", at.Format(time.RFC3339)))
		s.refreshConfig(ctx, t0.Add(3*time.Minute))
		if got := s.lastScan["127.0.0.1:19967:127.0.0.1"]; !got.Equal(t0) {
			t.Errorf("lastScanAt %s changed the local last scan to %s, want %s", at, got, t0)
		}
	}
}

func TestSeedInFutureCappedAtNow(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	api := newFakeAPI(t, "hosted-id")
	api.setEndpoints(seeded(19968, "5m", t0.Add(time.Hour).Format(time.RFC3339)))
	s := newRemoteScheduler(t, "hosted", time.Hour, api, "hosted-id")
	s.refreshConfig(context.Background(), t0)

	if got := s.lastScan["127.0.0.1:19968:127.0.0.1"]; !got.Equal(t0) {
		t.Errorf("future lastScanAt seeded %s, want it capped at %s", got, t0)
	}
	if len(s.dueEndpoints(t0.Add(5*time.Minute))) != 1 {
		t.Error("endpoint not due one interval after the capped seed")
	}
}

func withScanNow(m map[string]any) map[string]any {
	m["scanNow"] = true
	return m
}

func TestScanNowScansOnceAndRearms(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	api := newFakeAPI(t, "hosted-id")
	api.setEndpoints(ep(19969, "60m"), ep(19970, "60m"))
	s := newRemoteScheduler(t, "hosted", time.Hour, api, "hosted-id")
	ctx := context.Background()

	s.refreshConfig(ctx, t0)
	s.runDue(ctx, t0)

	// A user asks for a scan of 19969: it is due at once, alone.
	api.setEndpoints(withScanNow(ep(19969, "60m")), ep(19970, "60m"))
	s.refreshConfig(ctx, t0.Add(time.Minute))
	if got, want := s.nextDue(t0.Add(time.Minute)), t0.Add(time.Minute); !got.Equal(want) {
		t.Errorf("nextDue with pending scanNow = %s, want %s", got, want)
	}
	s.runDue(ctx, t0.Add(time.Minute))
	if got, want := api.lastReport(), []string{"127.0.0.1:19969"}; !slices.Equal(got, want) {
		t.Fatalf("scanNow report = %v, want %v", got, want)
	}
	reports := api.reportCount()

	// The cached config still says scanNow, and so may the next fetch:
	// no repeat scans.
	s.runDue(ctx, t0.Add(90*time.Second))
	s.refreshConfig(ctx, t0.Add(2*time.Minute))
	s.runDue(ctx, t0.Add(2*time.Minute))
	if api.reportCount() != reports {
		t.Errorf("scanNow scanned again before the API cleared it")
	}

	// The API clears scanNow; normal scheduling continues from the
	// scanNow scan.
	api.setEndpoints(ep(19969, "60m"), ep(19970, "60m"))
	s.refreshConfig(ctx, t0.Add(3*time.Minute))
	if got, want := ports(s.dueEndpoints(t0.Add(60*time.Minute))), []int{19970}; !slices.Equal(got, want) {
		t.Errorf("due after 60m = %v, want %v", got, want)
	}

	// A new request scans again.
	api.setEndpoints(withScanNow(ep(19969, "60m")), ep(19970, "60m"))
	s.refreshConfig(ctx, t0.Add(4*time.Minute))
	s.runDue(ctx, t0.Add(4*time.Minute))
	if got, want := api.lastReport(), []string{"127.0.0.1:19969"}; api.reportCount() != reports+1 || !slices.Equal(got, want) {
		t.Errorf("second scanNow: reports = %d, last = %v; want %d and %v", api.reportCount(), got, reports+1, want)
	}
}

func TestScanNowRearmsWhenEndpointReturns(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	recent := t0.Format(time.RFC3339)
	api := newFakeAPI(t, "hosted-id")
	api.setEndpoints(withScanNow(seeded(19971, "60m", recent)))
	s := newRemoteScheduler(t, "hosted", time.Hour, api, "hosted-id")
	ctx := context.Background()

	// scanNow wins over a recent seed.
	s.refreshConfig(ctx, t0)
	s.runDue(ctx, t0)
	if api.reportCount() != 1 {
		t.Fatalf("reports = %d, want 1", api.reportCount())
	}

	api.setEndpoints()
	s.refreshConfig(ctx, t0.Add(time.Minute))
	if len(s.scanNowDone) != 0 {
		t.Error("scanNow state kept for a removed endpoint")
	}

	api.setEndpoints(withScanNow(seeded(19971, "60m", recent)))
	s.refreshConfig(ctx, t0.Add(2*time.Minute))
	s.runDue(ctx, t0.Add(2*time.Minute))
	if api.reportCount() != 2 {
		t.Errorf("reports = %d, want 2 after the endpoint came back with scanNow", api.reportCount())
	}
}

func TestScanNowOrAcrossDuplicates(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	recent := t0.Format(time.RFC3339)
	api := newFakeAPI(t, "hosted-id")
	api.setEndpoints(seeded(19972, "60m", recent), withScanNow(seeded(19972, "60m", recent)))
	s := newRemoteScheduler(t, "hosted", time.Hour, api, "hosted-id")
	s.refreshConfig(context.Background(), t0)

	if got, want := ports(s.dueEndpoints(t0)), []int{19972}; !slices.Equal(got, want) {
		t.Errorf("due = %v, want %v", got, want)
	}
}
