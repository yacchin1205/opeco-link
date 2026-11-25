package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"opeco.link/internal/browser"
	"opeco.link/internal/notify"
)

const instructions = "Call session_create for the work the user wants to follow; it returns the session already running when there is one, this process's or, where the machine keeps one session for all its agents, the machine's, so an agent never manages more than one session and never asks for a second QR code. Add a device with session_pairing_create, and start a separate session only by closing the running one first, at the user's request. opeco opens the QR image in the browser on its own machine when it can: qr_opened true means the person can scan the browser window now, while qr_open_error explains why nothing opened, in which case give them the local QR image URL, or the pairing URL when their browser is on another machine. Do not claim a device is paired until session_wait_for_device confirms it, and do not send events before that. Send status and notifications as work changes. Every send result may carry responses from devices, including messages the user wrote without being asked; read that field every time, because responses are handed over once and nothing else will surface them. A request may have multiple choices. Forward every response to the agent; opeco does not select or aggregate responses. Close a session only when immediate removal is intended; normal process exit leaves it to expire."

type Server struct {
	sessions Sessions
	viewer   *notify.QRViewer
	opener   browser.Opener
	hooks    hookState
}

// Sessions is where the server's session lives: in this process, or in a
// directory every opeco process of the machine shares.
type Sessions interface {
	EnsureSession(ctx context.Context, title, color string) (notify.SessionState, error)
	// Use runs fn on the store that holds the session.
	Use(ctx context.Context, sessionID string, fn func(*notify.Store) error) error
	PendingResponses(ctx context.Context) (sessionID string, count int, through int64, err error)
}

// InProcess keeps the session in this process's memory: it ends with the
// process.
func InProcess(store *notify.Store) Sessions { return inProcess{store} }

type inProcess struct{ store *notify.Store }

func (p inProcess) EnsureSession(ctx context.Context, title, color string) (notify.SessionState, error) {
	return p.store.EnsureSession(ctx, title, color)
}

func (p inProcess) Use(_ context.Context, _ string, fn func(*notify.Store) error) error {
	return fn(p.store)
}

func (p inProcess) PendingResponses(ctx context.Context) (string, int, int64, error) {
	return p.store.PendingResponses(ctx)
}

func New(sessions Sessions, viewer *notify.QRViewer, opener browser.Opener) *Server {
	return &Server{sessions: sessions, viewer: viewer, opener: opener}
}

// use runs fn on the store that holds the session.
func (s *Server) use(ctx context.Context, sessionID string, fn func(*notify.Store) error) error {
	return s.sessions.Use(ctx, sessionID, fn)
}

func (s *Server) Run(ctx context.Context) error {
	server := mcp.NewServer(&mcp.Implementation{
		Name:        "opeco-link",
		Title:       "opeco.link",
		Description: "Ephemeral encrypted notifications between agent sessions and device groups",
		Version:     "0.1.0",
		WebsiteURL:  "https://opeco.link",
	}, &mcp.ServerOptions{Instructions: instructions})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "session_create",
		Description: "Return the session already running, this process's or the one the machine keeps for all its agents, or create one when there is none. A session identifies the agent, so do not expect a second one: to start a separate session, close the running one first. The pairing URL and QR image URL are returned only while no device group has joined yet.",
	}, s.create)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "session_pairing_create",
		Description: "Create another one-shot pairing so an additional device group can join an existing session. Use this to add a device rather than closing and recreating the session.",
	}, s.addPairing)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "session_wait_for_device",
		Description: "Wait until at least one device group has authenticated itself to a session.",
	}, s.waitForDevice)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "notify",
		Description: "Send an encrypted notification to every device group joined to a session. The result also hands over any responses received so far.",
	}, s.notify)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "status",
		Description: "Update the encrypted current status shown on a session card. Silent by default; a device that turned on attention for the session gets an OS alert. The result also hands over any responses received so far.",
	}, s.status)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "session_color",
		Description: "Change the encrypted #rrggbb color of a session card, or choose another random pastel color. The result also hands over any responses received so far.",
	}, s.color)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "request",
		Description: "Send an encrypted question with two or more choices to every joined device group. The result also hands over any responses received before the question.",
	}, s.request)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "request_close",
		Description: "End an open request on every joined device group. The result also hands over any responses received so far.",
	}, s.closeRequest)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "responses_wait",
		Description: "Wait for and return every encrypted choice, dismissal, or message not yet handed over. The agent decides how to interpret them.",
	}, s.waitResponses)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "codex_hook_check",
		Description: "Codex lifecycle hook. Remind the agent about unread client responses and stale status without consuming responses or creating a session. While no session is running, say only what without_session passes. Use responses_wait to retrieve responses and photos.",
	}, s.checkHook)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "claude_hook_check",
		Description: "Claude lifecycle hook. Remind the agent about unread client responses and stale status without consuming responses or creating a session. While no session is running, say only what without_session passes. Use responses_wait to retrieve responses and photos.",
	}, s.checkClaudeHook)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "session_close",
		Description: "Immediately close a session and remove its card from devices, handing over any unread responses. Call it when the user asks for a separate session, since session_create otherwise returns the running one. Do not call this for ordinary process exit.",
	}, s.close)

	return server.Run(ctx, &mcp.StdioTransport{})
}

