package types

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Severity represents the severity level of a crash
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityUnknown  Severity = "unknown"
)

// CrashType represents different types of crashes
type CrashType string

const (
	CrashTypeSegmentationFault  CrashType = "segmentation_fault"
	CrashTypeHeapOverflow       CrashType = "heap_overflow"
	CrashTypeStackOverflow      CrashType = "stack_overflow"
	CrashTypeAssertion          CrashType = "assertion"
	CrashTypeTimeout            CrashType = "timeout"
	CrashTypeMemoryLeak         CrashType = "memory_leak"
	CrashTypeUnhandledException CrashType = "unhandled_exception"
	CrashTypeOther              CrashType = "other"
)

// Crash represents a crash discovered during fuzzing
type Crash struct {
	ID              string            `json:"id"`
	Signature       *CrashSignature   `json:"signature"`
	Input           []byte            `json:"input"`
	InputHash       string            `json:"input_hash"`
	StackTrace      string            `json:"stack_trace"`
	Severity        Severity          `json:"severity"`
	Type            CrashType         `json:"type"`
	DiscoveredAt    time.Time         `json:"discovered_at"`
	LastSeenAt      time.Time         `json:"last_seen_at"`
	OccurrenceCount uint64            `json:"occurrence_count"`
	CorpusEntryID   string            `json:"corpus_entry_id,omitempty"`
	TargetInfo      TargetInfo        `json:"target_info"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Reproducible    bool              `json:"reproducible"`
	Fixed           bool              `json:"fixed"`
	FixedAt         *time.Time        `json:"fixed_at,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
}

// TargetInfo contains information about the fuzz target
type TargetInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Command     string `json:"command"`
	Environment string `json:"environment"`
}

// NewCrash creates a new crash instance
func NewCrash(input []byte, stackTrace string, targetInfo TargetInfo) (*Crash, error) {
	if len(input) == 0 {
		return nil, errors.New("crash input cannot be empty")
	}
	if stackTrace == "" {
		return nil, errors.New("stack trace cannot be empty")
	}
	if targetInfo.Name == "" {
		return nil, errors.New("target name cannot be empty")
	}

	hash := sha256.Sum256(input)
	inputHash := hex.EncodeToString(hash[:])

	now := time.Now().UTC()
	crash := &Crash{
		ID:              generateCrashID(inputHash, stackTrace),
		Input:           input,
		InputHash:       inputHash,
		StackTrace:      stackTrace,
		Severity:        SeverityUnknown,
		Type:            determineCrashType(stackTrace),
		DiscoveredAt:    now,
		LastSeenAt:      now,
		OccurrenceCount: 1,
		TargetInfo:      targetInfo,
		Metadata:        make(map[string]string),
		Reproducible:    true, // Assume reproducible until proven otherwise
		Fixed:           false,
		Tags:            make([]string, 0),
	}

	// Generate signature from stack trace
	signature, err := NewCrashSignature(stackTrace)
	if err != nil {
		return nil, fmt.Errorf("failed to create crash signature: %w", err)
	}
	crash.Signature = signature

	// Determine severity based on crash type
	crash.Severity = determineSeverity(crash.Type, stackTrace)

	return crash, nil
}

// generateCrashID creates a unique ID for the crash
func generateCrashID(inputHash, stackTrace string) string {
	combined := inputHash + stackTrace
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:])[:16]
}

// determineCrashType analyzes the stack trace to determine crash type
func determineCrashType(stackTrace string) CrashType {
	stackLower := strings.ToLower(stackTrace)

	switch {
	case strings.Contains(stackLower, "segmentation fault") || strings.Contains(stackLower, "sigsegv"):
		return CrashTypeSegmentationFault
	case strings.Contains(stackLower, "heap overflow") || strings.Contains(stackLower, "heap-buffer-overflow"):
		return CrashTypeHeapOverflow
	case strings.Contains(stackLower, "stack overflow") || strings.Contains(stackLower, "stack-buffer-overflow"):
		return CrashTypeStackOverflow
	case strings.Contains(stackLower, "assertion") || strings.Contains(stackLower, "assert"):
		return CrashTypeAssertion
	case strings.Contains(stackLower, "timeout"):
		return CrashTypeTimeout
	case strings.Contains(stackLower, "memory leak") || strings.Contains(stackLower, "leak"):
		return CrashTypeMemoryLeak
	case strings.Contains(stackLower, "exception"):
		return CrashTypeUnhandledException
	default:
		return CrashTypeOther
	}
}

