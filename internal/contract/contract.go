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

// Pre-compiled regexes for performance (compiled once at package init)
var (
	typeAliasRe     = regexp.MustCompile(`(?m)type\s+(\w+)\s*=\s*([^;]+);`)
	exportDefaultRe = regexp.MustCompile(`(?m)export\s+default\s+([^;]+);?`)
	constExportRe   = regexp.MustCompile(`(?m)const\s+(\w+)\s*:\s*([^=]+)\s*=`)
	multiLineRe     = regexp.MustCompile(`/\*[\s\S]*?\*/`)
	singleLineRe    = regexp.MustCompile(`//.*$`)
	propRe          = regexp.MustCompile(`^(\w+)(\?)?:\s*([\s\S]+)$`)
	funcCallRe      = regexp.MustCompile(`^(\w+)\(`)
	arrowFuncRe     = regexp.MustCompile(`^\([^)]*\)\s*:\s*([^=]+?)\s*=>`)
	funcDeclRe      = regexp.MustCompile(`^function\s*\w*\s*\([^)]*\)\s*:\s*([^{]+?)\s*\{`)
	promiseTypeRe   = regexp.MustCompile(`^Promise<(.+)>$`)
	declareVarRe    = regexp.MustCompile(`(?m)declare\s+(?:const|var|let)\s+(\w+)\s*:\s*([^;]+);?`)
	splitStmtRe     = regexp.MustCompile(`[;\n]`)
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

	// Parse type aliases first (so they're available when parsing interfaces)
	a.parseTypeAliases(code)

	// Parse interfaces
	a.parseInterfaces(code)

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

// extractInterfaceBody finds the body of an interface starting after the opening brace.
// Returns the body content and the position after the closing brace.
func extractInterfaceBody(code string, start int) (string, int) {
	depth := 1
	i := start
	for i < len(code) && depth > 0 {
		ch := code[i]
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
		case '"', '\'', '`':
			// Skip strings
			quote := ch
			i++
			for i < len(code) && code[i] != quote {
				if code[i] == '\\' && i+1 < len(code) {
					i++
				}
				i++
			}
		}
		i++
	}
	if depth != 0 {
		return "", start
	}
	return code[start : i-1], i
}

// parseInterfaces extracts interface definitions from code.
func (a *Analyzer) parseInterfaces(code string) {
	// Find all interface declarations manually to handle nested braces
	interfaceStartRe := regexp.MustCompile(`(?m)interface\s+(\w+)(?:\s+extends\s+(\w+))?\s*\{`)

	// First pass: collect all interfaces without resolving extends
	pendingExtends := make(map[string]string) // interface name -> base interface name

	for _, loc := range interfaceStartRe.FindAllStringSubmatchIndex(code, -1) {
		// Extract name and extends from the match
		fullMatch := code[loc[0]:loc[1]]
		nameMatch := interfaceStartRe.FindStringSubmatch(fullMatch)
		if nameMatch == nil {
			continue
		}
		name := nameMatch[1]
		extendsName := nameMatch[2]

		// Find the body by matching braces
		bodyStart := loc[1] // Position right after the opening {
		body, _ := extractInterfaceBody(code, bodyStart)

		props := a.parseObjectBody(body)

		a.interfaces[name] = &TypeDef{
			Kind:       "object",
			Name:       name,
			Properties: props,
		}

		if extendsName != "" {
			pendingExtends[name] = extendsName
		}
	}

	// Second pass: resolve extends by copying base properties
	for name, baseName := range pendingExtends {
		if baseIface, ok := a.interfaces[baseName]; ok {
			iface := a.interfaces[name]
			// Prepend base properties (so derived properties can override)
			combined := make([]Property, 0, len(baseIface.Properties)+len(iface.Properties))
			combined = append(combined, baseIface.Properties...)
			combined = append(combined, iface.Properties...)
			iface.Properties = combined
		}
	}
}

// parseTypeAliases extracts type alias definitions from code.
func (a *Analyzer) parseTypeAliases(code string) {
	// Match: type Name = ...;
	matches := typeAliasRe.FindAllStringSubmatch(code, -1)

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
	if match := exportDefaultRe.FindStringSubmatch(code); match != nil {
		expr := strings.TrimSpace(match[1])
		return a.inferTypeFromExpression(expr, code)
	}

	// Pattern 2: const name: Type = ...; export default name;
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
	statements := splitStmtRe.Split(clean, -1)

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
	code = multiLineRe.ReplaceAllString(code, "")

	// Remove single-line comments
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		lines[i] = singleLineRe.ReplaceAllString(line, "")
	}

	return strings.Join(lines, "\n")
}

