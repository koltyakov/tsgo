// Package sourcemap provides source map parsing and error mapping.
package sourcemap

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// SourceMap represents a parsed source map.
type SourceMap struct {
	Version        int      `json:"version"`
	Sources        []string `json:"sources"`
	Names          []string `json:"names"`
	Mappings       string   `json:"mappings"`
	SourcesContent []string `json:"sourcesContent"`
}

// Mapping represents a single source map mapping.
type Mapping struct {
	GeneratedLine   int
	GeneratedColumn int
	OriginalLine    int
	OriginalColumn  int
	SourceIndex     int
	NameIndex       int
}

// Parse parses a source map from JSON bytes.
func Parse(data []byte) (*SourceMap, error) {
	var sm SourceMap
	if err := json.Unmarshal(data, &sm); err != nil {
		return nil, fmt.Errorf("failed to parse source map: %w", err)
	}
	return &sm, nil
}

// ParseBase64 parses a base64-encoded source map.
func ParseBase64(encoded string) (*SourceMap, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}
	return Parse(decoded)
}

// MapLocation maps a generated location to original source location.
func MapLocation(sm *SourceMap, line, column int) (originalLine, originalColumn int, source string) {
	mappings := decodeMappings(sm.Mappings)

	for _, m := range mappings {
		if m.GeneratedLine == line && m.GeneratedColumn <= column {
			originalLine = m.OriginalLine
			originalColumn = m.OriginalColumn
			if m.SourceIndex >= 0 && m.SourceIndex < len(sm.Sources) {
				source = sm.Sources[m.SourceIndex]
			}
			return
		}
	}

	return line, column, ""
}

// MapError maps an error with JS location to TypeScript location.
func MapError(err error, sm *SourceMap) error {
	return err // Simplified - full implementation would parse error location
}

// FormatError formats an error with source location.
func FormatError(msg string, source string, line, column int, snippet string) string {
	result := fmt.Sprintf("%s\n  at %s:%d:%d", msg, source, line, column)
	if snippet != "" {
		result += fmt.Sprintf("\n  %s", snippet)
	}
	return result
}

// decodeMappings decodes VLQ-encoded mappings string.
func decodeMappings(encoded string) []Mapping {
	var mappings []Mapping
	var line, column, sourceIndex, originalLine, originalColumn, nameIndex int

	generatedLine := 1
	i := 0

	for i < len(encoded) {
		c := encoded[i]

		if c == ';' {
			generatedLine++
			column = 0
			i++
			continue
		}

		if c == ',' {
			i++
			continue
		}

		// Decode segment
		var values []int
		for i < len(encoded) && encoded[i] != ',' && encoded[i] != ';' {
			val, consumed := decodeVLQ(encoded[i:])
			values = append(values, val)
			i += consumed
		}

		if len(values) >= 1 {
			column += values[0]
		}
		if len(values) >= 2 {
			sourceIndex += values[1]
		}
		if len(values) >= 3 {
			originalLine += values[2]
		}
		if len(values) >= 4 {
			originalColumn += values[3]
		}
		if len(values) >= 5 {
			nameIndex += values[4]
		}

		if len(values) >= 4 {
			mappings = append(mappings, Mapping{
				GeneratedLine:   generatedLine,
				GeneratedColumn: column,
				OriginalLine:    originalLine + 1, // 1-based
				OriginalColumn:  originalColumn,
				SourceIndex:     sourceIndex,
				NameIndex:       nameIndex,
			})
		}
	}

	_ = line // unused

	return mappings
}

// decodeVLQ decodes a single VLQ value.
func decodeVLQ(s string) (int, int) {
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

	var result, shift int
	var continuation bool
	consumed := 0

	for i := 0; i < len(s); i++ {
		idx := -1
		for j, c := range base64Chars {
			if byte(c) == s[i] {
				idx = j
				break
			}
		}
		if idx == -1 {
			break
		}

		consumed++
		continuation = (idx & 32) != 0
		result += (idx & 31) << shift
		shift += 5

		if !continuation {
			break
		}
	}

	// Handle sign bit
	if result&1 != 0 {
		result = -(result >> 1)
	} else {
		result = result >> 1
	}

	return result, consumed
}
