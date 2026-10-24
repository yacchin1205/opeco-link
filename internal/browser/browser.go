package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const openTimeout = 5 * time.Second

// Opener shows a URL in the person's browser and returns why it could not.
type Opener func(ctx context.Context, url string) error

// Open shows url in the person's browser and reports any failure to the caller
// instead of guessing that a window appeared. The BROWSER environment variable
// names the command to run with url as its only argument; without it, the
// desktop's default opener is used, except inside an SSH session, where that
// browser would appear on the remote display rather than in front of the person.
func Open(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	command, err := openCommand(ctx, url)
	if err != nil {
		return err
	}
	output, err := command.CombinedOutput()
	if err != nil {
		if detail := strings.TrimSpace(string(output)); detail != "" {
			return fmt.Errorf("%s: %w: %s", command.Args[0], err, detail)
		}
		return fmt.Errorf("%s: %w", command.Args[0], err)
	}
	return nil
}

func openCommand(ctx context.Context, url string) (*exec.Cmd, error) {
	if browser := os.Getenv("BROWSER"); browser != "" {
		return exec.CommandContext(ctx, browser, url), nil
	}
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return nil, errors.New("running inside an SSH session, so a browser would open on the remote display; set BROWSER to a command that opens the URL for the person")
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.CommandContext(ctx, "open", url), nil
	case "windows":
		return exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url), nil
	default:
		return exec.CommandContext(ctx, "xdg-open", url), nil
	}
}
