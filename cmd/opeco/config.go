package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// config is the machine owner's choice, ~/.opeco/config.json. It is a file
// and not a flag because keeping a session's keys on disk is the owner's to
// decide: a flag would go in an agent's MCP settings, which the agent can
// edit as easily as anyone.
type config struct {
	// SessionDir keeps one session in that directory for every opeco
	// process of the machine to share, with its keys on disk.
	SessionDir string `json:"session_dir"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".opeco", "config.json"), nil
}

// loadConfig returns the defaults when the file does not exist.
func loadConfig() (config, error) {
	path, err := configPath()
	if err != nil {
		return config{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return config{}, nil
	}
	if err != nil {
		return config{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var cfg config
	if err := decoder.Decode(&cfg); err != nil {
		return config{}, fmt.Errorf("read %s: %w", path, err)
	}
	return cfg, nil
}
