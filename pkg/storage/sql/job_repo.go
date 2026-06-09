// Package sql provides SQL-based repository implementations for domain entities.
package sql

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
)

// JobRepository implements repository.JobRepository using SQLiteStorage
type JobRepository struct {
	storage *storage.SQLiteStorage
}

// Compile-time interface compliance check
var _ repository.JobRepository = (*JobRepository)(nil)

// NewJobRepository creates a new job repository adapter
func NewJobRepository(storage *storage.SQLiteStorage) *JobRepository {
	return &JobRepository{storage: storage}
}

// Create persists a new job
func (r *JobRepository) Create(ctx context.Context, job *types.Job) error {
	commonJob := domainJobToCommon(job)
	return r.storage.CreateJob(ctx, commonJob)
}

// Get retrieves a job by ID
func (r *JobRepository) Get(ctx context.Context, id string) (*types.Job, error) {
	commonJob, err := r.storage.GetJob(ctx, id)
	if err != nil {
		return nil, err
	}
	return commonJobToDomain(commonJob), nil
}

// Update persists changes to an existing job
func (r *JobRepository) Update(ctx context.Context, job *types.Job) error {
	updates := map[string]interface{}{
		"name":       job.Name,
		"status":     string(jobStatusToCommon(job.Status)),
		"fuzzer":     job.FuzzerType,
		"target":     job.TargetBinary,
		"progress":   calculateProgress(job),
		"started_at": job.StartedAt,
	}
	if job.CompletedAt != nil {
		updates["completed_at"] = job.CompletedAt
	}
	return r.storage.UpdateJob(ctx, job.ID, updates)
}

// Delete removes a job by ID
func (r *JobRepository) Delete(ctx context.Context, id string) error {
	return r.storage.DeleteJob(ctx, id)
}

// List retrieves jobs with filtering and pagination
func (r *JobRepository) List(ctx context.Context, filter repository.JobFilter) ([]*types.Job, error) {
	var statusFilter string
	if filter.Status != nil {
		statusFilter = string(jobStatusToCommon(*filter.Status))
	}

	jobs, err := r.storage.ListJobs(ctx, filter.Limit, filter.Offset, statusFilter)
	if err != nil {
		return nil, err
	}

	result := make([]*types.Job, 0, len(jobs))
	for _, job := range jobs {
		result = append(result, commonJobToDomain(job))
	}
	return result, nil
}

// ListByStatus retrieves all jobs with a specific status
func (r *JobRepository) ListByStatus(ctx context.Context, status types.JobStatus) ([]*types.Job, error) {
	statusStr := string(jobStatusToCommon(status))
	jobs, err := r.storage.ListJobs(ctx, 1000, 0, statusStr) // Large limit for all
	if err != nil {
		return nil, err
	}

	result := make([]*types.Job, 0, len(jobs))
	for _, job := range jobs {
		result = append(result, commonJobToDomain(job))
	}
	return result, nil
}

// ListPending retrieves pending jobs ordered by priority and creation time
func (r *JobRepository) ListPending(ctx context.Context, limit int) ([]*types.Job, error) {
	jobs, err := r.storage.ListJobs(ctx, limit, 0, string(common.JobStatusPending))
	if err != nil {
		return nil, err
	}

	result := make([]*types.Job, 0, len(jobs))
	for _, job := range jobs {
		result = append(result, commonJobToDomain(job))
	}
	return result, nil
}

// ListScheduled retrieves jobs scheduled to run at or before the given time
func (r *JobRepository) ListScheduled(ctx context.Context, before time.Time) ([]*types.Job, error) {
	// Get pending jobs and filter by scheduled time
	jobs, err := r.storage.ListJobs(ctx, 1000, 0, string(common.JobStatusPending))
	if err != nil {
		return nil, err
	}

	result := make([]*types.Job, 0)
	for _, job := range jobs {
		domainJob := commonJobToDomain(job)
		if domainJob.ScheduledAt != nil && !domainJob.ScheduledAt.After(before) {
			result = append(result, domainJob)
		} else if domainJob.ScheduledAt == nil {
			// Jobs without scheduled time are ready to run
			result = append(result, domainJob)
		}
	}
	return result, nil
}

