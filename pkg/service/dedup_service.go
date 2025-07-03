package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// dedupService implements the DeduplicationService interface
type dedupService struct {
	storage    common.Storage
	logger     logrus.FieldLogger
	topNFrames int // Number of top frames to use for deduplication
}

// NewDeduplicationService creates a new deduplication service instance
func NewDeduplicationService(storage common.Storage, logger logrus.FieldLogger) common.DeduplicationService {
	return &dedupService{
		storage:    storage,
		logger:     logger.WithField("service", "deduplication"),
		topNFrames: 5, // Default to top 5 frames
	}
}

// ProcessCrash processes a crash and determines if it's unique
func (ds *dedupService) ProcessCrash(ctx context.Context, crash *common.CrashResult) (*common.CrashGroup, bool, error) {
	// Parse stack trace
	stackTrace, err := ds.parseStackTrace(crash.StackTrace)
	if err != nil {
		ds.logger.WithError(err).WithField("crash_id", crash.ID).Warn("Failed to parse stack trace")
		// Continue without stack-based deduplication
		return nil, true, nil
	}

	// Store the parsed stack trace
	if err := ds.storage.CreateStackTrace(ctx, crash.ID, stackTrace); err != nil {
		ds.logger.WithError(err).WithField("crash_id", crash.ID).Error("Failed to store stack trace")
	}

	// Get campaign ID from job
	job, err := ds.storage.GetJob(ctx, crash.JobID)
	if err != nil {
		return nil, true, fmt.Errorf("failed to get job: %w", err)
	}

	// Get campaign jobs to find campaign ID
	// This is a bit inefficient but works for now
	campaigns, err := ds.storage.ListCampaigns(ctx, 0, 0, "")
	if err != nil {
		return nil, true, fmt.Errorf("failed to list campaigns: %w", err)
	}

	var campaignID string
	for _, campaign := range campaigns {
		jobs, err := ds.storage.GetCampaignJobs(ctx, campaign.ID)
		if err != nil {
			continue
		}
		for _, j := range jobs {
			if j.ID == job.ID {
				campaignID = campaign.ID
				break
			}
		}
		if campaignID != "" {
			break
		}
	}

	if campaignID == "" {
		// Job not associated with a campaign, treat as unique
		return nil, true, nil
	}

	// Update crash with campaign ID
	if err := ds.storage.UpdateCrashWithCampaign(ctx, crash.ID, campaignID); err != nil {
		ds.logger.WithError(err).Error("Failed to update crash with campaign ID")
	}

	// Check if this stack hash already exists
	existingGroup, err := ds.storage.GetCrashGroup(ctx, campaignID, stackTrace.TopNHash)
	if err != nil && err != common.ErrCrashGroupNotFound {
		return nil, true, fmt.Errorf("failed to check existing crash group: %w", err)
	}

	if existingGroup != nil {
		// Duplicate crash - update the group
		if err := ds.storage.UpdateCrashGroupCount(ctx, existingGroup.ID); err != nil {
			ds.logger.WithError(err).Error("Failed to update crash group count")
		}

		// Link crash to group
		if err := ds.storage.LinkCrashToGroup(ctx, crash.ID, existingGroup.ID); err != nil {
			ds.logger.WithError(err).Error("Failed to link crash to group")
		}

		ds.logger.WithFields(logrus.Fields{
			"crash_id":   crash.ID,
			"group_id":   existingGroup.ID,
			"stack_hash": stackTrace.TopNHash,
		}).Debug("Duplicate crash detected")

		return existingGroup, false, nil
	}

	// New unique crash - create a crash group
	crashGroup := &common.CrashGroup{
		ID:           "group-" + uuid.New().String(),
		CampaignID:   campaignID,
		StackHash:    stackTrace.TopNHash,
		FirstSeen:    crash.Timestamp,
		LastSeen:     crash.Timestamp,
		Count:        1,
		Severity:     ds.calculateSeverity(crash),
		StackFrames:  stackTrace.Frames[:min(len(stackTrace.Frames), ds.topNFrames)],
		ExampleCrash: crash.ID,
	}

	if err := ds.storage.CreateCrashGroup(ctx, crashGroup); err != nil {
		return nil, true, fmt.Errorf("failed to create crash group: %w", err)
	}

	// Link crash to group
	if err := ds.storage.LinkCrashToGroup(ctx, crash.ID, crashGroup.ID); err != nil {
		ds.logger.WithError(err).Error("Failed to link crash to group")
	}

	ds.logger.WithFields(logrus.Fields{
		"crash_id":   crash.ID,
		"group_id":   crashGroup.ID,
		"stack_hash": stackTrace.TopNHash,
	}).Info("New unique crash detected")

	return crashGroup, true, nil
}

// GetCrashGroups retrieves all crash groups for a campaign
func (ds *dedupService) GetCrashGroups(ctx context.Context, campaignID string) ([]*common.CrashGroup, error) {
	groups, err := ds.storage.ListCrashGroups(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("failed to list crash groups: %w", err)
	}
	return groups, nil
}

