package notify

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestQRViewerServesAnInvitationPageAtAnOpaqueLoopbackURL(t *testing.T) {
	viewer, err := NewQRViewer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := viewer.Close(); err != nil {
			t.Error(err)
		}
	})

	pairingURL := "https://opeco.link/join#v=1&s=session&p=pairing&t=secret"
	pageURL, err := viewer.Publish(pairingURL, func(context.Context) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
		t.Fatalf("viewer URL = %q, want an HTTP IPv4 loopback URL", pageURL)
	}
	if strings.Contains(pageURL, "session") || strings.Contains(pageURL, "secret") {
		t.Fatalf("viewer URL exposes pairing data: %q", pageURL)
	}

	response, page := get(t, pageURL)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if response.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want HTML", response.Header.Get("Content-Type"))
	}
	for name, want := range map[string]string{
		"Cache-Control":                "no-store",
		"Content-Security-Policy":      qrPageSecurityPolicy,
		"Cross-Origin-Resource-Policy": "same-origin",
		"Referrer-Policy":              "no-referrer",
		"X-Content-Type-Options":       "nosniff",
	} {
		if got := response.Header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if !strings.Contains(page, `value="https://opeco.link/join#v=1&amp;s=session&amp;p=pairing&amp;t=secret"`) {
		t.Fatal("page does not offer the pairing URL for copying")
	}
	if !strings.Contains(page, `src="data:image/png;base64,iVBORw0KGgo`) {
		t.Fatal("page does not embed the QR code as a PNG")
	}
	if !strings.Contains(page, `href="https://apps.apple.com/app/id6814018543" target="_blank"`) {
		t.Fatal("page does not link to the App Store listing in a new tab")
	}
	for _, content := range []string{qrPageStyle, qrPageScript} {
		if !strings.Contains(page, content) {
			t.Fatal("page content differs from the hashed style or script allowed by its Content-Security-Policy")
		}
	}
}

func TestQRViewerStopsOfferingAPairingOnceItIsUsed(t *testing.T) {
	viewer, err := NewQRViewer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := viewer.Close(); err != nil {
			t.Error(err)
		}
	})

	joined := false
	var checkErr error
	pageURL, err := viewer.Publish("https://opeco.link/join#secret", func(context.Context) (bool, error) {
		return joined, checkErr
	})
	if err != nil {
		t.Fatal(err)
	}
	statusURL := pageURL + qrStatusSuffix

	response, body := get(t, statusURL)
	if response.StatusCode != http.StatusOK || body != "{\"joined\":false}\n" {
		t.Fatalf("unused pairing status = %d %q", response.StatusCode, body)
	}

	checkErr = errors.New("relay unavailable")
	response, body = get(t, statusURL)
	if response.StatusCode != http.StatusBadGateway || body != "relay unavailable\n" {
		t.Fatalf("failed check status = %d %q, want the failure passed through", response.StatusCode, body)
	}
	response, _ = get(t, pageURL)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("page status after a failed check = %d, want it still offered", response.StatusCode)
	}

	checkErr = nil
	joined = true
	response, body = get(t, statusURL)
	if response.StatusCode != http.StatusOK || body != "{\"joined\":true}\n" {
		t.Fatalf("used pairing status = %d %q", response.StatusCode, body)
	}
	for _, used := range []string{pageURL, statusURL} {
		response, _ = get(t, used)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("%s after the pairing was used = %d, want %d", used, response.StatusCode, http.StatusNotFound)
		}
	}
}

func get(t *testing.T, target string) (*http.Response, string) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, string(body)
}

func TestQRViewerRejectsOtherHostsAndExpiredPaths(t *testing.T) {
	viewer, err := NewQRViewer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := viewer.Close(); err != nil {
			t.Error(err)
		}
	})

	imageURL, err := viewer.Publish("https://opeco.link/join#secret", func(context.Context) (bool, error) { return false, nil })
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodGet, imageURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "attacker.example"
	client := &http.Client{Timeout: time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign Host status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}

	parsed, err := url.Parse(imageURL)
	if err != nil {
		t.Fatal(err)
	}
	viewer.mu.Lock()
	viewer.entries[parsed.Path].expiresAt = time.Now().Add(-time.Second)
	viewer.mu.Unlock()
	response, err = client.Get(imageURL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expired path status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
}

func TestQRViewerReportsAnUnexpectedListenerFailure(t *testing.T) {
	viewer, err := NewQRViewer()
	if err != nil {
		t.Fatal(err)
	}
	if err := viewer.listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := viewer.Wait(); err == nil {
		t.Fatal("unexpected listener failure was not reported")
	}
	if err := viewer.Close(); err == nil {
		t.Fatal("Close did not retain the listener failure")
	}
}
