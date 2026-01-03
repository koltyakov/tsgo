// Package sandbox provides security sandboxing utilities.
package sandbox

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateCode checks if code contains restricted globals.
func ValidateCode(code string, restricted []string) error {
	for _, global := range restricted {
		if strings.Contains(code, global) {
			return fmt.Errorf("code contains restricted global: %s", global)
		}
	}
	return nil
}

// ValidatePath checks if a path is within allowed directories.
func ValidatePath(path string, allowedPaths []string) error {
	if len(allowedPaths) == 0 {
		return fmt.Errorf("no paths are allowed")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	for _, allowed := range allowedPaths {
		absAllowed, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		if strings.HasPrefix(absPath, absAllowed) {
			return nil
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