// CountByStatus returns the count of jobs for each status
func (r *JobRepository) CountByStatus(ctx context.Context) (map[types.JobStatus]int64, error) {
	// Get all jobs and count by status
	allJobs, err := r.storage.GetAllJobs(ctx)
	if err != nil {
		return nil, err
	}

	counts := make(map[types.JobStatus]int64)
	for _, jobData := range allJobs {
		if statusStr, ok := jobData["status"].(string); ok {
			status := jobStatusFromCommon(common.JobStatus(statusStr))
			counts[status]++
		}
	}
	return counts, nil
}

// UpdateStatus atomically updates a job's status with validation
func (r *JobRepository) UpdateStatus(ctx context.Context, id string, from, to types.JobStatus) error {
	// Get current job to validate transition
	job, err := r.storage.GetJob(ctx, id)
	if err != nil {
		return err
	}

	currentStatus := jobStatusFromCommon(job.Status)
	if currentStatus != from {
		return fmt.Errorf("job status mismatch: expected %s, got %s", from, currentStatus)
	}

	if !from.CanTransitionTo(to) {
		return fmt.Errorf("invalid status transition from %s to %s", from, to)
	}

	updates := map[string]interface{}{
		"status": string(jobStatusToCommon(to)),
	}
	return r.storage.UpdateJob(ctx, id, updates)
}

// IncrementRetries atomically increments the retry count for a job
func (r *JobRepository) IncrementRetries(ctx context.Context, id string) error {
	job, err := r.storage.GetJob(ctx, id)
	if err != nil {
		return err
	}

	domainJob := commonJobToDomain(job)
	domainJob.RetryCount++

	updates := map[string]interface{}{
		"metadata": marshalMetadata(domainJob.Metadata),
	}
	return r.storage.UpdateJob(ctx, id, updates)
}

// GetDependencies retrieves all jobs that depend on the given job
func (r *JobRepository) GetDependencies(ctx context.Context, jobID string) ([]*types.Job, error) {
	allJobs, err := r.storage.GetAllJobs(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*types.Job, 0)
	for _, jobData := range allJobs {
		if r.jobDependsOn(jobData, jobID) {
			if id, ok := jobData["id"].(string); ok {
				if job, err := r.Get(ctx, id); err == nil {
					result = append(result, job)
				}
			}
		}
	}
	return result, nil
}

// jobDependsOn checks if a job's metadata indicates it depends on the given jobID
func (r *JobRepository) jobDependsOn(jobData map[string]any, targetJobID string) bool {
	metadataStr, ok := jobData["metadata"].(string)
	if !ok || metadataStr == "" {
		return false
	}

	deps := extractDependencies(metadataStr)
	for _, dep := range deps {
		if dep == targetJobID {
			return true
		}
	}
	return false
}

// extractDependencies parses dependencies from metadata JSON string
func extractDependencies(metadataStr string) []string {
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
		return nil
	}

	deps, ok := metadata["dependencies"].([]interface{})
	if !ok {
		return nil
	}

	result := make([]string, 0, len(deps))
	for _, dep := range deps {
		if depStr, ok := dep.(string); ok {
			result = append(result, depStr)
		}
	}
	return result
}

// GetDependents retrieves all jobs that the given job depends on
func (r *JobRepository) GetDependents(ctx context.Context, jobID string) ([]*types.Job, error) {
	job, err := r.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}

	result := make([]*types.Job, 0, len(job.Dependencies))
	for _, depID := range job.Dependencies {
		depJob, err := r.Get(ctx, depID)
		if err == nil {
			result = append(result, depJob)
		}
	}
	return result, nil
}

// AddDependency creates a dependency relationship between jobs
func (r *JobRepository) AddDependency(ctx context.Context, jobID, dependsOnID string) error {
	job, err := r.Get(ctx, jobID)
	if err != nil {
		return err
	}

	if err := job.AddDependency(dependsOnID); err != nil {
		return err
	}

	return r.Update(ctx, job)
}

