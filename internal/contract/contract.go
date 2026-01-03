// Package contract extracts TypeScript contract definitions from scripts.
// It analyzes TypeScript code to determine the shape of the default export
// and generates both TypeScript type definitions and JSON Schema.
package contract

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Contract represents the extracted contract from a TypeScript script.
type Contract struct {
	// Name is the identifier for the contract (derived from export or custom).
	Name string `json:"name"`
	// Type is the TypeScript type of the default export.
	Type *TypeDef `json:"type"`
	// Inputs are the expected global variables the script uses.
	Inputs []Property `json:"inputs,omitempty"`
	// Description is an optional description of the contract.
	Description string `json:"description,omitempty"`
}

// TypeDef represents a TypeScript type definition.
type TypeDef struct {
	// Kind is the type category: "primitive", "object", "array", "union", "literal", "any".
	Kind string `json:"kind"`
	// Name is the type name for primitives/references (e.g., "string", "number", "User").
	Name string `json:"name,omitempty"`
	// Properties are the fields for object types.
	Properties []Property `json:"properties,omitempty"`
	// ElementType is the type of array elements.
	ElementType *TypeDef `json:"elementType,omitempty"`
	// UnionTypes are the types in a union.
	UnionTypes []*TypeDef `json:"unionTypes,omitempty"`
	// LiteralValue is the value for literal types.
	LiteralValue any `json:"literalValue,omitempty"`
	// Optional indicates if this type is optional (for properties).
	Optional bool `json:"optional,omitempty"`
	// Nullable indicates if the type can be null.
	Nullable bool `json:"nullable,omitempty"`
}

// Property represents a property in an object type.
type Property struct {
	// Name is the property name.
	Name string `json:"name"`
	// Type is the property's type definition.
	Type *TypeDef `json:"type"`
	// Description is an optional property description.
	Description string `json:"description,omitempty"`
	// Required indicates if the property is required.
	Required bool `json:"required"`
}

