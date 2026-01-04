// Package transpiler provides TypeScript to JavaScript transpilation with caching.
//
// The transpiler uses esbuild for fast TypeScript to JavaScript conversion,
// with an LRU cache to avoid re-transpiling the same code. It supports both
// IIFE format (for general execution) and ESM format (for top-level await).
package transpiler

import (
	"hash/fnv"
	"strconv"
	"strings"
	"sync"

	"github.com/evanw/esbuild/pkg/api"
)

// ============================================================================
// Configuration
// ============================================================================

// DefaultCacheSize is the default number of transpiled scripts to cache.
const DefaultCacheSize = 1000

// ============================================================================
// Transpiler
// ============================================================================

// Transpiler transpiles TypeScript to JavaScript with caching.
type Transpiler struct {
	cache     *lruCache
	cacheSize int
}

// New creates a new TypeScript transpiler.
func New() *Transpiler {
	return &Transpiler{
		cache:     newLRUCache(DefaultCacheSize),
		cacheSize: DefaultCacheSize,
	}
}

// NewWithCacheSize creates a new TypeScript transpiler with custom cache size.
func NewWithCacheSize(size int) *Transpiler {
	if size <= 0 {
		size = DefaultCacheSize
	}
	return &Transpiler{
		cache:     newLRUCache(size),
		cacheSize: size,
	}
}

// Transpile converts TypeScript code to JavaScript.
// Returns the JavaScript code and source map.
// This method is safe for concurrent use.
func (t *Transpiler) Transpile(code string) (string, string, error) {
	if len(code) == 0 {
		return "", "", &TranspileError{Message: "code cannot be empty"}
	}

	// Preprocess: if no export default, wrap trailing expression
	code = preprocessTrailingExpression(code)

	// Compute hash
	hash := hashCode(code)

	// Check cache (thread-safe)
	if cached, ok := t.cache.get(hash); ok {
		result := cached.(*transpileResult)
		return result.code, result.sourceMap, nil
	}

	// Transpile using esbuild
	opts := api.TransformOptions{
		Loader:            api.LoaderTS,
		Target:            api.ES2022,
		Format:            api.FormatIIFE,
		GlobalName:        "__tsgo_exports__",
		MinifyWhitespace:  false,
		MinifyIdentifiers: false,
		MinifySyntax:      false,
		Sourcemap:         api.SourceMapInline,
	}

	result := api.Transform(code, opts)

	if len(result.Errors) > 0 {
		err := result.Errors[0]
		return "", "", &TranspileError{
			Message: err.Text,
			Line:    err.Location.Line,
			Column:  err.Location.Column,
		}
	}

	jsCode := string(result.Code)
	sourceMap := extractInlineSourceMap(jsCode)

	// Cache result (thread-safe)
	t.cache.put(hash, &transpileResult{
		code:      jsCode,
		sourceMap: sourceMap,
	})

	return jsCode, sourceMap, nil
}

// TranspileESM converts TypeScript code to JavaScript using ESM format.
// This is needed for top-level await support (IIFE doesn't support it).
// Returns the JavaScript code and source map.
// This method is safe for concurrent use.
func (t *Transpiler) TranspileESM(code string) (string, string, error) {
	if len(code) == 0 {
		return "", "", &TranspileError{Message: "code cannot be empty"}
	}

	// Preprocess: if no export default, wrap trailing expression
	code = preprocessTrailingExpression(code)

	// Compute hash with ESM suffix to differentiate from IIFE cache
	hash := hashCode(code + ":esm")

	// Check cache (thread-safe)
	if cached, ok := t.cache.get(hash); ok {
		result := cached.(*transpileResult)
		return result.code, result.sourceMap, nil
	}

	// Transpile using esbuild with ESM format
	opts := api.TransformOptions{
		Loader:            api.LoaderTS,
		Target:            api.ES2022,
		Format:            api.FormatESModule,
		MinifyWhitespace:  false,
		MinifyIdentifiers: false,
		MinifySyntax:      false,
		Sourcemap:         api.SourceMapInline,
	}

	result := api.Transform(code, opts)

	if len(result.Errors) > 0 {
		err := result.Errors[0]
		return "", "", &TranspileError{
			Message: err.Text,
			Line:    err.Location.Line,
			Column:  err.Location.Column,
		}
	}

	jsCode := string(result.Code)
	sourceMap := extractInlineSourceMap(jsCode)

	// Cache result (thread-safe)
	t.cache.put(hash, &transpileResult{
		code:      jsCode,
		sourceMap: sourceMap,
	})

	return jsCode, sourceMap, nil
}

