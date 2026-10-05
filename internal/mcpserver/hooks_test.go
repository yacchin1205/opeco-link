package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"opeco.link/internal/notify"
)

func TestHookUnreadResponses(t *testing.T) {
	var h hookState
	now := time.Now()
	post := hookInput{Event: "PostToolUse"}
	first := h.output(post, "session", 2, 7, now)
	if first.SpecificOutput == nil || !strings.Contains(first.SpecificOutput.Context, "responses_wait") {
		t.Fatalf("missing retrieval reminder: %+v", first)
	}
	if again := h.output(post, "session", 2, 7, now); again.SpecificOutput != nil {
		t.Fatal("repeated tool hook repeated the same reminder")
	}
	if next := h.output(post, "session", 3, 8, now); next.SpecificOutput == nil {
		t.Fatal("new arrival was suppressed")
	}
	stop := h.output(hookInput{Event: "Stop"}, "session", 3, 8, now)
	if stop.Decision != "block" || stop.Reason == "" {
		t.Fatal("Stop did not prompt for an unread response after a tool reminder")
	}
	continued := h.output(hookInput{Event: "Stop", StopHookActive: true}, "session", 4, 9, now)
	if continued.Decision != "" {
		t.Fatal("a new arrival blocked Stop again")
	}
	if empty := h.output(hookInput{Event: "Stop"}, "session", 0, 0, now); empty.Decision != "" {
		t.Fatal("consumed responses still blocked Stop")
	}
	if other := h.output(post, "another-session", 1, 1, now); other.SpecificOutput == nil {
		t.Fatal("new session lost its reminder")
	}
}

func TestHookStatusReminderTiming(t *testing.T) {
	start := time.Now()
	h := hookState{statusSession: "session", status: "Running tests", statusUpdated: start}
	post := hookInput{Event: "PostToolUse"}
	for _, tc := range []struct {
		elapsed time.Duration
		want    string
	}{
		{5*time.Minute - time.Nanosecond, ""},
		{7 * time.Minute, "7 minutes ago"},
		{12*time.Minute - time.Nanosecond, ""},
		{12 * time.Minute, "12 minutes ago"},
	} {
		got := h.output(post, "session", 0, 0, start.Add(tc.elapsed))
		if tc.want == "" {
			if got.SpecificOutput != nil {
				t.Fatalf("early status reminder at %v: %+v", tc.elapsed, got)
			}
		} else if got.SpecificOutput == nil || !strings.Contains(got.SpecificOutput.Context, tc.want) || !strings.Contains(got.SpecificOutput.Context, `"Running tests"`) {
			t.Fatalf("reminder at %v = %+v, want elapsed time and status text", tc.elapsed, got)
		}
	}

	// A status update restarts the five-minute interval.
	h.status = "Reviewing results"
	h.statusUpdated = start.Add(13 * time.Minute)
	h.statusWarned = time.Time{}
	if got := h.output(post, "session", 0, 0, start.Add(18*time.Minute-time.Nanosecond)); got.SpecificOutput != nil {
		t.Fatal("new status inherited the old warning schedule")
	}
	got := h.output(post, "session", 0, 0, start.Add(18*time.Minute))
	if got.SpecificOutput == nil || !strings.Contains(got.SpecificOutput.Context, `"Reviewing results"`) || !strings.Contains(got.SpecificOutput.Context, "5 minutes ago") {
		t.Fatalf("missing reminder for updated status: %+v", got)
	}
}

func TestHookStatusAndResponsesAreIndependent(t *testing.T) {
	start := time.Now()
	for _, count := range []int{0, 1} {
		h := hookState{statusSession: "session", status: "Testing", statusUpdated: start}
		post := hookInput{Event: "PostToolUse"}
		if count > 0 {
			h.output(post, "session", count, 1, start)
		}
		got := h.output(post, "session", count, 1, start.Add(5*time.Minute))
		if got.SpecificOutput == nil || !strings.Contains(got.SpecificOutput.Context, "5 minutes ago") {
			t.Fatalf("status warning suppressed with %d previously announced responses", count)
		}
		if strings.Contains(got.SpecificOutput.Context, "responses_wait") {
			t.Fatal("status warning repeated an old response reminder")
		}
		got = h.output(post, "session", 2, 2, start.Add(6*time.Minute))
		if got.SpecificOutput == nil || !strings.Contains(got.SpecificOutput.Context, "responses_wait") || strings.Contains(got.SpecificOutput.Context, "minutes ago") {
			t.Fatal("new response reminder was suppressed or repeated a status warning")
		}
	}
}