// JSONSchema represents a JSON Schema definition.
type JSONSchema struct {
	Schema      string                 `json:"$schema,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	Properties  map[string]*JSONSchema `json:"properties,omitempty"`
	Items       *JSONSchema            `json:"items,omitempty"`
	Required    []string               `json:"required,omitempty"`
	AnyOf       []*JSONSchema          `json:"anyOf,omitempty"`
	Enum        []any                  `json:"enum,omitempty"`
	Const       any                    `json:"const,omitempty"`
	Ref         string                 `json:"$ref,omitempty"`
	Definitions map[string]*JSONSchema `json:"$defs,omitempty"`
}

// Analyzer extracts contract definitions from TypeScript code.
type Analyzer struct {
	interfaces  map[string]*TypeDef
	typeAliases map[string]*TypeDef
	globals     map[string]*TypeDef // External globals with known types
}

// NewAnalyzer creates a new contract analyzer.
func NewAnalyzer() *Analyzer {
	return &Analyzer{
		interfaces:  make(map[string]*TypeDef),
		typeAliases: make(map[string]*TypeDef),
		globals:     make(map[string]*TypeDef),
	}
}

// AddGlobal registers a global variable with its type definition.
// This is used for globals that are injected at runtime and aren't declared in the code.
func (a *Analyzer) AddGlobal(name string, typeDef *TypeDef) {
	a.globals[name] = typeDef
}

// AddGlobalFromTypeString registers a global variable by parsing a TypeScript type string.
func (a *Analyzer) AddGlobalFromTypeString(name string, typeStr string) {
	a.globals[name] = a.parseTypeExpression(typeStr)
}

// AddInterface registers an interface type definition.
func (a *Analyzer) AddInterface(name string, properties map[string]string) {
	var props []Property
	for propName, typeStr := range properties {
		props = append(props, Property{
			Name:     propName,
			Type:     a.parseTypeExpression(typeStr),
			Required: true,
		})
	}
	a.interfaces[name] = &TypeDef{
		Kind:       "object",
		Name:       name,
		Properties: props,
	}
}

// Analyze extracts the contract from TypeScript code.
func (a *Analyzer) Analyze(code string) (*Contract, error) {
	// Preserve pre-registered interfaces and globals, reset parsed state
	existingInterfaces := a.interfaces
	existingGlobals := a.globals

	a.interfaces = make(map[string]*TypeDef)
	a.typeAliases = make(map[string]*TypeDef)

	// Copy pre-registered interfaces
	for k, v := range existingInterfaces {
		a.interfaces[k] = v
	}
	// Restore globals
	a.globals = existingGlobals

	// Parse interfaces
	a.parseInterfaces(code)

	// Parse type aliases
	a.parseTypeAliases(code)

	// Find and parse the default export
	exportType := a.parseDefaultExport(code)
	if exportType == nil {
		exportType = &TypeDef{Kind: "any", Name: "any"}
	}

	// Extract inputs (global references)
	inputs := a.extractInputs(code)

	return &Contract{
		Name:   "Result",
		Type:   exportType,
		Inputs: inputs,
	}, nil
}

// parseInterfaces extracts interface definitions from code.
func (a *Analyzer) parseInterfaces(code string) {
	// Match: interface Name { ... }
	interfaceRe := regexp.MustCompile(`(?m)interface\s+(\w+)\s*\{([^}]*)\}`)
	matches := interfaceRe.FindAllStringSubmatch(code, -1)

	for _, match := range matches {
		name := match[1]
		body := match[2]
		props := a.parseObjectBody(body)

		a.interfaces[name] = &TypeDef{
			Kind:       "object",
			Name:       name,
			Properties: props,
		}
	}
}

// parseTypeAliases extracts type alias definitions from code.
func (a *Analyzer) parseTypeAliases(code string) {
	// Match: type Name = ...;
	typeRe := regexp.MustCompile(`(?m)type\s+(\w+)\s*=\s*([^;]+);`)
	matches := typeRe.FindAllStringSubmatch(code, -1)

	for _, match := range matches {
		name := match[1]
		typeStr := strings.TrimSpace(match[2])
		typeDef := a.parseTypeExpression(typeStr)
		typeDef.Name = name
		a.typeAliases[name] = typeDef
	}
}

// parseDefaultExport finds and parses the default export.
func (a *Analyzer) parseDefaultExport(code string) *TypeDef {
	// Pattern 1: export default expression;
	exportDefaultRe := regexp.MustCompile(`(?m)export\s+default\s+([^;]+);?`)
	if match := exportDefaultRe.FindStringSubmatch(code); match != nil {
		expr := strings.TrimSpace(match[1])
		return a.inferTypeFromExpression(expr, code)
	}

	// Pattern 2: const name: Type = ...; export default name;
	constExportRe := regexp.MustCompile(`(?m)const\s+(\w+)\s*:\s*([^=]+)\s*=`)
	if match := constExportRe.FindStringSubmatch(code); match != nil {
		constName := match[1]
		typeName := strings.TrimSpace(match[2])

		// Check if this const is exported as default
		exportNameRe := regexp.MustCompile(`export\s+default\s+` + constName + `\s*;?`)
		if exportNameRe.MatchString(code) {
			return a.parseTypeExpression(typeName)
		}
	}

	// Pattern 3: Infer from trailing expression (last statement without export)
	if trailingType := a.inferTrailingExpression(code); trailingType != nil {
		return trailingType
	}

	return nil
}

// inferTrailingExpression infers type from the last expression in code.
func (a *Analyzer) inferTrailingExpression(code string) *TypeDef {
	// Remove comments and find statements
	clean := a.removeComments(code)

	// Split into statements by semicolons and newlines
	statements := regexp.MustCompile(`[;\n]`).Split(clean, -1)

	// Find the last non-empty, non-declaration statement
	for i := len(statements) - 1; i >= 0; i-- {
		stmt := strings.TrimSpace(statements[i])
		if stmt == "" {
			continue
		}

		// Skip declarations, they're not expressions
		if strings.HasPrefix(stmt, "const ") ||
			strings.HasPrefix(stmt, "let ") ||
			strings.HasPrefix(stmt, "var ") ||
			strings.HasPrefix(stmt, "interface ") ||
			strings.HasPrefix(stmt, "type ") ||
			strings.HasPrefix(stmt, "function ") ||
			strings.HasPrefix(stmt, "declare ") ||
			strings.HasPrefix(stmt, "import ") ||
			strings.HasPrefix(stmt, "export ") {
			continue
		}

		// This looks like an expression - infer its type
		return a.inferTypeFromExpression(stmt, code)
	}

	return nil
}

// removeComments removes single-line and multi-line comments from code.
func (a *Analyzer) removeComments(code string) string {
	// Remove multi-line comments
	multiLineRe := regexp.MustCompile(`/\*[\s\S]*?\*/`)
	code = multiLineRe.ReplaceAllString(code, "")

	// Remove single-line comments
	singleLineRe := regexp.MustCompile(`//.*$`)
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		lines[i] = singleLineRe.ReplaceAllString(line, "")
	}

	return strings.Join(lines, "\n")
}

