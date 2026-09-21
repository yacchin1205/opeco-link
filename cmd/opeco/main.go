package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"opeco.link/internal/mcpserver"
	"opeco.link/internal/notify"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	flags := flag.NewFlagSet("opeco", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	baseURL := flags.String("base-url", "https://opeco.link", "opeco service URL")
	title := flags.String("title", "Development session", "new session title")
	color := flags.String("color", "random", "session panel color: random or #rrggbb")
	interactiveMode := flags.Bool("interactive", false, "run the long-lived interactive CLI instead of exporting a shell session")
	noTerminalQR := flags.Bool("no-terminal-qr", false, "print the pairing URL without drawing a terminal QR code")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: eval \"$(opeco [--title TITLE] [--color COLOR])\"\n       opeco COMMAND [ARGS...]\n       opeco --interactive [--title TITLE]\n       opeco mcp\n\nCommands: join, pair, notify TEXT, status TEXT, color COLOR, request PROMPT OPTION OPTION..., close-request ID, responses, close")
		flags.PrintDefaults()
	}
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *interactiveMode && flags.NArg() != 0 {
		return fmt.Errorf("--interactive does not take a command")
	}
	if flags.NArg() > 0 && flags.Arg(0) != "mcp" {
		var invalidFlag string
		flags.Visit(func(f *flag.Flag) {
			if f.Name != "no-terminal-qr" {
				invalidFlag = f.Name
			}
		})
		if invalidFlag != "" {
			return fmt.Errorf("--%s is only used when starting a session; commands use OPECO_SESSION_FILE", invalidFlag)
		}
		return shellCommand(ctx, os.Getenv("OPECO_SESSION_FILE"), flags.Args(), !*noTerminalQR && isCharacterDevice(os.Stdout), os.Stdout)
	}
	if flags.NArg() > 1 {
		return fmt.Errorf("mcp does not take arguments")
	}
	api, err := notify.NewAPI(*baseURL)
	if err != nil {
		return err
	}
	if !*interactiveMode && flags.NArg() == 0 {
		return startShellSession(ctx, api, *title, *color, !*noTerminalQR && isCharacterDevice(os.Stderr), os.Stdout, os.Stderr)
	}
	store := notify.NewStore(api)
	viewer, err := notify.NewQRViewer()
	if err != nil {
		return err
	}
	defer func() {
		runErr = errors.Join(runErr, viewer.Close())
	}()

	operation := func(ctx context.Context) error {
		if flags.NArg() == 1 {
			return mcpserver.New(store, viewer).Run(ctx)
		}
		terminalQR := !*noTerminalQR && isCharacterDevice(os.Stdout)
		return interactive(ctx, store, viewer, *title, *color, terminalQR, os.Stdin, os.Stdout, os.Stderr)
	}
	err = supervise(ctx, viewer, operation)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// isCharacterDevice reports whether output goes to a terminal. A block-character
// QR code depends on the cell geometry of a terminal, so it is only drawn when
// opeco writes to one; redirected output gets the URLs alone.
func isCharacterDevice(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func supervise(ctx context.Context, viewer *notify.QRViewer, operation func(context.Context) error) error {
	operationCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- operation(operationCtx)
	}()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-viewer.Done():
		if err := viewer.Wait(); err != nil {
			return err
		}
		return fmt.Errorf("QR code viewer stopped unexpectedly")
	}
}