// splitTopLevelProperties splits interface/object body into individual property definitions.
// Only splits at semicolons or newlines that are at the top level (not inside nested braces).
func splitTopLevelProperties(body string) []string {
	var result []string
	var current strings.Builder
	braceDepth := 0
	angleDepth := 0
	parenDepth := 0
	bracketDepth := 0

	for i := 0; i < len(body); i++ {
		ch := body[i]

		switch ch {
		case '{':
			braceDepth++
			current.WriteByte(ch)
		case '}':
			braceDepth--
			current.WriteByte(ch)
		case '(':
			parenDepth++
			current.WriteByte(ch)
		case ')':
			parenDepth--
			current.WriteByte(ch)
		case '[':
			bracketDepth++
			current.WriteByte(ch)
		case ']':
			bracketDepth--
			current.WriteByte(ch)
		case '<':
			angleDepth++
			current.WriteByte(ch)
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
			current.WriteByte(ch)
		case ';', '\n', ',':
			totalDepth := braceDepth + angleDepth + parenDepth + bracketDepth
			if totalDepth == 0 {
				// Top level delimiter - end current property
				if current.Len() > 0 {
					result = append(result, current.String())
					current.Reset()
				}
			} else {
				// Inside nested structure - keep as part of type
				current.WriteByte(ch)
			}
		case '"', '\'', '`':
			// Handle strings - find matching quote
			quote := ch
			current.WriteByte(ch)
			i++
			for i < len(body) && body[i] != quote {
				if body[i] == '\\' && i+1 < len(body) {
					current.WriteByte(body[i])
					i++
				}
				current.WriteByte(body[i])
				i++
			}
			if i < len(body) {
				current.WriteByte(body[i])
			}
		default:
			current.WriteByte(ch)
		}
	}

	// Add any remaining content
	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// parseObjectBody parses the body of an interface or object literal.
func (a *Analyzer) parseObjectBody(body string) []Property {
	var props []Property

	// Split by semicolons or newlines at top level only
	lines := splitTopLevelProperties(body)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Match: name?: type or name: type
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

	// Partial<T> utility type - makes all properties optional
	if strings.HasPrefix(typeStr, "Partial<") && strings.HasSuffix(typeStr, ">") {
		innerType := typeStr[8 : len(typeStr)-1]
		baseType := a.parseTypeExpression(innerType)
		// Make all properties optional (Required=false)
		for _, prop := range baseType.Properties {
			prop.Required = false
		}
		return &TypeDef{
			Kind:       baseType.Kind,
			Name:       baseType.Name,
			Properties: baseType.Properties,
			Nullable:   nullable,
		}
	}

	// Required<T> utility type - makes all properties required
	if strings.HasPrefix(typeStr, "Required<") && strings.HasSuffix(typeStr, ">") {
		innerType := typeStr[9 : len(typeStr)-1]
		baseType := a.parseTypeExpression(innerType)
		// Make all properties required
		for _, prop := range baseType.Properties {
			prop.Required = true
		}
		return baseType
	}

	// Readonly<T> utility type - returns same type (we don't track readonly)
	if strings.HasPrefix(typeStr, "Readonly<") && strings.HasSuffix(typeStr, ">") {
		innerType := typeStr[9 : len(typeStr)-1]
		return a.parseTypeExpression(innerType)
	}

	// Pick<T, K> and Omit<T, K> - for now, return the base type (partial support)
	if strings.HasPrefix(typeStr, "Pick<") || strings.HasPrefix(typeStr, "Omit<") {
		// Extract just the base type for now
		if idx := strings.Index(typeStr, ","); idx > 0 {
			start := strings.Index(typeStr, "<")
			if start > 0 {
				baseType := strings.TrimSpace(typeStr[start+1 : idx])
				return a.parseTypeExpression(baseType)
			}
		}
	}

	// Record<K, V> utility type
	if strings.HasPrefix(typeStr, "Record<") && strings.HasSuffix(typeStr, ">") {
		inner := typeStr[7 : len(typeStr)-1]
		parts := strings.SplitN(inner, ",", 2)
		if len(parts) == 2 {
			valueType := strings.TrimSpace(parts[1])
			// Return an object with index signature semantics
			return &TypeDef{
				Kind:     "object",
				Name:     "",
				Nullable: nullable,
				// Note: We could add index signature support later
				Properties: []Property{{
					Name:     "[key]",
					Type:     a.parseTypeExpression(valueType),
					Required: false,
				}},
			}
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

	// Handle await expression - strip await and infer from the inner expression
	// The awaited result is the unwrapped Promise type
	if strings.HasPrefix(expr, "await ") {
		innerExpr := strings.TrimPrefix(expr, "await ")
		innerType := a.inferTypeFromExpression(innerExpr, code)
		// Unwrap Promise<T> to T since await resolves the promise
		if innerType != nil && strings.HasPrefix(innerType.Name, "Promise<") {
			if match := promiseTypeRe.FindStringSubmatch(innerType.Name); match != nil {
				return a.parseTypeExpression(match[1])
			}
		}
		return innerType
	}

	// Arrow function or async function - extract return type
	// Pattern: async (): Promise<Type> => ... or (): Type => ...
	if returnType := a.inferFunctionReturnType(expr); returnType != nil {
		return returnType
	}

	// Ternary expression: condition ? trueValue : falseValue
	if ternaryType := a.inferTernaryExpressionType(expr, code); ternaryType != nil {
		return ternaryType
	}

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

	// Variable reference - look up its type (with type annotation)
	varTypeRe := regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(expr) + `\s*:\s*([^=]+)\s*=`)
	if match := varTypeRe.FindStringSubmatch(code); match != nil {
		return a.parseTypeExpression(strings.TrimSpace(match[1]))
	}

	// Variable reference - infer from initializer (no type annotation)
	varInitRe := regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(expr) + `\s*=\s*`)
	if loc := varInitRe.FindStringIndex(code); loc != nil {
		initStart := loc[1]
		initExpr := extractInitializer(code[initStart:])
		if initExpr != "" && initExpr != expr { // Avoid infinite recursion
			return a.inferTypeFromExpression(initExpr, code)
		}
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

	// String concatenation - expression contains string literal and +
	if a.isStringConcatenation(expr) {
		return &TypeDef{Kind: "primitive", Name: "string"}
	}

	// Arithmetic expressions (containing +, -, *, /, % between values) return number
	if a.isArithmeticExpression(expr) {
		return &TypeDef{Kind: "primitive", Name: "number"}
	}

	// Built-in method calls and property access patterns
	if builtinType := a.inferBuiltinMethodType(expr, code); builtinType != nil {
		return builtinType
	}

	// Function call - try to infer from return type
	if match := funcCallRe.FindStringSubmatch(expr); match != nil {
		funcName := match[1]
		// Try to find function with return type annotation
		// Pattern 1: function name(...): Type[] or function name(...): Type
		// Handle both sync and async functions
		funcDeclPattern := regexp.MustCompile(`(?:export\s+)?(?:async\s+)?function\s+` + regexp.QuoteMeta(funcName) + `\s*\([^)]*\)\s*:\s*`)
		if loc := funcDeclPattern.FindStringIndex(code); loc != nil {
			returnTypeStart := loc[1]
			returnType := extractReturnType(code[returnTypeStart:])
			if returnType != "" {
				// For async functions, the return type is Promise<T>
				// The caller may or may not await it, so we return as-is
				// The await handling will unwrap if needed
				return a.parseTypeExpression(returnType)
			}
		}

		// Pattern 2: function without explicit return type - infer from return statement
		funcBodyPattern := regexp.MustCompile(`(?:export\s+)?(?:async\s+)?function\s+` + regexp.QuoteMeta(funcName) + `\s*\([^)]*\)\s*\{`)
		if loc := funcBodyPattern.FindStringIndex(code); loc != nil {
			// Find the function body
			bodyStart := loc[1] - 1 // Position of the opening brace
			if bodyEnd := findMatchingBrace(code, bodyStart); bodyEnd > bodyStart {
				funcBody := code[bodyStart+1 : bodyEnd]
				// Find the return statement in the function body
				if returnType := a.inferReturnTypeFromFunctionBody(funcBody, code); returnType != nil {
					return returnType
				}
			}
		}
	}

	// Member access expression (e.g., config.apiUrl)
	if strings.Contains(expr, ".") {
		if memberType := a.inferMemberAccessType(expr, code); memberType != nil {
			return memberType
		}
	}

	// Math.* methods that return number
	if strings.HasPrefix(expr, "Math.") {
		return &TypeDef{Kind: "primitive", Name: "number"}
	}

	return &TypeDef{Kind: "any", Name: "any"}
}

// inferFunctionReturnType extracts the return type from a function expression.
// Handles: async (): Promise<Type> => ..., (): Type => ..., function(): Type { ... }
// For async functions, unwraps Promise<T> to return T (since runtime resolves promises).
func (a *Analyzer) inferFunctionReturnType(expr string) *TypeDef {
	expr = strings.TrimSpace(expr)

	// Check if it's a function expression (arrow or regular)
	isAsync := strings.HasPrefix(expr, "async ")
	if isAsync {
		expr = strings.TrimPrefix(expr, "async ")
		expr = strings.TrimSpace(expr)
	}

	// Arrow function: (...): ReturnType => ...
	// or function expression: function(...): ReturnType { ... }
	var returnTypeStr string

	// Pattern 1: Arrow function with return type - (...): Type =>
	if match := arrowFuncRe.FindStringSubmatch(expr); match != nil {
		returnTypeStr = strings.TrimSpace(match[1])
	}

	// Pattern 2: function(...): Type { ... }
	if returnTypeStr == "" {
		if match := funcDeclRe.FindStringSubmatch(expr); match != nil {
			returnTypeStr = strings.TrimSpace(match[1])
		}
	}

	if returnTypeStr == "" {
		return nil
	}

	// For async functions, unwrap Promise<T> to get the actual return type
	if isAsync || strings.HasPrefix(returnTypeStr, "Promise<") {
		if match := promiseTypeRe.FindStringSubmatch(returnTypeStr); match != nil {
			returnTypeStr = match[1]
		}
	}

	return a.parseTypeExpression(returnTypeStr)
}

// inferTernaryExpressionType infers the type from a ternary expression (cond ? a : b).
// Returns nil if not a ternary expression.
func (a *Analyzer) inferTernaryExpressionType(expr string, code string) *TypeDef {
	// Find the ? and : at the top level (not inside nested expressions)
	depth := 0
	questionIdx := -1
	colonIdx := -1

	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		switch ch {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '?':
			if depth == 0 && questionIdx == -1 {
				// Make sure it's not optional chaining (?.)
				if i+1 < len(expr) && expr[i+1] != '.' {
					questionIdx = i
				}
			}
		case ':':
			if depth == 0 && questionIdx >= 0 && colonIdx == -1 {
				colonIdx = i
			}
		case '"', '\'', '`':
			// Skip strings
			quote := ch
			i++
			for i < len(expr) && expr[i] != quote {
				if expr[i] == '\\' && i+1 < len(expr) {
					i++ // Skip escaped char
				}
				i++
			}
		}
	}

	if questionIdx < 0 || colonIdx < 0 {
		return nil
	}

	// Extract the true and false branches
	trueBranch := strings.TrimSpace(expr[questionIdx+1 : colonIdx])
	falseBranch := strings.TrimSpace(expr[colonIdx+1:])

	// Infer types from both branches
	trueType := a.inferTypeFromExpression(trueBranch, code)
	falseType := a.inferTypeFromExpression(falseBranch, code)

	// If both branches have the same primitive type, return that
	if trueType.Kind == "primitive" && falseType.Kind == "primitive" && trueType.Name == falseType.Name {
		return trueType
	}

	// If both are string literals, return string
	if (trueType.Kind == "primitive" && trueType.Name == "string") ||
		(falseType.Kind == "primitive" && falseType.Name == "string") {
		// If one is string, check if the other is also string-like
		if trueType.Name == "string" || falseType.Name == "string" {
			return &TypeDef{Kind: "primitive", Name: "string"}
		}
	}

	// Default: return the true branch type (common case is same types)
	return trueType
}

// isComparisonExpression checks if the expression contains comparison operators at the top level.
func (a *Analyzer) isComparisonExpression(expr string) bool {
	// Skip strings
	if strings.HasPrefix(expr, `"`) || strings.HasPrefix(expr, "'") || strings.HasPrefix(expr, "`") {
		return false
	}

	// Skip object literals - they may contain callbacks with arrows
	if strings.HasPrefix(expr, "{") {
		return false
	}

	// Look for comparison operators at the top level (outside parentheses, brackets, braces)
	depth := 0
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '=':
			if depth == 0 {
				// Check for ===, ==, but NOT => (arrow)
				if i+2 < len(expr) && expr[i+1] == '=' && expr[i+2] == '=' {
					return true // ===
				}
				if i+1 < len(expr) && expr[i+1] == '=' && (i+2 >= len(expr) || expr[i+2] != '=') {
					return true // ==
				}
				// Skip =>, it's arrow function
			}
		case '!':
			if depth == 0 {
				if i+2 < len(expr) && expr[i+1] == '=' && expr[i+2] == '=' {
					return true // !==
				}
				if i+1 < len(expr) && expr[i+1] == '=' && (i+2 >= len(expr) || expr[i+2] != '=') {
					return true // !=
				}
			}
		case '<':
			if depth == 0 {
				// Check it's not part of generic type like Array<T>
				if i+1 < len(expr) && expr[i+1] == '=' {
					return true // <=
				}
				// Simple < comparison (but be careful with generics)
				// Look for < followed by something that's not a type identifier pattern
				if i+1 < len(expr) {
					next := expr[i+1]
					if next == ' ' || (next >= '0' && next <= '9') || next == '-' || next == '(' {
						return true
					}
				}
			}
		case '>':
			if depth == 0 {
				// Check it's not part of => arrow or generic closing >
				if i > 0 && expr[i-1] == '=' {
					continue // This is =>, skip
				}
				if i+1 < len(expr) && expr[i+1] == '=' {
					return true // >=
				}
				// Simple > comparison (but be careful with generics)
				if i+1 < len(expr) {
					next := expr[i+1]
					if next == ' ' || (next >= '0' && next <= '9') || next == ')' || next == ';' {
						return true
					}
				}
			}
		}
	}
	return false
}

