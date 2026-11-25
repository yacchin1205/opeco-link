package notify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SharedSessions keeps the one session of a machine in a directory the owner
// named, so every opeco process there, each agent's MCP server and each shell
// command, uses the same session, and the session outlives them all. Each
// operation loads the session under its file's lock and saves it again, as a
// shell command does. Its keys are on disk for as long as the session lives.
type SharedSessions struct {
	dir string
	api *API
}

func NewSharedSessions(dir string, api *API) (*SharedSessions, error) {
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("session_dir must be an absolute path: %s", dir)
	}
	if err := privatePath(dir, true); err != nil {
		return nil, fmt.Errorf("session_dir: %w", err)
	}
	return &SharedSessions{dir: dir, api: api}, nil
}

// StatePath is the state file of the session the directory holds, or
// os.ErrNotExist when it holds none.
func (s *SharedSessions) StatePath() (string, error) { return SharedStatePath(s.dir) }

// SharedStatePath is the state file of the session dir holds, for a shell
// command run with no session of its own.
func SharedStatePath(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "opeco-session-*", "state.json"))
	if err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session in %s: %w", dir, os.ErrNotExist)
	case 1:
		return matches[0], nil
	}
	return "", fmt.Errorf("%s holds %d sessions; it keeps one", dir, len(matches))
}

// EnsureSession returns the session the directory holds and creates one only
// when there is none, or when the relay no longer knows the one there. The
// directory's lock keeps two processes from creating two.
func (s *SharedSessions) EnsureSession(ctx context.Context, title, color string) (_ SessionState, err error) {
	lock := &SessionFile{directory: s.dir}
	if err := lock.acquire(ctx); err != nil {
		return SessionState{}, err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	path, err := s.StatePath()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return SessionState{}, err
	}
	if err == nil {
		file, err := OpenSessionFile(ctx, path)
		if err != nil {
			return SessionState{}, err
		}
		state, err := file.Store.reuse(ctx, file.ID)
		if err == nil {
			return state, errors.Join(file.Save(), file.Close())
		}
		if !isMissingSession(err) {
			return SessionState{}, errors.Join(err, file.Save(), file.Close())
		}
		if err := file.Remove(); err != nil {
			return SessionState{}, err
		}
	}
	file, pairingURL, err := createSessionFileIn(ctx, s.api, s.dir, title, color)
	if err != nil {
		return SessionState{}, err
	}
	return SessionState{SessionID: file.ID, Title: title, PairingURL: pairingURL}, file.Close()
}

// Use runs fn on the session under its lock and saves what it changed. A
// session fn closed, or one the relay reports gone, is removed, as a shell
// command removes it.
func (s *SharedSessions) Use(ctx context.Context, sessionID string, fn func(*Store) error) (err error) {
	path, err := s.StatePath()
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("unknown local session %q", sessionID)
	}
	if err != nil {
		return err
	}
	file, err := OpenSessionFile(ctx, path)
	if errors.Is(err, os.ErrNotExist) {
		// Removed between the lookup and the lock.
		return fmt.Errorf("unknown local session %q", sessionID)
	}
	if err != nil {
		return err
	}
	if file.ID != sessionID {
		return errors.Join(fmt.Errorf("unknown local session %q", sessionID), file.Close())
	}
	err = fn(file.Store)
	_, open := file.Store.sessions[sessionID]
	var apiError *APIError
	expired := errors.As(err, &apiError) && (apiError.Code == "session_not_found" || apiError.Code == "session_expired")
	if !open || expired {
		return errors.Join(err, file.Remove())
	}
	// Even a failed operation can have observed a newer authenticated group
	// head. Persist it so later processes cannot accept an older history.
	if saveErr := file.Save(); saveErr != nil {
		err = errors.Join(err, fmt.Errorf("save session state: %w", saveErr))
	}
	return errors.Join(err, file.Close())
}

// PendingResponses is Store.PendingResponses for the session the directory
// holds; there is nothing pending while it holds none.
func (s *SharedSessions) PendingResponses(ctx context.Context) (sessionID string, count int, through int64, err error) {
	path, err := s.StatePath()
	if errors.Is(err, os.ErrNotExist) {
		return "", 0, 0, nil
	}
	if err != nil {
		return "", 0, 0, err
	}
	file, err := OpenSessionFile(ctx, path)
	if errors.Is(err, os.ErrNotExist) {
		return "", 0, 0, nil
	}
	if err != nil {
		return "", 0, 0, err
	}
	sessionID, count, through, err = file.Store.PendingResponses(ctx)
	return sessionID, count, through, errors.Join(err, file.Close())
}
