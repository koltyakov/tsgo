package sourcemap

import (
	"encoding/base64"
	"errors"
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
	// Simple mapping: AAAAA means (0,0,0,0)
	mappings := decodeMappings("AAAA")

	if len(mappings) == 0 {
		t.Fatal("expected at least one mapping")
	}

	m := mappings[0]
	if m.GeneratedLine != 1 {
		t.Errorf("expected generated line 1, got %d", m.GeneratedLine)
	}
}

func TestMapError(t *testing.T) {
	// Create a simple source map for testing
	sm := &SourceMap{
		Version:  3,
		Sources:  []string{"test.ts"},
		Mappings: "AAAA", // Simple identity mapping
	}

	tests := []struct {
		name string
		err  error
	}{
		{
			name: "GOJA format error",
			err:  errors.New(`ReferenceError: x is not defined at <anonymous>:1:5`),
		},
		{
			name: "nil error",
			err:  nil,
		},
		{
			name: "error without location",
			err:  errors.New(`generic error without location`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapError(tt.err, sm)

			if tt.err == nil {
				if result != nil {
					t.Errorf("expected nil for nil error, got %v", result)
				}
				return
			}

			if result == nil {
				t.Fatal("expected mapped error, got nil")
			}
		})
	}
}

func TestMapError_WithNilSourceMap(t *testing.T) {
	err := errors.New("some error at 1:5")
	result := MapError(err, nil)

	// Should return original error when source map is nil
	if result != err {
		t.Error("expected original error when source map is nil")
	}
}

func TestMappedError_Unwrap(t *testing.T) {
	original := errors.New("original error")
	mapped := &MappedError{
		Original: original,
		Message:  "mapped message",
	}

	if mapped.Error() != "mapped message" {
		t.Errorf("expected 'mapped message', got %q", mapped.Error())
	}

	if mapped.Unwrap() != original {
		t.Error("expected Unwrap to return original error")
	}
}

func TestMapLocation(t *testing.T) {
	tests := []struct {
		name       string
		mappings   string
		line       int
		col        int
		wantLine   int
		wantCol    int
		wantSource string
	}{
		{
			name:       "identity mapping",
			mappings:   "AAAA",
			line:       1,
			col:        0,
			wantLine:   1,
			wantCol:    0,
			wantSource: "test.ts",
		},
		{
			name:       "no mapping returns input",
			mappings:   "AAAA",
			line:       999,
			col:        999,
			wantLine:   999,
			wantCol:    999,
			wantSource: "",
		},
		{
			name:       "empty mappings",
			mappings:   "",
			line:       1,
			col:        0,
			wantLine:   1,
			wantCol:    0,
			wantSource: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := &SourceMap{Version: 3, Sources: []string{"test.ts"}, Mappings: tt.mappings}
			gotLine, gotCol, gotSource := MapLocation(sm, tt.line, tt.col)
			if gotLine != tt.wantLine || gotCol != tt.wantCol || gotSource != tt.wantSource {
				t.Errorf("MapLocation() = (%d, %d, %q), want (%d, %d, %q)",
					gotLine, gotCol, gotSource, tt.wantLine, tt.wantCol, tt.wantSource)
			}
		})
	}
}
