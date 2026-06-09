// Package mappers provides conversion functions between API types (generated),
// domain types, and common types.
package mappers

import (
	"time"

	"github.com/google/uuid"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

// DomainJobToAPI converts a domain Job to the generated API Job type.
func DomainJobToAPI(job *jobtypes.Job) generated.Job {
	if job == nil {
		return generated.Job{}
	}

	apiJob := generated.Job{
		Id:           uuid.MustParse(job.ID),
		Name:         job.Name,
		Status:       DomainStatusToAPI(job.Status),
		CreatedAt:    job.CreatedAt,
		TargetBinary: job.TargetBinary,
		Fuzzer:       generated.FuzzerType(job.FuzzerType),
	}

	// Calculate TimeoutAt from MaxDuration
	if job.MaxDuration > 0 {
		timeoutAt := job.CreatedAt.Add(job.MaxDuration)
		apiJob.TimeoutAt = timeoutAt
	} else {
		apiJob.TimeoutAt = time.Now().Add(1 * time.Hour) // Default
	}

	// Use LockedBy as AssignedBotID
	if job.LockedBy != "" {
		// Note: In a real implementation, you'd need to look up the bot UUID
		// For now, we try to parse it as UUID or generate a deterministic one
		botID, err := uuid.Parse(job.LockedBy)
		if err != nil {
			// Generate a deterministic UUID from the bot name
			botID = uuid.NewSHA1(uuid.Nil, []byte(job.LockedBy))
		}
		apiJob.AssignedBotId = &botID
	}

	if job.StartedAt != nil {
		apiJob.StartedAt = job.StartedAt
	}

	if job.CompletedAt != nil {
		apiJob.CompletedAt = job.CompletedAt
	}

	// Check if coverage is enabled
	if job.EnableCoverage {
		apiJob.EnableCoverage = &job.EnableCoverage
	}

	if len(job.FuzzerConfig) > 0 {
		config := make(map[string]interface{})
		for k, v := range job.FuzzerConfig {
			config[k] = v
		}
		apiJob.Config = &config
	}

	// Set priority
	priority := int(job.Priority)
	apiJob.Priority = &priority

	return apiJob
}

// CommonJobToAPI converts a common.Job to the generated API Job type.
func CommonJobToAPI(job *common.Job) generated.Job {
	if job == nil {
		return generated.Job{}
	}

	apiJob := generated.Job{
		Id:           uuid.MustParse(job.ID),
		Name:         job.Name,
		Status:       CommonStatusToAPI(job.Status),
		CreatedAt:    job.CreatedAt,
		TargetBinary: job.Target,
		Fuzzer:       generated.FuzzerType(job.Fuzzer),
	}

	// Set TimeoutAt
	if job.Config.Duration > 0 {
		apiJob.TimeoutAt = job.CreatedAt.Add(job.Config.Duration)
	} else {
		apiJob.TimeoutAt = time.Now().Add(1 * time.Hour) // Default
	}

	// Set AssignedBotID
	if job.AssignedBot != nil && *job.AssignedBot != "" {
		botID, err := uuid.Parse(*job.AssignedBot)
		if err != nil {
			botID = uuid.NewSHA1(uuid.Nil, []byte(*job.AssignedBot))
		}
		apiJob.AssignedBotId = &botID
	}

	if job.StartedAt != nil {
		apiJob.StartedAt = job.StartedAt
	}

	if job.CompletedAt != nil {
		apiJob.CompletedAt = job.CompletedAt
	}

	// Coverage
	if job.EnableCoverage {
		apiJob.EnableCoverage = &job.EnableCoverage
	}

	// Priority
	apiJob.Priority = &job.Priority

	return apiJob
}

// DomainJobsToAPI converts a slice of domain Jobs to API Jobs.
func DomainJobsToAPI(jobs []*jobtypes.Job) []generated.Job {
	result := make([]generated.Job, len(jobs))
	for i, job := range jobs {
		result[i] = DomainJobToAPI(job)
	}
	return result
}

// CommonJobsToAPI converts a slice of common.Jobs to API Jobs.
func CommonJobsToAPI(jobs []*common.Job) []generated.Job {
	result := make([]generated.Job, len(jobs))
	for i, job := range jobs {
		result[i] = CommonJobToAPI(job)
	}
	return result
}

// Status conversion functions

// DomainStatusToAPI converts domain JobStatus to API JobStatus.
func DomainStatusToAPI(status jobtypes.JobStatus) generated.JobStatus {
	switch status {
	case jobtypes.StatusPending:
		return generated.JobStatusPending
	case jobtypes.StatusQueued:
		return generated.JobStatusAssigned
	case jobtypes.StatusRunning:
		return generated.JobStatusRunning
	case jobtypes.StatusCompleted:
		return generated.JobStatusCompleted
	case jobtypes.StatusFailed:
		return generated.JobStatusFailed
	case jobtypes.StatusCancelled:
		return generated.JobStatusCancelled
	default:
		return generated.JobStatusPending
	}
}

// CommonStatusToAPI converts common.JobStatus to API JobStatus.
func CommonStatusToAPI(status common.JobStatus) generated.JobStatus {
	switch status {
	case common.JobStatusPending:
		return generated.JobStatusPending
	case common.JobStatusAssigned:
		return generated.JobStatusAssigned
	case common.JobStatusStarting:
		return generated.JobStatusRunning // Map starting to running
	case common.JobStatusRunning:
		return generated.JobStatusRunning
	case common.JobStatusCompleted:
		return generated.JobStatusCompleted
	case common.JobStatusFailed:
		return generated.JobStatusFailed
	case common.JobStatusCancelled:
		return generated.JobStatusCancelled
	case common.JobStatusTimedOut:
		return generated.JobStatusTimeout
	default:
		return generated.JobStatusPending
	}
}

// APIStatusToDomain converts API JobStatus to domain JobStatus.
func APIStatusToDomain(status generated.JobStatus) jobtypes.JobStatus {
	switch status {
	case generated.JobStatusPending:
		return jobtypes.StatusPending
	case generated.JobStatusAssigned:
		return jobtypes.StatusQueued
	case generated.JobStatusRunning:
		return jobtypes.StatusRunning
	case generated.JobStatusCompleted:
		return jobtypes.StatusCompleted
	case generated.JobStatusFailed:
		return jobtypes.StatusFailed
	case generated.JobStatusCancelled:
		return jobtypes.StatusCancelled
	case generated.JobStatusTimeout:
		return jobtypes.StatusFailed // Map timeout to failed in domain
	default:
		return jobtypes.StatusPending
	}
}

// APIStatusToCommon converts API JobStatus to common.JobStatus.
func APIStatusToCommon(status generated.JobStatus) common.JobStatus {
	switch status {
	case generated.JobStatusPending:
		return common.JobStatusPending
	case generated.JobStatusAssigned:
		return common.JobStatusAssigned
	case generated.JobStatusRunning:
		return common.JobStatusRunning
	case generated.JobStatusCompleted:
		return common.JobStatusCompleted
	case generated.JobStatusFailed:
		return common.JobStatusFailed
	case generated.JobStatusCancelled:
		return common.JobStatusCancelled
	case generated.JobStatusTimeout:
		return common.JobStatusTimedOut
	default:
		return common.JobStatusPending
	}
}