func TestHookStatusLifecycle(t *testing.T) {
	start := time.Now()
	for _, session := range []string{"", "other-session"} {
		h := hookState{statusSession: "session", status: "Old status", statusUpdated: start}
		if got := h.output(hookInput{Event: "PostToolUse"}, session, 0, 0, start.Add(time.Hour)); got.SpecificOutput != nil {
			t.Fatalf("session %q inherited old status", session)
		}
	}
	var h hookState
	if got := h.output(hookInput{Event: "PostToolUse"}, "session", 0, 0, start); got.SpecificOutput != nil {
		t.Fatal("warned before any status was sent")
	}
	h = hookState{statusSession: "session", status: "Testing", statusUpdated: start}
	stop := h.output(hookInput{Event: "Stop"}, "session", 0, 0, start.Add(5*time.Minute))
	if stop.Decision != "block" || !strings.Contains(stop.Reason, "5 minutes ago") {
		t.Fatal("Stop did not deliver a due status reminder")
	}
	continued := h.output(hookInput{Event: "Stop", StopHookActive: true}, "session", 0, 0, start.Add(10*time.Minute))
	if continued.Decision != "" {
		t.Fatal("stale status prolonged Stop a second time")
	}
}

func TestHookWithoutSessionIsSilent(t *testing.T) {
	s := New(notify.NewStore(nil), nil, nil)
	result, output, err := s.checkHook(context.Background(), nil, hookInput{Event: "Stop"})
	if err != nil {
		t.Fatal(err)
	}
	if output != (hookOutput{}) || len(result.Content) != 1 {
		t.Fatalf("inactive hook = %+v", output)
	}
	if _, _, err := s.checkHook(context.Background(), nil, hookInput{Event: "unsupported"}); err == nil {
		t.Fatal("unsupported event accepted")
	}
}

func TestHookPreservesAPIError(t *testing.T) {
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/sessions" {
			w.Write([]byte(`{"expiresAt":0}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"test_failure","message":"original cause"}`))
	}))
	defer relay.Close()
	api, err := notify.NewAPI(relay.URL)
	if err != nil {
		t.Fatal(err)
	}
	store := notify.NewStore(api)
	id, _, err := store.Create(context.Background(), "Test", "random")
	if err != nil {
		t.Fatal(err)
	}
	s := New(store, nil, nil)
	_, _, err = s.checkHook(context.Background(), nil, hookInput{Event: "Stop"})
	var apiError *notify.APIError
	if !errors.As(err, &apiError) || apiError.Code != "test_failure" || apiError.Message != "original cause" {
		t.Fatalf("API error was hidden: %v", err)
	}
	// A failed status send must not reset the last successful update.
	updated := time.Now().Add(-time.Hour)
	s.hooks.statusUpdated = updated
	_, _, err = s.status(context.Background(), nil, statusInput{SessionID: id, Status: "Not delivered"})
	if err == nil || s.hooks.statusUpdated != updated {
		t.Fatal("failed status send reset the warning clock")
	}
	if _, output, err := s.checkHook(context.Background(), nil, hookInput{Event: "Stop", StopHookActive: true}); err != nil || output != (hookOutput{}) {
		t.Fatal("continued Stop made another API request")
	}
}

func TestHookMCPTextMatchesStructuredOutput(t *testing.T) {
	for _, event := range []string{"PostToolUse", "Stop"} {
		var h hookState
		result, output, err := hookResult(h.output(hookInput{Event: event}, "session", 1, 1, time.Now()))
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(result.Content[0])
		if err != nil {
			t.Fatal(err)
		}
		var content struct{ Text string }
		if err := json.Unmarshal(data, &content); err != nil {
			t.Fatal(err)
		}
		var parsed hookOutput
		if err := json.Unmarshal([]byte(content.Text), &parsed); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(parsed, output) {
			t.Fatalf("MCP text differs from structured output for %s", event)
		}
	}
}

func TestClaudeHookInput(t *testing.T) {
	s := New(notify.NewStore(nil), nil, nil)
	for _, input := range []claudeHookInput{
		{Event: "PostToolUse"},
		{Event: "Stop", StopHookActive: "false"},
		{Event: "Stop", StopHookActive: "true"},
	} {
		if _, output, err := s.checkClaudeHook(context.Background(), nil, input); err != nil || output != (hookOutput{}) {
			t.Fatalf("Claude input %+v: output=%+v err=%v", input, output, err)
		}
	}
	for _, value := range []string{"", "invalid", "0"} {
		if _, _, err := s.checkClaudeHook(context.Background(), nil, claudeHookInput{Event: "Stop", StopHookActive: value}); err == nil {
			t.Fatalf("accepted invalid Stop flag %q", value)
		}
	}
}
