package reporter

import (
	"time"

	"github.com/krakenkey/probe/internal/scanner"
)

type ProbeInfo struct {
	ProbeID string `json:"probeId"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Mode    string `json:"mode"`
	Region  string `json:"region,omitempty"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
}

type ScanReport struct {
	ProbeID   string               `json:"probeId"`
	Mode      string               `json:"mode"`
	Region    string               `json:"region,omitempty"`
	Timestamp time.Time            `json:"timestamp"`
	Results   []scanner.ScanResult `json:"results"`
}

// HostedConfig is the response of GET /probes/:probeId/config, used by
// connected and hosted probes.
type HostedConfig struct {
	Endpoints []HostedEndpoint `json:"endpoints"`
	// Interval is the shortest endpoint interval (60m with no endpoints).
	// The scheduler ignores it and uses the per-endpoint values instead.
	Interval string `json:"interval"`
}

type HostedEndpoint struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	SNI    string `json:"sni,omitempty"`
	UserID string `json:"userId"`
	// Interval is how often to scan this endpoint, as a Go duration
	// ("60m", "5m"). For hosted probes it is the owner's plan interval;
	// for connected probes it is the shortest interval the plan allows.
	// Empty when the API does not send it.
	Interval string `json:"interval,omitempty"`
}
