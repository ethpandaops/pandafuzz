package mappers

import (
	"github.com/ethpandaops/pandafuzz/pkg/common"
	jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

// CommonStatusToDomain converts common.JobStatus to domain JobStatus.
// This function handles all status values with explicit mappings to prevent silent data loss.
func CommonStatusToDomain(cs common.JobStatus) jobtypes.JobStatus {
	switch cs {
	case common.JobStatusPending:
		return jobtypes.StatusPending
	case common.JobStatusAssigned:
		// 'assigned' maps to 'queued' in domain since both represent "ready to run"
		return jobtypes.StatusQueued
	case common.JobStatusStarting:
		return jobtypes.StatusStarting
	case common.JobStatusRunning:
		return jobtypes.StatusRunning
	case common.JobStatusCompleted:
		return jobtypes.StatusCompleted
	case common.JobStatusFailed:
		return jobtypes.StatusFailed
	case common.JobStatusTimedOut:
		// 'timed_out' maps to 'failed' in domain (timeout is a failure mode)
		return jobtypes.StatusFailed
	case common.JobStatusCancelled:
		return jobtypes.StatusCancelled
	default:
		// Defensive: unknown status defaults to pending
		return jobtypes.StatusPending
	}
}

// DomainStatusToCommon converts domain JobStatus to common.JobStatus.
// This function handles all status values with explicit mappings.
func DomainStatusToCommon(ds jobtypes.JobStatus) common.JobStatus {
	switch ds {
	case jobtypes.StatusPending:
		return common.JobStatusPending
	case jobtypes.StatusQueued:
		// 'queued' maps to 'assigned' in common since that's the closest equivalent
		return common.JobStatusAssigned
	case jobtypes.StatusStarting:
		return common.JobStatusStarting
	case jobtypes.StatusRunning:
		return common.JobStatusRunning
	case jobtypes.StatusCompleted:
		return common.JobStatusCompleted
	case jobtypes.StatusFailed:
		return common.JobStatusFailed
	case jobtypes.StatusCancelled:
		return common.JobStatusCancelled
	case jobtypes.StatusPaused:
		// 'paused' has no direct common equivalent; map to 'assigned' (ready state)
		return common.JobStatusAssigned
	default:
		// Defensive: unknown status defaults to pending
		return common.JobStatusPending
	}
}

// CommonPriorityToDomain converts an integer priority (0-100) to domain JobPriority.
func CommonPriorityToDomain(p int) jobtypes.JobPriority {
	switch {
	case p >= 90:
		return jobtypes.PriorityCritical
	case p >= 50:
		return jobtypes.PriorityHigh
	case p >= 20:
		return jobtypes.PriorityNormal
	default:
		return jobtypes.PriorityLow
	}
}

// DomainPriorityToCommon converts domain JobPriority to an integer (0-100).
// Uses the midpoint of each range for reversible mapping.
func DomainPriorityToCommon(p jobtypes.JobPriority) int {
	switch p {
	case jobtypes.PriorityCritical:
		return 95 // Midpoint of 90-100 range
	case jobtypes.PriorityHigh:
		return 70 // Midpoint of 50-89 range
	case jobtypes.PriorityNormal:
		return 35 // Midpoint of 20-49 range
	case jobtypes.PriorityLow:
		return 10 // Midpoint of 0-19 range
	default:
		return 35 // Default to normal priority
	}
}

// StatusStringToDomain converts a status string to domain JobStatus.
// This is useful for database row scanning.
func StatusStringToDomain(s string) jobtypes.JobStatus {
	status, err := jobtypes.ParseJobStatus(s)
	if err != nil {
		// Try common status mapping for backward compatibility
		commonStatus := common.JobStatus(s)
		return CommonStatusToDomain(commonStatus)
	}
	return status
}

// StatusStringToCommon converts a status string to common.JobStatus.
func StatusStringToCommon(s string) common.JobStatus {
	return common.JobStatus(s)
}
