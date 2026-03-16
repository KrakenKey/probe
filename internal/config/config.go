package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	API       APIConfig       `yaml:"api"`
	Probe     ProbeConfig     `yaml:"probe"`
	Endpoints []Endpoint      `yaml:"endpoints"`
	Health    HealthConfig    `yaml:"health"`
	Logging   LoggingConfig   `yaml:"logging"`
}

type APIConfig struct {
	URL string `yaml:"url"`
	Key string `yaml:"key"`
}

type ProbeConfig struct {
	ID        string        `yaml:"id"`
	Name      string        `yaml:"name"`
	Mode      string        `yaml:"mode"`
	Region    string        `yaml:"region"`
	Interval  time.Duration `yaml:"-"`
	RawInterval string      `yaml:"interval"`
	Timeout   time.Duration `yaml:"-"`
	RawTimeout  string      `yaml:"timeout"`
	StateFile string        `yaml:"state_file"`
}

type Endpoint struct {
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port" json:"port"`
	SNI  string `yaml:"sni"  json:"sni,omitempty"`
}

type HealthConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Load(path string) (*Config, error) {
	cfg := defaults()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file: %w", err)
		}
	}

	applyEnvOverrides(cfg)

	if err := parseDurations(cfg); err != nil {
		return nil, err
	}

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func defaults() *Config {
	return &Config{
		API: APIConfig{
			URL: "https://api.krakenkey.io",
		},
		Probe: ProbeConfig{
			Mode:        "self-hosted",
			RawInterval: "60m",
			RawTimeout:  "10s",
			StateFile:   "/var/lib/krakenkey-probe/state.json",
		},
		Health: HealthConfig{
			Enabled: true,
			Port:    8080,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("KK_PROBE_API_URL"); v != "" {
		cfg.API.URL = v
	}
	if v := os.Getenv("KK_PROBE_API_KEY"); v != "" {
		cfg.API.Key = v
	}
	if v := os.Getenv("KK_PROBE_ID"); v != "" {
		cfg.Probe.ID = v
	}
	if v := os.Getenv("KK_PROBE_NAME"); v != "" {
		cfg.Probe.Name = v
	}
	if v := os.Getenv("KK_PROBE_MODE"); v != "" {
		cfg.Probe.Mode = v
	}
	if v := os.Getenv("KK_PROBE_REGION"); v != "" {
		cfg.Probe.Region = v
	}
	if v := os.Getenv("KK_PROBE_INTERVAL"); v != "" {
		cfg.Probe.RawInterval = v
	}
	if v := os.Getenv("KK_PROBE_TIMEOUT"); v != "" {
		cfg.Probe.RawTimeout = v
	}
	if v := os.Getenv("KK_PROBE_STATE_FILE"); v != "" {
		cfg.Probe.StateFile = v
	}
	if v := os.Getenv("KK_PROBE_ENDPOINTS"); v != "" {
		cfg.Endpoints = parseEndpointsEnv(v)
	}
	if v := os.Getenv("KK_PROBE_HEALTH_ENABLED"); v != "" {
		cfg.Health.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("KK_PROBE_HEALTH_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Health.Port = port
		}
	}
	if v := os.Getenv("KK_PROBE_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("KK_PROBE_LOG_FORMAT"); v != "" {
		cfg.Logging.Format = v
	}
}

func parseEndpointsEnv(val string) []Endpoint {
	var endpoints []Endpoint
	for _, entry := range strings.Split(val, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		host, portStr, found := strings.Cut(entry, ":")
		if !found {
			endpoints = append(endpoints, Endpoint{Host: host, Port: 443})
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			port = 443
		}
		endpoints = append(endpoints, Endpoint{Host: host, Port: port})
	}
	return endpoints
}

func parseDurations(cfg *Config) error {
	if cfg.Probe.RawInterval != "" {
		d, err := time.ParseDuration(cfg.Probe.RawInterval)
		if err != nil {
			return fmt.Errorf("parsing interval %q: %w", cfg.Probe.RawInterval, err)
		}
		cfg.Probe.Interval = d
	}
	if cfg.Probe.RawTimeout != "" {
		d, err := time.ParseDuration(cfg.Probe.RawTimeout)
		if err != nil {
			return fmt.Errorf("parsing timeout %q: %w", cfg.Probe.RawTimeout, err)
		}
		cfg.Probe.Timeout = d
	}
	return nil
}

func validate(cfg *Config) error {
	if cfg.API.Key == "" {
		return fmt.Errorf("api.key is required")
	}

	switch cfg.Probe.Mode {
	case "self-hosted":
		if !strings.HasPrefix(cfg.API.Key, "kk_") {
			return fmt.Errorf("api.key must start with 'kk_' for self-hosted mode")
		}
		if len(cfg.Endpoints) == 0 {
			return fmt.Errorf("at least one endpoint is required for self-hosted mode")
		}
		if cfg.Probe.Interval < time.Minute {
			return fmt.Errorf("interval must be at least 1m, got %s", cfg.Probe.Interval)
		}
		if cfg.Probe.Interval > 24*time.Hour {
			return fmt.Errorf("interval must be at most 24h, got %s", cfg.Probe.Interval)
		}
	case "hosted":
		if cfg.Probe.Region == "" {
			return fmt.Errorf("probe.region is required for hosted mode")
		}
		if cfg.Probe.ID == "" {
			return fmt.Errorf("probe.id is required for hosted mode")
		}
	default:
		return fmt.Errorf("probe.mode must be 'self-hosted' or 'hosted', got %q", cfg.Probe.Mode)
	}

	for i, ep := range cfg.Endpoints {
		if ep.Host == "" {
			return fmt.Errorf("endpoint[%d].host is required", i)
		}
		if ep.Port < 1 || ep.Port > 65535 {
			return fmt.Errorf("endpoint[%d].port must be 1-65535, got %d", i, ep.Port)
		}
	}

	switch cfg.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be debug, info, warn, or error, got %q", cfg.Logging.Level)
	}

	switch cfg.Logging.Format {
	case "json", "text":
	default:
		return fmt.Errorf("logging.format must be json or text, got %q", cfg.Logging.Format)
	}

	return nil
}