// sessionOutput reports whether the caller got a new session or the one this
// process was already running, so an agent asked to notify does not start a
// second card for work the person is already following. The pairing fields are
// absent once a device group has joined: nothing needs to be scanned then.
type sessionOutput struct {
	SessionID        string `json:"session_id"`
	Title            string `json:"title"`
	Reused           bool   `json:"reused"`
	DeviceGroupCount int    `json:"device_group_count"`
	PairingURL       string `json:"pairing_url,omitempty"`
	QRImageURL       string `json:"qr_image_url,omitempty"`
	QROpened         bool   `json:"qr_opened,omitempty"`
	QROpenError      string `json:"qr_open_error,omitempty"`
}

type createInput struct {
	Title string `json:"title" jsonschema:"short title shown on the session card"`
	Color string `json:"color,omitempty" jsonschema:"optional panel color as #rrggbb; omit or use random to select from the pastel palette"`
}

// pairingOutput deliberately omits the terminal QR code. The MCP result passes
// through an agent's rendering before a person sees it, and neither opeco nor
// the agent can inspect the result, so a block-character QR cannot be trusted to
// stay scannable. Callers show the loopback image, or the pairing URL when the
// browser is not on this machine. opeco opens the image itself when it can and
// reports the outcome, because it is the only party that can observe it.
type pairingOutput struct {
	SessionID   string `json:"session_id"`
	PairingURL  string `json:"pairing_url"`
	QRImageURL  string `json:"qr_image_url"`
	QROpened    bool   `json:"qr_opened,omitempty"`
	QROpenError string `json:"qr_open_error,omitempty"`
}

func (s *Server) create(ctx context.Context, _ *mcp.CallToolRequest, input createInput) (*mcp.CallToolResult, sessionOutput, error) {
	state, err := s.sessions.EnsureSession(ctx, input.Title, input.Color)
	if err != nil {
		return nil, sessionOutput{}, err
	}
	output := sessionOutput{
		SessionID:        state.SessionID,
		Title:            state.Title,
		Reused:           state.Reused,
		DeviceGroupCount: state.DeviceGroupCount,
		PairingURL:       state.PairingURL,
	}
	if state.PairingURL != "" {
		imageURL, err := s.viewer.Publish(state.PairingURL, s.pairingJoined(state.SessionID, state.PairingURL))
		if err != nil {
			return nil, sessionOutput{}, err
		}
		output.QRImageURL = imageURL
		output.QROpened, output.QROpenError = s.openQR(ctx, imageURL)
	}
	return nil, output, nil
}

func (s *Server) openQR(ctx context.Context, imageURL string) (bool, string) {
	if err := s.opener(ctx, imageURL); err != nil {
		return false, err.Error()
	}
	return true, ""
}

type sessionInput struct {
	SessionID string `json:"session_id" jsonschema:"session identifier returned by session_create"`
}

func (s *Server) addPairing(ctx context.Context, _ *mcp.CallToolRequest, input sessionInput) (*mcp.CallToolResult, pairingOutput, error) {
	var pairingURL string
	err := s.use(ctx, input.SessionID, func(store *notify.Store) (err error) {
		pairingURL, err = store.AddPairing(ctx, input.SessionID)
		return err
	})
	if err != nil {
		return nil, pairingOutput{}, err
	}
	imageURL, err := s.viewer.Publish(pairingURL, s.pairingJoined(input.SessionID, pairingURL))
	if err != nil {
		return nil, pairingOutput{}, err
	}
	output := pairingOutput{SessionID: input.SessionID, PairingURL: pairingURL, QRImageURL: imageURL}
	output.QROpened, output.QROpenError = s.openQR(ctx, imageURL)
	return nil, output, nil
}

type waitInput struct {
	SessionID     string `json:"session_id" jsonschema:"session identifier"`
	TimeoutSecond int    `json:"timeout_seconds" jsonschema:"maximum wait in seconds, from 1 through 600"`
}

type waitDeviceOutput struct {
	DeviceGroupCount int `json:"device_group_count"`
}

