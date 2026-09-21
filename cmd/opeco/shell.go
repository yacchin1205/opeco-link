package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"opeco.link/internal/notify"
)

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func startShellSession(ctx context.Context, api *notify.API, title, color string, terminalQR bool, output, errorOutput io.Writer) (err error) {
	file, pairingURL, err := notify.CreateSessionFile(ctx, api, title, color)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if err := writeTerminalPairing(errorOutput, pairingURL, terminalQR); err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "export OPECO_SESSION_FILE=%s;\nexport OPECO_SESSION_ID=%s;\n", shellQuote(file.Path()), shellQuote(file.ID))
	return err
}

func writeTerminalPairing(output io.Writer, pairingURL string, terminalQR bool) error {
	if terminalQR {
		qr, err := notify.QRCode(pairingURL)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(output, qr); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(output, pairingURL)
	return err
}

func shellCommand(ctx context.Context, path string, args []string, terminalQR bool, output io.Writer) (err error) {
	if err := validateShellCommand(args); err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("no shell session; run eval \"$(opeco --title TITLE)\" first")
	}
	file, err := notify.OpenSessionFile(ctx, path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("shell session state is missing or closed; run eval \"$(opeco --title TITLE)\" to start another: %w", err)
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	var result bytes.Buffer
	if args[0] == "pair" {
		var url string
		url, err = file.Store.AddPairing(ctx, file.ID)
		if err == nil {
			err = writeTerminalPairing(&result, url, terminalQR)
		}
	} else {
		err = executeCommand(ctx, file.Store, file.ID, args, &result)
	}
	var apiError *notify.APIError
	expired := errors.As(err, &apiError) && (apiError.Code == "session_not_found" || apiError.Code == "session_expired")
	if (err == nil && args[0] == "close") || expired {
		err = errors.Join(err, file.Remove())
	} else {
		// Even a failed operation can have observed a newer authenticated group
		// head. Persist it so later processes cannot accept an older history.
		if saveErr := file.Save(); saveErr != nil {
			return errors.Join(err, fmt.Errorf("save session state: %w", saveErr))
		}
	}
	if err != nil {
		return err
	}
	_, err = io.Copy(output, &result)
	return err
}

func validateShellCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("a command is required")
	}
	switch args[0] {
	case "join", "pair", "responses", "close":
		if len(args) != 1 {
			return fmt.Errorf("%s takes no arguments", args[0])
		}
	case "notify", "status":
		if len(args) < 2 {
			return fmt.Errorf("usage: opeco %s TEXT", args[0])
		}
	case "color", "close-request":
		if len(args) != 2 {
			return fmt.Errorf("usage: opeco %s VALUE", args[0])
		}
	case "request":
		if len(args) < 4 {
			return fmt.Errorf("usage: opeco request PROMPT OPTION OPTION...")
		}
	default:
		return fmt.Errorf("unknown command %q; run opeco --help", args[0])
	}
	return nil
}

func executeCommand(ctx context.Context, store *notify.Store, id string, args []string, output io.Writer) (err error) {
	switch args[0] {
	case "join":
		count, err := store.RefreshGroups(ctx, id)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "%d device group(s) joined\n", count)
		return err
	case "notify":
		item, err := store.SendNotify(ctx, id, strings.Join(args[1:], " "))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "notification item=%s sent\n", item)
		return err
	case "status":
		if err := store.SendStatus(ctx, id, strings.Join(args[1:], " ")); err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, "sent")
	case "color":
		if err := store.SetColor(ctx, id, args[1]); err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, "color changed")
	case "request":
		request, choices, err := store.SendRequest(ctx, id, args[1], args[2:])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "request %s sent: %v\n", request, choices)
		return err
	case "close-request":
		if err := store.CloseRequest(ctx, id, args[1]); err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, "request closed")
	case "responses":
		responses, err := store.Responses(ctx, id)
		if err != nil {
			return err
		}
		for _, response := range responses {
			if err := writeResponse(output, response); err != nil {
				return err
			}
		}
	case "close":
		if err := store.Close(ctx, id); err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, "closed")
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	return err
}