// parseObjectBody parses the body of an interface or object literal.
func (a *Analyzer) parseObjectBody(body string) []Property {
	var props []Property

	// Split by semicolons or newlines
	lines := regexp.MustCompile(`[;\n,]`).Split(body, -1)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Match: name?: type or name: type
		propRe := regexp.MustCompile(`^(\w+)(\?)?:\s*(.+)$`)
		if match := propRe.FindStringSubmatch(line); match != nil {
			propName := match[1]
			optional := match[2] == "?"
			typeStr := strings.TrimSpace(match[3])

			props = append(props, Property{
				Name:     propName,
				Type:     a.parseTypeExpression(typeStr),
				Required: !optional,
			})
		}
	}

	return props
}

// parseTypeExpression parses a TypeScript type expression string.
func (a *Analyzer) parseTypeExpression(typeStr string) *TypeDef {
	typeStr = strings.TrimSpace(typeStr)

	// Check for null/undefined union
	nullable := false
	if strings.Contains(typeStr, "| null") || strings.Contains(typeStr, "null |") {
		nullable = true
		typeStr = strings.ReplaceAll(typeStr, "| null", "")
		typeStr = strings.ReplaceAll(typeStr, "null |", "")
		typeStr = strings.TrimSpace(typeStr)
	}
	if strings.Contains(typeStr, "| undefined") || strings.Contains(typeStr, "undefined |") {
		typeStr = strings.ReplaceAll(typeStr, "| undefined", "")
		typeStr = strings.ReplaceAll(typeStr, "undefined |", "")
		typeStr = strings.TrimSpace(typeStr)
	}

	// Array type: Type[] or Array<Type>
	if strings.HasSuffix(typeStr, "[]") {
		elemType := strings.TrimSuffix(typeStr, "[]")
		return &TypeDef{
			Kind:        "array",
			ElementType: a.parseTypeExpression(elemType),
			Nullable:    nullable,
		}
	}
	if strings.HasPrefix(typeStr, "Array<") && strings.HasSuffix(typeStr, ">") {
		elemType := typeStr[6 : len(typeStr)-1]
		return &TypeDef{
			Kind:        "array",
			ElementType: a.parseTypeExpression(elemType),
			Nullable:    nullable,
		}
	}

	// Union type (not already handled as nullable)
	if strings.Contains(typeStr, "|") {
		parts := strings.Split(typeStr, "|")
		var unionTypes []*TypeDef
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				unionTypes = append(unionTypes, a.parseTypeExpression(part))
			}
		}
		return &TypeDef{
			Kind:       "union",
			UnionTypes: unionTypes,
			Nullable:   nullable,
		}
	}

	// Object literal type: { ... }
	if strings.HasPrefix(typeStr, "{") && strings.HasSuffix(typeStr, "}") {
		body := typeStr[1 : len(typeStr)-1]
		return &TypeDef{
			Kind:       "object",
			Properties: a.parseObjectBody(body),
			Nullable:   nullable,
		}
	}

	// String literal type: "value"
	if strings.HasPrefix(typeStr, `"`) && strings.HasSuffix(typeStr, `"`) {
		value := typeStr[1 : len(typeStr)-1]
		return &TypeDef{
			Kind:         "literal",
			Name:         "string",
			LiteralValue: value,
			Nullable:     nullable,
		}
	}

	// Number literal type
	if isNumeric(typeStr) {
		return &TypeDef{
			Kind:         "literal",
			Name:         "number",
			LiteralValue: typeStr,
			Nullable:     nullable,
		}
	}

	// Boolean literal
	if typeStr == "true" || typeStr == "false" {
		return &TypeDef{
			Kind:         "literal",
			Name:         "boolean",
			LiteralValue: typeStr == "true",
			Nullable:     nullable,
		}
	}

	// Primitive types
	switch typeStr {
	case "string", "number", "boolean", "any", "unknown", "void", "never":
		return &TypeDef{
			Kind:     "primitive",
			Name:     typeStr,
			Nullable: nullable,
		}
	}

	// Check if it's a known interface
	if iface, ok := a.interfaces[typeStr]; ok {
		return &TypeDef{
			Kind:       iface.Kind,
			Name:       iface.Name,
			Properties: iface.Properties,
			Nullable:   nullable,
		}
	}

	// Check if it's a known type alias
	if alias, ok := a.typeAliases[typeStr]; ok {
		result := *alias
		result.Nullable = result.Nullable || nullable
		return &result
	}

	// Unknown type - treat as reference
	return &TypeDef{
		Kind:     "object",
		Name:     typeStr,
		Nullable: nullable,
	}
}