// isLogicalExpression checks if the expression contains logical operators.
func (a *Analyzer) isLogicalExpression(expr string) bool {
	// Check for logical operators: &&, ||
	return strings.Contains(expr, "&&") || strings.Contains(expr, "||")
}

// isArithmeticExpression checks if the expression is an arithmetic operation.
// Returns true for expressions like: x + y, x * 100, Math.round(x) / 100
// Returns false if it's string concatenation (involving string literals)
func (a *Analyzer) isArithmeticExpression(expr string) bool {
	// Skip if it's a string with operators inside
	if strings.HasPrefix(expr, `"`) || strings.HasPrefix(expr, "'") || strings.HasPrefix(expr, "`") {
		return false
	}

	// Check if expression contains string literals - if so, + is concatenation
	if strings.Contains(expr, `"`) || strings.Contains(expr, "'") || strings.Contains(expr, "`") {
		return false // String concatenation, not arithmetic
	}

	// Look for arithmetic operators at the top level (outside parentheses)
	depth := 0
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '+', '*', '/', '%':
			if depth == 0 {
				return true
			}
		case '-':
			// Minus could be unary or binary, check if preceded by an operand
			if depth == 0 && i > 0 {
				prev := expr[i-1]
				// If preceded by a closing paren, number, or identifier char, it's binary minus
				if prev == ')' || prev == ']' || (prev >= '0' && prev <= '9') || (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') || prev == '_' {
					return true
				}
			}
		}
	}
	return false
}

