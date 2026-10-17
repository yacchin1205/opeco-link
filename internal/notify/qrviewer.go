package notify

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const qrViewerLifetime = 10 * time.Minute

const qrStatusSuffix = "/status"

const appStoreURL = "https://apps.apple.com/app/id6814018543"

//go:embed assets/opeco.svg
var opecoLogo []byte

// The official badge from Apple's marketing tools, embedded because the page
// loads nothing from other origins.
//
//go:embed assets/app-store-badge.svg
var appStoreBadge []byte

const qrPageStyle = `
:root { color-scheme: light dark; --bg: #f6f6f4; --fg: #1d1d1f; --muted: #6e6e73; --card: #ffffff; --line: #d8d8dc; --accent: #0a66c2; --on-accent: #ffffff; --error: #b3261e; }
@media (prefers-color-scheme: dark) { :root { --bg: #161618; --fg: #f2f2f2; --muted: #a1a1a6; --card: #232326; --line: #3a3a3e; --accent: #5ea8ff; --on-accent: #0b1b2e; --error: #ff8a80; } }
* { box-sizing: border-box; }
body { margin: 0; min-height: 100vh; display: grid; place-items: center; padding: 24px 16px; background: var(--bg); color: var(--fg); font: 16px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
main { width: min(100%, 440px); display: grid; gap: 20px; justify-items: center; text-align: center; }
.logo { width: 72px; height: 72px; }
h1 { margin: 0; font-size: 22px; font-weight: 600; }
p { margin: 0; color: var(--muted); }
.qr { width: min(100%, 360px); aspect-ratio: 1; margin: 0 auto; padding: 12px; background: #ffffff; border-radius: 12px; image-rendering: pixelated; }
#invitation { width: 100%; display: grid; gap: 16px; }
.link { display: flex; gap: 8px; }
.app { display: grid; gap: 8px; justify-items: center; font-size: 14px; }
.app img { display: block; height: 40px; }
input { flex: 1; min-width: 0; padding: 8px 10px; border: 1px solid var(--line); border-radius: 8px; background: var(--card); color: var(--fg); font: 13px ui-monospace, SFMono-Regular, Menlo, monospace; }
button { padding: 8px 14px; border: 0; border-radius: 8px; background: var(--accent); color: var(--on-accent); font: inherit; font-weight: 600; cursor: pointer; }
#message { min-height: 1.5em; }
#message.error { color: var(--error); }
`

const qrPageScript = `
const link = document.getElementById("link");
const message = document.getElementById("message");
function show(text, error) {
  message.textContent = text;
  message.className = error ? "error" : "";
}
document.getElementById("copy").addEventListener("click", async () => {
  try {
    await navigator.clipboard.writeText(link.value);
    show("Link copied.", false);
  } catch (error) {
    show("Could not copy the link: " + error.message, true);
  }
});
function finish(heading, text) {
  document.getElementById("invitation").remove();
  document.querySelector("h1").textContent = heading;
  show(text, false);
}
async function poll() {
  let response;
  try {
    response = await fetch(location.pathname + "` + qrStatusSuffix + `", { cache: "no-store" });
  } catch (error) {
    show("Cannot reach opeco: " + error.message, true);
    setTimeout(poll, 2000);
    return;
  }
  if (response.status === 404) {
    finish("QR code expired", "Ask for a new QR code to join.");
    return;
  }
  if (!response.ok) {
    show("Cannot check whether a device joined: " + (await response.text()), true);
    setTimeout(poll, 2000);
    return;
  }
  if ((await response.json()).joined) {
    finish("Joined", "A device group joined the session. You can close this tab.");
    window.close();
    return;
  }
  if (message.className === "error") show("", false);
  setTimeout(poll, 2000);
}
poll();
`

