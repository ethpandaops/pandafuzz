package mappers

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/models"
)

// logger for mapper warnings (can be overridden)
var mapperLogger logrus.FieldLogger = logrus.NewEntry(logrus.StandardLogger())

// SetMapperLogger sets the logger for mapper warnings
func SetMapperLogger(logger logrus.FieldLogger) {
	if logger != nil {
		mapperLogger = logger
	}
}

// CommonJobToDomain converts common.Job to domain Job.
// Some fields may not have direct equivalents; these are logged as warnings.
func CommonJobToDomain(cj *common.Job) *jobtypes.Job {
	if cj == nil {
		return nil
	}

	dj := &jobtypes.Job{
		ID:           cj.ID,
		Name:         cj.Name,
		Status:       CommonStatusToDomain(cj.Status),
		FuzzerType:   cj.Fuzzer,
		TargetBinary: cj.Target,
		CreatedAt:    cj.CreatedAt,
		Priority:     CommonPriorityToDomain(cj.Priority),
		OutputPath:   cj.WorkDir,
		Metadata:     convertMetadataToStringMap(cj.Metadata),

		// Coverage fields
		EnableCoverage: cj.EnableCoverage,
		CoverageFormat: cj.CoverageFormat,

		// Lease fields
		LeaseToken:     cj.LeaseToken,
		LeaseExpiresAt: cj.LeaseExpiresAt,
		LastHeartbeat:  cj.LastHeartbeat,
	}

	// Handle pointer time fields
	if cj.StartedAt != nil {
		dj.StartedAt = cj.StartedAt
	}
	if cj.CompletedAt != nil {
		dj.CompletedAt = cj.CompletedAt
	}

	// Set UpdatedAt - use CreatedAt as fallback
	dj.UpdatedAt = cj.CreatedAt

	// Map JobConfig to FuzzerConfig
	dj.FuzzerConfig = make(map[string]any, 5)
	if cj.Config.Duration > 0 {
		dj.FuzzerConfig["duration"] = cj.Config.Duration
		dj.MaxDuration = cj.Config.Duration
	}
	if cj.Config.MemoryLimit > 0 {
		dj.FuzzerConfig["memory_limit"] = cj.Config.MemoryLimit
	}
	if cj.Config.Timeout > 0 {
		dj.FuzzerConfig["timeout"] = cj.Config.Timeout
	}
	if cj.Config.Dictionary != "" {
		dj.FuzzerConfig["dictionary"] = cj.Config.Dictionary
	}
	if len(cj.Config.SeedCorpus) > 0 {
		dj.FuzzerConfig["seed_corpus"] = cj.Config.SeedCorpus
	}

	// Fields that don't have direct mapping in domain Job:
	// - Type (JobType: fuzzing, minimization, reproduction) - logged as warning
	// - CampaignID, CollectionID, UseCampaignCorpus - not in domain Job
	if cj.Type != "" {
		mapperLogger.WithField("job_id", cj.ID).WithField("type", cj.Type).
			Debug("JobType field not mapped to domain Job")
	}

	return dj
}

// DomainJobToCommon converts domain Job to common.Job.
// Some fields may not have direct equivalents; these are logged as warnings.
func DomainJobToCommon(dj *jobtypes.Job) *common.Job {
	if dj == nil {
		return nil
	}

	cj := &common.Job{
		ID:             dj.ID,
		Name:           dj.Name,
		Target:         dj.TargetBinary,
		Fuzzer:         dj.FuzzerType,
		Status:         DomainStatusToCommon(dj.Status),
		CreatedAt:      dj.CreatedAt,
		WorkDir:        dj.OutputPath,
		Priority:       DomainPriorityToCommon(dj.Priority),
		EnableCoverage: dj.EnableCoverage,
		CoverageFormat: dj.CoverageFormat,
		LeaseToken:     dj.LeaseToken,
		LeaseExpiresAt: dj.LeaseExpiresAt,
		LastHeartbeat:  dj.LastHeartbeat,
	}

	// Handle pointer time fields
	cj.StartedAt = dj.StartedAt
	cj.CompletedAt = dj.CompletedAt

	// Map progress from domain Progress struct
	if dj.Progress != nil {
		// Use coverage percentage as approximate progress
		cj.Progress = int(dj.Progress.Coverage)
	}

	// Map FuzzerConfig back to JobConfig
	cj.Config = common.JobConfig{}
	if dj.MaxDuration > 0 {
		cj.Config.Duration = dj.MaxDuration
	}
	if dj.FuzzerConfig != nil {
		if ml, ok := dj.FuzzerConfig["memory_limit"].(int64); ok {
			cj.Config.MemoryLimit = ml
		}
		if to, ok := dj.FuzzerConfig["timeout"].(time.Duration); ok {
			cj.Config.Timeout = to
		}
		if dict, ok := dj.FuzzerConfig["dictionary"].(string); ok {
			cj.Config.Dictionary = dict
		}
	}

	// Convert string metadata back to interface map
	cj.Metadata = convertStringMapToMetadata(dj.Metadata)

	return cj
}

