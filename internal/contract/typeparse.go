package contract

import (
	"slices"
	"strconv"
	"strings"
)

// ParseTypeString parses a type string into a TypeDef structure.
// If kind is empty, it will be inferred from the type string.
func ParseTypeString(typeStr string, kind string) *TypeDef {
	typeStr = strings.TrimSpace(typeStr)

	// Determine kind if not provided
	if kind == "" {
		kind = InferKind(typeStr)
	}

	switch kind {
	case "primitive":
		return &TypeDef{Kind: "primitive", Name: typeStr}

	case "literal":
		return &TypeDef{Kind: "literal", Name: typeStr, LiteralValue: typeStr}

	case "array":
		elemType := ""
		if strings.HasSuffix(typeStr, "[]") {
			elemType = strings.TrimSuffix(typeStr, "[]")
			// Handle wrapped types like (string | number)[]
			if strings.HasPrefix(elemType, "(") && strings.HasSuffix(elemType, ")") {
				elemType = elemType[1 : len(elemType)-1]
			}
		}
		return &TypeDef{
			Kind:        "array",
			Name:        typeStr,
			ElementType: ParseTypeString(elemType, ""),
		}

	case "union":
		parts := SplitUnion(typeStr)
		var unionTypes []*TypeDef
		for _, part := range parts {
			unionTypes = append(unionTypes, ParseTypeString(strings.TrimSpace(part), ""))
		}
		return &TypeDef{Kind: "union", Name: typeStr, UnionTypes: unionTypes}

	case "function":
		return &TypeDef{Kind: "function", Name: typeStr}

	case "object":
		// Parse object type like { name: string; age: number; }
		if strings.HasPrefix(typeStr, "{") && strings.HasSuffix(typeStr, "}") {
			return ParseObjectType(typeStr)
		}
		return &TypeDef{Kind: "object", Name: typeStr}

	default:
		return &TypeDef{Kind: "any", Name: typeStr}
	}
}

// InferKind infers the TypeDef kind from a type string.
func InferKind(typeStr string) string {
	typeStr = strings.TrimSpace(typeStr)

	// Primitives
	primitives := []string{"string", "number", "boolean", "bigint", "symbol", "null", "undefined", "void", "never", "any", "unknown"}
	if slices.Contains(primitives, typeStr) {
		return "primitive"
	}

	// Literals
	if strings.HasPrefix(typeStr, `"`) && strings.HasSuffix(typeStr, `"`) {
		return "literal"
	}
	if typeStr == "true" || typeStr == "false" {
		return "literal"
	}
	if _, err := strconv.ParseFloat(typeStr, 64); err == nil {
		return "literal"
	}

	// Arrays
	if strings.HasSuffix(typeStr, "[]") {
		return "array"
	}

	// Unions (but not within objects or functions)
	if ContainsTopLevelUnion(typeStr) {
		return "union"
	}

	// Functions
	if strings.Contains(typeStr, "=>") {
		return "function"
	}

	// Objects
	if strings.HasPrefix(typeStr, "{") {
		return "object"
	}

	// Generic types like Promise<T>
	if strings.Contains(typeStr, "<") {
		return "object"
	}

	return "object"
}

// ContainsTopLevelUnion checks if the type string contains a union operator at the top level
// (not nested inside braces, parentheses, or angle brackets).
func ContainsTopLevelUnion(typeStr string) bool {
	depth := 0
	for i := 0; i < len(typeStr); i++ {
		switch typeStr[i] {
		case '{', '(', '<':
			depth++
		case '}', ')', '>':
			depth--
		case '|':
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// SplitUnion splits a union type string at the top level pipe operators.
func SplitUnion(typeStr string) []string {
	var parts []string
	var current strings.Builder
	depth := 0

	for i := 0; i < len(typeStr); i++ {
		ch := typeStr[i]
		switch ch {
		case '{', '(', '<':
			depth++
			current.WriteByte(ch)
		case '}', ')', '>':
			depth--
			current.WriteByte(ch)
		case '|':
			if depth == 0 {
				parts = append(parts, current.String())
				current.Reset()
			} else {
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// ParseObjectType parses an object type string like "{ name: string; age: number; }"
// into a TypeDef with populated Properties.
func ParseObjectType(typeStr string) *TypeDef {
	// Remove outer braces and whitespace
	inner := strings.TrimSpace(typeStr[1 : len(typeStr)-1])
	if inner == "" {
		return &TypeDef{Kind: "object", Name: typeStr}
	}

	// Parse properties
	props := ParseProperties(inner)
	typeDef := &TypeDef{Kind: "object", Name: typeStr}

	for _, propStr := range props {
		propStr = strings.TrimSpace(propStr)
		if propStr == "" {
			continue
		}

		// Parse "name?: type" or "name: type"
		colonIdx := findPropertyColon(propStr)
		if colonIdx == -1 {
			continue
		}

		namePart := strings.TrimSpace(propStr[:colonIdx])
		typePart := strings.TrimSpace(propStr[colonIdx+1:])

		optional := false
		if strings.HasSuffix(namePart, "?") {
			optional = true
			namePart = strings.TrimSuffix(namePart, "?")
		}

		propTypeDef := ParseTypeString(typePart, "")
		typeDef.Properties = append(typeDef.Properties, Property{
			Name:     namePart,
			Type:     propTypeDef,
			Required: !optional,
		})
	}

	return typeDef
}

// findPropertyColon finds the colon that separates property name from type,
// handling cases like method signatures with colons in the type.
func findPropertyColon(propStr string) int {
	depth := 0
	for i := 0; i < len(propStr); i++ {
		switch propStr[i] {
		case '{', '(', '<', '[':
			depth++
		case '}', ')', '>', ']':
			depth--
		case ':':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// ParseProperties splits object properties at semicolons, respecting nesting.
func ParseProperties(inner string) []string {
	var props []string
	var current strings.Builder
	depth := 0

	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		switch ch {
		case '{', '(', '<', '[':
			depth++
			current.WriteByte(ch)
		case '}', ')', '>', ']':
			depth--
			current.WriteByte(ch)
		case ';':
			if depth == 0 {
				props = append(props, current.String())
				current.Reset()
			} else {
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}
	// Add last property if no trailing semicolon
	if current.Len() > 0 {
		props = append(props, current.String())
	}
	return props
}