// isStringConcatenation checks if expression involves string concatenation.
// Returns true for expressions like: "Hello " + name, value + "%", etc.
func (a *Analyzer) isStringConcatenation(expr string) bool {
	// Must contain a string literal
	hasString := strings.Contains(expr, `"`) || strings.Contains(expr, "'") || strings.Contains(expr, "`")
	if !hasString {
		return false
	}

	// Must contain + operator at top level
	depth := 0
	inString := false
	stringChar := byte(0)
	for i := 0; i < len(expr); i++ {
		c := expr[i]

		// Track strings
		if !inString && (c == '"' || c == '\'' || c == '`') {
			inString = true
			stringChar = c
			continue
		}
		if inString {
			if c == stringChar && (i == 0 || expr[i-1] != '\\') {
				inString = false
			}
			continue
		}

		// Track nesting
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '+':
			if depth == 0 {
				return true
			}
		}
	}
	return false
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

	// Check local variable declarations (with optional export keyword)
	if baseType == nil {
		varTypeRe := regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(baseName) + `\s*:\s*`)
		if loc := varTypeRe.FindStringIndex(code); loc != nil {
			typeStart := loc[1]
			typeStr := extractTypeUntilEquals(code[typeStart:])
			if typeStr != "" {
				baseType = a.parseTypeExpression(typeStr)
			}
		}
	}

	// Variable without type annotation - infer from initializer
	if baseType == nil {
		varInitRe := regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(baseName) + `\s*=\s*`)
		if loc := varInitRe.FindStringIndex(code); loc != nil {
			initStart := loc[1]
			initExpr := extractInitializer(code[initStart:])
			if initExpr != "" {
				baseType = a.inferTypeFromExpression(initExpr, code)
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

// inferBuiltinMethodType infers type from common JavaScript built-in methods and properties.
func (a *Analyzer) inferBuiltinMethodType(expr string, code string) *TypeDef {
	// .length property - always returns number
	if strings.HasSuffix(expr, ".length") {
		return &TypeDef{Kind: "primitive", Name: "number"}
	}

	// Number methods that return string
	stringMethods := []string{".toFixed(", ".toExponential(", ".toPrecision(", ".toString(", ".toLocaleString("}
	for _, method := range stringMethods {
		if strings.Contains(expr, method) {
			return &TypeDef{Kind: "primitive", Name: "string"}
		}
	}

	// String methods that return string
	strStringMethods := []string{".toLowerCase(", ".toUpperCase(", ".trim(", ".trimStart(", ".trimEnd(",
		".slice(", ".substring(", ".substr(", ".replace(", ".replaceAll(",
		".padStart(", ".padEnd(", ".repeat(", ".charAt(", ".normalize(",
		".concat("}
	for _, method := range strStringMethods {
		if strings.Contains(expr, method) {
			return &TypeDef{Kind: "primitive", Name: "string"}
		}
	}

	// String methods that return number
	strNumberMethods := []string{".indexOf(", ".lastIndexOf(", ".search(", ".charCodeAt(", ".codePointAt(", ".localeCompare("}
	for _, method := range strNumberMethods {
		if strings.Contains(expr, method) {
			return &TypeDef{Kind: "primitive", Name: "number"}
		}
	}

	// String/Array methods that return boolean
	boolMethods := []string{".includes(", ".startsWith(", ".endsWith(", ".every(", ".some("}
	for _, method := range boolMethods {
		if strings.Contains(expr, method) {
			return &TypeDef{Kind: "primitive", Name: "boolean"}
		}
	}

	// Array methods that return arrays
	if strings.Contains(expr, ".map(") || strings.Contains(expr, ".filter(") ||
		strings.Contains(expr, ".slice(") || strings.Contains(expr, ".concat(") ||
		strings.Contains(expr, ".flat(") || strings.Contains(expr, ".flatMap(") ||
		strings.Contains(expr, ".sort(") || strings.Contains(expr, ".reverse(") ||
		strings.Contains(expr, ".splice(") {
		// Try to infer element type from .map() callback
		if strings.Contains(expr, ".map(") {
			if elemType := a.inferMapCallbackReturnType(expr, code); elemType != nil {
				return &TypeDef{
					Kind:        "array",
					ElementType: elemType,
				}
			}
		}
		// Default to any[] for other array methods
		return &TypeDef{
			Kind:        "array",
			ElementType: &TypeDef{Kind: "any", Name: "any"},
		}
	}

	// Array methods that return single element (any)
	if strings.Contains(expr, ".find(") || strings.Contains(expr, ".pop(") ||
		strings.Contains(expr, ".shift(") {
		return &TypeDef{Kind: "any", Name: "any"}
	}

	// Array methods that return number
	if strings.Contains(expr, ".push(") || strings.Contains(expr, ".unshift(") ||
		strings.Contains(expr, ".findIndex(") {
		return &TypeDef{Kind: "primitive", Name: "number"}
	}

	// Array.reduce - check the initial value to determine return type
	if strings.Contains(expr, ".reduce(") || strings.Contains(expr, ".reduceRight(") {
		// Look for the initial value (second argument)
		// Pattern: .reduce((acc, x) => acc + x.value, 0) - the 0 is the initial value
		reduceIdx := strings.Index(expr, ".reduce(")
		if reduceIdx < 0 {
			reduceIdx = strings.Index(expr, ".reduceRight(")
		}
		if reduceIdx >= 0 {
			// Find the callback and initial value
			callbackStart := strings.Index(expr[reduceIdx:], "(")
			if callbackStart >= 0 {
				callbackStart += reduceIdx + 1
				// Find the comma that separates callback from initial value (at depth 0)
				depth := 0
				for i := callbackStart; i < len(expr)-1; i++ {
					switch expr[i] {
					case '(', '{', '[':
						depth++
					case ')', '}', ']':
						depth--
					case ',':
						if depth == 0 {
							// Found the separator, check initial value
							initValue := strings.TrimSpace(expr[i+1 : len(expr)-1])
							if initValue == "0" || isNumeric(initValue) {
								return &TypeDef{Kind: "primitive", Name: "number"}
							}
							if initValue == `""` || initValue == "''" || initValue == "``" {
								return &TypeDef{Kind: "primitive", Name: "string"}
							}
							if initValue == "true" || initValue == "false" {
								return &TypeDef{Kind: "primitive", Name: "boolean"}
							}
						}
					}
				}
			}
		}
		return &TypeDef{Kind: "any", Name: "any"}
	}

	// Object static methods
	if strings.HasPrefix(expr, "Object.keys(") {
		return &TypeDef{
			Kind:        "array",
			ElementType: &TypeDef{Kind: "primitive", Name: "string"},
		}
	}
	if strings.HasPrefix(expr, "Object.values(") {
		return &TypeDef{
			Kind:        "array",
			ElementType: &TypeDef{Kind: "any", Name: "any"},
		}
	}
	if strings.HasPrefix(expr, "Object.entries(") {
		return &TypeDef{
			Kind:        "array",
			ElementType: &TypeDef{Kind: "any", Name: "any"},
		}
	}

	// JSON methods
	if strings.HasPrefix(expr, "JSON.stringify(") {
		return &TypeDef{Kind: "primitive", Name: "string"}
	}
	if strings.HasPrefix(expr, "JSON.parse(") {
		return &TypeDef{Kind: "any", Name: "any"}
	}

	// Date methods
	if strings.Contains(expr, ".getTime(") || strings.Contains(expr, ".getFullYear(") ||
		strings.Contains(expr, ".getMonth(") || strings.Contains(expr, ".getDate(") ||
		strings.Contains(expr, ".getDay(") || strings.Contains(expr, ".getHours(") ||
		strings.Contains(expr, ".getMinutes(") || strings.Contains(expr, ".getSeconds(") ||
		strings.Contains(expr, ".getMilliseconds(") || strings.Contains(expr, ".valueOf(") {
		return &TypeDef{Kind: "primitive", Name: "number"}
	}
	if strings.Contains(expr, ".toISOString(") || strings.Contains(expr, ".toDateString(") ||
		strings.Contains(expr, ".toTimeString(") || strings.Contains(expr, ".toJSON(") {
		return &TypeDef{Kind: "primitive", Name: "string"}
	}

	// String.split returns string[]
	if strings.Contains(expr, ".split(") {
		return &TypeDef{
			Kind:        "array",
			ElementType: &TypeDef{Kind: "primitive", Name: "string"},
		}
	}

	// Array.join returns string
	if strings.Contains(expr, ".join(") {
		return &TypeDef{Kind: "primitive", Name: "string"}
	}

	return nil
}

// inferMapCallbackReturnType tries to infer the return type of a .map() callback.
func (a *Analyzer) inferMapCallbackReturnType(expr string, code string) *TypeDef {
	// Look for patterns like: array.map(x => x.property) or array.map(x => ({ ... }))
	mapIdx := strings.Index(expr, ".map(")
	if mapIdx < 0 {
		return nil
	}

	// Get the source array name
	sourceArray := strings.TrimSpace(expr[:mapIdx])

	// Try to get the element type of the source array
	var elementType *TypeDef

	// Method 0: Check for Object.values(varName) pattern
	if strings.HasPrefix(sourceArray, "Object.values(") {
		varName := strings.TrimPrefix(sourceArray, "Object.values(")
		varName = strings.TrimSuffix(varName, ")")
		varName = strings.TrimSpace(varName)

		// Look for the variable type: const varName: Record<string, Type> = ...
		recordTypeRe := regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(varName) + `\s*:\s*Record<[^,]+,\s*(\w+)>`)
		if match := recordTypeRe.FindStringSubmatch(code); match != nil {
			valueTypeName := strings.TrimSpace(match[1])
			if iface, ok := a.interfaces[valueTypeName]; ok {
				elementType = iface
			} else {
				elementType = &TypeDef{Kind: "object", Name: valueTypeName}
			}
		}
	}

	// Method 1: Look for array declaration with explicit type: const arr: Type[] = ...
	if elementType == nil {
		arrTypeRe := regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(sourceArray) + `\s*:\s*([^=\[]+)\[\]`)
		if match := arrTypeRe.FindStringSubmatch(code); match != nil {
			elemTypeName := strings.TrimSpace(match[1])
			// Skip if it looks like an inline object type (starts with {)
			if !strings.HasPrefix(elemTypeName, "{") {
				if iface, ok := a.interfaces[elemTypeName]; ok {
					elementType = iface
				} else {
					elementType = &TypeDef{Kind: "object", Name: elemTypeName}
				}
			}
		}
	}

	// Method 1b: Look for inline object type annotation: const arr: { prop: Type; ... }[] = ...
	if elementType == nil {
		// Find the variable declaration with type starting with {
		inlineTypeRe := regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(sourceArray) + `\s*:\s*\{`)
		if match := inlineTypeRe.FindStringIndex(code); match != nil {
			// Find the opening brace
			braceStart := match[1] - 1
			// Match to find closing brace
			depth := 1
			braceEnd := braceStart + 1
			for i := braceStart + 1; i < len(code) && depth > 0; i++ {
				switch code[i] {
				case '{':
					depth++
				case '}':
					depth--
				}
				braceEnd = i
			}
			if depth == 0 && braceEnd > braceStart {
				// Check if followed by []
				afterBrace := strings.TrimSpace(code[braceEnd+1:])
				if strings.HasPrefix(afterBrace, "[]") {
					inlineType := code[braceStart : braceEnd+1]
					elementType = a.parseInlineTypeAnnotation(inlineType, code)
				}
			}
		}
	}

	// Method 2: If no explicit type, check if it's assigned from a function call
	if elementType == nil {
		varInitRe := regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(sourceArray) + `\s*=\s*(\w+)\(`)
		if match := varInitRe.FindStringSubmatch(code); match != nil {
			funcName := match[1]
			// Look for function return type: function name(...): Type[] { or function name(...): Type[] {
			funcReturnRe := regexp.MustCompile(`(?m)function\s+` + regexp.QuoteMeta(funcName) + `\s*\([^)]*\)\s*:\s*([^=\[{\s]+)\[\]`)
			if funcMatch := funcReturnRe.FindStringSubmatch(code); funcMatch != nil {
				elemTypeName := strings.TrimSpace(funcMatch[1])
				if iface, ok := a.interfaces[elemTypeName]; ok {
					elementType = iface
				} else {
					elementType = &TypeDef{Kind: "object", Name: elemTypeName}
				}
			}
		}
	}

	// Method 3: Check for inline array literal: const arr = [{ prop: value, ... }, ...]
	if elementType == nil {
		arrLiteralRe := regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(sourceArray) + `\s*=\s*\[`)
		if match := arrLiteralRe.FindStringIndex(code); match != nil {
			// Find the opening bracket
			bracketStart := match[1] - 1
			// Match to find closing bracket
			depth := 1
			bracketEnd := bracketStart + 1
			for i := bracketStart + 1; i < len(code) && depth > 0; i++ {
				switch code[i] {
				case '[':
					depth++
				case ']':
					depth--
				}
				bracketEnd = i
			}
			if depth == 0 && bracketEnd > bracketStart {
				arrayContent := code[bracketStart+1 : bracketEnd]
				arrayContent = strings.TrimSpace(arrayContent)
				// If the first element is an object literal, infer type from it
				if strings.HasPrefix(arrayContent, "{") {
					// Find the end of the first object
					objDepth := 1
					objEnd := 1
					for i := 1; i < len(arrayContent) && objDepth > 0; i++ {
						switch arrayContent[i] {
						case '{':
							objDepth++
						case '}':
							objDepth--
						}
						objEnd = i
					}
					if objDepth == 0 {
						firstObj := arrayContent[:objEnd+1]
						// Parse this object literal to infer element type
						elementType = a.inferObjectLiteralType(firstObj, code)
					}
				}
			}
		}
	}

	callbackStart := mapIdx + 5
	// Find matching closing paren
	depth := 1
	callbackEnd := callbackStart
	for i := callbackStart; i < len(expr) && depth > 0; i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		callbackEnd = i
	}

	if callbackEnd <= callbackStart {
		return nil
	}

	callback := strings.TrimSpace(expr[callbackStart:callbackEnd])

	// Arrow function: x => expr or (x) => expr
	if arrowIdx := strings.Index(callback, "=>"); arrowIdx > 0 {
		// Extract the parameter name (e.g., "p" from "p => p.name")
		paramPart := strings.TrimSpace(callback[:arrowIdx])
		paramName := strings.Trim(paramPart, "()")
		paramName = strings.TrimSpace(paramName)
		// Handle destructuring or multiple params - just use first identifier
		if commaIdx := strings.Index(paramName, ","); commaIdx > 0 {
			paramName = strings.TrimSpace(paramName[:commaIdx])
		}

		returnExpr := strings.TrimSpace(callback[arrowIdx+2:])

		// If it starts with (, it might be a wrapped object literal
		if strings.HasPrefix(returnExpr, "(") {
			returnExpr = strings.TrimPrefix(returnExpr, "(")
			returnExpr = strings.TrimSuffix(returnExpr, ")")
			returnExpr = strings.TrimSpace(returnExpr)
		}

		// Object literal in map callback - pass element type context
		if strings.HasPrefix(returnExpr, "{") {
			return a.inferObjectLiteralTypeWithContext(returnExpr, code, paramName, elementType)
		}

		// Property access like p.name where p is the callback parameter
		if strings.HasPrefix(returnExpr, paramName+".") {
			propPath := strings.TrimPrefix(returnExpr, paramName+".")
			// Simple property access
			if !strings.Contains(propPath, ".") && !strings.Contains(propPath, "(") {
				// If we know the element type, look up the property
				if elementType != nil {
					if propType := a.getPropertyType(elementType, propPath); propType != nil {
						return propType
					}
				}
				// Built-in properties
				if propPath == "length" {
					return &TypeDef{Kind: "primitive", Name: "number"}
				}
			}
		}

		// For shorthand returns like just the parameter variable
		if returnExpr == paramName && elementType != nil {
			return elementType
		}

		// Try to infer from the expression
		return a.inferTypeFromExpression(returnExpr, code)
	}

	return nil
}

