package types

import (
	"testing"
	"time"
)

func TestEngineTypeString(t *testing.T) {
	tests := []struct {
		engine   EngineType
		expected string
	}{
		{EngineAuto, "auto"},
		{EngineGOJA, "goja"},
		{EngineBun, "bun"},
		{EngineWASM, "wasm"},
		{EngineType(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.engine.String(); got != tt.expected {
			t.Errorf("EngineType(%d).String() = %q, want %q", tt.engine, got, tt.expected)
		}
	}
}

func TestDefaultSecurityPolicy(t *testing.T) {
	policy := DefaultSecurityPolicy()

	if policy.NetworkAccess {
		t.Error("expected NetworkAccess to be false by default")
	}
	if policy.DiskAccess {
		t.Error("expected DiskAccess to be false by default")
	}
	if policy.UntrustedSource {
		t.Error("expected UntrustedSource to be false by default")
	}
	if policy.MaxMemoryMB != 64 {
		t.Errorf("expected MaxMemoryMB = 64, got %d", policy.MaxMemoryMB)
	}
	if policy.MaxExecutionTime != 30*time.Second {
		t.Errorf("expected MaxExecutionTime = 30s, got %v", policy.MaxExecutionTime)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Engine != EngineAuto {
		t.Errorf("expected Engine = EngineAuto, got %v", cfg.Engine)
	}
	if cfg.Timeout.Duration() != 30*time.Second {
		t.Errorf("expected Timeout = 30s, got %v", cfg.Timeout)
	}
	if !cfg.SourceMaps {
		t.Error("expected SourceMaps to be true by default")
	}
}

func TestExecutionError(t *testing.T) {
	err := &ExecutionError{
		Message: "test error",
		Code:    "let x = 1",
		Line:    5,
		Column:  10,
	}

	if err.Error() != "test error" {
		t.Errorf("expected error message 'test error', got %q", err.Error())
	}
}

func TestDuration(t *testing.T) {
	d := Duration(5 * time.Second)
	if d.Duration() != 5*time.Second {
		t.Errorf("expected 5s, got %v", d.Duration())
	}
}