var qrPage = template.Must(template.New("qr").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>opeco.link invitation</title>
<style>` + qrPageStyle + `</style>
</head>
<body>
<main>
<img class="logo" src="{{.Logo}}" alt="opeco.link">
<h1>Join this session</h1>
<div id="invitation">
<img class="qr" src="{{.Image}}" alt="QR code for joining the session">
<p>Scan with the opeco.link app or a phone camera, or copy the link.</p>
<div class="link">
<input id="link" type="text" readonly aria-label="Pairing link" value="{{.PairingURL}}">
<button id="copy" type="button">Copy link</button>
</div>
<div class="app">
<p>Get the app for iPhone, iPad, and Mac.</p>
<a href="{{.AppStoreURL}}" target="_blank" rel="noopener noreferrer"><img src="{{.AppStoreBadge}}" alt="Download on the App Store"></a>
</div>
</div>
<p id="message" role="status" aria-live="polite"></p>
</main>
<script>` + qrPageScript + `</script>
</body>
</html>
`))

var qrPageSecurityPolicy = "default-src 'none'; img-src data:; connect-src 'self'; " +
	"style-src '" + contentHash(qrPageStyle) + "'; script-src '" + contentHash(qrPageScript) + "'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

func dataURL(mediaType string, content []byte) template.URL {
	return template.URL("data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(content))
}

func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256-" + base64.StdEncoding.EncodeToString(sum[:])
}

type qrViewerEntry struct {
	page      []byte
	joined    func(context.Context) (bool, error)
	expiresAt time.Time
	timer     *time.Timer
}

type QRViewer struct {
	listener net.Listener
	server   *http.Server
	host     string

	mu       sync.Mutex
	entries  map[string]*qrViewerEntry
	closed   bool
	serveErr error
	done     chan struct{}
}

func NewQRViewer() (*QRViewer, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for QR code viewer: %w", err)
	}
	viewer := &QRViewer{
		listener: listener,
		host:     listener.Addr().String(),
		entries:  make(map[string]*qrViewerEntry),
		done:     make(chan struct{}),
	}
	viewer.server = &http.Server{
		Handler:           viewer,
		ReadHeaderTimeout: 5 * time.Second,
		// A status request waits for the relay, whose client allows 20 seconds.
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    30 * time.Second,
		MaxHeaderBytes: 8 << 10,
	}
	go viewer.serve()
	return viewer, nil
}

// Publish serves an invitation page for pairingURL. The page polls joined and
// stops offering the pairing once it reports that a device group used it.
func (v *QRViewer) Publish(pairingURL string, joined func(context.Context) (bool, error)) (string, error) {
	image, err := QRCodePNG(pairingURL)
	if err != nil {
		return "", err
	}
	var page bytes.Buffer
	if err := qrPage.Execute(&page, struct {
		Logo          template.URL
		Image         template.URL
		PairingURL    string
		AppStoreURL   string
		AppStoreBadge template.URL
	}{
		Logo:          dataURL("image/svg+xml", opecoLogo),
		Image:         dataURL("image/png", image),
		PairingURL:    pairingURL,
		AppStoreURL:   appStoreURL,
		AppStoreBadge: dataURL("image/svg+xml", appStoreBadge),
	}); err != nil {
		return "", fmt.Errorf("render invitation page: %w", err)
	}
	token, err := randomValue(24)
	if err != nil {
		return "", fmt.Errorf("create QR code viewer path: %w", err)
	}
	path := "/qr/" + token

	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return "", fmt.Errorf("QR code viewer is closed")
	}
	if v.serveErr != nil {
		return "", fmt.Errorf("serve QR code viewer: %w", v.serveErr)
	}
	if _, exists := v.entries[path]; exists {
		return "", fmt.Errorf("QR code viewer path collision")
	}
	entry := &qrViewerEntry{page: page.Bytes(), joined: joined, expiresAt: time.Now().Add(qrViewerLifetime)}
	entry.timer = time.AfterFunc(qrViewerLifetime, func() {
		v.mu.Lock()
		delete(v.entries, path)
		v.mu.Unlock()
	})
	v.entries[path] = entry
	return "http://" + v.host + path, nil
}

func (v *QRViewer) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Content-Security-Policy", qrPageSecurityPolicy)
	response.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	response.Header().Set("Referrer-Policy", "no-referrer")
	response.Header().Set("X-Content-Type-Options", "nosniff")

	if request.Host != v.host || !isLoopbackAddress(request.RemoteAddr) {
		http.Error(response, "forbidden", http.StatusForbidden)
		return
	}
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", http.MethodGet)
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if request.URL.RawQuery != "" {
		http.NotFound(response, request)
		return
	}

	path, status := strings.CutSuffix(request.URL.Path, qrStatusSuffix)
	entry := v.entry(path)
	if entry == nil {
		http.NotFound(response, request)
		return
	}
	if !status {
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		if _, err := response.Write(entry.page); err != nil {
			return
		}
		return
	}

	joined, err := entry.joined(request.Context())
	if err != nil {
		http.Error(response, err.Error(), http.StatusBadGateway)
		return
	}
	if joined {
		v.remove(path, entry)
	}
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(struct {
		Joined bool `json:"joined"`
	}{joined}); err != nil {
		return
	}
}

func (v *QRViewer) entry(path string) *qrViewerEntry {
	v.mu.Lock()
	defer v.mu.Unlock()
	entry, exists := v.entries[path]
	if !exists {
		return nil
	}
	if !time.Now().Before(entry.expiresAt) {
		entry.timer.Stop()
		delete(v.entries, path)
		return nil
	}
	return entry
}

// remove stops offering a pairing that has been used; its secret no longer
// admits anyone, so the page has nothing left to show.
func (v *QRViewer) remove(path string, entry *qrViewerEntry) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.entries[path] == entry {
		entry.timer.Stop()
		delete(v.entries, path)
	}
}

func (v *QRViewer) Close() error {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		<-v.done
		return v.serveError()
	}
	v.closed = true
	for path, entry := range v.entries {
		entry.timer.Stop()
		delete(v.entries, path)
	}
	v.mu.Unlock()

	listenerErr := v.listener.Close()
	if errors.Is(listenerErr, net.ErrClosed) {
		listenerErr = nil
	}
	closeErr := v.server.Close()
	if errors.Is(closeErr, http.ErrServerClosed) || errors.Is(closeErr, net.ErrClosed) {
		closeErr = nil
	}
	<-v.done
	return errors.Join(listenerErr, closeErr, v.serveError())
}

func (v *QRViewer) Done() <-chan struct{} {
	return v.done
}

func (v *QRViewer) Wait() error {
	<-v.done
	return v.serveError()
}

func (v *QRViewer) serve() {
	err := v.server.Serve(v.listener)
	v.mu.Lock()
	if v.closed && (errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed)) {
		err = nil
	}
	v.serveErr = err
	v.mu.Unlock()
	close(v.done)
}

func (v *QRViewer) serveError() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.serveErr == nil {
		return nil
	}
	return fmt.Errorf("serve QR code viewer: %w", v.serveErr)
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