// inferObjectLiteralTypeWithContext infers type from an object literal within a callback context.
// paramName is the callback parameter name (e.g., "cat" in map(cat => ...))
// paramType is the type of that parameter (e.g., string for Object.keys().map())
func (a *Analyzer) inferObjectLiteralTypeWithContext(expr string, code string, paramName string, paramType *TypeDef) *TypeDef {
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

	propParts := splitObjectProperties(body)

	for _, part := range propParts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if colonIdx := strings.Index(part, ":"); colonIdx > 0 {
			propName := strings.TrimSpace(part[:colonIdx])
			valueExpr := strings.TrimSpace(part[colonIdx+1:])

			var propType *TypeDef

			// Check if the value is just the callback parameter
			if valueExpr == paramName {
				// For Object.keys().map(), the parameter is string
				propType = &TypeDef{Kind: "primitive", Name: "string"}
			} else if paramType != nil && strings.HasPrefix(valueExpr, paramName+".") {
				// Property access on the parameter (including nested paths)
				propPath := strings.TrimPrefix(valueExpr, paramName+".")
				if !strings.Contains(propPath, "(") {
					propType = a.resolvePropertyPath(paramType, propPath)
				}
			}

			// Check for indexed Record access: varName[param.prop].property
			// e.g., loyaltyTiers[s.tier].name where loyaltyTiers: Record<string, LoyaltyTier>
			if propType == nil {
				propType = a.inferIndexedRecordAccess(valueExpr, code)
			}

			if propType == nil {
				propType = a.inferTypeFromExpression(valueExpr, code)
			}

			props = append(props, Property{
				Name:     propName,
				Type:     propType,
				Required: true,
			})
		} else {
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

// resolvePropertyPath resolves a dotted property path like "context.cartTotal" on a type
func (a *Analyzer) resolvePropertyPath(typeDef *TypeDef, path string) *TypeDef {
	if typeDef == nil || path == "" {
		return nil
	}

	parts := strings.SplitN(path, ".", 2)
	propName := parts[0]

	propType := a.getPropertyType(typeDef, propName)
	if propType == nil {
		return nil
	}

	// If there's more path to traverse, recurse
	if len(parts) > 1 {
		return a.resolvePropertyPath(propType, parts[1])
	}

	return propType
}

// inferIndexedRecordAccess handles expressions like: varName[key].property
// where varName: Record<string, SomeType> and we're accessing a property on SomeType
func (a *Analyzer) inferIndexedRecordAccess(expr string, code string) *TypeDef {
	// Match pattern: identifier[...].property
	indexedAccessRe := regexp.MustCompile(`^(\w+)\[([^\]]+)\]\.(\w+)$`)
	match := indexedAccessRe.FindStringSubmatch(expr)
	if match == nil {
		return nil
	}

	varName := match[1]
	// keyExpr := match[2] // We don't need this for type inference
	propName := match[3]

	// Look for the variable type: const varName: Record<string, Type> = ...
	recordTypeRe := regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(varName) + `\s*:\s*Record<[^,]+,\s*(\w+)>`)
	if typeMatch := recordTypeRe.FindStringSubmatch(code); typeMatch != nil {
		valueTypeName := strings.TrimSpace(typeMatch[1])
		if iface, ok := a.interfaces[valueTypeName]; ok {
			// Get the property type from the interface
			return a.getPropertyType(iface, propName)
		}
	}

	return nil
}

// parseInlineTypeAnnotation parses an inline object type annotation like { name: string; context: PricingContext }
func (a *Analyzer) parseInlineTypeAnnotation(typeStr string, code string) *TypeDef {
	var props []Property

	body := strings.TrimPrefix(typeStr, "{")
	body = strings.TrimSuffix(body, "}")
	body = strings.TrimSpace(body)

	if body == "" {
		return &TypeDef{
			Kind:       "object",
			Properties: props,
		}
	}

	// Split by ; for type annotations
	parts := strings.Split(body, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Handle optional marker
		optional := false
		if colonIdx := strings.Index(part, ":"); colonIdx > 0 {
			propName := strings.TrimSpace(part[:colonIdx])
			typeExpr := strings.TrimSpace(part[colonIdx+1:])

			if strings.HasSuffix(propName, "?") {
				propName = strings.TrimSuffix(propName, "?")
				optional = true
			}

			propType := a.resolveTypeAnnotation(typeExpr, code)
			props = append(props, Property{
				Name:     propName,
				Type:     propType,
				Required: !optional,
			})
		}
	}

	return &TypeDef{
		Kind:       "object",
		Properties: props,
	}
}

// resolveTypeAnnotation resolves a type annotation string to a TypeDef
func (a *Analyzer) resolveTypeAnnotation(typeStr string, code string) *TypeDef {
	typeStr = strings.TrimSpace(typeStr)

	// Check if it's a known interface
	if iface, ok := a.interfaces[typeStr]; ok {
		return iface
	}

	// Check if it's a known type alias
	if alias, ok := a.typeAliases[typeStr]; ok {
		return alias
	}

	// Handle primitives
	switch typeStr {
	case "string":
		return &TypeDef{Kind: "primitive", Name: "string"}
	case "number":
		return &TypeDef{Kind: "primitive", Name: "number"}
	case "boolean":
		return &TypeDef{Kind: "primitive", Name: "boolean"}
	case "any":
		return &TypeDef{Kind: "primitive", Name: "any"}
	case "void":
		return &TypeDef{Kind: "primitive", Name: "void"}
	case "null":
		return &TypeDef{Kind: "null", Name: "null"}
	case "undefined":
		return &TypeDef{Kind: "undefined", Name: "undefined"}
	}

	// Handle array types: Type[]
	if strings.HasSuffix(typeStr, "[]") {
		elemType := strings.TrimSuffix(typeStr, "[]")
		return &TypeDef{
			Kind:        "array",
			ElementType: a.resolveTypeAnnotation(elemType, code),
		}
	}

	// Handle union types
	if strings.Contains(typeStr, "|") {
		parts := strings.Split(typeStr, "|")
		var unionTypes []*TypeDef
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				unionTypes = append(unionTypes, a.resolveTypeAnnotation(part, code))
			}
		}
		if len(unionTypes) > 0 {
			return &TypeDef{Kind: "union", UnionTypes: unionTypes}
		}
	}

	// Default to object with name
	return &TypeDef{Kind: "object", Name: typeStr}
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

