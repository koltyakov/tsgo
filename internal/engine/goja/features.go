// Package goja provides a GOJA-based JavaScript execution engine.
package goja

import "strings"

// UnsupportedFeature represents a feature not supported by GOJA engine.
type UnsupportedFeature struct {
	Name        string
	Description string
}

// DetectUnsupportedFeatures checks code for features not supported by GOJA engine.
// Returns a list of unsupported features found in the code.
func DetectUnsupportedFeatures(code string) []UnsupportedFeature {
	var features []UnsupportedFeature
	n := len(code)

	// Track what we've found to avoid duplicates
	found := make(map[string]bool)

	for i := 0; i < n; i++ {
		c := code[i]

		switch c {
		case 'a':
			// async/await
			if i+6 <= n {
				sub := code[i : i+6]
				if sub == "async " && !found["async"] {
					found["async"] = true
					features = append(features, UnsupportedFeature{
						Name:        "async/await",
						Description: "async functions and await expressions require Bun engine",
					})
				} else if sub == "await " && !found["async"] {
					found["async"] = true
					features = append(features, UnsupportedFeature{
						Name:        "async/await",
						Description: "async functions and await expressions require Bun engine",
					})
				}
			}
		case 'f':
			// fetch API
			if i+6 <= n && code[i:i+6] == "fetch(" && !found["fetch"] {
				found["fetch"] = true
				features = append(features, UnsupportedFeature{
					Name:        "fetch",
					Description: "the Fetch API requires Bun engine",
				})
			}
		case 'W':
			// WebSocket
			if i+9 <= n && code[i:i+9] == "WebSocket" && !found["websocket"] {
				found["websocket"] = true
				features = append(features, UnsupportedFeature{
					Name:        "WebSocket",
					Description: "WebSocket API requires Bun engine",
				})
			}
		case 'r':
			// readFile (Bun file API)
			if i+8 <= n && code[i:i+8] == "readFile" && !found["fileio"] {
				found["fileio"] = true
				features = append(features, UnsupportedFeature{
					Name:        "File I/O",
					Description: "file system operations (readFile/writeFile) require Bun engine",
				})
			}
		case 'w':
			// writeFile (Bun file API)
			if i+9 <= n && code[i:i+9] == "writeFile" && !found["fileio"] {
				found["fileio"] = true
				features = append(features, UnsupportedFeature{
					Name:        "File I/O",
					Description: "file system operations (readFile/writeFile) require Bun engine",
				})
			}
		case 's':
			// setTimeout/setInterval (limited in GOJA)
			if i+11 <= n && code[i:i+11] == "setTimeout(" && !found["timers"] {
				found["timers"] = true
				features = append(features, UnsupportedFeature{
					Name:        "Timers",
					Description: "setTimeout/setInterval require Bun engine for proper async behavior",
				})
			}
			if i+12 <= n && code[i:i+12] == "setInterval(" && !found["timers"] {
				found["timers"] = true
				features = append(features, UnsupportedFeature{
					Name:        "Timers",
					Description: "setTimeout/setInterval require Bun engine for proper async behavior",
				})
			}
		}
	}

	return features
}

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
		sb.WriteString("\n")
	}
	sb.WriteString("\nSolution: Use Bun engine by setting Engine: tsgo.EngineBun in your config, or select 'Bun' in Monaco.")
	return sb.String()
}

// ContainsTopLevelAwait checks if code has top-level await (await outside async function).
// This is a simple heuristic check for common patterns.
func ContainsTopLevelAwait(code string) bool {
	// Look for "await " at the start of a line or after common statement starters
	// This catches: `await fetch()`, `const x = await ...`, `export default await ...`
	n := len(code)
	for i := 0; i < n-6; i++ {
		if code[i:i+6] == "await " {
			// Check if this await is at top level (not inside async function)
			// Simple heuristic: if we see "async " before this await on the same "block level",
			// it's likely inside an async function. Otherwise, it's top-level.
			// For accuracy, check if await appears after "export default" pattern
			if i >= 15 && code[i-15:i] == "export default " {
				return true
			}
			// Check for assignment patterns like "const x = await" or "let x = await"
			if i >= 2 && code[i-2:i] == "= " {
				// Could be top-level, but need to verify not inside async function
				// Check backwards for "async function" or "async ("
				start := i - 100
				if start < 0 {
					start = 0
				}
				prefix := code[start:i]
				// If no "async " in the nearby prefix, likely top-level
				if !strings.Contains(prefix, "async ") {
					return true
				}
			}
		}
	}
	return false
}