// inferTypeFromExpression infers the type from an expression.
func (a *Analyzer) inferTypeFromExpression(expr string, code string) *TypeDef {
	expr = strings.TrimSpace(expr)

	// Comparison operators - always return boolean
	if a.isComparisonExpression(expr) {
		return &TypeDef{Kind: "primitive", Name: "boolean"}
	}

	// Logical operators (&&, ||) - return boolean
	if a.isLogicalExpression(expr) {
		return &TypeDef{Kind: "primitive", Name: "boolean"}
	}

	// Unary NOT operator
	if strings.HasPrefix(expr, "!") {
		return &TypeDef{Kind: "primitive", Name: "boolean"}
	}

	// Object literal: { ... }
	if strings.HasPrefix(expr, "{") {
		return a.inferObjectLiteralType(expr, code)
	}

	// Array literal: [ ... ]
	if strings.HasPrefix(expr, "[") {
		return a.inferArrayLiteralType(expr, code)
	}

	// String literal
	if strings.HasPrefix(expr, `"`) || strings.HasPrefix(expr, "'") || strings.HasPrefix(expr, "`") {
		return &TypeDef{Kind: "primitive", Name: "string"}
	}

	// Number literal
	if isNumeric(expr) {
		return &TypeDef{Kind: "primitive", Name: "number"}
	}

	// Boolean literal
	if expr == "true" || expr == "false" {
		return &TypeDef{Kind: "primitive", Name: "boolean"}
	}

	// Variable reference - look up its type
	varTypeRe := regexp.MustCompile(`(?m)(?:const|let|var)\s+` + regexp.QuoteMeta(expr) + `\s*:\s*([^=]+)\s*=`)
	if match := varTypeRe.FindStringSubmatch(code); match != nil {
		return a.parseTypeExpression(strings.TrimSpace(match[1]))
	}

	// Built-in constructor calls that return known types
	builtinTypes := map[string]string{
		"Symbol":  "symbol",
		"BigInt":  "bigint",
		"Number":  "number",
		"String":  "string",
		"Boolean": "boolean",
	}
	for builtin, typeName := range builtinTypes {
		if strings.HasPrefix(expr, builtin+"(") {
			return &TypeDef{Kind: "primitive", Name: typeName}
		}
	}

	// Function call - try to infer from return type
	funcCallRe := regexp.MustCompile(`^(\w+)\(`)
	if match := funcCallRe.FindStringSubmatch(expr); match != nil {
		funcName := match[1]
		funcDeclRe := regexp.MustCompile(`function\s+` + regexp.QuoteMeta(funcName) + `\s*\([^)]*\)\s*:\s*(\w+)`)
		if m := funcDeclRe.FindStringSubmatch(code); m != nil {
			return a.parseTypeExpression(m[1])
		}
	}

	// Member access expression (e.g., config.apiUrl)
	if strings.Contains(expr, ".") {
		if memberType := a.inferMemberAccessType(expr, code); memberType != nil {
			return memberType
		}
	}

	return &TypeDef{Kind: "any", Name: "any"}
}

// isComparisonExpression checks if the expression contains comparison operators.
func (a *Analyzer) isComparisonExpression(expr string) bool {
	// Check for comparison operators: ===, !==, ==, !=, <=, >=, <, >
	// Order matters - check longer operators first
	comparisonOps := []string{"===", "!==", "==", "!=", "<=", ">=", "<", ">"}
	for _, op := range comparisonOps {
		if strings.Contains(expr, op) {
			return true
		}
	}
	return false
}

