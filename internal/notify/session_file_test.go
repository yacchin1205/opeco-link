package notify

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func newTestSessionFile(t *testing.T) *SessionFile {
	t.Helper()
	api, err := NewAPI("https://opeco.link")
	if err != nil {
		t.Fatal(err)
	}
	api.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"expiresAt":1}`)), Header: make(http.Header)}, nil
	})
	file, _, err := CreateSessionFile(context.Background(), api, "Shell session", "#aabbcc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
		if err := os.RemoveAll(file.directory); err != nil {
			t.Error(err)
		}
	})
	return file
}

func TestSessionFilePreservesCreatorAndAuthenticatedState(t *testing.T) {
	t.Parallel()
	file := newTestSessionFile(t)
	original := file.Store.sessions[file.ID]
	original.responseCursor = 42
	original.openRequests["request"] = struct{}{}
	original.groups["group"] = &Group{
		ID: "group", PairingID: "pairing", InitialTimestamp: 1,
		InitialPublicKey: "initial", InitialTransitionHash: "anchor", HeadTransitionHash: "head",
		Timestamp: 2, PublicKey: "current", Keys: map[int64][]byte{1: {1, 2}, 2: {3, 4}},
		PublicKeys: map[int64]string{1: "initial", 2: "current"},
	}
	if err := file.Save(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSessionFile(context.Background(), file.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	}()
	got := reopened.Store.sessions[file.ID]
	if got.id != original.id || got.title != original.title || got.color != original.color || got.sessionToken != original.sessionToken || got.publicKey != original.publicKey || !got.privateKey.Equal(original.privateKey) || got.responseCursor != 42 || got.protocolVersion != 4 || got.tempDir != original.tempDir {
		t.Fatal("creator identity or cursor changed after reopening")
	}
	if !reflect.DeepEqual(got.pairings, original.pairings) || !reflect.DeepEqual(got.groups, original.groups) || !reflect.DeepEqual(got.openRequests, original.openRequests) {
		t.Fatal("pairing secrets, authenticated group history, or requests changed after reopening")
	}
	if runtime.GOOS != "windows" {
		for path, mode := range map[string]os.FileMode{file.directory: 0o700, file.Path(): 0o600, filepath.Join(file.directory, "lock"): 0o600, got.tempDir: 0o700} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != mode {
				t.Errorf("%s permissions = %o, want %o", filepath.Base(path), info.Mode().Perm(), mode)
			}
		}
	}
}

func TestSessionFileLockCanBeCancelledAndReleased(t *testing.T) {
	t.Parallel()
	file := newTestSessionFile(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := OpenSessionFile(ctx, file.Path()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("concurrent open error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenSessionFile(context.Background(), file.Path())
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSessionFileRemoveInvalidatesStateAndAttachments(t *testing.T) {
	t.Parallel()
	file := newTestSessionFile(t)
	attachment := filepath.Join(file.directory, "attachments", "photo.jpg")
	if err := os.WriteFile(attachment, []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := file.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory remains: %v", err)
	}
	if _, err := OpenSessionFile(context.Background(), file.Path()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed state opened: %v", err)
	}
}

func TestSessionFileRejectsCorruptionAndReleasesLock(t *testing.T) {
	t.Parallel()
	file := newTestSessionFile(t)
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file.Path(), []byte(`{"version":1`), 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err := OpenSessionFile(ctx, file.Path())
		cancel()
		if err == nil || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("corrupt state error = %v", err)
		}
	}
}

func TestSessionFileRejectsExposedState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	t.Parallel()
	file := newTestSessionFile(t)
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file.Path(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSessionFile(context.Background(), file.Path()); err == nil || !strings.Contains(err.Error(), "other users") {
		t.Fatalf("exposed state error = %v", err)
	}
}