func interactive(ctx context.Context, store *notify.Store, viewer *notify.QRViewer, title, color string, terminalQR bool, input io.Reader, output, errorOutput io.Writer) error {
	sessionID, pairingURL, err := store.Create(ctx, title, color)
	if err != nil {
		return err
	}
	imageURL, err := viewer.Publish(pairingURL)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "Session: %s\n", sessionID)
	if err := writePairing(output, pairingURL, imageURL, terminalQR); err != nil {
		return err
	}
	fmt.Fprintln(output, "Commands: join, pair, notify TEXT, status TEXT, color #rrggbb|random, request PROMPT | OPTION | OPTION, close-request REQUEST_ID, responses, close, quit")

	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 300_000)
	lines := make(chan string)
	scanDone := make(chan error, 1)
	go func() {
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		scanDone <- scanner.Err()
	}()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	knownGroups := 0
	fmt.Fprint(output, "opeco> ")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-scanDone:
			return err
		case <-ticker.C:
			count, err := store.RefreshGroups(ctx, sessionID)
			if err != nil {
				if notify.IsTransientAPIError(ctx, err) {
					fmt.Fprintf(errorOutput, "\ntemporarily unable to check joined device groups: %v; will retry\n", err)
					fmt.Fprint(output, "opeco> ")
					continue
				}
				return fmt.Errorf("detect joined device groups: %w", err)
			}
			if count > knownGroups {
				fmt.Fprintf(output, "\n%d new device group(s) joined; %d total\nopeco> ", count-knownGroups, count)
				knownGroups = count
			}
			continue
		case scanned := <-lines:
			line := strings.TrimSpace(scanned)
			if line == "" {
				fmt.Fprint(output, "opeco> ")
				continue
			}
			command, argument, _ := strings.Cut(line, " ")
			exit, err := runCommand(ctx, store, viewer, sessionID, command, argument, &knownGroups, terminalQR, output)
			if err != nil {
				return err
			}
			if exit {
				return nil
			}
			fmt.Fprint(output, "opeco> ")
		}
	}
}

func writePairing(output io.Writer, pairingURL, imageURL string, terminalQR bool) error {
	if err := writeTerminalPairing(output, pairingURL, terminalQR); err != nil {
		return err
	}
	_, err := fmt.Fprintf(output, "QR image: %s\n", imageURL)
	return err
}

func runCommand(ctx context.Context, store *notify.Store, viewer *notify.QRViewer, sessionID, command, argument string, knownGroups *int, terminalQR bool, output io.Writer) (bool, error) {
	if command == "quit" {
		return true, nil
	}
	args := []string{command}
	if command == "request" {
		for _, part := range strings.Split(argument, "|") {
			args = append(args, strings.TrimSpace(part))
		}
	} else if argument != "" {
		args = append(args, argument)
	}
	if err := validateShellCommand(args); err != nil {
		return false, err
	}
	switch command {
	case "join":
		count, err := store.RefreshGroups(ctx, sessionID)
		if err != nil {
			return false, err
		}
		*knownGroups = count
		_, err = fmt.Fprintf(output, "%d device group(s) joined\n", count)
		return false, err
	case "pair":
		url, err := store.AddPairing(ctx, sessionID)
		if err != nil {
			return false, err
		}
		imageURL, err := viewer.Publish(url)
		if err != nil {
			return false, err
		}
		return false, writePairing(output, url, imageURL, terminalQR)
	default:
		return command == "close", executeCommand(ctx, store, sessionID, args, output)
	}
}

func writeResponse(output io.Writer, response notify.Response) (err error) {
	timestamp := response.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	switch response.Type {
	case "feedback":
		attachment := ""
		for i, image := range response.Attachments {
			attachment += fmt.Sprintf(" attachment[%d]=%s", i+1, image.Path)
		}
		_, err = fmt.Fprintf(output, "feedback message=%q%s group=%s at=%s\n", response.Message, attachment, response.GroupID, timestamp)
	case "dismiss":
		if response.ItemID != "" {
			_, err = fmt.Fprintf(output, "dismiss item=%s group=%s at=%s\n", response.ItemID, response.GroupID, timestamp)
		} else {
			_, err = fmt.Fprintf(output, "dismiss request=%s group=%s at=%s\n", response.RequestID, response.GroupID, timestamp)
		}
	case "response":
		_, err = fmt.Fprintf(output, "response request=%s option=%s group=%s at=%s\n", response.RequestID, response.OptionID, response.GroupID, timestamp)
	}
	return err
}