// isLogicalExpression checks if the expression contains logical operators.
func (a *Analyzer) isLogicalExpression(expr string) bool {
	// Check for logical operators: &&, ||
	return strings.Contains(expr, "&&") || strings.Contains(expr, "||")
}

// inferMemberAccessType infers the type of a member access expression (e.g., config.apiUrl).
func (a *Analyzer) inferMemberAccessType(expr string, code string) *TypeDef {
	parts := strings.Split(expr, ".")
	if len(parts) < 2 {
		return nil
	}

	// Get the base object type
	baseName := parts[0]
	var baseType *TypeDef

	// Check pre-registered globals first
	if global, ok := a.globals[baseName]; ok {
		baseType = global
	}

	// Check declared globals
	if baseType == nil {
		declareRe := regexp.MustCompile(`(?m)declare\s+(?:const|var|let)\s+` + regexp.QuoteMeta(baseName) + `\s*:\s*`)
		if loc := declareRe.FindStringIndex(code); loc != nil {
			typeStart := loc[1]
			typeStr := extractTypeAnnotation(code[typeStart:])
			if typeStr != "" {
				baseType = a.parseTypeExpression(typeStr)
			}
		}
	}

	// Check local variable declarations
	if baseType == nil {
		varTypeRe := regexp.MustCompile(`(?m)(?:const|let|var)\s+` + regexp.QuoteMeta(baseName) + `\s*:\s*`)
		if loc := varTypeRe.FindStringIndex(code); loc != nil {
			typeStart := loc[1]
			typeStr := extractTypeUntilEquals(code[typeStart:])
			if typeStr != "" {
				baseType = a.parseTypeExpression(typeStr)
			}
		}
	}

	if baseType == nil {
		return nil
	}

	// Navigate through the property chain
	currentType := baseType
	for i := 1; i < len(parts); i++ {
		propName := parts[i]
		currentType = a.getPropertyType(currentType, propName)
		if currentType == nil {
			return nil
		}
	}

	return currentType
}

// getPropertyType looks up the type of a property in an object type.
func (a *Analyzer) getPropertyType(objType *TypeDef, propName string) *TypeDef {
	if objType == nil {
		return nil
	}

	// If the type is a reference to a named interface, look it up
	if objType.Kind == "object" && objType.Name != "" && len(objType.Properties) == 0 {
		if iface, ok := a.interfaces[objType.Name]; ok {
			objType = iface
		}
	}

	// Search properties
	for _, prop := range objType.Properties {
		if prop.Name == propName {
			return prop.Type
		}
	}

	return nil
}

// inferObjectLiteralType infers type from an object literal.
func (a *Analyzer) inferObjectLiteralType(expr string, code string) *TypeDef {
	var props []Property

	body := strings.TrimPrefix(expr, "{")
	body = strings.TrimSuffix(body, "}")
	body = strings.TrimSpace(body)

	if body == "" {
		return &TypeDef{
			Kind:       "object",
			Properties: props,
		}
	}

	// Parse properties - handle both "name: value" and shorthand "name" syntax
	propParts := splitObjectProperties(body)

	for _, part := range propParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check for "name: value" syntax
		if colonIdx := strings.Index(part, ":"); colonIdx > 0 {
			propName := strings.TrimSpace(part[:colonIdx])
			valueExpr := strings.TrimSpace(part[colonIdx+1:])

			props = append(props, Property{
				Name:     propName,
				Type:     a.inferTypeFromExpression(valueExpr, code),
				Required: true,
			})
		} else {
			// Shorthand property syntax: { currentUser } means { currentUser: currentUser }
			propName := strings.TrimSpace(part)
			if propName != "" && isIdentifier(propName) {
				props = append(props, Property{
					Name:     propName,
					Type:     a.inferShorthandPropertyType(propName, code),
					Required: true,
				})
			}
		}
	}

	return &TypeDef{
		Kind:       "object",
		Properties: props,
	}
}

