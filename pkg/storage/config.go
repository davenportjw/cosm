package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WorkspaceConfig manages repository-level settings, micro-universe defaults, and ledger mode.
type WorkspaceConfig struct {
	LedgerMode      bool                   `json:"ledger_mode"`
	DefaultUniverse string                 `json:"default_universe"`
	CreatedAt       time.Time              `json:"created_at"`
	Targets         map[string]interface{} `json:"targets,omitempty"`
}

// LoadConfig reads and parses <cosmDir>/config.json.
// If the configuration file does not exist, it returns a default configuration with
// LedgerMode: false and DefaultUniverse: "universe-main".
func LoadConfig(cosmDir string) (*WorkspaceConfig, error) {
	configPath := filepath.Join(cosmDir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &WorkspaceConfig{
				LedgerMode:      false,
				DefaultUniverse: "universe-main",
				CreatedAt:       time.Now().UTC(),
			}, nil
		}
		return nil, fmt.Errorf("failed to read workspace config from %s: %w", configPath, err)
	}

	var cfg WorkspaceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse workspace config: %w", err)
	}

	if cfg.DefaultUniverse == "" {
		cfg.DefaultUniverse = "universe-main"
	}

	return &cfg, nil
}

// SaveConfig atomically writes the configuration to <cosmDir>/config.json with fsync durability.
func SaveConfig(cosmDir string, cfg *WorkspaceConfig) error {
	if cfg == nil {
		return fmt.Errorf("workspace config cannot be nil")
	}

	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = time.Now().UTC()
	}
	if cfg.DefaultUniverse == "" {
		cfg.DefaultUniverse = "universe-main"
	}

	if err := os.MkdirAll(cosmDir, 0755); err != nil {
		return fmt.Errorf("failed to create cosm directory %s: %w", cosmDir, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize workspace config: %w", err)
	}
	data = append(data, '\n')

	tmpFile, err := os.CreateTemp(cosmDir, "config.*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary config file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write to temporary config file: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to sync temporary config file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary config file: %w", err)
	}

	destPath := filepath.Join(cosmDir, "config.json")
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to atomically update config at %s: %w", destPath, err)
	}

	return nil
}

// IsLedgerMode is a convenience helper checking whether ledger mode is active in the repository.
func IsLedgerMode(cosmDir string) bool {
	cfg, err := LoadConfig(cosmDir)
	if err != nil || cfg == nil {
		return false
	}
	return cfg.LedgerMode
}
