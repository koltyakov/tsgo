// Package transpiler provides TypeScript to JavaScript transpilation with caching.
package transpiler

import (
	"hash/fnv"
	"strconv"
	"strings"
	"sync"

	"github.com/evanw/esbuild/pkg/api"
)

// Transpiler transpiles TypeScript to JavaScript with caching.
type Transpiler struct {
	cache     *lruCache
	cacheSize int
}

// DefaultCacheSize is the default number of transpiled scripts to cache.
const DefaultCacheSize = 1000

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
		Target:            api.ES2020,
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

// ClearCache clears the transpilation cache.
// This method is safe for concurrent use.
func (t *Transpiler) ClearCache() {
	t.cache = newLRUCache(t.cacheSize)
}

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
