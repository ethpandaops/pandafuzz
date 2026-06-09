package common

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PathValidationError represents a path validation failure
type PathValidationError struct {
	Path    string
	Reason  string
	Details string
}

func (e *PathValidationError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("path validation failed for %q: %s (%s)", e.Path, e.Reason, e.Details)
	}
	return fmt.Sprintf("path validation failed for %q: %s", e.Path, e.Reason)
}

// IsPathTraversal detects path traversal attempts in a given path.
// Returns true if the path contains traversal sequences like ".." or "./"
func IsPathTraversal(path string) bool {
	if path == "" {
		return false
	}

	// Check for explicit ".." sequences
	if strings.Contains(path, "..") {
		return true
	}

	// Check for "./" at start or "/./" in middle (current dir reference that could be abused)
	if strings.HasPrefix(path, "./") || strings.Contains(path, "/./") {
		return true
	}

	// Clean the path and compare - if different, there was normalization needed
	// which indicates potential traversal attempt
	cleaned := filepath.Clean(path)
	if cleaned != path && !strings.HasPrefix(path, "./") {
		// Allow simple cases where Clean just removes trailing slashes
		if strings.TrimSuffix(path, "/") != cleaned {
			return true
		}
	}

	return false
}

// ValidateBinaryPath performs comprehensive validation of a binary path.
// It checks for:
// - Empty path
// - Path traversal attempts
// - Symlink resolution and validation
// - File existence
// - Executable permissions
// - Optional: allowed directory whitelisting
//
// If allowedDirs is nil or empty, directory whitelisting is skipped.
func ValidateBinaryPath(path string, allowedDirs []string) error {
	// Check for empty path
	if path == "" {
		return &PathValidationError{
			Path:   path,
			Reason: "path cannot be empty",
		}
	}

	// Check for path traversal attempts
	if IsPathTraversal(path) {
		return &PathValidationError{
			Path:   path,
			Reason: "path traversal detected",
		}
	}

	// Convert to absolute path if relative
	absPath := path
	if !filepath.IsAbs(path) {
		var err error
		absPath, err = filepath.Abs(path)
		if err != nil {
			return &PathValidationError{
				Path:    path,
				Reason:  "failed to resolve absolute path",
				Details: err.Error(),
			}
		}
	}

	// Resolve symlinks to get the real path
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &PathValidationError{
				Path:   path,
				Reason: "binary not found",
			}
		}
		return &PathValidationError{
			Path:    path,
			Reason:  "failed to resolve symlinks",
			Details: err.Error(),
		}
	}

	// Check if resolved path also has traversal (symlink could point to traversal)
	if IsPathTraversal(realPath) {
		return &PathValidationError{
			Path:   path,
			Reason: "resolved path contains traversal",
		}
	}

	// Check allowed directories (if specified)
	if len(allowedDirs) > 0 {
		allowed := false
		for _, dir := range allowedDirs {
			// Clean and resolve the allowed directory
			cleanDir := filepath.Clean(dir)
			if !filepath.IsAbs(cleanDir) {
				cleanDir, _ = filepath.Abs(cleanDir)
			}

			// Check if realPath is under the allowed directory
			if strings.HasPrefix(realPath, cleanDir+string(filepath.Separator)) || realPath == cleanDir {
				allowed = true
				break
			}
		}
		if !allowed {
			return &PathValidationError{
				Path:   path,
				Reason: "binary path not in allowed directories",
			}
		}
	}

	// Check file exists and get info
	info, err := os.Stat(realPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &PathValidationError{
				Path:   path,
				Reason: "binary not found",
			}
		}
		return &PathValidationError{
			Path:    path,
			Reason:  "failed to stat binary",
			Details: err.Error(),
		}
	}

	// Check it's a regular file (not a directory)
	if !info.Mode().IsRegular() {
		return &PathValidationError{
			Path:   path,
			Reason: "path is not a regular file",
		}
	}

	// Check executable permission (at least one execute bit set)
	if info.Mode().Perm()&0111 == 0 {
		return &PathValidationError{
			Path:   path,
			Reason: "binary is not executable",
		}
	}

	// Check file is not empty
	if info.Size() == 0 {
		return &PathValidationError{
			Path:   path,
			Reason: "binary file is empty",
		}
	}

	return nil
}

// ValidateJobName checks if a job name contains only allowed characters.
// Allowed: alphanumeric, hyphens, underscores, dots, and spaces.
// Max length: 200 characters.
func ValidateJobName(name string) error {
	if name == "" {
		return fmt.Errorf("job name cannot be empty")
	}

	if len(name) > 200 {
		return fmt.Errorf("job name exceeds maximum length of 200 characters")
	}

	// Allow alphanumeric, hyphens, underscores, dots, and spaces
	// Disallow shell metacharacters, quotes, semicolons, etc.
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\-_. ]*$`)
	if !validPattern.MatchString(name) {
		return fmt.Errorf("job name contains invalid characters (allowed: alphanumeric, hyphens, underscores, dots, spaces)")
	}

	// Additional checks for potentially dangerous patterns
	dangerousPatterns := []string{
		";", "&", "|", "`", "$", "(", ")", "{", "}", "[", "]",
		"<", ">", "!", "?", "*", "\\", "'", "\"", "\n", "\r", "\t",
	}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(name, pattern) {
			return fmt.Errorf("job name contains forbidden character: %q", pattern)
		}
	}

	return nil
}

// SanitizePath removes potentially dangerous elements from a path
// while preserving its basic structure. This is a best-effort sanitization
// and should be used alongside validation, not as a replacement.
func SanitizePath(path string) string {
	// Remove null bytes
	path = strings.ReplaceAll(path, "\x00", "")

	// Clean the path (removes .., ., multiple slashes, etc.)
	path = filepath.Clean(path)

	return path
}

// IsPathValidationError checks if an error is a PathValidationError
func IsPathValidationError(err error) bool {
	_, ok := err.(*PathValidationError)
	return ok
}