// splitObjectProperties splits object literal body into individual property parts.
// Handles nested objects and arrays properly.
func splitObjectProperties(body string) []string {
	var parts []string
	var current strings.Builder
	depth := 0
	inString := false
	stringChar := byte(0)

	for i := 0; i < len(body); i++ {
		c := body[i]

		// Handle strings
		if !inString && (c == '"' || c == '\'' || c == '`') {
			inString = true
			stringChar = c
			current.WriteByte(c)
			continue
		}
		if inString {
			current.WriteByte(c)
			if c == stringChar && (i == 0 || body[i-1] != '\\') {
				inString = false
			}
			continue
		}

		// Track nesting depth
		if c == '{' || c == '[' || c == '(' {
			depth++
		} else if c == '}' || c == ']' || c == ')' {
			depth--
		}

		// Split on comma at top level only
		if c == ',' && depth == 0 {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}

		current.WriteByte(c)
	}

	// Don't forget the last part
	if s := current.String(); strings.TrimSpace(s) != "" {
		parts = append(parts, s)
	}

	return parts
}

// isIdentifier checks if a string is a valid JavaScript identifier.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if i == 0 {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '$') {
				return false
			}
		} else {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '$') {
				return false
			}
		}
	}
	return true
}

// inferShorthandPropertyType infers the type for a shorthand property.
// For { currentUser }, it looks up the type of currentUser from:
// 1. Pre-registered globals (via AddGlobal)
// 2. Declared globals (declare const currentUser: Type)
// 3. Local variables (const currentUser: Type = ...)
// 4. Known interfaces/type aliases
func (a *Analyzer) inferShorthandPropertyType(name string, code string) *TypeDef {
	// Check pre-registered globals first (highest priority)
	if global, ok := a.globals[name]; ok {
		return global
	}

	// Check declared globals - need to handle inline object types with semicolons
	declareRe := regexp.MustCompile(`(?m)declare\s+(?:const|var|let)\s+` + regexp.QuoteMeta(name) + `\s*:\s*`)
	if loc := declareRe.FindStringIndex(code); loc != nil {
		typeStart := loc[1]
		typeStr := extractTypeAnnotation(code[typeStart:])
		if typeStr != "" {
			return a.parseTypeExpression(typeStr)
		}
	}

	// Check local variable declarations
	varTypeRe := regexp.MustCompile(`(?m)(?:const|let|var)\s+` + regexp.QuoteMeta(name) + `\s*:\s*`)
	if loc := varTypeRe.FindStringIndex(code); loc != nil {
		typeStart := loc[1]
		// Extract until = sign, but handle nested braces
		typeStr := extractTypeUntilEquals(code[typeStart:])
		if typeStr != "" {
			return a.parseTypeExpression(typeStr)
		}
	}

	// Check if it matches a known interface name
	if iface, ok := a.interfaces[name]; ok {
		return iface
	}

	// Check if it matches a known type alias
	if alias, ok := a.typeAliases[name]; ok {
		return alias
	}

	// Unknown - return any
	return &TypeDef{Kind: "any", Name: "any"}
}

// extractTypeAnnotation extracts a type annotation from code, handling nested braces.
// It reads until a semicolon at depth 0 or end of declaration.
func extractTypeAnnotation(code string) string {
	var result strings.Builder
	depth := 0
	inString := false
	stringChar := byte(0)

	for i := 0; i < len(code); i++ {
		c := code[i]

		// Handle strings
		if !inString && (c == '"' || c == '\'' || c == '`') {
			inString = true
			stringChar = c
			result.WriteByte(c)
			continue
		}
		if inString {
			result.WriteByte(c)
			if c == stringChar && (i == 0 || code[i-1] != '\\') {
				inString = false
			}
			continue
		}

		// Track depth for braces, brackets, parens
		if c == '{' || c == '[' || c == '(' || c == '<' {
			depth++
		} else if c == '}' || c == ']' || c == ')' || c == '>' {
			depth--
		}

		// End at semicolon or newline at depth 0
		if depth == 0 && (c == ';' || c == '\n') {
			break
		}

		result.WriteByte(c)
	}

	return strings.TrimSpace(result.String())
}

// extractTypeUntilEquals extracts a type annotation until an equals sign, handling nested braces.
func extractTypeUntilEquals(code string) string {
	var result strings.Builder
	depth := 0
	inString := false
	stringChar := byte(0)

	for i := 0; i < len(code); i++ {
		c := code[i]

		// Handle strings
		if !inString && (c == '"' || c == '\'' || c == '`') {
			inString = true
			stringChar = c
			result.WriteByte(c)
			continue
		}
		if inString {
			result.WriteByte(c)
			if c == stringChar && (i == 0 || code[i-1] != '\\') {
				inString = false
			}
			continue
		}

		// Track depth for braces, brackets, parens
		if c == '{' || c == '[' || c == '(' || c == '<' {
			depth++
		} else if c == '}' || c == ']' || c == ')' || c == '>' {
			depth--
		}

		// End at equals sign at depth 0
		if depth == 0 && c == '=' {
			break
		}

		result.WriteByte(c)
	}

	return strings.TrimSpace(result.String())
}

