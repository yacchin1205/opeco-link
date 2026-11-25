//go:build integration

package notify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Two agents' MCP servers and a shell command of one machine use the one
// session its session_dir keeps: the second agent reuses the session the
// first paired, a response reaches the first process that reads it, the shell
// sends to the same card, and a closed session gives way to a new one.
func TestSharedSessionAcrossProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	baseURL := os.Getenv("OPECO_INTEGRATION_BASE_URL")
	if baseURL == "" {
		t.Fatal("OPECO_INTEGRATION_BASE_URL is required")
	}
	api, err := NewAPI(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	binary := filepath.Join(t.TempDir(), "opeco")
	if output, err := exec.CommandContext(ctx, "go", "build", "-o", binary, "../../cmd/opeco").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	home, sessionDir := t.TempDir(), t.TempDir()
	if err := os.Chmod(sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".opeco"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".opeco", "config.json"), []byte(`{"session_dir": "`+sessionDir+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "HOME="+home, "OPECO_SESSION_FILE=", "OPECO_SESSION_ID=")
	agent := func(name string) *mcp.ClientSession {
		command := exec.CommandContext(ctx, binary, "--base-url", baseURL, "--no-browser", "mcp")
		command.Env, command.Stderr = env, os.Stderr
		client := mcp.NewClient(&mcp.Implementation{Name: name, Version: "0.1.0"}, nil)
		session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
		if err != nil {
			t.Fatalf("connect %s: %v", name, err)
		}
		t.Cleanup(func() {
			if err := session.Close(); err != nil {
				t.Errorf("close %s: %v", name, err)
			}
		})
		return session
	}
	first, second := agent("first-agent"), agent("second-agent")

	created := callTool[sessionToolOutput](t, ctx, first, "session_create", map[string]any{"title": "Shared session"})
	if created.Reused || created.PairingURL == "" {
		t.Fatalf("first session_create: %+v", created)
	}
	group := joinFromPairingURL(t, ctx, api, created.PairingURL)
	reused := callTool[sessionToolOutput](t, ctx, second, "session_create", map[string]any{"title": "Another title"})
	if !reused.Reused || reused.SessionID != created.SessionID || reused.DeviceGroupCount != 1 || reused.PairingURL != "" {
		t.Fatalf("the second agent did not get the paired session: %+v", reused)
	}

	callTool[notifiedToolOutput](t, ctx, second, "notify", map[string]any{"session_id": created.SessionID, "message": "From the second agent"})
	shell := exec.CommandContext(ctx, binary, "status", "From the shell")
	shell.Env = env
	if output, err := shell.CombinedOutput(); err != nil || string(output) != "sent\n" {
		t.Fatalf("shell status: %v\n%s", err, output)
	}
	events, expiry := fetchAndDecryptEvents(t, ctx, api, created.SessionID, group)
	if len(events) != 2 || events[0].Type != "notify" || events[0].Message != "From the second agent" || events[1].Type != "status" || events[1].Status != "From the shell" {
		t.Fatalf("events on the card: %+v", events)
	}

	responseID := postEncryptedFeedback(t, ctx, api, created.SessionID, "Reply", group, expiry)
	got := callTool[responsesToolOutput](t, ctx, first, "responses_wait", map[string]any{"session_id": created.SessionID, "timeout_seconds": 5})
	if len(got.Responses) != 1 || got.Responses[0].ID != responseID {
		t.Fatalf("first agent's responses: %+v", got.Responses)
	}
	none := callTool[responsesToolOutput](t, ctx, second, "responses_wait", map[string]any{"session_id": created.SessionID, "timeout_seconds": 1})
	if len(none.Responses) != 0 {
		t.Fatalf("a response was handed over twice: %+v", none.Responses)
	}

	callTool[closeToolOutput](t, ctx, first, "session_close", map[string]any{"session_id": created.SessionID})
	if path, err := SharedStatePath(sessionDir); err == nil {
		t.Fatalf("the closed session is still kept: %s", path)
	}
	next := callTool[sessionToolOutput](t, ctx, second, "session_create", map[string]any{"title": "Next"})
	if next.Reused || next.SessionID == created.SessionID || next.PairingURL == "" {
		t.Fatalf("session_create after close: %+v", next)
	}
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatal(err)
	}
	var sessions []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "opeco-session-") {
			sessions = append(sessions, e.Name())
		}
	}
	if len(sessions) != 1 {
		t.Fatalf("session_dir holds %v", sessions)
	}
	callTool[closeToolOutput](t, ctx, second, "session_close", map[string]any{"session_id": next.SessionID})
}