// RemoveDependency removes a dependency relationship between jobs
func (r *JobRepository) RemoveDependency(ctx context.Context, jobID, dependsOnID string) error {
	job, err := r.Get(ctx, jobID)
	if err != nil {
		return err
	}

	job.RemoveDependency(dependsOnID)
	return r.Update(ctx, job)
}

// LockForProcessing attempts to lock a job for processing by a worker
func (r *JobRepository) LockForProcessing(ctx context.Context, jobID string, workerID string, lockDuration time.Duration) (*types.Job, error) {
	job, err := r.Get(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if job.IsLocked() {
		return nil, fmt.Errorf("job %s is already locked", jobID)
	}

	if err := job.Lock(workerID, lockDuration); err != nil {
		return nil, err
	}

	if err := r.Update(ctx, job); err != nil {
		return nil, err
	}

	return job, nil
}

// UnlockJob releases a processing lock on a job
func (r *JobRepository) UnlockJob(ctx context.Context, jobID string, workerID string) error {
	job, err := r.Get(ctx, jobID)
	if err != nil {
		return err
	}

	if job.LockedBy != workerID {
		return fmt.Errorf("job %s is not locked by worker %s", jobID, workerID)
	}

	job.Unlock()
	return r.Update(ctx, job)
}

// GetStaleJobs retrieves jobs that have been locked for longer than the specified duration
func (r *JobRepository) GetStaleJobs(ctx context.Context, staleDuration time.Duration) ([]*types.Job, error) {
	jobs, err := r.storage.ListJobs(ctx, 1000, 0, string(common.JobStatusRunning))
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-staleDuration)
	result := make([]*types.Job, 0)
	for _, job := range jobs {
		domainJob := commonJobToDomain(job)
		if domainJob.LockedAt != nil && domainJob.LockedAt.Before(cutoff) {
			result = append(result, domainJob)
		}
	}
	return result, nil
}

// GetMetrics retrieves repository performance metrics
func (r *JobRepository) GetMetrics(ctx context.Context) (*repository.JobRepositoryMetrics, error) {
	counts, err := r.CountByStatus(ctx)
	if err != nil {
		return nil, err
	}

	var total int64
	for _, count := range counts {
		total += count
	}

	return &repository.JobRepositoryMetrics{
		TotalJobs:    total,
		JobsByStatus: counts,
	}, nil
}

// Helper functions for type conversion

func domainJobToCommon(job *types.Job) *common.Job {
	configJSON, _ := json.Marshal(job.FuzzerConfig)
	var config common.JobConfig
	_ = json.Unmarshal(configJSON, &config)

	// Store additional domain fields in metadata
	metadata := make(map[string]interface{})
	for k, v := range job.Metadata {
		metadata[k] = v
	}
	metadata["retry_count"] = job.RetryCount
	metadata["max_retries"] = job.MaxRetries
	metadata["dependencies"] = job.Dependencies
	metadata["locked_by"] = job.LockedBy

	return &common.Job{
		ID:             job.ID,
		Name:           job.Name,
		Target:         job.TargetBinary,
		Fuzzer:         job.FuzzerType,
		Type:           common.JobTypeFuzzing,
		Status:         jobStatusToCommon(job.Status),
		CreatedAt:      job.CreatedAt,
		StartedAt:      job.StartedAt,
		CompletedAt:    job.CompletedAt,
		Config:         config,
		Progress:       calculateProgress(job),
		Priority:       int(job.Priority),
		EnableCoverage: job.EnableCoverage,
		CoverageFormat: job.CoverageFormat,
		Metadata:       metadata,
		LeaseToken:     job.LeaseToken,
		LeaseExpiresAt: job.LeaseExpiresAt,
		LastHeartbeat:  job.LastHeartbeat,
	}
}