func (s *Server) waitForDevice(ctx context.Context, _ *mcp.CallToolRequest, input waitInput) (*mcp.CallToolResult, waitDeviceOutput, error) {
	timeout, err := timeoutDuration(input.TimeoutSecond)
	if err != nil {
		return nil, waitDeviceOutput{}, err
	}
	var count int
	err = poll(ctx, timeout, func() (bool, error) {
		err := s.use(ctx, input.SessionID, func(store *notify.Store) (err error) {
			count, err = store.RefreshGroups(ctx, input.SessionID)
			return err
		})
		return count > 0, err
	})
	if errors.Is(err, errTimedOut) {
		return nil, waitDeviceOutput{}, context.DeadlineExceeded
	}
	if err != nil {
		return nil, waitDeviceOutput{}, err
	}
	return nil, waitDeviceOutput{DeviceGroupCount: count}, nil
}

type messageInput struct {
	SessionID string `json:"session_id" jsonschema:"session identifier"`
	Message   string `json:"message" jsonschema:"notification body"`
}

type deliveredOutput struct {
	Delivered bool              `json:"delivered"`
	Responses []notify.Response `json:"responses,omitempty"`
}

type notifiedOutput struct {
	Delivered bool              `json:"delivered"`
	ItemID    string            `json:"item_id"`
	Responses []notify.Response `json:"responses,omitempty"`
}

func (s *Server) notify(ctx context.Context, _ *mcp.CallToolRequest, input messageInput) (*mcp.CallToolResult, notifiedOutput, error) {
	var itemID string
	err := s.use(ctx, input.SessionID, func(store *notify.Store) (err error) {
		itemID, err = store.SendNotify(ctx, input.SessionID, input.Message)
		return err
	})
	if err != nil {
		return nil, notifiedOutput{}, err
	}
	responses := s.drainResponses(ctx, input.SessionID)
	return attachmentContent(responses), notifiedOutput{Delivered: true, ItemID: itemID, Responses: responses}, nil
}

type statusInput struct {
	SessionID string `json:"session_id" jsonschema:"session identifier"`
	Status    string `json:"status" jsonschema:"current status"`
}

func (s *Server) status(ctx context.Context, _ *mcp.CallToolRequest, input statusInput) (*mcp.CallToolResult, deliveredOutput, error) {
	s.hooks.mu.Lock()
	err := s.use(ctx, input.SessionID, func(store *notify.Store) error {
		return store.SendStatus(ctx, input.SessionID, input.Status)
	})
	if err == nil {
		s.hooks.statusSession, s.hooks.status = input.SessionID, input.Status
		s.hooks.statusUpdated = time.Now()
		s.hooks.statusWarned = time.Time{}
	}
	s.hooks.mu.Unlock()
	if err != nil {
		return nil, deliveredOutput{}, err
	}
	responses := s.drainResponses(ctx, input.SessionID)
	return attachmentContent(responses), deliveredOutput{Delivered: true, Responses: responses}, nil
}

type colorInput struct {
	SessionID string `json:"session_id" jsonschema:"session identifier"`
	Color     string `json:"color" jsonschema:"panel color as #rrggbb or random"`
}

func (s *Server) color(ctx context.Context, _ *mcp.CallToolRequest, input colorInput) (*mcp.CallToolResult, deliveredOutput, error) {
	if err := s.use(ctx, input.SessionID, func(store *notify.Store) error {
		return store.SetColor(ctx, input.SessionID, input.Color)
	}); err != nil {
		return nil, deliveredOutput{}, err
	}
	responses := s.drainResponses(ctx, input.SessionID)
	return attachmentContent(responses), deliveredOutput{Delivered: true, Responses: responses}, nil
}

type requestInput struct {
	SessionID string   `json:"session_id" jsonschema:"session identifier"`
	Prompt    string   `json:"prompt" jsonschema:"question shown to the device group"`
	Options   []string `json:"options" jsonschema:"two or more choice labels"`
}

type requestOutput struct {
	RequestID string            `json:"request_id"`
	Choices   []notify.Choice   `json:"choices"`
	Responses []notify.Response `json:"responses,omitempty"`
}

func (s *Server) request(ctx context.Context, _ *mcp.CallToolRequest, input requestInput) (*mcp.CallToolResult, requestOutput, error) {
	var requestID string
	var choices []notify.Choice
	err := s.use(ctx, input.SessionID, func(store *notify.Store) (err error) {
		requestID, choices, err = store.SendRequest(ctx, input.SessionID, input.Prompt, input.Options)
		return err
	})
	if err != nil {
		return nil, requestOutput{}, err
	}
	responses := s.drainResponses(ctx, input.SessionID)
	return attachmentContent(responses), requestOutput{RequestID: requestID, Choices: choices, Responses: responses}, nil
}

type closeRequestInput struct {
	SessionID string `json:"session_id" jsonschema:"session identifier"`
	RequestID string `json:"request_id" jsonschema:"request identifier returned by request"`
}