// inferArrayLiteralType infers type from an array literal.
func (a *Analyzer) inferArrayLiteralType(expr string, code string) *TypeDef {
	body := strings.TrimPrefix(expr, "[")
	body = strings.TrimSuffix(body, "]")
	body = strings.TrimSpace(body)

	if body == "" {
		return &TypeDef{
			Kind:        "array",
			ElementType: &TypeDef{Kind: "any", Name: "any"},
		}
	}

	firstElem := strings.Split(body, ",")[0]
	firstElem = strings.TrimSpace(firstElem)

	return &TypeDef{
		Kind:        "array",
		ElementType: a.inferTypeFromExpression(firstElem, code),
	}
}

// extractInputs finds global variable references in the code.
func (a *Analyzer) extractInputs(code string) []Property {
	var inputs []Property
	seen := make(map[string]bool)

	// Look for declared globals
	declareRe := regexp.MustCompile(`(?m)declare\s+(?:const|var|let)\s+(\w+)\s*:\s*([^;]+);?`)
	matches := declareRe.FindAllStringSubmatch(code, -1)

	for _, match := range matches {
		name := match[1]
		typeStr := strings.TrimSpace(match[2])
		if !seen[name] {
			seen[name] = true
			inputs = append(inputs, Property{
				Name:     name,
				Type:     a.parseTypeExpression(typeStr),
				Required: true,
			})
		}
	}

	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i].Name < inputs[j].Name
	})

	return inputs
}

// ToTypeScript generates TypeScript type definitions for the contract.
func (c *Contract) ToTypeScript() string {
	var sb strings.Builder

	sb.WriteString("// Contract definition\n")
	sb.WriteString(fmt.Sprintf("export type %s = %s;\n", c.Name, typeDefToTSFormatted(c.Type, 0)))

	if len(c.Inputs) > 0 {
		sb.WriteString("\nexport interface Inputs {\n")
		for _, input := range c.Inputs {
			req := ""
			if !input.Required {
				req = "?"
			}
			sb.WriteString(fmt.Sprintf("  %s%s: %s;\n", input.Name, req, typeDefToTS(input.Type)))
		}
		sb.WriteString("}\n")
	}

	return sb.String()
}

// ToJSONSchema generates JSON Schema for the contract.
func (c *Contract) ToJSONSchema() *JSONSchema {
	return &JSONSchema{
		Schema:      "https://json-schema.org/draft/2020-12/schema",
		Title:       c.Name,
		Description: c.Description,
		Type:        "object",
		Properties: map[string]*JSONSchema{
			"result": typeDefToJSONSchema(c.Type),
		},
		Required: []string{"result"},
	}
}

// ToJSON returns the contract as JSON.
func (c *Contract) ToJSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "  ")
}

// ToJSONSchemaJSON returns the JSON Schema as JSON.
func (c *Contract) ToJSONSchemaJSON() ([]byte, error) {
	return json.MarshalIndent(c.ToJSONSchema(), "", "  ")
}

// typeDefToTS converts a TypeDef to TypeScript syntax.
func typeDefToTS(t *TypeDef) string {
	if t == nil {
		return "any"
	}

	var result string

	switch t.Kind {
	case "primitive":
		result = t.Name

	case "literal":
		if t.Name == "string" {
			result = fmt.Sprintf(`"%v"`, t.LiteralValue)
		} else {
			result = fmt.Sprintf("%v", t.LiteralValue)
		}

	case "object":
		if t.Name != "" && len(t.Properties) == 0 {
			result = t.Name
		} else {
			var props []string
			for _, p := range t.Properties {
				req := ""
				if !p.Required {
					req = "?"
				}
				props = append(props, fmt.Sprintf("%s%s: %s", p.Name, req, typeDefToTS(p.Type)))
			}
			result = "{ " + strings.Join(props, "; ") + " }"
		}

	case "array":
		elemType := typeDefToTS(t.ElementType)
		if strings.Contains(elemType, "|") || strings.Contains(elemType, " ") {
			result = fmt.Sprintf("(%s)[]", elemType)
		} else {
			result = elemType + "[]"
		}

	case "union":
		var parts []string
		for _, ut := range t.UnionTypes {
			parts = append(parts, typeDefToTS(ut))
		}
		result = strings.Join(parts, " | ")

	default:
		result = "any"
	}

	if t.Nullable {
		result = result + " | null"
	}

	return result
}

