// Package monaco provides Monaco editor integration for live TypeScript editing.
package monaco

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/koltyakov/tsgo/internal/typegen"
)

// Config configures Monaco integration.
type Config struct {
	Host         string
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DefaultConfig returns the default Monaco configuration.
func DefaultConfig() Config {
	return Config{
		Host:         "localhost",
		Port:         8080,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}

// Handler provides WebSocket-based Monaco integration.
type Handler struct {
	upgrader websocket.Upgrader
	types    *typegen.Builder
	clients  map[*websocket.Conn]bool
	mu       sync.RWMutex
}

// NewHandler creates a new Monaco handler.
func NewHandler() *Handler {
	return &Handler{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		types:   typegen.NewBuilder(),
		clients: make(map[*websocket.Conn]bool),
	}
}

// SetTypes updates the TypeScript type definitions.
func (h *Handler) SetTypes(builder *typegen.Builder) {
	h.mu.Lock()
	h.types = builder
	h.mu.Unlock()
	h.broadcastTypes()
}

// ServeHTTP handles HTTP requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/ws":
		h.handleWebSocket(w, r)
	case "/types":
		h.handleTypes(w, r)
	case "/client.js":
		h.handleClientScript(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
	}()

	h.mu.RLock()
	dts := h.types.Build()
	h.mu.RUnlock()

	h.sendTypes(conn, dts)

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			return
		}
	}
}

func (h *Handler) handleTypes(w http.ResponseWriter, _ *http.Request) {
	h.mu.RLock()
	dts := h.types.Build()
	h.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"types": dts})
}

func (h *Handler) handleClientScript(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Write([]byte(ClientScript()))
}

func (h *Handler) sendTypes(conn *websocket.Conn, dts string) error {
	msg, _ := json.Marshal(map[string]string{"type": "types", "types": dts})
	return conn.WriteMessage(websocket.TextMessage, msg)
}

func (h *Handler) broadcastTypes() {
	h.mu.RLock()
	dts := h.types.Build()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for conn := range h.clients {
		clients = append(clients, conn)
	}
	h.mu.RUnlock()

	for _, conn := range clients {
		if err := h.sendTypes(conn, dts); err != nil {
			h.mu.Lock()
			delete(h.clients, conn)
			h.mu.Unlock()
		}
	}
}

// ClientScript returns JavaScript for Monaco integration.
func ClientScript() string {
	return `(function() {
  let ws = null, monaco = null, extraLib = null;
  function connect(url) {
    ws = new WebSocket(url);
    ws.onmessage = function(e) {
      const data = JSON.parse(e.data);
      if (data.type === 'types' && monaco) {
        if (extraLib) extraLib.dispose();
        extraLib = monaco.languages.typescript.typescriptDefaults.addExtraLib(
          data.types, 'file:///node_modules/@types/tsgo/index.d.ts'
        );
      }
    };
    ws.onclose = function() { setTimeout(() => connect(url), 2000); };
  }
  window.tsgoMonaco = {
    init: function(m, url) {
      monaco = m;
      connect(url || 'ws://localhost:8080/ws');
    }
  };
})();`
}

// Serve starts the Monaco integration server.
func Serve(cfg Config) error {
	handler := NewHandler()
	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}
	return server.ListenAndServe()
}
