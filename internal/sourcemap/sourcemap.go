// Package sourcemap provides source map parsing and error mapping.
package sourcemap

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// base64Lookup is a pre-computed lookup table for base64 character to index conversion.
var base64Lookup = func() [256]int8 {
	var table [256]int8
	for i := range table {
		table[i] = -1
	}
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	for i, c := range base64Chars {
		table[c] = int8(i)
	}
	return table
}()

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
// Pre-allocates slice capacity based on estimated mapping count.
func decodeMappings(encoded string) []Mapping {
	if len(encoded) == 0 {
		return nil
	}

	// Estimate capacity: roughly 1 mapping per 5 characters
	estimatedCap := len(encoded) / 5
	if estimatedCap < 16 {
		estimatedCap = 16
	}
	mappings := make([]Mapping, 0, estimatedCap)

	var column, sourceIndex, originalLine, originalColumn, nameIndex int
	generatedLine := 1
	i := 0

	// Pre-allocate values slice outside loop
	values := make([]int, 0, 5)

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

		// Decode segment - reuse values slice
		values = values[:0]
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

	return mappings
}

// decodeVLQ decodes a single VLQ value using pre-computed lookup table.
func decodeVLQ(s string) (int, int) {
	var result, shift int
	consumed := 0

	for i := 0; i < len(s); i++ {
		idx := base64Lookup[s[i]]
		if idx < 0 {
			break
		}

		consumed++
		result += (int(idx) & 31) << shift
		shift += 5

		// Check continuation bit
		if idx&32 == 0 {
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