// determineSeverity determines the severity based on crash type and stack trace
func determineSeverity(crashType CrashType, stackTrace string) Severity {
	// Check for critical indicators in stack trace
	if strings.Contains(strings.ToLower(stackTrace), "arbitrary code execution") ||
		strings.Contains(strings.ToLower(stackTrace), "remote code execution") {
		return SeverityCritical
	}

	// Assign severity based on crash type
	switch crashType {
	case CrashTypeHeapOverflow, CrashTypeStackOverflow:
		return SeverityHigh
	case CrashTypeSegmentationFault:
		return SeverityMedium
	case CrashTypeAssertion, CrashTypeUnhandledException:
		return SeverityMedium
	case CrashTypeTimeout:
		return SeverityLow
	case CrashTypeMemoryLeak:
		return SeverityLow
	default:
		return SeverityUnknown
	}
}

// RecordOccurrence records another occurrence of this crash
func (c *Crash) RecordOccurrence() {
	c.OccurrenceCount++
	c.LastSeenAt = time.Now().UTC()
}

// MarkAsFixed marks the crash as fixed
func (c *Crash) MarkAsFixed() {
	c.Fixed = true
	now := time.Now().UTC()
	c.FixedAt = &now
}

// MarkAsNotReproducible marks the crash as not reproducible
func (c *Crash) MarkAsNotReproducible() {
	c.Reproducible = false
}

// UpdateSeverity updates the crash severity
func (c *Crash) UpdateSeverity(severity Severity) error {
	if !isValidSeverity(severity) {
		return errors.New("invalid severity level")
	}
	c.Severity = severity
	return nil
}

// AddTag adds a tag to the crash
func (c *Crash) AddTag(tag string) {
	for _, t := range c.Tags {
		if t == tag {
			return // Tag already exists
		}
	}
	c.Tags = append(c.Tags, tag)
}

// RemoveTag removes a tag from the crash
func (c *Crash) RemoveTag(tag string) {
	for i, t := range c.Tags {
		if t == tag {
			c.Tags = append(c.Tags[:i], c.Tags[i+1:]...)
			return
		}
	}
}

// SetMetadata sets a metadata key-value pair
func (c *Crash) SetMetadata(key, value string) {
	if c.Metadata == nil {
		c.Metadata = make(map[string]string)
	}
	c.Metadata[key] = value
}

// GetMetadata retrieves a metadata value
func (c *Crash) GetMetadata(key string) (string, bool) {
	value, exists := c.Metadata[key]
	return value, exists
}

// Age returns the duration since the crash was discovered
func (c *Crash) Age() time.Duration {
	return time.Since(c.DiscoveredAt)
}

// IsRecent checks if the crash occurred recently (within the specified duration)
func (c *Crash) IsRecent(duration time.Duration) bool {
	return time.Since(c.LastSeenAt) <= duration
}

// IsDuplicate checks if this crash is likely a duplicate of another
func (c *Crash) IsDuplicate(other *Crash) bool {
	if other == nil {
		return false
	}

	// Same signature means likely duplicate
	if c.Signature != nil && other.Signature != nil {
		return c.Signature.Hash == other.Signature.Hash
	}

	// Fallback to input hash comparison
	return c.InputHash == other.InputHash
}

// Validate ensures the crash is in a valid state
func (c *Crash) Validate() error {
	if c.ID == "" {
		return errors.New("crash ID cannot be empty")
	}
	if len(c.Input) == 0 {
		return errors.New("crash input cannot be empty")
	}
	if c.StackTrace == "" {
		return errors.New("stack trace cannot be empty")
	}
	if c.Signature == nil {
		return errors.New("crash signature cannot be nil")
	}
	if !isValidSeverity(c.Severity) {
		return errors.New("invalid severity level")
	}
	if !isValidCrashType(c.Type) {
		return errors.New("invalid crash type")
	}
	if c.TargetInfo.Name == "" {
		return errors.New("target name cannot be empty")
	}
	return nil
}

// isValidSeverity checks if the severity is valid
func isValidSeverity(severity Severity) bool {
	switch severity {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityUnknown:
		return true
	default:
		return false
	}
}

// isValidCrashType checks if the crash type is valid
func isValidCrashType(crashType CrashType) bool {
	switch crashType {
	case CrashTypeSegmentationFault, CrashTypeHeapOverflow, CrashTypeStackOverflow,
		CrashTypeAssertion, CrashTypeTimeout, CrashTypeMemoryLeak,
		CrashTypeUnhandledException, CrashTypeOther:
		return true
	default:
		return false
	}
}

// String returns a string representation of the crash
func (c *Crash) String() string {
	return fmt.Sprintf("Crash[ID=%s, Type=%s, Severity=%s, Target=%s, Reproducible=%v, Fixed=%v]",
		c.ID, c.Type, c.Severity, c.TargetInfo.Name, c.Reproducible, c.Fixed)
}
