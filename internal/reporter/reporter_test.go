package reporter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/krakenkey/probe/internal/scanner"
)

func TestRegisterSuccess(t *testing.T) {
	var received ProbeInfo

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/probes/register" {
			t.Errorf("path = %s, want /probes/register", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer kk_testkey" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer kk_testkey")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}
		if ua := r.Header.Get("User-Agent"); ua == "" {
			t.Error("User-Agent is empty")
		}

		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rep := New(srv.URL, "kk_testkey", "0.1.0", "linux", "amd64")

	info := ProbeInfo{
		ProbeID: "test-uuid",
		Name:    "test-probe",
		Version: "0.1.0",
		Mode:    "standalone",
		Region:  "us-east-1",
		OS:      "linux",
		Arch:    "amd64",
	}

	err := rep.Register(context.Background(), info)
	if err != nil {
		t.Fatalf("Register() error: %v", err)
	}

	if received.ProbeID != "test-uuid" {
		t.Errorf("received probeId = %q, want %q", received.ProbeID, "test-uuid")
	}
	if received.Name != "test-probe" {
		t.Errorf("received name = %q, want %q", received.Name, "test-probe")
	}
}

func TestReportSuccess(t *testing.T) {
	var received ScanReport

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/probes/report" {
			t.Errorf("path = %s, want /probes/report", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rep := New(srv.URL, "kk_testkey", "0.1.0", "linux", "amd64")

	report := ScanReport{
		ProbeID:   "test-uuid",
		Mode:      "standalone",
		Region:    "us-east-1",
		Timestamp: time.Now().UTC(),
		Results: []scanner.ScanResult{
			{
				Endpoint: scanner.EndpointResult{
					Host: "example.com",
					Port: 443,
					SNI:  "example.com",
				},
				Connection: scanner.ConnectionResult{
					Success:    true,
					LatencyMs:  42,
					TLSVersion: "TLS 1.3",
				},
			},
		},
	}

	err := rep.Report(context.Background(), report)
	if err != nil {
		t.Fatalf("Report() error: %v", err)
	}

	if received.ProbeID != "test-uuid" {
		t.Errorf("received probeId = %q, want %q", received.ProbeID, "test-uuid")
	}
	if len(received.Results) != 1 {
		t.Fatalf("received %d results, want 1", len(received.Results))
	}
	if received.Results[0].Endpoint.Host != "example.com" {
		t.Errorf("received host = %q, want %q", received.Results[0].Endpoint.Host, "example.com")
	}
}

func TestReport401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()

	rep := New(srv.URL, "kk_badkey", "0.1.0", "linux", "amd64")

	err := rep.Report(context.Background(), ScanReport{})
	if err == nil {
		t.Fatal("expected error for 401")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
	if got := apiErr.Error(); got != "API authentication failed (HTTP 401): check your API key" {
		t.Errorf("error message = %q", got)
	}
}

func TestReport429WithRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	rep := New(srv.URL, "kk_testkey", "0.1.0", "linux", "amd64")

	err := rep.Report(context.Background(), ScanReport{})
	if err == nil {
		t.Fatal("expected error for 429")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 429 {
		t.Errorf("StatusCode = %d, want 429", apiErr.StatusCode)
	}
	if apiErr.RetryAfter != 60*time.Second {
		t.Errorf("RetryAfter = %v, want 60s", apiErr.RetryAfter)
	}
}

func TestReport5xxRetry(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rep := New(srv.URL, "kk_testkey", "0.1.0", "linux", "amd64")

	err := rep.Report(context.Background(), ScanReport{})
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestFetchHostedConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/probes/test-id/config" {
			t.Errorf("path = %s, want /probes/test-id/config", r.URL.Path)
		}

		cfg := HostedConfig{
			Endpoints: []HostedEndpoint{
				{Host: "example.com", Port: 443, UserID: "user1"},
				{Host: "api.example.com", Port: 443, SNI: "api.example.com", UserID: "user2"},
			},
			Interval: "5m",
		}
		_ = json.NewEncoder(w).Encode(cfg)
	}))
	defer srv.Close()

	rep := New(srv.URL, "service_key", "0.1.0", "linux", "amd64")

	cfg, err := rep.FetchHostedConfig(context.Background(), "test-id")
	if err != nil {
		t.Fatalf("FetchHostedConfig() error: %v", err)
	}

	if len(cfg.Endpoints) != 2 {
		t.Fatalf("len(Endpoints) = %d, want 2", len(cfg.Endpoints))
	}
	if cfg.Endpoints[0].Host != "example.com" {
		t.Errorf("Endpoints[0].Host = %q, want %q", cfg.Endpoints[0].Host, "example.com")
	}
	if cfg.Endpoints[1].UserID != "user2" {
		t.Errorf("Endpoints[1].UserID = %q, want %q", cfg.Endpoints[1].UserID, "user2")
	}
	if cfg.Interval != "5m" {
		t.Errorf("Interval = %q, want %q", cfg.Interval, "5m")
	}
}

func TestUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		want := "krakenkey-probe/1.2.3 (linux/amd64)"
		if ua != want {
			t.Errorf("User-Agent = %q, want %q", ua, want)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	rep := New(srv.URL, "kk_testkey", "1.2.3", "linux", "amd64")
	_ = rep.Register(context.Background(), ProbeInfo{})
}
