package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthzReturnsStatus(t *testing.T) {
	s := New(0, "0.1.0", "test-uuid", "self-hosted", "us-east-1")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	s.handleHealthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var status Status
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if status.State != "ok" {
		t.Errorf("state = %q, want %q", status.State, "ok")
	}
	if status.Version != "0.1.0" {
		t.Errorf("version = %q, want %q", status.Version, "0.1.0")
	}
	if status.ProbeID != "test-uuid" {
		t.Errorf("probeId = %q, want %q", status.ProbeID, "test-uuid")
	}
	if status.Mode != "self-hosted" {
		t.Errorf("mode = %q, want %q", status.Mode, "self-hosted")
	}
	if status.Region != "us-east-1" {
		t.Errorf("region = %q, want %q", status.Region, "us-east-1")
	}
}

func TestHealthzWithScanTimes(t *testing.T) {
	s := New(0, "0.1.0", "test-uuid", "self-hosted", "")

	last := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	next := time.Date(2026, 3, 15, 13, 0, 0, 0, time.UTC)
	s.UpdateScanTimes(last, next)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	s.handleHealthz(rec, req)

	var status Status
	json.NewDecoder(rec.Body).Decode(&status)

	if status.LastScan != "2026-03-15T12:00:00Z" {
		t.Errorf("lastScan = %q, want %q", status.LastScan, "2026-03-15T12:00:00Z")
	}
	if status.NextScan != "2026-03-15T13:00:00Z" {
		t.Errorf("nextScan = %q, want %q", status.NextScan, "2026-03-15T13:00:00Z")
	}
}

func TestReadyzNotReady(t *testing.T) {
	s := New(0, "0.1.0", "test-uuid", "self-hosted", "")

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	s.handleReadyz(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "not ready" {
		t.Errorf("status = %q, want %q", body["status"], "not ready")
	}
}

func TestReadyzAfterReady(t *testing.T) {
	s := New(0, "0.1.0", "test-uuid", "self-hosted", "")
	s.SetReady()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	s.handleReadyz(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "ready" {
		t.Errorf("status = %q, want %q", body["status"], "ready")
	}
}

func TestServerStartAndShutdown(t *testing.T) {
	s := New(0, "0.1.0", "test-uuid", "self-hosted", "")

	// Use port 0 to get a random available port
	s.server.Addr = ":0"

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start()
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
}

func TestContentTypeJSON(t *testing.T) {
	s := New(0, "0.1.0", "test-uuid", "self-hosted", "")

	tests := []struct {
		name    string
		path    string
		handler http.HandlerFunc
	}{
		{"healthz", "/healthz", s.handleHealthz},
		{"readyz", "/readyz", s.handleReadyz},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			tt.handler(rec, req)

			ct := rec.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/json")
			}
		})
	}
}
