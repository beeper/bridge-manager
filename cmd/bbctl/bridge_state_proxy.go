package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"maunium.net/go/mautrix/appservice"
)

// maxStateBodyBytes bounds the size of a single status_endpoint POST body.
// A bridge state object is a few hundred bytes at most; anything larger is
// almost certainly a misconfiguration or an attack, not a state update.
const maxStateBodyBytes = 1 << 20 // 1 MiB

// bridgeStateProxy receives per-remote bridge state from a Python bridge over
// HTTP (the bridge's status_endpoint) and forwards it to Beeper over the
// appservice websocket as a bridge_status command.
//
// Go bridges (discord, whatsapp, telegram, ...) already send bridge_status over
// the appservice websocket natively, so Beeper Desktop can build their
// Space/Account. Python bridges (e.g. mautrix-googlechat) instead POST to an
// HTTP status_endpoint, which Beeper rejects (400) and which cannot set
// per-remote state anyway (its request struct has no remote_id). As a result
// Beeper's per-remote remoteState map stays empty and the account never shows
// up in Desktop. This proxy closes that gap: it listens on the appservice port
// + 1, accepts the Python bridge's status_endpoint POSTs, caches the last state
// per remote_id, and forwards the JSON over the appservice websocket.
type bridgeStateProxy struct {
	mu     sync.Mutex
	cache  map[string]json.RawMessage // remote_id -> last known state (raw JSON)
	sendMu sync.Mutex                 // serializes websocket sends (replay vs live)
	as     *appservice.AppService
}

func newBridgeStateProxy(as *appservice.AppService) *bridgeStateProxy {
	return &bridgeStateProxy{
		cache: make(map[string]json.RawMessage),
		as:    as,
	}
}

// listenBridgeStateReceiver listens on the same address as the appservice HTTP
// server but on port+1, so it never collides with the bridge's own appservice
// HTTP port. The Python bridge's status_endpoint is pointed at this address.
func listenBridgeStateReceiver(asURL string) (net.Listener, error) {
	u, err := url.Parse(asURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse appservice URL %q: %w", asURL, err)
	}
	host := u.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	portStr := u.Port()
	if portStr == "" {
		return nil, fmt.Errorf("appservice URL %q has no port", asURL)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q in appservice URL: %w", portStr, err)
	}
	if port < 1 || port > 65534 {
		return nil, fmt.Errorf("appservice port %d out of range; cannot derive receiver port", port)
	}
	return net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port+1)))
}

// serveBridgeStateReceiver runs the HTTP server that accepts the Python
// bridge's status_endpoint POSTs and forwards them over the websocket.
func (p *bridgeStateProxy) serveBridgeStateReceiver(ln net.Listener) {
	mux := http.NewServeMux()
	mux.HandleFunc("/bridge_state", p.handleState)
	mux.HandleFunc("/", p.handleState)
	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		p.as.Log.Warn().Err(err).Msg("Bridge state receiver stopped")
	}
}

// handleState reads one status_endpoint POST, validates it, caches it by
// remote_id, and forwards it over the appservice websocket.
func (p *bridgeStateProxy) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxStateBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	// Require a JSON object (not null, array, string, or number) so we never
	// forward a malformed state that Beeper would reject.
	var probe struct {
		RemoteID string `json:"remote_id"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		http.Error(w, "invalid JSON object", http.StatusBadRequest)
		return
	}
	key := probe.RemoteID
	if key == "" {
		key = "__global__"
	}
	p.mu.Lock()
	p.cache[key] = body
	p.mu.Unlock()
	p.forwardState(r.Context(), key, body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// forwardState sends the state JSON over the appservice websocket as a
// bridge_status command. The bytes are forwarded verbatim (as json.RawMessage)
// so the payload preserves the exact JSON structure Beeper expects — the same
// field names and types (snake_case state_event / remote_id / remote_name) that
// Go bridges send over the same command.
func (p *bridgeStateProxy) forwardState(ctx context.Context, key string, raw []byte) {
	if p.as == nil || !p.as.HasWebsocket() {
		return
	}
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	err := p.as.SendWebsocket(ctx, &appservice.WebsocketRequest{
		Command: "bridge_status",
		Data:    json.RawMessage(raw),
	})
	if err != nil {
		p.as.Log.Warn().Err(err).Str("remote_id", key).Msg("Failed to forward bridge state over websocket")
	} else {
		p.as.Log.Info().Str("remote_id", key).Msg("Forwarded bridge state over appservice websocket")
	}
}

// replayStates re-sends cached states after a websocket (re)connect, so Beeper
// retains the correct per-remote state across transient disconnects. The Python
// bridge only emits CONNECTED once, so without replay a Beeper-side websocket
// reconnect would drop the Space/Account.
//
// A state is only replayed if it is still the latest one cached for its
// remote_id; if a newer state arrived (and was already sent live) while the
// replay was in flight, the stale snapshot is skipped so the client never sees
// an older state after a newer one.
func (p *bridgeStateProxy) replayStates(ctx context.Context, as *appservice.AppService) {
	p.mu.Lock()
	type kv struct {
		key string
		raw json.RawMessage
	}
	items := make([]kv, 0, len(p.cache))
	for k, v := range p.cache {
		items = append(items, kv{k, v})
	}
	p.mu.Unlock()
	for _, it := range items {
		p.mu.Lock()
		current, ok := p.cache[it.key]
		stillCurrent := ok && bytes.Equal(current, it.raw)
		p.mu.Unlock()
		if !stillCurrent {
			continue
		}
		p.forwardState(ctx, it.key, it.raw)
	}
}
