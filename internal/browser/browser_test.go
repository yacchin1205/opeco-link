package browser

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestOpenRefusesInsideAnSSHSession(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "10.0.0.2 52814 10.0.0.1 22")
	t.Setenv("PATH", t.TempDir())

	err := Open(context.Background(), "http://127.0.0.1:1/qr/x")
	if err == nil || !strings.Contains(err.Error(), "SSH") {
		t.Fatalf("Open() error = %v, want an SSH refusal", err)
	}
}

func TestOpenReportsAMissingOpener(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	t.Setenv("PATH", t.TempDir())

	err := Open(context.Background(), "http://127.0.0.1:1/qr/x")
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("Open() error = %v, want %v", err, exec.ErrNotFound)
	}
}
