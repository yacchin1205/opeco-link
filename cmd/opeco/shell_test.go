package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"opeco.link/internal/notify"
)

func TestShellQuoteSurvivesEvaluation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell")
	}
	t.Parallel()
	value := "spaces ' quotes \" ; $(exit 29) `exit 31`\nnext line"
	output, err := exec.Command("sh", "-c", "value="+shellQuote(value)+"; printf '%s' \"$value\"").Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != value {
		t.Fatalf("round trip = %q", output)
	}
}

func TestShellSessionLifecycleAndErrors(t *testing.T) {
	t.Parallel()
	closed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/sessions":
			fmt.Fprint(w, `{"expiresAt":1}`)
		case r.Method == "DELETE":
			closed = true
			w.WriteHeader(http.StatusNoContent)
		case r.Method == "GET":
			fmt.Fprint(w, `{"groups":[],"expiresAt":1}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	defer server.Close()
	api, err := notify.NewAPI(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := startShellSession(context.Background(), api, "Shell test", "#aabbcc", false, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "export OPECO_SESSION_FILE='") || !strings.HasPrefix(lines[1], "export OPECO_SESSION_ID='") {
		t.Fatal("stdout must contain only shell exports")
	}
	path := strings.TrimSuffix(strings.TrimPrefix(lines[0], "export OPECO_SESSION_FILE='"), "';")
	t.Cleanup(func() {
		if err := os.RemoveAll(filepath.Dir(path)); err != nil {
			t.Error(err)
		}
	})
	if !strings.HasPrefix(stderr.String(), server.URL+"/join#") {
		t.Fatal("pairing URL missing from stderr")
	}
	stdout.Reset()
	if err := shellCommand(context.Background(), path, []string{"join"}, false, &stdout); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "0 device group(s) joined\n" {
		t.Fatalf("join output = %q", stdout.String())
	}
	if err := shellCommand(context.Background(), path, []string{"notify", "hello"}, false, io.Discard); err == nil {
		t.Fatal("notification without recipients succeeded")
	}
	if err := shellCommand(context.Background(), path, []string{"close"}, false, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !closed {
		t.Fatal("remote session was not closed")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed state remains: %v", err)
	}
	if err := shellCommand(context.Background(), path, []string{"join"}, false, io.Discard); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing state error = %v", err)
	}
}

func TestValidateShellCommand(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"unknown"}, {"notify"}, {"status"}, {"pair", "extra"}, {"close", "extra"}, {"color"}, {"request", "question", "only one"}} {
		if err := validateShellCommand(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	for _, args := range [][]string{{"join"}, {"notify", "one", "two"}, {"request", "question", "Yes", "No"}, {"close-request", "id"}} {
		if err := validateShellCommand(args); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
}

func TestShellRemovesExpiredStateButPreservesOtherAPIErrors(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		code    string
		status  int
		removed bool
	}{
		{"session_expired", 410, true},
		{"session_not_found", 404, true},
		{"attachment_not_found", 404, false},
		{"forbidden", 403, false},
	} {
		t.Run(test.code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" {
					fmt.Fprint(w, `{"expiresAt":1}`)
					return
				}
				w.WriteHeader(test.status)
				fmt.Fprintf(w, `{"error":%q,"message":"test failure"}`, test.code)
			}))
			defer server.Close()
			api, err := notify.NewAPI(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			file, _, err := notify.CreateSessionFile(context.Background(), api, "Expiration test", "random")
			if err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.RemoveAll(filepath.Dir(file.Path())); err != nil {
					t.Error(err)
				}
			})
			err = shellCommand(context.Background(), file.Path(), []string{"join"}, false, io.Discard)
			var apiError *notify.APIError
			if !errors.As(err, &apiError) || apiError.Code != test.code {
				t.Fatalf("original API error lost: %v", err)
			}
			_, stateErr := os.Stat(file.Path())
			if test.removed {
				if !errors.Is(stateErr, os.ErrNotExist) {
					t.Fatalf("expired state remains: %v", stateErr)
				}
			} else if stateErr != nil {
				t.Fatalf("unexpired state removed: %v", stateErr)
			}
		})
	}
}
