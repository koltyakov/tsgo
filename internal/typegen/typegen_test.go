package typegen

import (
	"strings"
	"testing"
)

func TestGenerateContextDTS(t *testing.T) {
	globals := map[string]any{
		"userId":   123,
		"userName": "test",
		"isActive": true,
		"tags":     []string{"a", "b"},
	}

	dts := GenerateContextDTS(globals)

	expected := []string{
		"declare global",
		"const userId: number",
		"const userName: string",
		"const isActive: boolean",
		"const tags: string[]",
		"export {}",
	}

	for _, exp := range expected {
		if !strings.Contains(dts, exp) {
			t.Errorf("expected DTS to contain %q, got:\n%s", exp, dts)
		}
	}
}

func TestBuilder(t *testing.T) {
	builder := NewBuilder()

	builder.
		AddInterface("User", map[string]string{
			"id":   "number",
			"name": "string",
		}).
		AddGlobal("currentUser", "User").
		AddFunction("getUser", "id: number", "Promise<User>", "Fetches a user by ID")

	dts := builder.Build()

	expected := []string{
		"interface User",
		"id: number",
		"name: string",
		"const currentUser: User",
		"function getUser",
		"declare global",
	}

	for _, exp := range expected {
		if !strings.Contains(dts, exp) {
			t.Errorf("expected DTS to contain %q, got:\n%s", exp, dts)
		}
	}
}

func TestGoToTSType(t *testing.T) {
	tests := []struct {
		value    any
		expected string
	}{
		{123, "number"},
		{"hello", "string"},
		{true, "boolean"},
		{nil, "undefined"},
		{[]int{1, 2}, "number[]"},
		{map[string]int{"a": 1}, "Record<string, number>"},
	}

	for _, tt := range tests {
		result := goToTSType(tt.value)
		if result != tt.expected {
			t.Errorf("goToTSType(%v) = %q, want %q", tt.value, result, tt.expected)
		}
	}
}
