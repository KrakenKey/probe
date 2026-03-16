package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Server struct {
	port    int
	server  *http.Server
	mu      sync.RWMutex
	status  Status
	ready   bool
}

type Status struct {
	State    string `json:"status"`
	Version  string `json:"version"`
	ProbeID  string `json:"probeId"`
	Mode     string `json:"mode"`
	Region   string `json:"region,omitempty"`
	LastScan string `json:"lastScan,omitempty"`
	NextScan string `json:"nextScan,omitempty"`
}

func New(port int, version, probeID, mode, region string) *Server {
	s := &Server{
		port: port,
		status: Status{
			State:   "ok",
			Version: version,
			ProbeID: probeID,
			Mode:    mode,
			Region:  region,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	return s
}

func (s *Server) Start() error {
	err := s.server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// SetReady marks the probe as ready (first scan completed).
func (s *Server) SetReady() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = true
}

// UpdateScanTimes records the last and next scan timestamps.
func (s *Server) UpdateScanTimes(lastScan, nextScan time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.LastScan = lastScan.UTC().Format(time.RFC3339)
	s.status.NextScan = nextScan.UTC().Format(time.RFC3339)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	status := s.status
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	ready := s.ready
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}
