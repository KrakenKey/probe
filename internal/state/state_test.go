package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/krakenkey/probe/internal/config"
)

func TestLoadOrCreateGeneratesID(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")

	cfg := &config.Config{
		Probe: config.ProbeConfig{
			Mode:      "self-hosted",
			StateFile: stateFile,
		},
	}

	id, err := LoadOrCreate(cfg)
	if err != nil {
		t.Fatalf("LoadOrCreate() error: %v", err)
	}

	if id == "" {
		t.Fatal("expected non-empty probe ID")
	}

	// Verify file was created
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("reading state file: %v", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parsing state file: %v", err)
	}

	if s.ProbeID != id {
		t.Errorf("state file probe_id = %q, want %q", s.ProbeID, id)
	}
	if s.RegisteredAt == "" {
		t.Error("state file registered_at is empty")
	}
}

func TestLoadOrCreateReusesExistingID(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")

	existing := &State{
		ProbeID:      "existing-uuid-1234",
		RegisteredAt: "2026-01-01T00:00:00Z",
	}
	data, _ := json.Marshal(existing)
	if err := os.WriteFile(stateFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Probe: config.ProbeConfig{
			Mode:      "self-hosted",
			StateFile: stateFile,
		},
	}

	id, err := LoadOrCreate(cfg)
	if err != nil {
		t.Fatalf("LoadOrCreate() error: %v", err)
	}

	if id != "existing-uuid-1234" {
		t.Errorf("got %q, want %q", id, "existing-uuid-1234")
	}
}

func TestLoadOrCreateUsesConfigID(t *testing.T) {
	cfg := &config.Config{
		Probe: config.ProbeConfig{
			ID:   "config-id-5678",
			Mode: "self-hosted",
		},
	}

	id, err := LoadOrCreate(cfg)
	if err != nil {
		t.Fatalf("LoadOrCreate() error: %v", err)
	}

	if id != "config-id-5678" {
		t.Errorf("got %q, want %q", id, "config-id-5678")
	}
}

func TestLoadOrCreateHostedMode(t *testing.T) {
	cfg := &config.Config{
		Probe: config.ProbeConfig{
			ID:   "hosted-uuid",
			Mode: "hosted",
		},
	}

	id, err := LoadOrCreate(cfg)
	if err != nil {
		t.Fatalf("LoadOrCreate() error: %v", err)
	}

	if id != "hosted-uuid" {
		t.Errorf("got %q, want %q", id, "hosted-uuid")
	}
}

func TestGenerateIDFormat(t *testing.T) {
	id, err := generateID()
	if err != nil {
		t.Fatalf("generateID() error: %v", err)
	}

	// UUID format: 8-4-4-4-12 hex chars
	if len(id) != 36 {
		t.Errorf("id length = %d, want 36", len(id))
	}
	if id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		t.Errorf("id = %q, does not match UUID format", id)
	}
}
