//go:build integration

package notify

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestShellCommandsAcrossProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell")
	}
	baseURL := os.Getenv("OPECO_INTEGRATION_BASE_URL")
	if baseURL == "" {
		t.Fatal("OPECO_INTEGRATION_BASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "opeco")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../cmd/opeco")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	var exports, pairing bytes.Buffer
	create := exec.CommandContext(ctx, "sh", "-c", `eval "$("$1" --base-url "$2" --title 'Shell integration' --no-terminal-qr)"; printf '%s\n%s\n' "$OPECO_SESSION_FILE" "$OPECO_SESSION_ID"`, "shell-init", binary, baseURL)
	create.Stdout, create.Stderr = &exports, &pairing
	if err := create.Run(); err != nil {
		t.Fatalf("create: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(exports.String()), "\n")
	if len(lines) != 2 || lines[0] == "" || lines[1] == "" {
		t.Fatal("missing session exports")
	}
	path, id := lines[0], lines[1]
	t.Cleanup(func() {
		if err := os.RemoveAll(filepath.Dir(path)); err != nil {
			t.Error(err)
		}
	})
	command := func(args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = append(os.Environ(), "OPECO_SESSION_FILE="+path, "OPECO_SESSION_ID="+id)
		return cmd
	}
	run := func(args ...string) string {
		t.Helper()
		output, err := command(args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", args[0], err, output)
		}
		return string(output)
	}
	api, err := NewAPI(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	group := joinFromPairingURL(t, ctx, api, strings.TrimSpace(pairing.String()))
	if output := run("join"); output != "1 device group(s) joined\n" {
		t.Fatalf("join: %q", output)
	}
	run("status", "Building")
	var concurrent sync.WaitGroup
	for range 4 {
		concurrent.Go(func() {
			if output, err := command("notify", "Parallel notification").CombinedOutput(); err != nil {
				t.Errorf("parallel notify: %v\n%s", err, output)
			}
		})
	}
	concurrent.Wait()
	run("request", "Continue?", "Yes", "No")
	events, expiry := fetchAndDecryptEvents(t, ctx, api, id, group)
	if len(events) != 6 {
		t.Fatalf("events = %d, want 6", len(events))
	}
	request := events[5]
	if request.Type != "request" || request.Prompt != "Continue?" {
		t.Fatal("request was not decrypted")
	}
	postEncryptedRequestResult(t, ctx, api, id, request.RequestID, "response", request.Options[0].ID, group, expiry)
	postEncryptedFeedback(t, ctx, api, id, "A shell reply", group, expiry)
	responses := run("responses")
	if !strings.Contains(responses, "response request="+request.RequestID) || !strings.Contains(responses, `feedback message="A shell reply"`) {
		t.Fatalf("responses = %q", responses)
	}
	if output := run("responses"); output != "" {
		t.Fatalf("responses repeated: %q", output)
	}
	_, jpeg := postEncryptedAttachment(t, ctx, api, id, group, expiry)
	if output := run("responses"); !strings.Contains(output, "attachment[3]=") {
		t.Fatalf("photo paths missing: %q", output)
	}
	run("join")
	attachmentDirectory := filepath.Join(filepath.Dir(path), "attachments")
	photos, err := os.ReadDir(attachmentDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != 3 {
		t.Fatalf("saved photos = %d, want 3", len(photos))
	}
	for _, photo := range photos {
		data, err := os.ReadFile(filepath.Join(attachmentDirectory, photo.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, jpeg) {
			t.Fatal("decrypted photo changed across commands")
		}
	}
	run("close-request", request.RequestID)
	run("close")
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed state exists: %v", err)
	}
	if _, err := os.Stat(attachmentDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed attachments remain: %v", err)
	}
	if err := command("status", "after close").Run(); err == nil {
		t.Fatal("closed command exited successfully")
	}
	interactive := command("--interactive", "--base-url", baseURL, "--no-terminal-qr")
	interactive.Stdin = strings.NewReader("close\n")
	output, err := interactive.CombinedOutput()
	if err != nil {
		t.Fatalf("interactive: %v", err)
	}
	if !bytes.Contains(output, []byte("QR image: http://127.0.0.1:")) || !bytes.Contains(output, []byte("opeco> closed")) {
		t.Fatal("explicit interactive mode did not create and close a session")
	}
}
