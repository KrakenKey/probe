package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")

	yaml := `
api:
  url: "https://api.example.com"
  key: "kk_testkey123"

probe:
  name: "test-probe"
  mode: "self-hosted"
  interval: "30m"
  timeout: "5s"
  state_file: "/tmp/test-state.json"

endpoints:
  - host: "example.com"
    port: 443
  - host: "internal.corp"
    port: 8443
    sni: "internal.corp"

health:
  enabled: true
  port: 9090

logging:
  level: "debug"
  format: "text"
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.API.URL != "https://api.example.com" {
		t.Errorf("API.URL = %q, want %q", cfg.API.URL, "https://api.example.com")
	}
	if cfg.API.Key != "kk_testkey123" {
		t.Errorf("API.Key = %q, want %q", cfg.API.Key, "kk_testkey123")
	}
	if cfg.Probe.Name != "test-probe" {
		t.Errorf("Probe.Name = %q, want %q", cfg.Probe.Name, "test-probe")
	}
	if cfg.Probe.Interval.Minutes() != 30 {
		t.Errorf("Probe.Interval = %v, want 30m", cfg.Probe.Interval)
	}
	if cfg.Probe.Timeout.Seconds() != 5 {
		t.Errorf("Probe.Timeout = %v, want 5s", cfg.Probe.Timeout)
	}
	if len(cfg.Endpoints) != 2 {
		t.Fatalf("len(Endpoints) = %d, want 2", len(cfg.Endpoints))
	}
	if cfg.Endpoints[0].Host != "example.com" || cfg.Endpoints[0].Port != 443 {
		t.Errorf("Endpoints[0] = %+v, want example.com:443", cfg.Endpoints[0])
	}
	if cfg.Endpoints[1].SNI != "internal.corp" {
		t.Errorf("Endpoints[1].SNI = %q, want %q", cfg.Endpoints[1].SNI, "internal.corp")
	}
	if cfg.Health.Port != 9090 {
		t.Errorf("Health.Port = %d, want 9090", cfg.Health.Port)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "debug")
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("Logging.Format = %q, want %q", cfg.Logging.Format, "text")
	}
}

func TestEnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")

	yaml := `
api:
  url: "https://api.example.com"
  key: "kk_yamlkey"

probe:
  name: "yaml-probe"
  interval: "60m"

endpoints:
  - host: "yaml.example.com"
    port: 443

logging:
  level: "info"
  format: "json"
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("KK_PROBE_API_KEY", "kk_envkey")
	t.Setenv("KK_PROBE_NAME", "env-probe")
	t.Setenv("KK_PROBE_ENDPOINTS", "env1.example.com:443,env2.example.com:8443")
	t.Setenv("KK_PROBE_INTERVAL", "15m")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.API.Key != "kk_envkey" {
		t.Errorf("API.Key = %q, want %q (env override)", cfg.API.Key, "kk_envkey")
	}
	if cfg.Probe.Name != "env-probe" {
		t.Errorf("Probe.Name = %q, want %q (env override)", cfg.Probe.Name, "env-probe")
	}
	if len(cfg.Endpoints) != 2 {
		t.Fatalf("len(Endpoints) = %d, want 2 (env override)", len(cfg.Endpoints))
	}
	if cfg.Endpoints[0].Host != "env1.example.com" {
		t.Errorf("Endpoints[0].Host = %q, want %q", cfg.Endpoints[0].Host, "env1.example.com")
	}
	if cfg.Endpoints[1].Port != 8443 {
		t.Errorf("Endpoints[1].Port = %d, want 8443", cfg.Endpoints[1].Port)
	}
	if cfg.Probe.Interval.Minutes() != 15 {
		t.Errorf("Probe.Interval = %v, want 15m (env override)", cfg.Probe.Interval)
	}
}

func TestEnvOnlyNoFile(t *testing.T) {
	t.Setenv("KK_PROBE_API_KEY", "kk_envonly")
	t.Setenv("KK_PROBE_ENDPOINTS", "example.com:443")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.API.Key != "kk_envonly" {
		t.Errorf("API.Key = %q, want %q", cfg.API.Key, "kk_envonly")
	}
	if cfg.API.URL != "https://api.krakenkey.io" {
		t.Errorf("API.URL = %q, want default", cfg.API.URL)
	}
}

func TestValidationSelfHostedMissingKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")

	yaml := `
api:
  key: ""
endpoints:
  - host: "example.com"
    port: 443
`
	os.WriteFile(cfgPath, []byte(yaml), 0o644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestValidationSelfHostedBadKeyPrefix(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")

	yaml := `
api:
  key: "bad_prefix"
endpoints:
  - host: "example.com"
    port: 443
`
	os.WriteFile(cfgPath, []byte(yaml), 0o644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for bad key prefix")
	}
}

func TestValidationSelfHostedNoEndpoints(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")

	yaml := `
api:
  key: "kk_testkey"
endpoints: []
`
	os.WriteFile(cfgPath, []byte(yaml), 0o644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for no endpoints")
	}
}

func TestValidationIntervalTooLow(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")

	yaml := `
api:
  key: "kk_testkey"
probe:
  interval: "30s"
endpoints:
  - host: "example.com"
    port: 443
`
	os.WriteFile(cfgPath, []byte(yaml), 0o644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for interval too low")
	}
}

func TestValidationIntervalTooHigh(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")

	yaml := `
api:
  key: "kk_testkey"
probe:
  interval: "48h"
endpoints:
  - host: "example.com"
    port: 443
`
	os.WriteFile(cfgPath, []byte(yaml), 0o644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for interval too high")
	}
}

func TestValidationBadPort(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")

	yaml := `
api:
  key: "kk_testkey"
endpoints:
  - host: "example.com"
    port: 0
`
	os.WriteFile(cfgPath, []byte(yaml), 0o644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for bad port")
	}
}

func TestValidationBadMode(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")

	yaml := `
api:
  key: "kk_testkey"
probe:
  mode: "invalid"
endpoints:
  - host: "example.com"
    port: 443
`
	os.WriteFile(cfgPath, []byte(yaml), 0o644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for bad mode")
	}
}

func TestValidationHostedMissingRegion(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")

	yaml := `
api:
  key: "service_key_123"
probe:
  mode: "hosted"
  id: "some-uuid"
`
	os.WriteFile(cfgPath, []byte(yaml), 0o644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for hosted mode missing region")
	}
}

func TestValidationHostedMissingID(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")

	yaml := `
api:
  key: "service_key_123"
probe:
  mode: "hosted"
  region: "us-east-1"
`
	os.WriteFile(cfgPath, []byte(yaml), 0o644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for hosted mode missing ID")
	}
}

func TestValidationHostedValid(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")

	yaml := `
api:
  key: "service_key_123"
probe:
  mode: "hosted"
  id: "some-uuid"
  region: "us-east-1"
`
	os.WriteFile(cfgPath, []byte(yaml), 0o644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Probe.Mode != "hosted" {
		t.Errorf("Probe.Mode = %q, want %q", cfg.Probe.Mode, "hosted")
	}
}

func TestValidationBadLogLevel(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "probe.yaml")

	yaml := `
api:
  key: "kk_testkey"
probe:
  interval: "60m"
endpoints:
  - host: "example.com"
    port: 443
logging:
  level: "trace"
  format: "json"
`
	os.WriteFile(cfgPath, []byte(yaml), 0o644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for bad log level")
	}
}

func TestParseEndpointsEnv(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []Endpoint
	}{
		{
			name:  "single with port",
			input: "example.com:443",
			want:  []Endpoint{{Host: "example.com", Port: 443}},
		},
		{
			name:  "multiple",
			input: "a.com:443,b.com:8443",
			want:  []Endpoint{{Host: "a.com", Port: 443}, {Host: "b.com", Port: 8443}},
		},
		{
			name:  "no port defaults to 443",
			input: "example.com",
			want:  []Endpoint{{Host: "example.com", Port: 443}},
		},
		{
			name:  "whitespace handling",
			input: " a.com:443 , b.com:443 ",
			want:  []Endpoint{{Host: "a.com", Port: 443}, {Host: "b.com", Port: 443}},
		},
		{
			name:  "empty entries skipped",
			input: "a.com:443,,b.com:443",
			want:  []Endpoint{{Host: "a.com", Port: 443}, {Host: "b.com", Port: 443}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEndpointsEnv(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].Host != tt.want[i].Host || got[i].Port != tt.want[i].Port {
					t.Errorf("[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
