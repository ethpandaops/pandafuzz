package types

import (
	"errors"
	"fmt"
	"time"
)

// Job represents a fuzzing job execution
type Job struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	Status        JobStatus         `json:"status"`
	FuzzerType    string            `json:"fuzzer_type"`
	FuzzerConfig  map[string]any    `json:"fuzzer_config"`
	TargetBinary  string            `json:"target_binary"`
	TargetArgs    []string          `json:"target_args,omitempty"`
	Environment   map[string]string `json:"environment,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	StartedAt     *time.Time        `json:"started_at,omitempty"`
	CompletedAt   *time.Time        `json:"completed_at,omitempty"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Priority      JobPriority       `json:"priority"`
	MaxDuration   time.Duration     `json:"max_duration,omitempty"`
	CorpusPath    string            `json:"corpus_path"`
	OutputPath    string            `json:"output_path"`
	CrashCount    uint64            `json:"crash_count"`
	ExecutionTime time.Duration     `json:"execution_time"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	ErrorMessage  string            `json:"error_message,omitempty"`
	Progress      *JobProgress      `json:"progress,omitempty"`

	// Scheduling fields
	ScheduledAt  *time.Time    `json:"scheduled_at,omitempty"` // When the job should be executed
	RetryCount   int           `json:"retry_count"`            // Number of retry attempts
	MaxRetries   int           `json:"max_retries"`            // Maximum retry attempts allowed
	RetryDelay   time.Duration `json:"retry_delay,omitempty"`  // Delay between retries
	Dependencies []string      `json:"dependencies,omitempty"` // IDs of jobs this job depends on

	// Processing fields
	LockedBy      string     `json:"locked_by,omitempty"`       // Worker ID that has locked this job
	LockedAt      *time.Time `json:"locked_at,omitempty"`       // When the job was locked
	LockExpiresAt *time.Time `json:"lock_expires_at,omitempty"` // When the lock expires

	// Lease management fields
	LeaseToken     *string    `json:"lease_token,omitempty"`      // Secure token for lease validation
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"` // When the lease expires
	LastHeartbeat  *time.Time `json:"last_heartbeat,omitempty"`   // Last heartbeat from assigned bot

	// Queue tracking
	QueuedAt     *time.Time `json:"queued_at,omitempty"` // When the job was added to queue
	DequeueCount int        `json:"dequeue_count"`       // Number of times dequeued

	// Coverage tracking
	EnableCoverage   bool           `json:"enable_coverage"`
	CoverageFormat   string         `json:"coverage_format,omitempty"`
	CoverageReportID string         `json:"coverage_report_id,omitempty"`
	CoverageStats    *CoverageStats `json:"coverage_stats,omitempty"`
}

// JobPriority represents the priority level of a job
type JobPriority int

const (
	PriorityLow JobPriority = iota
	PriorityNormal
	PriorityHigh
	PriorityCritical
)

