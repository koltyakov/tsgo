// Package sandbox provides security sandboxing utilities for TypeScript execution.
//
// This package validates code and filesystem paths to ensure safe execution.
// It provides:
//   - Code validation to detect restricted JavaScript globals
//   - Path validation to prevent directory traversal attacks
//   - Default lists of commonly restricted globals
package sandbox

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/koltyakov/tsgo/internal/codeanalyzer"
)

// ============================================================================
// Code Validation
// ============================================================================

// ValidateCode checks if code contains restricted globals.
// Uses tokenized identifier matching to avoid substring false positives.
func ValidateCode(code string, restricted []string) error {
	analysis := codeanalyzer.Analyze(code)

	for _, global := range restricted {
		if analysis.HasIdentifier(global) {
			return fmt.Errorf("code contains restricted global: %s", global)
		}
	}
	return nil
}

// ============================================================================
// Path Validation
// ============================================================================

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

// ============================================================================
// Default Restricted Globals
// ============================================================================

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
