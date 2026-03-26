package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/krakenkey/probe/internal/config"
)

type State struct {
	ProbeID      string `json:"probe_id"`
	RegisteredAt string `json:"registered_at"`
}

// LoadOrCreate reads the probe ID from the state file. If the file does not
// exist and the mode is standalone, it generates a new UUID-style ID and
// persists it. For hosted/connected mode the ID must already be set in config.
func LoadOrCreate(cfg *config.Config) (string, error) {
	if cfg.Probe.Mode == "hosted" {
		return cfg.Probe.ID, nil
	}

	if cfg.Probe.ID != "" {
		return cfg.Probe.ID, nil
	}

	s, err := load(cfg.Probe.StateFile)
	if err == nil && s.ProbeID != "" {
		return s.ProbeID, nil
	}

	id, err := generateID()
	if err != nil {
		return "", fmt.Errorf("generating probe ID: %w", err)
	}

	s = &State{
		ProbeID:      id,
		RegisteredAt: time.Now().UTC().Format(time.RFC3339),
	}

	if err := save(cfg.Probe.StateFile, s); err != nil {
		return "", fmt.Errorf("saving state: %w", err)
	}

	return id, nil
}

func load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	return &s, nil
}

func save(path string, s *State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func generateID() (string, error) {
	// Generate a UUIDv4 without external dependencies.
	b := make([]byte, 16)
	f, err := os.Open("/dev/urandom")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