// ClearCache clears the transpilation cache.
// This method is safe for concurrent use.
func (t *Transpiler) ClearCache() {
	t.cache = newLRUCache(t.cacheSize)
}

// ============================================================================
// Internal Types
// ============================================================================

type transpileResult struct {
	code      string
	sourceMap string
}

// TranspileError represents a transpilation error.
type TranspileError struct {
	Message string
	Line    int
	Column  int
}

func (e *TranspileError) Error() string {
	return e.Message
}

// ============================================================================
// Hashing & Source Map Extraction
// ============================================================================

// hashCode computes a fast FNV-1a hash of the code.
// FNV-1a is much faster than SHA-256 and sufficient for cache keys.
func hashCode(code string) string {
	h := fnv.New64a()
	h.Write([]byte(code))
	return strconv.FormatUint(h.Sum64(), 36)
}

// extractInlineSourceMap extracts the inline source map from transpiled code.
// Uses strings.LastIndex for O(n) performance instead of manual scanning.
func extractInlineSourceMap(code string) string {
	const prefix = "//# sourceMappingURL=data:application/json;base64,"
	idx := strings.LastIndex(code, prefix)
	if idx == -1 {
		return ""
	}
	// Trim any trailing newlines/whitespace
	result := code[idx+len(prefix):]
	if newlineIdx := strings.IndexByte(result, '\n'); newlineIdx != -1 {
		result = result[:newlineIdx]
	}
	return strings.TrimSpace(result)
}

// ============================================================================
// Expression Preprocessing
// ============================================================================

// preprocessTrailingExpression converts trailing expressions to export default.
// This ensures expressions like "1 === 2" or "a && b" return their value.
func preprocessTrailingExpression(code string) string {
	// Fast path: if already has export default, return as-is
	if strings.Contains(code, "export default") || strings.Contains(code, "export {") {
		return code
	}

	// Remove comments for analysis
	clean := removeComments(code)

	// Split into statements
	statements := splitStatements(clean)
	if len(statements) == 0 {
		return code
	}

	// Find the last non-empty, non-declaration statement
	lastIdx := -1
	for i := len(statements) - 1; i >= 0; i-- {
		stmt := strings.TrimSpace(statements[i])
		if stmt == "" {
			continue
		}

		// Skip declarations - they can't be exported as expressions
		if isDeclaration(stmt) {
			continue
		}

		// This looks like an expression
		lastIdx = i
		break
	}

	// No trailing expression found
	if lastIdx == -1 {
		return code
	}

	// Find the position of this statement in the original code
	// and wrap it with export default
	lastStmt := strings.TrimSpace(statements[lastIdx])

	// Handle trailing semicolon
	lastStmt = strings.TrimSuffix(lastStmt, ";")

	// Find and replace the last occurrence in original code
	// We need to be careful to find the exact statement
	idx := strings.LastIndex(code, lastStmt)
	if idx == -1 {
		return code
	}

	// Check what comes after to preserve semicolons/newlines
	afterIdx := idx + len(lastStmt)
	suffix := ""
	if afterIdx < len(code) {
		remaining := code[afterIdx:]
		// Take any trailing semicolon and whitespace
		for i, c := range remaining {
			if c == ';' || c == ' ' || c == '\t' || c == '\n' || c == '\r' {
				suffix = remaining[:i+1]
			} else {
				break
			}
		}
		if suffix == "" && len(remaining) > 0 && (remaining[0] == ';' || remaining[0] == '\n') {
			suffix = string(remaining[0])
		}
	}

	// Build the new code with export default
	return code[:idx] + "export default (" + lastStmt + ")" + suffix + code[afterIdx+len(suffix):]
}

