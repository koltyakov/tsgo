package monaco

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/koltyakov/tsgo/internal/typegen"
)

func TestNewHandler(t *testing.T) {
	h := NewHandler()
	if h == nil {
		t.Fatal("expected handler")
	}
}

func TestSetTypes(t *testing.T) {
	h := NewHandler()
	b := typegen.NewBuilder()
	b.AddGlobal("test", "string")
	h.SetTypes(b)

	h.mu.RLock()
	dts := h.types.Build()
	h.mu.RUnlock()

	if !strings.Contains(dts, "test") {
		t.Error("expected types to contain test")
	}
}

func TestHandleTypes(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest("GET", "/types", nil)
	w := httptest.NewRecorder()

	h.handleTypes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected application/json")
	}
}

func TestHandleClientScript(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest("GET", "/client.js", nil)
	w := httptest.NewRecorder()

	h.handleClientScript(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "tsgoMonaco") {
		t.Error("expected tsgoMonaco in script")
	}
}

func TestServeHTTP_NotFound(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest("GET", "/unknown", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Host != "localhost" {
		t.Errorf("expected localhost, got %s", cfg.Host)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected 8080, got %d", cfg.Port)
	}
}

func TestClientScript(t *testing.T) {
	script := ClientScript()
	if !strings.Contains(script, "WebSocket") {
		t.Error("expected WebSocket in script")
	}
}
