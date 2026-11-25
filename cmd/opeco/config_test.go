package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if cfg, err := loadConfig(); err != nil || cfg.SessionDir != "" {
		t.Fatalf("without a file: %+v %v", cfg, err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".opeco"), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(content string) {
		if err := os.WriteFile(filepath.Join(home, ".opeco", "config.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"session_dir": "/var/lib/opeco"}`)
	if cfg, err := loadConfig(); err != nil || cfg.SessionDir != "/var/lib/opeco" {
		t.Fatalf("session_dir: %+v %v", cfg, err)
	}
	write(`{"session_directory": "/var/lib/opeco"}`)
	if _, err := loadConfig(); err == nil || !strings.Contains(err.Error(), "session_directory") {
		t.Fatalf("unknown key: %v", err)
	}
}

// A shell command without a session of its own uses the configured one, and
// says how one starts when there is none yet.
func TestShellSessionPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if path, err := shellSessionPath("/own/state.json"); err != nil || path != "/own/state.json" {
		t.Fatalf("own session: %s %v", path, err)
	}
	if path, err := shellSessionPath(""); err != nil || path != "" {
		t.Fatalf("no config: %s %v", path, err)
	}
	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(filepath.Join(home, ".opeco"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".opeco", "config.json"), []byte(`{"session_dir": "`+dir+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := shellSessionPath(""); err == nil || !strings.Contains(err.Error(), "session_create") {
		t.Fatalf("no session yet: %v", err)
	}
	state := filepath.Join(dir, "opeco-session-x", "state.json")
	if err := os.MkdirAll(filepath.Dir(state), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if path, err := shellSessionPath(""); err != nil || path != state {
		t.Fatalf("configured session: %s %v", path, err)
	}
}
