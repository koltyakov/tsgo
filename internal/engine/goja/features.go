// Package goja provides a GOJA-based JavaScript execution engine.
package goja

import (
	"strings"

	"github.com/koltyakov/tsgo/internal/codeanalyzer"
)

// ============================================================================
// Unsupported Feature Detection
// ============================================================================

// UnsupportedFeature represents a feature not supported by GOJA engine.
type UnsupportedFeature struct {
	Name        string
	Description string
}

// Feature descriptions for error messages.
var featureDescriptions = map[string]string{
	"async":     "async functions and await expressions require Bun engine",
	"fetch":     "the Fetch API requires Bun engine",
	"websocket": "WebSocket API requires Bun engine",
	"fileio":    "file system operations (readFile/writeFile) require Bun engine",
	"timers":    "setTimeout/setInterval require Bun engine for proper async behavior",
}

// DetectUnsupportedFeatures checks code for features not supported by GOJA engine.
// Returns a list of unsupported features found in the code.
func DetectUnsupportedFeatures(code string) []UnsupportedFeature {
	var features []UnsupportedFeature
	analysis := codeanalyzer.Analyze(code)

	if analysis.HasAsync || analysis.HasAwait {
		features = append(features, UnsupportedFeature{
			Name:        "async/await",
			Description: featureDescriptions["async"],
		})
	}
	if analysis.HasCall("fetch") {
		features = append(features, UnsupportedFeature{
			Name:        "fetch",
			Description: featureDescriptions["fetch"],
		})
	}
	if analysis.HasIdentifier("WebSocket") {
		features = append(features, UnsupportedFeature{
			Name:        "WebSocket",
			Description: featureDescriptions["websocket"],
		})
	}
	if analysis.HasCall("readFile") || analysis.HasCall("writeFile") {
		features = append(features, UnsupportedFeature{
			Name:        "File I/O",
			Description: featureDescriptions["fileio"],
		})
	}
	if analysis.HasCall("setTimeout") || analysis.HasCall("setInterval") {
		features = append(features, UnsupportedFeature{
			Name:        "Timers",
			Description: featureDescriptions["timers"],
		})
	}

	return features
}

// ============================================================================
// Error Formatting
// ============================================================================

// FormatUnsupportedFeaturesError creates a user-friendly error message for unsupported features.
func FormatUnsupportedFeaturesError(features []UnsupportedFeature) string {
	if len(features) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("GOJA engine does not support the following features used in your code:\n")
	for _, f := range features {
		sb.WriteString("  • ")
		sb.WriteString(f.Name)
		sb.WriteString(": ")
		sb.WriteString(f.Description)
		sb.WriteByte('\n')
	}
	sb.WriteString("\nSolution: Use Bun engine by setting Engine: tsgo.EngineBun in your config, or select 'Bun' in Monaco.")
	return sb.String()
}

// ============================================================================
// Top-Level Await Detection
// ============================================================================

// ContainsTopLevelAwait checks if code has top-level await (await outside async function).
// This is a simple heuristic check for common patterns.
func ContainsTopLevelAwait(code string) bool {
	n := len(code)
	for i := 0; i < n-6; i++ {
		if code[i:i+6] != "await " {
			continue
		}

		// Check for "export default await" pattern
		if i >= 15 && code[i-15:i] == "export default " {
			return true
		}

		// Check for assignment patterns like "const x = await"
		if i >= 2 && code[i-2:i] == "= " {
			// Verify not inside async function by checking nearby prefix
			start := i - 100
			if start < 0 {
				start = 0
			}
			prefix := code[start:i]
			if !strings.Contains(prefix, "async ") {
				return true
			}
		}
	}
	return false
}