// JobRowToDomain converts a database JobRow to domain Job.
func JobRowToDomain(row *models.JobRow) *jobtypes.Job {
	if row == nil {
		return nil
	}

	job := &jobtypes.Job{
		ID:            row.ID,
		Name:          row.Name,
		Status:        StatusStringToDomain(row.Status),
		FuzzerType:    row.Fuzzer,
		TargetBinary:  row.Target,
		CreatedAt:     row.CreatedAt,
		Priority:      CommonPriorityToDomain(row.Priority),
		DequeueCount:  row.DequeueCount,
		RetryCount:    row.RetryCount,
		MaxRetries:    row.MaxRetries,
		RetryDelay:    time.Duration(row.RetryDelayNanos),
		ExecutionTime: time.Duration(row.ExecutionTimeNanos),
		CrashCount:    0, // Not stored directly, derived from crashes table

		// Coverage
		EnableCoverage: row.EnableCoverage,
	}

	// Handle nullable string fields
	if row.Description.Valid {
		job.Description = row.Description.String
	}
	if row.CoverageFormat.Valid {
		job.CoverageFormat = row.CoverageFormat.String
	}
	if row.CoverageReportID.Valid {
		job.CoverageReportID = row.CoverageReportID.String
	}
	if row.ErrorMessage.Valid {
		job.ErrorMessage = row.ErrorMessage.String
	}
	if row.LockedBy.Valid {
		job.LockedBy = row.LockedBy.String
	}
	if row.CorpusPath.Valid {
		job.CorpusPath = row.CorpusPath.String
	}
	if row.OutputPath.Valid {
		job.OutputPath = row.OutputPath.String
	} else {
		// Fallback to WorkDir for backward compatibility
		job.OutputPath = row.WorkDir
	}

	// Handle nullable time fields
	if row.StartedAt.Valid {
		job.StartedAt = &row.StartedAt.Time
	}
	if row.CompletedAt.Valid {
		job.CompletedAt = &row.CompletedAt.Time
	}
	if row.UpdatedAt.Valid {
		job.UpdatedAt = row.UpdatedAt.Time
	} else {
		job.UpdatedAt = row.CreatedAt
	}
	if row.ScheduledAt.Valid {
		job.ScheduledAt = &row.ScheduledAt.Time
	}
	if row.QueuedAt.Valid {
		job.QueuedAt = &row.QueuedAt.Time
	}
	if row.LockedAt.Valid {
		job.LockedAt = &row.LockedAt.Time
	}
	if row.LockExpiresAt.Valid {
		job.LockExpiresAt = &row.LockExpiresAt.Time
	}
	if row.LeaseToken.Valid {
		job.LeaseToken = &row.LeaseToken.String
	}
	if row.LeaseExpiresAt.Valid {
		job.LeaseExpiresAt = &row.LeaseExpiresAt.Time
	}
	if row.LastHeartbeat.Valid {
		job.LastHeartbeat = &row.LastHeartbeat.Time
	}

	// Parse config JSON into FuzzerConfig
	if row.ConfigJSON.Valid && row.ConfigJSON.String != "" {
		var config map[string]any
		if err := json.Unmarshal([]byte(row.ConfigJSON.String), &config); err == nil {
			job.FuzzerConfig = config
			// Extract MaxDuration if present
			if d, ok := config["duration"]; ok {
				switch v := d.(type) {
				case float64:
					job.MaxDuration = time.Duration(v)
				case int64:
					job.MaxDuration = time.Duration(v)
				}
			}
		}
	}

	// Parse metadata JSON
	if row.MetadataJSON.Valid && row.MetadataJSON.String != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(row.MetadataJSON.String), &metadata); err == nil {
			job.Metadata = make(map[string]string, len(metadata))
			for k, v := range metadata {
				if s, ok := v.(string); ok {
					job.Metadata[k] = s
				}
			}
		}
	}

	return job
}