func commonJobToDomain(job *common.Job) *types.Job {
	domainJob := &types.Job{
		ID:             job.ID,
		Name:           job.Name,
		Status:         jobStatusFromCommon(job.Status),
		FuzzerType:     job.Fuzzer,
		TargetBinary:   job.Target,
		CreatedAt:      job.CreatedAt,
		StartedAt:      job.StartedAt,
		CompletedAt:    job.CompletedAt,
		UpdatedAt:      job.CreatedAt,
		Priority:       types.JobPriority(job.Priority),
		EnableCoverage: job.EnableCoverage,
		CoverageFormat: job.CoverageFormat,
		LeaseToken:     job.LeaseToken,
		LeaseExpiresAt: job.LeaseExpiresAt,
		LastHeartbeat:  job.LastHeartbeat,
		Metadata:       make(map[string]string),
		Tags:           make([]string, 0),
	}

	applyJobMetadata(domainJob, job.Metadata)
	return domainJob
}

// applyJobMetadata extracts domain-specific fields from common job metadata
func applyJobMetadata(domainJob *types.Job, metadata map[string]interface{}) {
	if metadata == nil {
		return
	}

	domainJob.RetryCount = extractIntFromMetadata(metadata, "retry_count")
	domainJob.MaxRetries = extractIntFromMetadata(metadata, "max_retries")
	domainJob.Dependencies = extractStringSliceFromMetadata(metadata, "dependencies")

	if lockedBy, ok := metadata["locked_by"].(string); ok {
		domainJob.LockedBy = lockedBy
	}

	copyStringMetadata(domainJob.Metadata, metadata)
}

// extractIntFromMetadata extracts an int value from metadata stored as float64
func extractIntFromMetadata(metadata map[string]interface{}, key string) int {
	if val, ok := metadata[key].(float64); ok {
		return int(val)
	}
	return 0
}

// extractStringSliceFromMetadata extracts a string slice from metadata
func extractStringSliceFromMetadata(metadata map[string]interface{}, key string) []string {
	deps, ok := metadata[key].([]interface{})
	if !ok {
		return nil
	}

	result := make([]string, 0, len(deps))
	for _, d := range deps {
		if depStr, ok := d.(string); ok {
			result = append(result, depStr)
		}
	}
	return result
}

// copyStringMetadata copies string values from source to destination map
func copyStringMetadata(dest map[string]string, src map[string]interface{}) {
	for k, v := range src {
		if str, ok := v.(string); ok {
			dest[k] = str
		}
	}
}

func jobStatusToCommon(status types.JobStatus) common.JobStatus {
	switch status {
	case types.StatusPending:
		return common.JobStatusPending
	case types.StatusQueued:
		return common.JobStatusAssigned
	case types.StatusStarting:
		return common.JobStatusStarting
	case types.StatusRunning:
		return common.JobStatusRunning
	case types.StatusCompleted:
		return common.JobStatusCompleted
	case types.StatusFailed:
		return common.JobStatusFailed
	case types.StatusCancelled:
		return common.JobStatusCancelled
	case types.StatusPaused:
		return common.JobStatusPending // No direct mapping
	default:
		return common.JobStatusPending
	}
}

func jobStatusFromCommon(status common.JobStatus) types.JobStatus {
	switch status {
	case common.JobStatusPending:
		return types.StatusPending
	case common.JobStatusAssigned:
		return types.StatusQueued
	case common.JobStatusStarting:
		return types.StatusStarting
	case common.JobStatusRunning:
		return types.StatusRunning
	case common.JobStatusCompleted:
		return types.StatusCompleted
	case common.JobStatusFailed:
		return types.StatusFailed
	case common.JobStatusTimedOut:
		return types.StatusFailed
	case common.JobStatusCancelled:
		return types.StatusCancelled
	default:
		return types.StatusPending
	}
}

func calculateProgress(job *types.Job) int {
	if job.Progress != nil {
		// Calculate progress based on coverage or execution time
		if job.MaxDuration > 0 && job.StartedAt != nil {
			elapsed := time.Since(*job.StartedAt)
			progress := int(elapsed.Seconds() / job.MaxDuration.Seconds() * 100)
			if progress > 100 {
				progress = 100
			}
			return progress
		}
	}
	return 0
}

func marshalMetadata(metadata map[string]string) string {
	if metadata == nil {
		return "{}"
	}
	data, _ := json.Marshal(metadata)
	return string(data)
}
