// Package typegen generates TypeScript type definitions from Go types.
package typegen

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// GenerateContextDTS generates TypeScript declarations for global context values.
func GenerateContextDTS(globals map[string]any) string {
	var sb strings.Builder

	sb.WriteString("declare global {\n")

	// Sort keys for consistent output
	keys := make([]string, 0, len(globals))
	for k := range globals {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, name := range keys {
		value := globals[name]
		tsType := goToTSType(value)
		sb.WriteString(fmt.Sprintf("  const %s: %s;\n", name, tsType))
	}

	sb.WriteString("}\n\nexport {}\n")

	return sb.String()
}

// goToTSType converts a Go value to its TypeScript type representation.
func goToTSType(value any) string {
	if value == nil {
		return "undefined"
	}

	v := reflect.ValueOf(value)
	return reflectToTS(v.Type())
}

func reflectToTS(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.String:
		return "string"
	case reflect.Slice, reflect.Array:
		elemType := reflectToTS(t.Elem())
		return elemType + "[]"
	case reflect.Map:
		keyType := reflectToTS(t.Key())
		valType := reflectToTS(t.Elem())
		return fmt.Sprintf("Record<%s, %s>", keyType, valType)
	case reflect.Struct:
		return "object"
	case reflect.Ptr:
		return reflectToTS(t.Elem()) + " | null"
	case reflect.Interface:
		return "any"
	default:
		return "any"
	}
}

// Builder builds TypeScript type definitions incrementally.
type Builder struct {
	interfaces []string
	globals    []string
	functions  []string
}

// NewBuilder creates a new TypeScript definition builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// AddInterface adds an interface definition.
func (b *Builder) AddInterface(name string, fields map[string]string) *Builder {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("export interface %s {\n", name))

	// Sort for consistent output
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, field := range keys {
		tsType := fields[field]
		sb.WriteString(fmt.Sprintf("  %s: %s;\n", field, tsType))
	}

	sb.WriteString("}\n")
	b.interfaces = append(b.interfaces, sb.String())
	return b
}

// AddGlobal adds a global variable declaration.
func (b *Builder) AddGlobal(name, tsType string) *Builder {
	b.globals = append(b.globals, fmt.Sprintf("const %s: %s", name, tsType))
	return b
}

// AddFunction adds a function declaration.
func (b *Builder) AddFunction(name, params, returnType, doc string) *Builder {
	var sb strings.Builder
	if doc != "" {
		sb.WriteString(fmt.Sprintf("/** %s */\n", doc))
	}
	sb.WriteString(fmt.Sprintf("declare function %s(%s): %s;", name, params, returnType))
	b.functions = append(b.functions, sb.String())
	return b
}

// Build generates the final TypeScript definition file content.
func (b *Builder) Build() string {
	var sb strings.Builder

	// Interfaces first
	for _, iface := range b.interfaces {
		sb.WriteString(iface)
		sb.WriteString("\n")
	}

	// Global declarations
	if len(b.globals) > 0 {
		sb.WriteString("declare global {\n")
		for _, global := range b.globals {
			sb.WriteString("  ")
			sb.WriteString(global)
			sb.WriteString(";\n")
		}
		sb.WriteString("}\n\n")
	}

	// Functions
	for _, fn := range b.functions {
		sb.WriteString(fn)
		sb.WriteString("\n")
	}

	sb.WriteString("export {}\n")

	return sb.String()
}
