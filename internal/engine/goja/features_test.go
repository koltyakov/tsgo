package goja

import (
	"strings"
	"testing"
)

func TestDetectUnsupportedFeatures(t *testing.T) {
	tests := []struct {
		name          string
		code          string
		expectFeature string // empty means no features expected
	}{
		{
			name:          "sync code - no issues",
			code:          `const x = 1 + 2; export default x;`,
			expectFeature: "",
		},
		{
			name:          "async function",
			code:          `async function getData() { return 42; }`,
			expectFeature: "async/await",
		},
		{
			name:          "await expression",
			code:          `const data = await fetch('/api');`,
			expectFeature: "async/await",
		},
		{
			name:          "fetch API",
			code:          `const response = fetch('/api/data');`,
			expectFeature: "fetch",
		},
		{
			name:          "WebSocket",
			code:          `const ws = new WebSocket('ws://localhost');`,
			expectFeature: "WebSocket",
		},
		{
			name:          "readFile",
			code:          `const data = readFile('./config.json');`,
			expectFeature: "File I/O",
		},
		{
			name:          "writeFile",
			code:          `writeFile('./output.txt', data);`,
			expectFeature: "File I/O",
		},
		{
			name:          "setTimeout",
			code:          `setTimeout(() => console.log('hi'), 1000);`,
			expectFeature: "Timers",
		},
		{
			name:          "setInterval",
			code:          `setInterval(() => tick(), 100);`,
			expectFeature: "Timers",
		},
		{
			name: "multiple features",
			code: `
				async function fetchData() {
					const response = await fetch('/api');
					return response.json();
				}
			`,
			expectFeature: "async/await", // Should detect async first
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			features := DetectUnsupportedFeatures(tc.code)

			if tc.expectFeature == "" {
				if len(features) > 0 {
					t.Errorf("expected no features, got %v", features)
				}
				return
			}

			if len(features) == 0 {
				t.Errorf("expected feature %q, got none", tc.expectFeature)
				return
			}

			found := false
			for _, f := range features {
				if f.Name == tc.expectFeature {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected feature %q, got %v", tc.expectFeature, features)
			}
		})
	}
}

func TestDetectUnsupportedFeatures_NoDuplicates(t *testing.T) {
	// Code with multiple async/await should only report once
	code := `
		async function a() { await fetch('/a'); }
		async function b() { await fetch('/b'); }
		const c = async () => await fetch('/c');
	`

	features := DetectUnsupportedFeatures(code)

	// Count occurrences of each feature name
	counts := make(map[string]int)
	for _, f := range features {
		counts[f.Name]++
	}

	for name, count := range counts {
		if count > 1 {
			t.Errorf("feature %q reported %d times, expected 1", name, count)
		}
	}
}

func TestDetectUnsupportedFeatures_IgnoresCommentsAndStrings(t *testing.T) {
	code := `
		// async await fetch('/x')
		const text = "setTimeout(readFile('/tmp'))";
		/* WebSocket writeFile */
		export default 1;
	`

	features := DetectUnsupportedFeatures(code)
	if len(features) != 0 {
		t.Fatalf("expected no features, got %v", features)
	}
}

func TestFormatUnsupportedFeaturesError(t *testing.T) {
	features := []UnsupportedFeature{
		{Name: "async/await", Description: "async functions require Bun"},
		{Name: "fetch", Description: "Fetch API requires Bun"},
	}

	msg := FormatUnsupportedFeaturesError(features)

	if !strings.Contains(msg, "GOJA engine does not support") {
		t.Error("expected error message header")
	}
	if !strings.Contains(msg, "async/await") {
		t.Error("expected async/await in message")
	}
	if !strings.Contains(msg, "fetch") {
		t.Error("expected fetch in message")
	}
	if !strings.Contains(msg, "Solution:") {
		t.Error("expected solution hint in message")
	}
}

func TestFormatUnsupportedFeaturesError_Empty(t *testing.T) {
	msg := FormatUnsupportedFeaturesError(nil)
	if msg != "" {
		t.Errorf("expected empty string for nil features, got %q", msg)
	}

	msg = FormatUnsupportedFeaturesError([]UnsupportedFeature{})
	if msg != "" {
		t.Errorf("expected empty string for empty features, got %q", msg)
	}
}

func TestContainsTopLevelAwait(t *testing.T) {
	tests := []struct {
		name   string
		code   string
		expect bool
	}{
		{
			name:   "no await",
			code:   `const x = 1; export default x;`,
			expect: false,
		},
		{
			name:   "await inside async function",
			code:   `async function getData() { const x = await fetch('/api'); return x; }`,
			expect: false,
		},
		{
			name:   "export default await",
			code:   `async function main() { return 42; } export default await main();`,
			expect: true,
		},
		{
			name:   "top-level const with await",
			code:   `const data = await fetch('/api'); export default data;`,
			expect: true,
		},
		{
			name: "await in async arrow inside sync code",
			code: `
				const fn = async () => {
					const x = await fetch('/api');
					return x;
				};
				export default fn;
			`,
			expect: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ContainsTopLevelAwait(tc.code)
			if result != tc.expect {
				t.Errorf("ContainsTopLevelAwait() = %v, want %v", result, tc.expect)
			}
		})
	}
}
