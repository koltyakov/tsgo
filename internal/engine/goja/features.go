// Package goja provides a GOJA-based JavaScript execution engine.
package goja

import "strings"

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
	found := make(map[string]bool)
	n := len(code)

	for i := 0; i < n; i++ {
		c := code[i]

		switch c {
		case 'a':
			if i+6 <= n && !found["async"] {
				sub := code[i : i+6]
				if sub == "async " || sub == "await " {
					found["async"] = true
					features = append(features, UnsupportedFeature{
						Name:        "async/await",
						Description: featureDescriptions["async"],
					})
				}
			}
		case 'f':
			if i+6 <= n && code[i:i+6] == "fetch(" && !found["fetch"] {
				found["fetch"] = true
				features = append(features, UnsupportedFeature{
					Name:        "fetch",
					Description: featureDescriptions["fetch"],
				})
			}
		case 'W':
			if i+9 <= n && code[i:i+9] == "WebSocket" && !found["websocket"] {
				found["websocket"] = true
				features = append(features, UnsupportedFeature{
					Name:        "WebSocket",
					Description: featureDescriptions["websocket"],
				})
			}
		case 'r':
			if i+8 <= n && code[i:i+8] == "readFile" && !found["fileio"] {
				found["fileio"] = true
				features = append(features, UnsupportedFeature{
					Name:        "File I/O",
					Description: featureDescriptions["fileio"],
				})
			}
		case 'w':
			if i+9 <= n && code[i:i+9] == "writeFile" && !found["fileio"] {
				found["fileio"] = true
				features = append(features, UnsupportedFeature{
					Name:        "File I/O",
					Description: featureDescriptions["fileio"],
				})
			}
		case 's':
			if !found["timers"] {
				if i+11 <= n && code[i:i+11] == "setTimeout(" {
					found["timers"] = true
					features = append(features, UnsupportedFeature{
						Name:        "Timers",
						Description: featureDescriptions["timers"],
					})
				} else if i+12 <= n && code[i:i+12] == "setInterval(" {
					found["timers"] = true
					features = append(features, UnsupportedFeature{
						Name:        "Timers",
						Description: featureDescriptions["timers"],
					})
				}
			}
		}
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
