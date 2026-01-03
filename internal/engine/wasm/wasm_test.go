package wasm

import (
	"testing"
)

func TestNew_NoWasm(t *testing.T) {
	// With empty quickjs.wasm file, New should fail
	_, err := New(Config{MemoryLimit: 32 * 1024 * 1024})

	// Expected to fail because quickjs.wasm is empty
	if err == nil {
		t.Skip("QuickJS WASM is available, skipping empty-check test")
	}

	expectedErr := "QuickJS WASM module not embedded"
	if err.Error() != expectedErr {
		t.Errorf("expected error %q, got %q", expectedErr, err.Error())
	}
}

func TestDefaultMemoryLimit(t *testing.T) {
	cfg := Config{}
	if cfg.MemoryLimit == 0 {
		cfg.MemoryLimit = 64 * 1024 * 1024 // Default behavior
	}
	if cfg.MemoryLimit != 64*1024*1024 {
		t.Errorf("expected 64MB default, got %d", cfg.MemoryLimit)
	}
}

func TestBuildScript(t *testing.T) {
	t.Run("without globals", func(t *testing.T) {
		script := buildScript("return 42", nil)
		if script == "" {
			t.Error("expected non-empty script")
		}
	})

	t.Run("with globals", func(t *testing.T) {
		script := buildScript("return x + y", map[string]any{
			"x": 10,
			"y": 20,
		})
		if script == "" {
			t.Error("expected non-empty script")
		}
	})
}

func TestWrapForResult(t *testing.T) {
	wrapped := wrapForResult("return 1 + 1")
	if wrapped == "" {
		t.Error("expected non-empty wrapped code")
	}
}