func (s *Server) closeRequest(ctx context.Context, _ *mcp.CallToolRequest, input closeRequestInput) (*mcp.CallToolResult, closeOutput, error) {
	if err := s.use(ctx, input.SessionID, func(store *notify.Store) error {
		return store.CloseRequest(ctx, input.SessionID, input.RequestID)
	}); err != nil {
		return nil, closeOutput{}, err
	}
	responses := s.drainResponses(ctx, input.SessionID)
	return attachmentContent(responses), closeOutput{Closed: true, Responses: responses}, nil
}

type responsesOutput struct {
	Responses []notify.Response `json:"responses"`
}

func (s *Server) waitResponses(ctx context.Context, _ *mcp.CallToolRequest, input waitInput) (*mcp.CallToolResult, responsesOutput, error) {
	timeout, err := timeoutDuration(input.TimeoutSecond)
	if err != nil {
		return nil, responsesOutput{}, err
	}
	var responses []notify.Response
	err = poll(ctx, timeout, func() (bool, error) {
		err := s.use(ctx, input.SessionID, func(store *notify.Store) (err error) {
			responses, err = store.Responses(ctx, input.SessionID)
			return err
		})
		return len(responses) > 0, err
	})
	if errors.Is(err, errTimedOut) {
		responses, err = []notify.Response{}, nil
	}
	if err != nil {
		return nil, responsesOutput{}, err
	}
	return attachmentContent(responses), responsesOutput{Responses: responses}, nil
}

type closeOutput struct {
	Closed    bool              `json:"closed"`
	Responses []notify.Response `json:"responses,omitempty"`
}

func (s *Server) close(ctx context.Context, _ *mcp.CallToolRequest, input sessionInput) (*mcp.CallToolResult, closeOutput, error) {
	// Closing discards whatever the session still holds, so hand over unread
	// responses first rather than dropping a device message silently.
	responses := s.drainResponses(ctx, input.SessionID)
	if err := s.use(ctx, input.SessionID, func(store *notify.Store) error {
		return store.Close(ctx, input.SessionID)
	}); err != nil {
		return nil, closeOutput{}, err
	}
	return attachmentContent(responses), closeOutput{Closed: true, Responses: responses}, nil
}

func attachmentContent(responses []notify.Response) *mcp.CallToolResult {
	var content []mcp.Content
	for _, response := range responses {
		for i, attachment := range response.Attachments {
			size := attachment.ByteLength
			content = append(content, &mcp.ResourceLink{
				URI:         attachment.URI,
				Name:        attachment.ID + ".jpg",
				Title:       fmt.Sprintf("Photo %d attached to response %s", i+1, response.ID),
				Description: "End-to-end encrypted attachment decrypted by opeco into a local temporary file",
				MIMEType:    attachment.MediaType,
				Size:        &size,
			})
		}
	}
	if len(content) == 0 {
		return nil
	}
	return &mcp.CallToolResult{Content: content}
}

// drainResponses returns the responses received before this call so that a
// choice, dismissal, or device message cannot sit unseen until the agent happens
// to wait for one. Nothing pushes a response to the agent, and an agent that is
// working rather than waiting would otherwise never look.
//
// The send has already taken effect by the time this runs, so a failure here
// must not become a tool error: that would invite the caller to send again and
// deliver the event twice. responses_wait remains the path that reports such a
// failure. Responses are handed over once, so a caller that ignores this field
// will not see them again.
func (s *Server) drainResponses(ctx context.Context, sessionID string) []notify.Response {
	var responses []notify.Response
	err := s.use(ctx, sessionID, func(store *notify.Store) (err error) {
		responses, err = store.Responses(ctx, sessionID)
		return err
	})
	if err != nil {
		return nil
	}
	return responses
}

func (s *Server) pairingJoined(sessionID, pairingURL string) func(context.Context) (bool, error) {
	return func(ctx context.Context) (joined bool, err error) {
		err = s.use(ctx, sessionID, func(store *notify.Store) (err error) {
			joined, err = store.PairingJoined(ctx, sessionID, pairingURL)
			return err
		})
		return joined, err
	}
}

var errTimedOut = errors.New("timed out")

// poll checks once a second until check reports done or the timeout passes.
// It holds nothing between checks, so a session shared with other processes
// is theirs to use while this one waits.
func poll(ctx context.Context, timeout time.Duration, check func() (bool, error)) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		done, err := check()
		if err != nil || done {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errTimedOut
		case <-ticker.C:
		}
	}
}

func timeoutDuration(seconds int) (time.Duration, error) {
	if seconds < 1 || seconds > 600 {
		return 0, fmt.Errorf("timeout_seconds must be between 1 and 600")
	}
	return time.Duration(seconds) * time.Second, nil
}