// DomainJobToRow converts a domain Job to a database JobRow for insertion.
func DomainJobToRow(job *jobtypes.Job) *models.JobRow {
	if job == nil {
		return nil
	}

	row := &models.JobRow{
		ID:                 job.ID,
		Name:               job.Name,
		Target:             job.TargetBinary,
		Fuzzer:             job.FuzzerType,
		Status:             job.Status.String(),
		CreatedAt:          job.CreatedAt,
		TimeoutAt:          job.CreatedAt.Add(job.MaxDuration), // Calculate timeout
		WorkDir:            job.OutputPath,
		Progress:           0,
		Priority:           DomainPriorityToCommon(job.Priority),
		EnableCoverage:     job.EnableCoverage,
		DequeueCount:       job.DequeueCount,
		RetryCount:         job.RetryCount,
		MaxRetries:         job.MaxRetries,
		RetryDelayNanos:    int64(job.RetryDelay),
		ExecutionTimeNanos: int64(job.ExecutionTime),
	}

	// Handle nullable string fields
	if job.Description != "" {
		row.Description = sql.NullString{String: job.Description, Valid: true}
	}
	if job.CoverageFormat != "" {
		row.CoverageFormat = sql.NullString{String: job.CoverageFormat, Valid: true}
	}
	if job.CoverageReportID != "" {
		row.CoverageReportID = sql.NullString{String: job.CoverageReportID, Valid: true}
	}
	if job.ErrorMessage != "" {
		row.ErrorMessage = sql.NullString{String: job.ErrorMessage, Valid: true}
	}
	if job.LockedBy != "" {
		row.LockedBy = sql.NullString{String: job.LockedBy, Valid: true}
	}
	if job.CorpusPath != "" {
		row.CorpusPath = sql.NullString{String: job.CorpusPath, Valid: true}
	}
	if job.OutputPath != "" {
		row.OutputPath = sql.NullString{String: job.OutputPath, Valid: true}
	}

	// Handle nullable time fields
	if job.StartedAt != nil {
		row.StartedAt = sql.NullTime{Time: *job.StartedAt, Valid: true}
	}
	if job.CompletedAt != nil {
		row.CompletedAt = sql.NullTime{Time: *job.CompletedAt, Valid: true}
	}
	row.UpdatedAt = sql.NullTime{Time: job.UpdatedAt, Valid: true}
	if job.ScheduledAt != nil {
		row.ScheduledAt = sql.NullTime{Time: *job.ScheduledAt, Valid: true}
	}
	if job.QueuedAt != nil {
		row.QueuedAt = sql.NullTime{Time: *job.QueuedAt, Valid: true}
	}
	if job.LockedAt != nil {
		row.LockedAt = sql.NullTime{Time: *job.LockedAt, Valid: true}
	}
	if job.LockExpiresAt != nil {
		row.LockExpiresAt = sql.NullTime{Time: *job.LockExpiresAt, Valid: true}
	}
	if job.LeaseToken != nil {
		row.LeaseToken = sql.NullString{String: *job.LeaseToken, Valid: true}
	}
	if job.LeaseExpiresAt != nil {
		row.LeaseExpiresAt = sql.NullTime{Time: *job.LeaseExpiresAt, Valid: true}
	}
	if job.LastHeartbeat != nil {
		row.LastHeartbeat = sql.NullTime{Time: *job.LastHeartbeat, Valid: true}
	}

	// Serialize FuzzerConfig to JSON
	if job.FuzzerConfig != nil {
		if data, err := json.Marshal(job.FuzzerConfig); err == nil {
			row.ConfigJSON = sql.NullString{String: string(data), Valid: true}
		}
	}

	// Serialize Metadata to JSON
	if job.Metadata != nil {
		if data, err := json.Marshal(job.Metadata); err == nil {
			row.MetadataJSON = sql.NullString{String: string(data), Valid: true}
		}
	}

	return row
}

// Helper functions

// convertMetadataToStringMap converts map[string]interface{} to map[string]string.
// Non-string values are logged and skipped.
func convertMetadataToStringMap(m map[string]interface{}) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case string:
			result[k] = val
		case nil:
			// Skip nil values
		default:
			// Try JSON serialization for complex types
			if data, err := json.Marshal(val); err == nil {
				result[k] = string(data)
			} else {
				mapperLogger.WithField("key", k).WithField("type", v).
					Debug("Metadata value not convertible to string, skipping")
			}
		}
	}
	return result
}

// convertStringMapToMetadata converts map[string]string back to map[string]interface{}.
func convertStringMapToMetadata(m map[string]string) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