// stripComments removes single-line (//) and multi-line (/* */) comments from code.
func stripComments(code string) string {
	var result strings.Builder
	i := 0
	for i < len(code) {
		// Check for string - skip comments inside strings
		if code[i] == '"' || code[i] == '\'' || code[i] == '`' {
			quote := code[i]
			result.WriteByte(code[i])
			i++
			for i < len(code) && code[i] != quote {
				if code[i] == '\\' && i+1 < len(code) {
					result.WriteByte(code[i])
					i++
				}
				result.WriteByte(code[i])
				i++
			}
			if i < len(code) {
				result.WriteByte(code[i])
				i++
			}
			continue
		}

		// Check for single-line comment
		if i+1 < len(code) && code[i] == '/' && code[i+1] == '/' {
			// Skip until end of line
			for i < len(code) && code[i] != '\n' {
				i++
			}
			// Keep the newline
			if i < len(code) {
				result.WriteByte('\n')
				i++
			}
			continue
		}

		// Check for multi-line comment
		if i+1 < len(code) && code[i] == '/' && code[i+1] == '*' {
			i += 2
			for i+1 < len(code) && (code[i] != '*' || code[i+1] != '/') {
				i++
			}
			i += 2 // Skip */
			continue
		}

		result.WriteByte(code[i])
		i++
	}
	return result.String()
}

