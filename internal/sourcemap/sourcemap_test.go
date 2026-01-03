package sourcemap

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	sm := `{"version":3,"sources":["test.ts"],"names":[],"mappings":"AAAA"}`

	parsed, err := Parse([]byte(sm))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if parsed.Version != 3 {
		t.Errorf("expected version 3, got %d", parsed.Version)
	}
	if len(parsed.Sources) != 1 {
		t.Errorf("expected 1 source, got %d", len(parsed.Sources))
	}
	if parsed.Sources[0] != "test.ts" {
		t.Errorf("expected source test.ts, got %s", parsed.Sources[0])
	}
}

func TestParseBase64(t *testing.T) {
	sm := `{"version":3,"sources":["test.ts"],"names":[],"mappings":"AAAA"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(sm))

	parsed, err := ParseBase64(encoded)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if parsed.Version != 3 {
		t.Errorf("expected version 3, got %d", parsed.Version)
	}
}

func TestFormatError(t *testing.T) {
	msg := FormatError("ReferenceError: x is not defined", "test.ts", 10, 5, "const y = x;")

	if !strings.Contains(msg, "ReferenceError") {
		t.Error("expected error message")
	}
	if !strings.Contains(msg, "test.ts:10:5") {
		t.Error("expected location")
	}
	if !strings.Contains(msg, "const y = x;") {
		t.Error("expected snippet")
	}
}

func TestDecodeVLQ(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		consumed int
	}{
		{"A", 0, 1},
		{"C", 1, 1},
		{"D", -1, 1},
		{"E", 2, 1},
		{"F", -2, 1},
	}

	for _, tt := range tests {
		val, consumed := decodeVLQ(tt.input)
		if val != tt.expected || consumed != tt.consumed {
			t.Errorf("decodeVLQ(%q) = (%d, %d), want (%d, %d)",
				tt.input, val, consumed, tt.expected, tt.consumed)
		}
	}
}

func TestDecodeMappings(t *testing.T) {
	// Simple mapping: AAAA means (0,0,0,0)
	mappings := decodeMappings("AAAA")

	if len(mappings) == 0 {
		t.Fatal("expected at least one mapping")
	}

	m := mappings[0]
	if m.GeneratedLine != 1 {
		t.Errorf("expected generated line 1, got %d", m.GeneratedLine)
	}
}
