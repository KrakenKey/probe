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

type HostedConfig struct {
	Endpoints []HostedEndpoint `json:"endpoints"`
	Interval  string           `json:"interval"`
}

type HostedEndpoint struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	SNI    string `json:"sni,omitempty"`
	UserID string `json:"userId"`
}
