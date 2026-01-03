// Package transpiler provides TypeScript to JavaScript transpilation with caching.
package transpiler

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/evanw/esbuild/pkg/api"
)

// Transpiler transpiles TypeScript to JavaScript with caching.
type Transpiler struct {
	cache *lruCache
	mu    sync.RWMutex
}

// New creates a new TypeScript transpiler.
func New() *Transpiler {
	return &Transpiler{
		cache: newLRUCache(1000),
	}
}

// Transpile converts TypeScript code to JavaScript.
// Returns the JavaScript code and source map.
func (t *Transpiler) Transpile(code string) (string, string, error) {
	// Compute hash
	hash := hashCode(code)

	// Check cache
	t.mu.RLock()
	if cached, ok := t.cache.get(hash); ok {
		t.mu.RUnlock()
		result := cached.(*transpileResult)
		return result.code, result.sourceMap, nil
	}
	t.mu.RUnlock()

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
			Line:    0,
			Column:  0,
		}
	}

	jsCode := string(result.Code)
	sourceMap := extractInlineSourceMap(jsCode)

	// Cache result
	t.mu.Lock()
	t.cache.put(hash, &transpileResult{
		code:      jsCode,
		sourceMap: sourceMap,
	})
	t.mu.Unlock()

	return jsCode, sourceMap, nil
}

// ClearCache clears the transpilation cache.
func (t *Transpiler) ClearCache() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cache = newLRUCache(1000)
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

// hashCode computes a SHA-256 hash of the code.
func hashCode(code string) string {
	h := sha256.New()
	h.Write([]byte(code))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// extractInlineSourceMap extracts the inline source map from transpiled code.
func extractInlineSourceMap(code string) string {
	const prefix = "//# sourceMappingURL=data:application/json;base64,"
	for i := len(code) - 1; i >= 0; i-- {
		if i+len(prefix) > len(code) {
			continue
		}
		if code[i:i+len(prefix)] == prefix {
			return code[i+len(prefix):]
		}
	}
	return ""
}

// lruCache is a simple LRU cache implementation.
type lruCache struct {
	capacity int
	items    map[string]*lruItem
	head     *lruItem
	tail     *lruItem
	mu       sync.Mutex
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
		items:    make(map[string]*lruItem),
	}
}

func (c *lruCache) get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.items[key]
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

// DurationSince returns time since a start time.
func DurationSince(start time.Time) time.Duration {
	return time.Since(start)
}
