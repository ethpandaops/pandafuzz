package models

import (
	"database/sql"
	"time"
)

// JobRow represents the database row for the jobs table.
// This is the "source of truth" for what's stored in the database.
// Field names match the existing database column names.
type JobRow struct {
	ID                string
	Name              string
	Target            string         // Target binary path
	Fuzzer            string         // Fuzzer type (afl++, libfuzzer, etc.)
	Type              sql.NullString // Job type (fuzzing, minimization, reproduction)
	Status            string
	AssignedBot       sql.NullString
	CreatedAt         time.Time
	StartedAt         sql.NullTime
	CompletedAt       sql.NullTime
	TimeoutAt         time.Time
	WorkDir           string
	ConfigJSON        sql.NullString // JSON blob for JobConfig
	Progress          int
	CampaignID        sql.NullString
	CollectionID      sql.NullString
	UseCampaignCorpus bool
	MetadataJSON      sql.NullString // JSON blob for metadata
	Priority          int
	EnableCoverage    bool
	CoverageFormat    sql.NullString
	CoverageReportID  sql.NullString
	LeaseToken        sql.NullString
	LeaseExpiresAt    sql.NullTime
	LastHeartbeat     sql.NullTime
	UpdatedAt         sql.NullTime

	// New scheduling fields (added via migration)
	Description        sql.NullString
	ScheduledAt        sql.NullTime
	QueuedAt           sql.NullTime
	DequeueCount       int
	RetryCount         int
	MaxRetries         int
	RetryDelayNanos    int64
	LockedBy           sql.NullString
	LockedAt           sql.NullTime
	LockExpiresAt      sql.NullTime
	ErrorMessage       sql.NullString
	ExecutionTimeNanos int64
	CorpusPath         sql.NullString
	OutputPath         sql.NullString
}

// BotRow represents the database row for the bots table.
type BotRow struct {
	ID               string
	Name             string
	Hostname         string
	Status           string
	LastSeen         time.Time
	RegisteredAt     time.Time
	CurrentJob       sql.NullString
	CapabilitiesJSON sql.NullString // JSON array
	TimeoutAt        time.Time
	IsOnline         bool
	FailureCount     int
	APIEndpoint      string
	CreatedAt        sql.NullTime
	UpdatedAt        sql.NullTime
}

// CrashRow represents the database row for the crashes table.
type CrashRow struct {
	ID         string
	JobID      string
	BotID      string
	Hash       string
	FilePath   string
	Type       string
	Signal     sql.NullInt64
	ExitCode   sql.NullInt64
	Timestamp  time.Time
	Size       sql.NullInt64
	IsUnique   bool
	Output     sql.NullString
	StackTrace sql.NullString
	CreatedAt  sql.NullTime
}

// CoverageRow represents the database row for the coverage table.
type CoverageRow struct {
	ID        string
	JobID     string
	BotID     string
	Edges     int
	NewEdges  int
	Timestamp time.Time
	ExecCount int64
	CreatedAt sql.NullTime
}

// CampaignRow represents the database row for the campaigns table.
type CampaignRow struct {
	ID           string
	Name         string
	Description  sql.NullString
	Status       string
	TargetBinary string
	BinaryHash   sql.NullString
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CompletedAt  sql.NullTime
	AutoRestart  bool
	MaxDuration  int64 // Duration in nanoseconds
	MaxJobs      int
	JobTemplate  sql.NullString // JSON blob
	SharedCorpus bool
	TagsJSON     sql.NullString // JSON array
}

// JobLogRow represents the database row for job_logs table.
type JobLogRow struct {
	ID           int64
	JobID        string
	Level        string
	Source       sql.NullString
	Message      string
	Timestamp    time.Time
	MetadataJSON sql.NullString // JSON blob
	CreatedAt    sql.NullTime
}

// JobDependencyRow represents the database row for job_dependencies table.
type JobDependencyRow struct {
	JobID       string
	DependsOnID string
	CreatedAt   time.Time
}