// splitObjectProperties splits object literal body into individual property parts.
// Handles nested objects and arrays properly.
func splitObjectProperties(body string) []string {
	// Strip comments first
	body = stripComments(body)

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
		switch c {
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
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
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && c != '_' && c != '$' {
				return false
			}
		} else {
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' && c != '$' {
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
// 4. Local variables without type annotation (const currentUser = ...) - infer from initializer
// 5. Known interfaces/type aliases
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

	// Check local variable declarations WITH type annotation (with optional export keyword)
	varTypeRe := regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(name) + `\s*:\s*`)
	if loc := varTypeRe.FindStringIndex(code); loc != nil {
		typeStart := loc[1]
		// Extract until = sign, but handle nested braces
		typeStr := extractTypeUntilEquals(code[typeStart:])
		if typeStr != "" {
			return a.parseTypeExpression(typeStr)
		}
	}

	// Check local variable declarations WITHOUT type annotation - infer from initializer (with optional export)
	varInitRe := regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+` + regexp.QuoteMeta(name) + `\s*=\s*`)
	if loc := varInitRe.FindStringIndex(code); loc != nil {
		initStart := loc[1]
		initExpr := extractInitializer(code[initStart:])
		if initExpr != "" {
			return a.inferTypeFromExpression(initExpr, code)
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
		switch c {
		case '{', '[', '(', '<':
			depth++
		case '}', ']', ')', '>':
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
		switch c {
		case '{', '[', '(', '<':
			depth++
		case '}', ']', ')', '>':
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

// extractInitializer extracts the initializer expression from code after an equals sign.
// It reads until a semicolon or newline at depth 0.
func extractInitializer(code string) string {
	var result strings.Builder
	depth := 0
	inString := false
	stringChar := byte(0)

	for i := 0; i < len(code); i++ {
		c := code[i]

		// Handle strings (including template literals)
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
		switch c {
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
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

// extractReturnType extracts a return type from after the colon in a function declaration.
// Handles both simple types (string, number[]) and inline object types ({ foo: string; bar: number }).
func extractReturnType(code string) string {
	var result strings.Builder
	braceDepth := 0
	angleDepth := 0
	parenDepth := 0
	bracketDepth := 0
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

		// Track depth for different bracket types
		totalDepth := braceDepth + angleDepth + parenDepth + bracketDepth

		switch c {
		case '{':
			// At depth 0, a { starts the function body, not an object type in return type
			// But if we haven't seen any content yet, it's an object type
			content := strings.TrimSpace(result.String())
			if totalDepth == 0 && content != "" && !strings.HasSuffix(content, ":") && !strings.HasSuffix(content, ",") && !strings.HasSuffix(content, "<") {
				// This is the function body, not part of the return type
				return content
			}
			braceDepth++
			result.WriteByte(c)
		case '}':
			braceDepth--
			result.WriteByte(c)
			// If we just closed the outermost brace of an object type, we're done
			if braceDepth == 0 && angleDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
				return strings.TrimSpace(result.String())
			}
		case '<':
			angleDepth++
			result.WriteByte(c)
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
			result.WriteByte(c)
		case '(':
			parenDepth++
			result.WriteByte(c)
		case ')':
			parenDepth--
			result.WriteByte(c)
		case '[':
			bracketDepth++
			result.WriteByte(c)
		case ']':
			bracketDepth--
			result.WriteByte(c)
		default:
			// At top level, stop at other terminators
			if totalDepth == 0 {
				// End of statement
				if c == ';' || c == '\n' {
					return strings.TrimSpace(result.String())
				}
			}
			result.WriteByte(c)
		}
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
	matches := declareVarRe.FindAllStringSubmatch(code, -1)

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

// findMatchingBrace finds the index of the closing brace that matches
// the opening brace at position start. Returns -1 if not found.
func findMatchingBrace(code string, start int) int {
	if start >= len(code) || code[start] != '{' {
		return -1
	}
	depth := 1
	for i := start + 1; i < len(code); i++ {
		switch code[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		case '"', '\'', '`':
			// Skip string literals
			quote := code[i]
			for i++; i < len(code); i++ {
				if code[i] == quote && code[i-1] != '\\' {
					break
				}
			}
		}
	}
	return -1
}