// removeComments removes single-line and multi-line comments from code.
func removeComments(code string) string {
	var result strings.Builder
	i := 0
	for i < len(code) {
		if i+1 < len(code) {
			if code[i] == '/' && code[i+1] == '/' {
				// Single-line comment
				for i < len(code) && code[i] != '\n' {
					i++
				}
				continue
			}
			if code[i] == '/' && code[i+1] == '*' {
				// Multi-line comment
				i += 2
				for i+1 < len(code) && (code[i] != '*' || code[i+1] != '/') {
					i++
				}
				i += 2
				continue
			}
		}
		if i < len(code) {
			result.WriteByte(code[i])
		}
		i++
	}
	return result.String()
}

// splitStatements splits code into statements by semicolons and newlines.
func splitStatements(code string) []string {
	var statements []string
	var current strings.Builder
	braceDepth := 0
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
			current.WriteByte(c)
			continue
		}
		if inString {
			current.WriteByte(c)
			if c == stringChar && (i == 0 || code[i-1] != '\\') {
				inString = false
			}
			continue
		}

		// Track braces/parens/brackets
		switch c {
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		}

		// Statement separator only at top level
		if (c == ';' || c == '\n') && braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteByte(c)
	}

	// Don't forget the last statement
	if stmt := strings.TrimSpace(current.String()); stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

// isDeclaration checks if a statement is a declaration (not an expression).
func isDeclaration(stmt string) bool {
	declarationPrefixes := []string{
		"const ", "let ", "var ",
		"interface ", "type ", "class ",
		"function ", "async function ",
		"declare ", "import ", "export ",
		"enum ", "namespace ", "module ",
	}
	for _, prefix := range declarationPrefixes {
		if strings.HasPrefix(stmt, prefix) {
			return true
		}
	}
	return false
}

// ============================================================================
// LRU Cache Implementation
// ============================================================================

// lruCache is a simple LRU cache implementation.
// Uses RWMutex for better read concurrency since cache hits are common.
type lruCache struct {
	capacity int
	items    map[string]*lruItem
	head     *lruItem
	tail     *lruItem
	mu       sync.RWMutex
}

type lruItem struct {
	key   string
	value any
	prev  *lruItem
	next  *lruItem
}

func newLRUCache(capacity int) *lruCache {
	return &lruCache{
		capacity: capacity,
		items:    make(map[string]*lruItem, capacity),
	}
}

func (c *lruCache) get(key string) (any, bool) {
	// Fast path: read lock to check existence
	c.mu.RLock()
	item, ok := c.items[key]
	if !ok {
		c.mu.RUnlock()
		return nil, false
	}
	// Check if already at front (common case for hot items)
	if item == c.head {
		value := item.value
		c.mu.RUnlock()
		return value, true
	}
	c.mu.RUnlock()

	// Slow path: need write lock to move to front
	c.mu.Lock()
	defer c.mu.Unlock()

	// Re-check after acquiring write lock
	item, ok = c.items[key]
	if !ok {
		return nil, false
	}

	c.moveToFront(item)
	return item.value, true
}

func (c *lruCache) put(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, ok := c.items[key]; ok {
		item.value = value
		c.moveToFront(item)
		return
	}

	item := &lruItem{key: key, value: value}
	c.items[key] = item
	c.addToFront(item)

	if len(c.items) > c.capacity {
		c.removeLast()
	}
}

func (c *lruCache) moveToFront(item *lruItem) {
	if item == c.head {
		return
	}
	c.remove(item)
	c.addToFront(item)
}

func (c *lruCache) addToFront(item *lruItem) {
	item.prev = nil
	item.next = c.head
	if c.head != nil {
		c.head.prev = item
	}
	c.head = item
	if c.tail == nil {
		c.tail = item
	}
}

func (c *lruCache) remove(item *lruItem) {
	if item.prev != nil {
		item.prev.next = item.next
	} else {
		c.head = item.next
	}
	if item.next != nil {
		item.next.prev = item.prev
	} else {
		c.tail = item.prev
	}
}

func (c *lruCache) removeLast() {
	if c.tail == nil {
		return
	}
	delete(c.items, c.tail.key)
	c.remove(c.tail)
}
