package notify

import (
	"context"
	"crypto/ecdh"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// SessionFile holds an exclusive lock for one shell command's complete
// read/refresh/operate/write cycle, including authenticated group history.
type SessionFile struct {
	Store     *Store
	ID        string
	directory string
	lock      *os.File
}

type savedSession struct {
	Version         int                 `json:"version"`
	BaseURL         string              `json:"baseURL"`
	ID              string              `json:"id"`
	Title           string              `json:"title"`
	Color           string              `json:"color"`
	Token           string              `json:"token"`
	PrivateKey      []byte              `json:"privateKey"`
	Pairings        map[string]Pairing  `json:"pairings"`
	Groups          map[string]*Group   `json:"groups"`
	OpenRequests    map[string]struct{} `json:"openRequests"`
	ResponseCursor  int64               `json:"responseCursor"`
	ProtocolVersion int                 `json:"protocolVersion"`
}

func CreateSessionFile(ctx context.Context, api *API, title, color string) (_ *SessionFile, _ string, err error) {
	directory, err := os.MkdirTemp("", "opeco-session-")
	if err != nil {
		return nil, "", err
	}
	file := &SessionFile{Store: NewStore(api), directory: directory}
	defer func() {
		if err != nil {
			err = errors.Join(err, file.Close(), os.RemoveAll(directory))
		}
	}()
	if err := file.acquire(ctx); err != nil {
		return nil, "", err
	}
	id, pairingURL, err := file.Store.Create(ctx, title, color)
	if err != nil {
		return nil, "", err
	}
	file.ID = id
	session := file.Store.sessions[id]
	session.tempDir = filepath.Join(directory, "attachments")
	if err := os.Mkdir(session.tempDir, 0o700); err != nil {
		return nil, "", err
	}
	if err := file.Save(); err != nil {
		return nil, "", err
	}
	return file, pairingURL, nil
}

func OpenSessionFile(ctx context.Context, path string) (_ *SessionFile, err error) {
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	directory := filepath.Dir(path)
	if filepath.Base(path) != "state.json" || !strings.HasPrefix(filepath.Base(directory), "opeco-session-") {
		return nil, fmt.Errorf("invalid OPECO_SESSION_FILE; start a session with eval \"$(opeco --title TITLE)\"")
	}
	if err := privatePath(directory, true); err != nil {
		return nil, err
	}
	file := &SessionFile{directory: directory}
	if err := file.acquire(ctx); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, file.Close())
		}
	}()
	if err := privatePath(path, false); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var saved savedSession
	if err := decodeJSON(data, &saved); err != nil {
		return nil, fmt.Errorf("read session state: %w", err)
	}
	if saved.Version != 1 || saved.ProtocolVersion != 4 || !attachmentIDPattern.MatchString(saved.ID) || saved.ResponseCursor < 0 {
		return nil, fmt.Errorf("unsupported or invalid session state")
	}
	if saved.Pairings == nil || saved.Groups == nil || saved.OpenRequests == nil || saved.Token == "" {
		return nil, fmt.Errorf("incomplete session state")
	}
	for _, group := range saved.Groups {
		if group == nil || group.Keys == nil || group.PublicKeys == nil || group.HeadTransitionHash == "" {
			return nil, fmt.Errorf("incomplete authenticated group state")
		}
	}
	key, err := ecdh.P256().NewPrivateKey(saved.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid saved session key: %w", err)
	}
	api, err := NewAPI(saved.BaseURL)
	if err != nil {
		return nil, err
	}
	file.ID, file.Store = saved.ID, NewStore(api)
	file.Store.sessions[saved.ID] = &managedSession{
		id: saved.ID, title: saved.Title, color: saved.Color,
		sessionToken: saved.Token, privateKey: key, publicKey: encode(key.PublicKey().Bytes()),
		pairings: saved.Pairings, groups: saved.Groups, openRequests: saved.OpenRequests,
		responseCursor: saved.ResponseCursor, protocolVersion: saved.ProtocolVersion,
		tempDir: filepath.Join(directory, "attachments"),
	}
	return file, nil
}

func (f *SessionFile) Path() string { return filepath.Join(f.directory, "state.json") }

func (f *SessionFile) Save() (err error) {
	session, err := f.Store.session(f.ID)
	if err != nil {
		return err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	saved := savedSession{
		Version: 1, BaseURL: f.Store.api.baseURL.String(), ID: session.id,
		Title: session.title, Color: session.color, Token: session.sessionToken,
		PrivateKey: session.privateKey.Bytes(), Pairings: session.pairings,
		Groups: session.groups, OpenRequests: session.openRequests,
		ResponseCursor: session.responseCursor, ProtocolVersion: session.protocolVersion,
	}
	output, err := os.CreateTemp(f.directory, ".state-")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(output.Name()))
		}
	}()
	if err := json.NewEncoder(output).Encode(saved); err != nil {
		return errors.Join(err, output.Close())
	}
	if err := output.Sync(); err != nil {
		return errors.Join(err, output.Close())
	}
	if err := output.Close(); err != nil {
		return err
	}
	return os.Rename(output.Name(), f.Path())
}

// Remove deletes only the private, per-session temporary directory created by
// CreateSessionFile. Call after a successful remote close, or session expiry.
func (f *SessionFile) Remove() error {
	// Invalidate state while locked, before any waiting command can load it.
	if err := os.Remove(f.Path()); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.RemoveAll(f.directory)
}

func (f *SessionFile) Close() error {
	if f.lock == nil {
		return nil
	}
	err := f.lock.Close()
	f.lock = nil
	return err
}

func (f *SessionFile) acquire(ctx context.Context) error {
	path := filepath.Join(f.directory, "lock")
	if err := privatePath(path, false); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		locked, err := trySessionLock(lock)
		if err != nil {
			return errors.Join(err, lock.Close())
		}
		if locked {
			f.lock = lock
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), lock.Close())
		case <-ticker.C:
		}
	}
}

func privatePath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) {
		return fmt.Errorf("session path must be a regular private file or directory: %s", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("session state must not be accessible to other users: %s", path)
	}
	return nil
}
