package browser

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenRefusesInsideAnSSHSession(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "10.0.0.2 52814 10.0.0.1 22")
	t.Setenv("BROWSER", "")
	t.Setenv("PATH", t.TempDir())

	err := Open(context.Background(), "http://127.0.0.1:1/qr/x")
	if err == nil || !strings.Contains(err.Error(), "SSH") {
		t.Fatalf("Open() error = %v, want an SSH refusal", err)
	}
}

func TestOpenReportsAMissingOpener(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	t.Setenv("BROWSER", "")
	t.Setenv("PATH", t.TempDir())

	err := Open(context.Background(), "http://127.0.0.1:1/qr/x")
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("Open() error = %v, want %v", err, exec.ErrNotFound)
	}
}

func TestOpenRunsTheBrowserCommandWithTheURL(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, "opened")
	script := filepath.Join(dir, "browser.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf %s \"$1\" > "+record+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// BROWSER names where the person wants the page, so an SSH session is no reason to refuse.
	t.Setenv("SSH_CONNECTION", "10.0.0.2 52814 10.0.0.1 22")
	t.Setenv("BROWSER", script)

	if err := Open(context.Background(), "http://127.0.0.1:1/qr/x"); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	opened, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if string(opened) != "http://127.0.0.1:1/qr/x" {
		t.Fatalf("browser command received %q", opened)
	}
}

func TestOpenReportsABrowserCommandFailure(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "browser.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho no display >&2\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BROWSER", script)

	err := Open(context.Background(), "http://127.0.0.1:1/qr/x")
	if err == nil || !strings.Contains(err.Error(), "no display") {
		t.Fatalf("Open() error = %v, want the command's failure detail", err)
	}
}
