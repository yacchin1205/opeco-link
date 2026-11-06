package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type hookState struct {
	mu            sync.Mutex
	seenSession   string
	seenThrough   int64
	statusSession string
	status        string
	statusUpdated time.Time
	statusWarned  time.Time
}

type hookInput struct {
	Event          string `json:"event" jsonschema:"Hook event: UserPromptSubmit, PostToolUse or Stop"`
	StopHookActive bool   `json:"stop_hook_active,omitempty" jsonschema:"Pass stop_hook_active for Stop to prevent repeated continuation"`
	WithoutSession string `json:"without_session,omitempty" jsonschema:"What to tell the agent while no session is running; the hook is silent then when this is left out"`
}

type hookOutput struct {
	Decision       string       `json:"decision,omitempty"`
	Reason         string       `json:"reason,omitempty"`
	SpecificOutput *hookContext `json:"hookSpecificOutput,omitempty"`
}

type hookContext struct {
	Event   string `json:"hookEventName"`
	Context string `json:"additionalContext"`
}

type claudeHookInput struct {
	Event          string `json:"event" jsonschema:"Hook event: UserPromptSubmit, PostToolUse or Stop"`
	StopHookActive string `json:"stop_hook_active,omitempty" jsonschema:"Pass the interpolated stop_hook_active value for Stop"`
	WithoutSession string `json:"without_session,omitempty" jsonschema:"What to tell the agent while no session is running; the hook is silent then when this is left out"`
}

func (s *Server) checkClaudeHook(ctx context.Context, request *mcp.CallToolRequest, input claudeHookInput) (*mcp.CallToolResult, hookOutput, error) {
	converted := hookInput{Event: input.Event, WithoutSession: input.WithoutSession}
	if input.Event == "Stop" {
		// Claude's MCP hook interpolation turns booleans into strings.
		switch input.StopHookActive {
		case "true":
			converted.StopHookActive = true
		case "false":
		default:
			return nil, hookOutput{}, fmt.Errorf("stop_hook_active must be true or false")
		}
	}
	return s.checkHook(ctx, request, converted)
}

func (s *Server) checkHook(ctx context.Context, _ *mcp.CallToolRequest, input hookInput) (*mcp.CallToolResult, hookOutput, error) {
	switch input.Event {
	case "UserPromptSubmit", "PostToolUse", "Stop":
	default:
		return nil, hookOutput{}, fmt.Errorf("event must be UserPromptSubmit, PostToolUse or Stop")
	}
	if input.Event == "Stop" && input.StopHookActive {
		return hookResult(hookOutput{})
	}
	s.hooks.mu.Lock()
	defer s.hooks.mu.Unlock()
	sessionID, count, through, err := s.store.PendingResponses(ctx)
	if err != nil {
		return nil, hookOutput{}, err
	}
	return hookResult(s.hooks.output(input, sessionID, count, through, time.Now()))
}

func (h *hookState) output(input hookInput, sessionID string, count int, through int64, now time.Time) hookOutput {
	if input.Event == "Stop" && input.StopHookActive {
		return hookOutput{}
	}
	if sessionID == "" {
		// Only this process knows that no session is running, but whether
		// the agent should then be pushed to pair is up to the environment
		// that installs the hook, so the words are its own.
		return reminder(input.Event, input.WithoutSession)
	}
	var messages []string
	if count > 0 && (input.Event == "Stop" || h.seenSession != sessionID || through > h.seenThrough) {
		h.seenSession, h.seenThrough = sessionID, through
		messages = append(messages, fmt.Sprintf("opeco.link has %d unread client response(s). Call responses_wait with session_id=%q and timeout_seconds=1 to read them and any photos, then consider their effect on the current task. This hook has not consumed the responses.", count, sessionID))
	}
	const statusInterval = 5 * time.Minute
	if h.statusSession == sessionID && !h.statusUpdated.IsZero() && now.Sub(h.statusUpdated) >= statusInterval && now.Sub(h.statusWarned) >= statusInterval {
		messages = append(messages, fmt.Sprintf("opeco.link: your last status update was %d minutes ago. Last status (quoted data): %q. Review your current work and decide whether to update status.", int(now.Sub(h.statusUpdated)/time.Minute), h.status))
		h.statusWarned = now
	}
	return reminder(input.Event, strings.Join(messages, "\n"))
}

// reminder puts message in the form the event's hook contract takes; an
// empty message is no reminder.
func reminder(event, message string) hookOutput {
	if message == "" {
		return hookOutput{}
	}
	if event == "Stop" {
		return hookOutput{Decision: "block", Reason: message}
	}
	return hookOutput{SpecificOutput: &hookContext{Event: event, Context: message}}
}

func hookResult(output hookOutput) (*mcp.CallToolResult, hookOutput, error) {
	// Include the hook contract in both MCP text and structured content so
	// runtimes can parse the result without an additional command adapter.
	data, err := json.Marshal(output)
	if err != nil {
		return nil, hookOutput{}, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, output, nil
}
