package notify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func sharedRelay(t *testing.T) (*API, *atomic.Bool) {
	t.Helper()
	gone := &atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/sessions":
			fmt.Fprint(w, `{"expiresAt":1}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/sessions/") && gone.Load():
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"error":"session_not_found"}`)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/sessions/"):
			fmt.Fprint(w, `{"groups":[],"expiresAt":1}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	api, err := NewAPI(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return api, gone
}

func privateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The directory keeps one session: asking again returns it, and a session
// the relay no longer knows gives way to a new one in its place.
func TestSharedSessionsKeepOneSession(t *testing.T) {
	api, gone := sharedRelay(t)
	dir := privateDir(t)
	sessions, err := NewSharedSessions(dir, api)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	created, err := sessions.EnsureSession(ctx, "Shared", "#aabbcc")
	if err != nil || created.Reused || created.PairingURL == "" {
		t.Fatalf("create: %+v %v", created, err)
	}
	again, err := sessions.EnsureSession(ctx, "Other", "#aabbcc")
	if err != nil || !again.Reused || again.SessionID != created.SessionID || again.Title != "Shared" {
		t.Fatalf("reuse: %+v %v", again, err)
	}
	if err := sessions.Use(ctx, "another", func(*Store) error { return nil }); err == nil || !strings.Contains(err.Error(), "unknown local session") {
		t.Fatalf("use of another session: %v", err)
	}

	gone.Store(true)
	replaced, err := sessions.EnsureSession(ctx, "Replaced", "#aabbcc")
	if err != nil || replaced.Reused || replaced.SessionID == created.SessionID {
		t.Fatalf("replace: %+v %v", replaced, err)
	}
	path, err := SharedStatePath(dir)
	if err != nil || !strings.Contains(path, "opeco-session-") {
		t.Fatalf("state after replace: %s %v", path, err)
	}
	if file, err := OpenSessionFile(ctx, path); err != nil || file.ID != replaced.SessionID {
		t.Fatalf("the kept session is not the new one: %v", err)
	} else if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSharedSessionsRefuseAnUnsafeDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	api, _ := sharedRelay(t)
	if _, err := NewSharedSessions("relative", api); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative path: %v", err)
	}
	open := t.TempDir()
	if err := os.Chmod(open, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSharedSessions(open, api); err == nil || !strings.Contains(err.Error(), "other users") {
		t.Fatalf("directory others can read: %v", err)
	}
}

func TestSharedStatePath(t *testing.T) {
	dir := privateDir(t)
	if _, err := SharedStatePath(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty directory: %v", err)
	}
	for _, name := range []string{"opeco-session-a", "opeco-session-b"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "state.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := SharedStatePath(dir); err == nil || !strings.Contains(err.Error(), "holds 2 sessions") {
		t.Fatalf("two sessions: %v", err)
	}
}