// GetStackTrace retrieves the stack trace for a crash
func (ds *dedupService) GetStackTrace(ctx context.Context, crashID string) (*common.StackTrace, error) {
	stackTrace, err := ds.storage.GetStackTrace(ctx, crashID)
	if err != nil {
		return nil, fmt.Errorf("failed to get stack trace: %w", err)
	}
	return stackTrace, nil
}

// parseStackTrace parses a raw stack trace into structured format
func (ds *dedupService) parseStackTrace(rawTrace string) (*common.StackTrace, error) {
	if rawTrace == "" {
		return nil, fmt.Errorf("empty stack trace")
	}

	frames := ds.extractFrames(rawTrace)
	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames found in stack trace")
	}

	// Compute hashes
	topNHash := ds.computeStackHash(frames, ds.topNFrames)
	fullHash := ds.computeStackHash(frames, len(frames))

	return &common.StackTrace{
		Frames:   frames,
		TopNHash: topNHash,
		FullHash: fullHash,
		RawTrace: rawTrace,
	}, nil
}

// extractFrames extracts stack frames from various formats
func (ds *dedupService) extractFrames(trace string) []common.StackFrame {
	var frames []common.StackFrame

	// Try different stack trace formats
	// Format 1: GDB/LLDB style (#0 0x00000000 in function at file:line)
	gdbPattern := regexp.MustCompile(`#\d+\s+0x[0-9a-fA-F]+\s+in\s+([^\s]+)\s+(?:at\s+)?([^:]+):(\d+)`)
	if matches := gdbPattern.FindAllStringSubmatch(trace, -1); len(matches) > 0 {
		for _, match := range matches {
			line, _ := strconv.Atoi(match[3])
			frames = append(frames, common.StackFrame{
				Function: match[1],
				File:     match[2],
				Line:     line,
			})
		}
		return frames
	}

	// Format 2: AddressSanitizer style (    #0 0x7f... in function file:line:col)
	asanPattern := regexp.MustCompile(`#\d+\s+0x[0-9a-fA-F]+\s+in\s+([^\s]+)\s+([^:]+):(\d+)(?::\d+)?`)
	if matches := asanPattern.FindAllStringSubmatch(trace, -1); len(matches) > 0 {
		for _, match := range matches {
			line, _ := strconv.Atoi(match[3])
			frames = append(frames, common.StackFrame{
				Function: match[1],
				File:     match[2],
				Line:     line,
			})
		}
		return frames
	}

	// Format 3: Simple format (function at file:line)
	simplePattern := regexp.MustCompile(`([^\s]+)\s+at\s+([^:]+):(\d+)`)
	if matches := simplePattern.FindAllStringSubmatch(trace, -1); len(matches) > 0 {
		for _, match := range matches {
			line, _ := strconv.Atoi(match[3])
			frames = append(frames, common.StackFrame{
				Function: match[1],
				File:     match[2],
				Line:     line,
			})
		}
		return frames
	}

	// Format 4: Line-by-line with just function names
	lines := strings.Split(trace, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "==") {
			continue
		}

		// Extract function name from various formats
		var function string
		if idx := strings.Index(line, " in "); idx > 0 {
			parts := strings.Fields(line[idx+4:])
			if len(parts) > 0 {
				function = parts[0]
			}
		} else if strings.Contains(line, "0x") {
			// Try to extract function after address
			parts := strings.Fields(line)
			for i, part := range parts {
				if strings.HasPrefix(part, "0x") && i+1 < len(parts) {
					function = parts[i+1]
					break
				}
			}
		}

		if function != "" && function != "??" && function != "<unknown>" {
			frames = append(frames, common.StackFrame{
				Function: function,
			})
		}
	}

	return frames
}

// computeStackHash computes a hash of the top N stack frames
func (ds *dedupService) computeStackHash(frames []common.StackFrame, n int) string {
	if n > len(frames) {
		n = len(frames)
	}

	// Create a normalized representation of the top N frames
	var parts []string
	for i := 0; i < n; i++ {
		frame := frames[i]
		// Include function name and file (but not line number for better grouping)
		part := fmt.Sprintf("%s@%s", frame.Function, frame.File)
		parts = append(parts, part)
	}

	// Compute hash
	data := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// calculateSeverity estimates crash severity based on type and signal
func (ds *dedupService) calculateSeverity(crash *common.CrashResult) string {
	// Check for specific security-relevant signals
	switch crash.Signal {
	case 11: // SIGSEGV
		if strings.Contains(crash.Output, "heap") || strings.Contains(crash.Output, "buffer overflow") {
			return "critical"
		}
		return "high"
	case 6: // SIGABRT
		if strings.Contains(crash.Output, "double free") || strings.Contains(crash.Output, "heap corruption") {
			return "critical"
		}
		return "medium"
	case 4: // SIGILL
		return "high"
	case 8: // SIGFPE
		return "medium"
	}

	// Check crash type
	switch crash.Type {
	case "heap_overflow", "stack_overflow", "use_after_free":
		return "critical"
	case "segfault", "null_deref":
		return "high"
	case "assertion", "timeout":
		return "low"
	}

	return "medium"
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
