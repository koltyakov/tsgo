// Package sourcemap provides source map parsing and error mapping.
//
// Source maps allow mapping transpiled JavaScript locations back to the
// original TypeScript source. This is essential for providing meaningful
// error messages when runtime errors occur in transpiled code.
//
// The package implements VLQ (Variable-Length Quantity) decoding for
// efficient parsing of source map mappings strings.
package sourcemap

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ============================================================================
// VLQ Decoding Lookup Table
// ============================================================================

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

// ============================================================================
// Types
// ============================================================================

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

// ============================================================================
// Parsing
// ============================================================================

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

// ============================================================================
// Location Mapping
// ============================================================================

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
// It parses stack traces from GOJA errors and maps line/column numbers
// back to the original TypeScript source locations.
func MapError(err error, sm *SourceMap) error {
	if err == nil || sm == nil {
		return err
	}

	errStr := err.Error()
	if errStr == "" {
		return err
	}

	// Parse and map all locations in the error message
	mappedStr := mapErrorLocations(errStr, sm)
	if mappedStr == errStr {
		// No mapping was possible, return original
		return err
	}

	return &MappedError{
		Original: err,
		Message:  mappedStr,
	}
}

// MappedError represents an error with mapped source locations.
type MappedError struct {
	Original error
	Message  string
}

func (e *MappedError) Error() string {
	return e.Message
}

func (e *MappedError) Unwrap() error {
	return e.Original
}

// locationPattern matches common JavaScript stack trace location formats:
// - "at <anonymous>:3:9" (GOJA format)
// - "at functionName (file.js:10:5)" (standard format)
// - "file.js:10:5" (simple format)
var locationPattern = regexp.MustCompile(`(?:(?:at\s+)?\(?)([^:\s()]+)?:?(\d+):(\d+)\)?`)

// mapErrorLocations finds and maps all locations in an error string.
func mapErrorLocations(errStr string, sm *SourceMap) string {
	var result strings.Builder
	lastEnd := 0

	for _, match := range locationPattern.FindAllStringSubmatchIndex(errStr, -1) {
		// Write text between matches
		result.WriteString(errStr[lastEnd:match[0]])

		// Extract location components
		// match[0], match[1] = full match
		// match[2], match[3] = file name (optional)
		// match[4], match[5] = line number
		// match[6], match[7] = column number

		lineStr := errStr[match[4]:match[5]]
		colStr := errStr[match[6]:match[7]]

		line, _ := strconv.Atoi(lineStr)
		column, _ := strconv.Atoi(colStr)

		// Map the location
		origLine, origCol, source := MapLocation(sm, line, column)

		// Build the mapped location string
		if source != "" {
			result.WriteString(fmt.Sprintf("%s:%d:%d", source, origLine, origCol))
		} else {
			result.WriteString(fmt.Sprintf("%d:%d", origLine, origCol))
		}

		lastEnd = match[1]
	}

	// Write remaining text
	result.WriteString(errStr[lastEnd:])

	return result.String()
}

// FormatError formats an error with source location.
func FormatError(msg string, source string, line, column int, snippet string) string {
	result := fmt.Sprintf("%s\n  at %s:%d:%d", msg, source, line, column)
	if snippet != "" {
		result += fmt.Sprintf("\n  %s", snippet)
	}
	return result
}

// ============================================================================
// VLQ Decoding
// ============================================================================

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