// inferReturnTypeFromFunctionBody infers the return type from a function body
// by finding the return statement and analyzing what it returns.
func (a *Analyzer) inferReturnTypeFromFunctionBody(funcBody string, fullCode string) *TypeDef {
	// Find the last return statement in the function body
	// Pattern: return <expression>;
	// We need to find the outermost return (not nested in closures)

	// Simple approach: find "return " followed by an expression
	returnRe := regexp.MustCompile(`(?m)^\s*return\s+`)
	matches := returnRe.FindAllStringIndex(funcBody, -1)
	if len(matches) == 0 {
		return nil
	}

	// Use the last return statement (usually the main one)
	lastMatch := matches[len(matches)-1]
	returnStart := lastMatch[1]

	// Extract the return expression
	returnExpr := extractReturnExpression(funcBody[returnStart:])
	if returnExpr == "" {
		return nil
	}

	// Infer the type of the return expression
	return a.inferTypeFromExpression(returnExpr, fullCode)
}

// extractReturnExpression extracts the expression from a return statement.
func extractReturnExpression(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}

	// Handle object literal returns: return { ... };
	if strings.HasPrefix(code, "{") {
		depth := 1
		for i := 1; i < len(code); i++ {
			switch code[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return strings.TrimSpace(code[:i+1])
				}
			case '"', '\'', '`':
				// Skip string literals
				quote := code[i]
				for i++; i < len(code); i++ {
					if code[i] == quote && code[i-1] != '\\' {
						break
					}
				}
			}
		}
		return ""
	}

	// For other expressions, find the semicolon or newline
	for i := 0; i < len(code); i++ {
		if code[i] == ';' || code[i] == '\n' {
			return strings.TrimSpace(code[:i])
		}
	}

	return strings.TrimSpace(code)
}
