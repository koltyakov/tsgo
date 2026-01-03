// Package sandbox provides security sandboxing utilities.
package sandbox

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// patternCache caches compiled regex patterns for restricted globals.
var (
	patternCache   = make(map[string]*regexp.Regexp)
	patternCacheMu sync.RWMutex
)

// getPattern returns a cached compiled regex pattern for word boundary matching.
func getPattern(word string) *regexp.Regexp {
	patternCacheMu.RLock()
	if pattern, ok := patternCache[word]; ok {
		patternCacheMu.RUnlock()
		return pattern
	}
	patternCacheMu.RUnlock()

	// Compile and cache
	patternCacheMu.Lock()
	defer patternCacheMu.Unlock()

	// Double-check after acquiring write lock
	if pattern, ok := patternCache[word]; ok {
		return pattern
	}

	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
	patternCache[word] = pattern
	return pattern
}

// ValidateCode checks if code contains restricted globals.
// Uses word boundary matching to avoid false positives.
func ValidateCode(code string, restricted []string) error {
	for _, global := range restricted {
		// Use cached pre-compiled pattern
		pattern := getPattern(global)
		if pattern.MatchString(code) {
			return fmt.Errorf("code contains restricted global: %s", global)
		}
	}
	return nil
}

// ValidatePath checks if a path is within allowed directories.
// Resolves symlinks to prevent directory traversal attacks.
func ValidatePath(path string, allowedPaths []string) error {
	if len(allowedPaths) == 0 {
		return fmt.Errorf("no paths are allowed")
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Attempt to resolve symlinks for path (file might not exist yet, so also try parent)
	evalPath := absPath
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		evalPath = resolved
	} else if resolved, err := filepath.EvalSymlinks(filepath.Dir(absPath)); err == nil {
		evalPath = filepath.Join(resolved, filepath.Base(absPath))
	}

	for _, allowed := range allowedPaths {
		absAllowed, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}

		// Resolve symlinks for allowed path too
		evalAllowed := absAllowed
		if resolved, err := filepath.EvalSymlinks(absAllowed); err == nil {
			evalAllowed = resolved
		}

		// Check both resolved and unresolved paths for flexibility
		checkPaths := []string{evalPath, absPath}
		checkAllowed := []string{evalAllowed, absAllowed}

		for _, checkPath := range checkPaths {
			for _, checkAllow := range checkAllowed {
				// Exact match
				if checkPath == checkAllow {
					return nil
				}

				// Prefix match with proper path separator
				allowedPrefix := checkAllow
				if !strings.HasSuffix(allowedPrefix, string(filepath.Separator)) {
					allowedPrefix += string(filepath.Separator)
				}

				if strings.HasPrefix(checkPath, allowedPrefix) {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("path %s is not within allowed directories", path)
}

// RestrictedGlobals returns a list of commonly restricted JavaScript globals.
func RestrictedGlobals() []string {
	return []string{
		"eval",
		"Function",
		"fetch",
		"XMLHttpRequest",
		"WebSocket",
		"Worker",
		"importScripts",
		"require",
		"process",
		"__dirname",
		"__filename",
	}
}