// String returns the string representation of JobPriority
func (p JobPriority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityNormal:
		return "normal"
	case PriorityHigh:
		return "high"
	case PriorityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// JobProgress tracks the progress of a running job
type JobProgress struct {
	TotalExecs        uint64         `json:"total_execs"`
	ExecsPerSecond    uint64         `json:"execs_per_second"`
	CorpusSize        uint64         `json:"corpus_size"`
	Coverage          float64        `json:"coverage"`
	LastUpdated       time.Time      `json:"last_updated"`
	EstimatedTimeLeft *time.Duration `json:"estimated_time_left,omitempty"`
}

// CoverageStats contains detailed coverage information for a job
type CoverageStats struct {
	LineCoverage     float64   `json:"line_coverage"`
	FunctionCoverage float64   `json:"function_coverage"`
	BranchCoverage   float64   `json:"branch_coverage,omitempty"`
	CollectedAt      time.Time `json:"collected_at"`
	ReportPath       string    `json:"report_path"`
}

// NewJob creates a new job instance with required fields
func NewJob(name, fuzzerType, targetBinary, corpusPath, outputPath string) (*Job, error) {
	if name == "" {
		return nil, errors.New("job name cannot be empty")
	}
	if fuzzerType == "" {
		return nil, errors.New("fuzzer type cannot be empty")
	}
	if targetBinary == "" {
		return nil, errors.New("target binary cannot be empty")
	}
	if corpusPath == "" {
		return nil, errors.New("corpus path cannot be empty")
	}
	if outputPath == "" {
		return nil, errors.New("output path cannot be empty")
	}

	now := time.Now().UTC()
	job := &Job{
		ID:           generateJobID(name, now),
		Name:         name,
		Status:       StatusPending,
		FuzzerType:   fuzzerType,
		FuzzerConfig: make(map[string]any),
		TargetBinary: targetBinary,
		TargetArgs:   make([]string, 0),
		Environment:  make(map[string]string),
		CreatedAt:    now,
		UpdatedAt:    now,
		Priority:     PriorityNormal,
		CorpusPath:   corpusPath,
		OutputPath:   outputPath,
		CrashCount:   0,
		Metadata:     make(map[string]string),
		Tags:         make([]string, 0),
	}

	return job, nil
}

// generateJobID creates a unique ID for the job
func generateJobID(name string, timestamp time.Time) string {
	// Simple ID generation - in production might use UUID
	return fmt.Sprintf("job_%s_%d", name, timestamp.UnixNano())
}

// Start marks the job as started
func (j *Job) Start() error {
	if j.Status != StatusPending && j.Status != StatusQueued {
		return fmt.Errorf("cannot start job in status: %s", j.Status)
	}

	now := time.Now().UTC()
	j.Status = StatusRunning
	j.StartedAt = &now
	j.UpdatedAt = now
	return nil
}

// Complete marks the job as completed successfully
func (j *Job) Complete() error {
	if j.Status != StatusRunning {
		return fmt.Errorf("cannot complete job in status: %s", j.Status)
	}

	now := time.Now().UTC()
	j.Status = StatusCompleted
	j.CompletedAt = &now
	j.UpdatedAt = now

	if j.StartedAt != nil {
		j.ExecutionTime = now.Sub(*j.StartedAt)
	}

	return nil
}

// Fail marks the job as failed with an error message
func (j *Job) Fail(errorMessage string) error {
	if j.Status == StatusCompleted || j.Status == StatusFailed {
		return fmt.Errorf("cannot fail job in status: %s", j.Status)
	}

	now := time.Now().UTC()
	j.Status = StatusFailed
	j.CompletedAt = &now
	j.UpdatedAt = now
	j.ErrorMessage = errorMessage

	if j.StartedAt != nil {
		j.ExecutionTime = now.Sub(*j.StartedAt)
	}

	return nil
}

// Cancel marks the job as cancelled
func (j *Job) Cancel() error {
	if j.Status == StatusCompleted || j.Status == StatusFailed {
		return fmt.Errorf("cannot cancel job in status: %s", j.Status)
	}

	now := time.Now().UTC()
	j.Status = StatusCancelled
	j.CompletedAt = &now
	j.UpdatedAt = now

	if j.StartedAt != nil {
		j.ExecutionTime = now.Sub(*j.StartedAt)
	}

	return nil
}

// UpdateProgress updates the job's progress information
func (j *Job) UpdateProgress(progress *JobProgress) error {
	if j.Status != StatusRunning {
		return fmt.Errorf("cannot update progress for job in status: %s", j.Status)
	}

	j.Progress = progress
	j.UpdatedAt = time.Now().UTC()
	return nil
}

// IncrementCrashCount increments the crash counter
func (j *Job) IncrementCrashCount() {
	j.CrashCount++
	j.UpdatedAt = time.Now().UTC()
}

// SetFuzzerConfig sets a fuzzer configuration parameter
func (j *Job) SetFuzzerConfig(key string, value any) {
	if j.FuzzerConfig == nil {
		j.FuzzerConfig = make(map[string]any)
	}
	j.FuzzerConfig[key] = value
	j.UpdatedAt = time.Now().UTC()
}

// GetFuzzerConfig retrieves a fuzzer configuration value
func (j *Job) GetFuzzerConfig(key string) (any, bool) {
	value, exists := j.FuzzerConfig[key]
	return value, exists
}

// SetEnvironment sets an environment variable for the job
func (j *Job) SetEnvironment(key, value string) {
	if j.Environment == nil {
		j.Environment = make(map[string]string)
	}
	j.Environment[key] = value
	j.UpdatedAt = time.Now().UTC()
}

// SetMetadata sets a metadata key-value pair
func (j *Job) SetMetadata(key, value string) {
	if j.Metadata == nil {
		j.Metadata = make(map[string]string)
	}
	j.Metadata[key] = value
	j.UpdatedAt = time.Now().UTC()
}

// GetMetadata retrieves a metadata value
func (j *Job) GetMetadata(key string) (string, bool) {
	value, exists := j.Metadata[key]
	return value, exists
}

// AddTag adds a tag to the job
func (j *Job) AddTag(tag string) {
	for _, t := range j.Tags {
		if t == tag {
			return // Tag already exists
		}
	}
	j.Tags = append(j.Tags, tag)
	j.UpdatedAt = time.Now().UTC()
}

// RemoveTag removes a tag from the job
func (j *Job) RemoveTag(tag string) {
	for i, t := range j.Tags {
		if t == tag {
			j.Tags = append(j.Tags[:i], j.Tags[i+1:]...)
			j.UpdatedAt = time.Now().UTC()
			return
		}
	}
}

// IsRunning checks if the job is currently running
func (j *Job) IsRunning() bool {
	return j.Status == StatusRunning
}

// IsFinished checks if the job has finished (completed, failed, or cancelled)
func (j *Job) IsFinished() bool {
	return j.Status == StatusCompleted || j.Status == StatusFailed || j.Status == StatusCancelled
}

// CanTransitionTo checks if the job can transition to the given status
func (j *Job) CanTransitionTo(status JobStatus) bool {
	return j.Status.CanTransitionTo(status)
}

// Duration returns the job's execution duration
func (j *Job) Duration() time.Duration {
	if j.StartedAt == nil {
		return 0
	}

	if j.CompletedAt != nil {
		return j.CompletedAt.Sub(*j.StartedAt)
	}

	// Job is still running
	return time.Since(*j.StartedAt)
}

// IsExpired checks if the job has exceeded its maximum duration
func (j *Job) IsExpired() bool {
	if j.MaxDuration == 0 || !j.IsRunning() {
		return false
	}

	return j.Duration() > j.MaxDuration
}

// Validate ensures the job is in a valid state
func (j *Job) Validate() error {
	if j.ID == "" {
		return errors.New("job ID cannot be empty")
	}
	if j.Name == "" {
		return errors.New("job name cannot be empty")
	}
	if j.FuzzerType == "" {
		return errors.New("fuzzer type cannot be empty")
	}
	if j.TargetBinary == "" {
		return errors.New("target binary cannot be empty")
	}
	if j.CorpusPath == "" {
		return errors.New("corpus path cannot be empty")
	}
	if j.OutputPath == "" {
		return errors.New("output path cannot be empty")
	}
	if !j.Status.IsValid() {
		return fmt.Errorf("invalid job status: %s", j.Status)
	}
	if j.Priority < PriorityLow || j.Priority > PriorityCritical {
		return fmt.Errorf("invalid job priority: %d", j.Priority)
	}

	// Validate coverage format if coverage is enabled
	if j.EnableCoverage && j.CoverageFormat != "" {
		if !isValidCoverageFormat(j.CoverageFormat) {
			return fmt.Errorf("invalid coverage format: %s (must be one of: json, html, lcov, cobertura)", j.CoverageFormat)
		}
	}

	return nil
}

// String returns a string representation of the job
func (j *Job) String() string {
	return fmt.Sprintf("Job[ID=%s, Name=%s, Status=%s, Fuzzer=%s, Target=%s, Crashes=%d]",
		j.ID, j.Name, j.Status, j.FuzzerType, j.TargetBinary, j.CrashCount)
}

// IsScheduled checks if the job is scheduled for future execution
func (j *Job) IsScheduled() bool {
	if j.ScheduledAt == nil {
		return false
	}
	return j.ScheduledAt.After(time.Now().UTC())
}

// IsReadyToRun checks if the job is ready to be executed
func (j *Job) IsReadyToRun() bool {
	// Check if job is in correct status
	if j.Status != StatusPending && j.Status != StatusQueued {
		return false
	}

	// Check if scheduled time has passed
	if j.ScheduledAt != nil && j.ScheduledAt.After(time.Now().UTC()) {
		return false
	}

	return true
}

// CanRetry checks if the job can be retried
func (j *Job) CanRetry() bool {
	return j.Status == StatusFailed && j.RetryCount < j.MaxRetries
}

// IncrementRetries increments the retry counter and calculates next retry time
func (j *Job) IncrementRetries() {
	j.RetryCount++
	j.UpdatedAt = time.Now().UTC()

	// Calculate exponential backoff if retry delay is set
	if j.RetryDelay > 0 {
		backoffDelay := j.RetryDelay * time.Duration(1<<uint(j.RetryCount-1))
		nextRetry := time.Now().UTC().Add(backoffDelay)
		j.ScheduledAt = &nextRetry
	}
}

// Lock marks the job as locked by a worker
func (j *Job) Lock(workerID string, lockDuration time.Duration) error {
	if j.Status != StatusQueued {
		return fmt.Errorf("cannot lock job in status: %s", j.Status)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(lockDuration)

	j.LockedBy = workerID
	j.LockedAt = &now
	j.LockExpiresAt = &expiresAt
	j.UpdatedAt = now

	return nil
}

// Unlock releases the lock on the job
func (j *Job) Unlock() {
	j.LockedBy = ""
	j.LockedAt = nil
	j.LockExpiresAt = nil
	j.UpdatedAt = time.Now().UTC()
}

// IsLocked checks if the job is currently locked
func (j *Job) IsLocked() bool {
	if j.LockedBy == "" || j.LockExpiresAt == nil {
		return false
	}
	return j.LockExpiresAt.After(time.Now().UTC())
}

// HasDependencies checks if the job has dependencies
func (j *Job) HasDependencies() bool {
	return len(j.Dependencies) > 0
}

// AddDependency adds a job ID to the dependencies list
func (j *Job) AddDependency(jobID string) error {
	// Check for self-dependency
	if jobID == j.ID {
		return errors.New("job cannot depend on itself")
	}

	// Check if dependency already exists
	for _, dep := range j.Dependencies {
		if dep == jobID {
			return fmt.Errorf("dependency %s already exists", jobID)
		}
	}

	j.Dependencies = append(j.Dependencies, jobID)
	j.UpdatedAt = time.Now().UTC()
	return nil
}

// RemoveDependency removes a job ID from the dependencies list
func (j *Job) RemoveDependency(jobID string) {
	for i, dep := range j.Dependencies {
		if dep == jobID {
			j.Dependencies = append(j.Dependencies[:i], j.Dependencies[i+1:]...)
			j.UpdatedAt = time.Now().UTC()
			return
		}
	}
}

// Queue marks the job as queued
func (j *Job) Queue() error {
	if j.Status != StatusPending {
		return fmt.Errorf("cannot queue job in status: %s", j.Status)
	}

	now := time.Now().UTC()
	j.Status = StatusQueued
	j.QueuedAt = &now
	j.UpdatedAt = now
	j.DequeueCount++

	return nil
}

// IsCoverageEnabled checks if coverage collection is enabled for this job
func (j *Job) IsCoverageEnabled() bool {
	return j.EnableCoverage
}

// UpdateCoverageStats updates the job's coverage statistics
func (j *Job) UpdateCoverageStats(stats *CoverageStats) error {
	if !j.EnableCoverage {
		return errors.New("cannot update coverage stats when coverage is disabled")
	}
	if stats == nil {
		return errors.New("coverage stats cannot be nil")
	}

	j.CoverageStats = stats
	j.UpdatedAt = time.Now().UTC()
	return nil
}

// isValidCoverageFormat validates the coverage format
func isValidCoverageFormat(format string) bool {
	switch format {
	case "json", "html", "lcov", "cobertura":
		return true
	default:
		return false
	}
}
