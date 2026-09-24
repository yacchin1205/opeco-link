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

// Open shows url in the default browser of the desktop that runs this process.
// It refuses inside an SSH session, where the browser would appear on the remote
// display rather than in front of the person, and reports any failure to the
// caller instead of guessing that a window appeared.
func Open(ctx context.Context, url string) error {
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return errors.New("running inside an SSH session, so a browser would open on the remote display")
	}
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	command := openCommand(ctx, url)
	output, err := command.CombinedOutput()
	if err != nil {
		if detail := strings.TrimSpace(string(output)); detail != "" {
			return fmt.Errorf("%s: %w: %s", command.Args[0], err, detail)
		}
		return fmt.Errorf("%s: %w", command.Args[0], err)
	}
	return nil
}

func openCommand(ctx context.Context, url string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.CommandContext(ctx, "open", url)
	case "windows":
		return exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return exec.CommandContext(ctx, "xdg-open", url)
	}
}