// typeDefToTSFormatted converts a TypeDef to formatted TypeScript syntax with indentation.
func typeDefToTSFormatted(t *TypeDef, indent int) string {
	if t == nil {
		return "any"
	}

	indentStr := strings.Repeat("  ", indent)
	nextIndent := strings.Repeat("  ", indent+1)

	var result string

	switch t.Kind {
	case "primitive":
		result = t.Name

	case "literal":
		if t.Name == "string" {
			result = fmt.Sprintf(`"%v"`, t.LiteralValue)
		} else {
			result = fmt.Sprintf("%v", t.LiteralValue)
		}

	case "object":
		if t.Name != "" && len(t.Properties) == 0 {
			result = t.Name
		} else if len(t.Properties) == 0 {
			result = "{}"
		} else {
			var sb strings.Builder
			sb.WriteString("{\n")
			for _, p := range t.Properties {
				req := ""
				if !p.Required {
					req = "?"
				}
				sb.WriteString(fmt.Sprintf("%s%s%s: %s;\n", nextIndent, p.Name, req, typeDefToTSFormatted(p.Type, indent+1)))
			}
			sb.WriteString(indentStr + "}")
			result = sb.String()
		}

	case "array":
		elemType := typeDefToTSFormatted(t.ElementType, indent)
		if strings.Contains(elemType, "|") && !strings.HasPrefix(elemType, "(") {
			result = fmt.Sprintf("(%s)[]", elemType)
		} else {
			result = elemType + "[]"
		}

	case "union":
		var parts []string
		for _, ut := range t.UnionTypes {
			parts = append(parts, typeDefToTSFormatted(ut, indent))
		}
		result = strings.Join(parts, " | ")

	default:
		result = "any"
	}

	if t.Nullable {
		result = result + " | null"
	}

	return result
}

// typeDefToJSONSchema converts a TypeDef to JSON Schema.
func typeDefToJSONSchema(t *TypeDef) *JSONSchema {
	if t == nil {
		return &JSONSchema{}
	}

	schema := &JSONSchema{}

	switch t.Kind {
	case "primitive":
		switch t.Name {
		case "string":
			schema.Type = "string"
		case "number":
			schema.Type = "number"
		case "boolean":
			schema.Type = "boolean"
		case "any", "unknown":
			// No type constraint
		default:
			schema.Type = "object"
		}

	case "literal":
		schema.Const = t.LiteralValue

	case "object":
		schema.Type = "object"
		if len(t.Properties) > 0 {
			schema.Properties = make(map[string]*JSONSchema)
			var required []string
			for _, p := range t.Properties {
				schema.Properties[p.Name] = typeDefToJSONSchema(p.Type)
				if p.Required {
					required = append(required, p.Name)
				}
			}
			if len(required) > 0 {
				schema.Required = required
			}
		}

	case "array":
		schema.Type = "array"
		schema.Items = typeDefToJSONSchema(t.ElementType)

	case "union":
		var anyOf []*JSONSchema
		for _, ut := range t.UnionTypes {
			anyOf = append(anyOf, typeDefToJSONSchema(ut))
		}
		schema.AnyOf = anyOf
	}

	if t.Nullable {
		if schema.AnyOf == nil {
			original := *schema
			schema = &JSONSchema{
				AnyOf: []*JSONSchema{&original, {Type: "null"}},
			}
		} else {
			schema.AnyOf = append(schema.AnyOf, &JSONSchema{Type: "null"})
		}
	}

	return schema
}

// isNumeric checks if a string represents a number.
func isNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if s[0] == '-' {
		s = s[1:]
	}
	hasDecimal := false
	for _, c := range s {
		if c == '.' {
			if hasDecimal {
				return false
			}
			hasDecimal = true
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
