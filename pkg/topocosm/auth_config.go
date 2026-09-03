package topocosm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AuthConfig stores client credentials on the developer workstation.
type AuthConfig struct {
	HubURL    string `json:"hub_url"`
	Token     string `json:"token"`
	UserEmail string `json:"user_email,omitempty"`
	UserUID   string `json:"user_uid,omitempty"`
	CallerDID string `json:"caller_did,omitempty"`
}

// GlobalAuthConfigPath returns the standard path ~/.cosm/auth.json.
func GlobalAuthConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cosm", "auth.json"), nil
}

// LoadAuthConfig loads the active configuration from environment, local repo, or global ~/.cosm/auth.json.
func LoadAuthConfig() (*AuthConfig, error) {
	cfg := &AuthConfig{
		HubURL:    "https://topocosm.dev",
		CallerDID: "did:key:z6MkuDeveloper",
	}

	// 1. Try global path
	if globalPath, err := GlobalAuthConfigPath(); err == nil {
		if data, err := os.ReadFile(globalPath); err == nil {
			_ = json.Unmarshal(data, cfg)
		}
	}

	// 2. Try repository local path (.cosm/auth.json)
	if data, err := os.ReadFile(filepath.Join(".cosm", "auth.json")); err == nil {
		_ = json.Unmarshal(data, cfg)
	}

	// 3. Environment overrides
	if envHub := os.Getenv("COSM_HUB_URL"); envHub != "" {
		cfg.HubURL = envHub
	}
	if envToken := os.Getenv("COSM_AUTH_TOKEN"); envToken != "" {
		cfg.Token = envToken
	}
	if envDID := os.Getenv("COSM_CALLER_DID"); envDID != "" {
		cfg.CallerDID = envDID
	}

	return cfg, nil
}

// SaveAuthConfig writes the configuration to ~/.cosm/auth.json.
func SaveAuthConfig(cfg *AuthConfig) error {
	globalPath, err := GlobalAuthConfigPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(globalPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create auth config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(globalPath, data, 0600)
}
