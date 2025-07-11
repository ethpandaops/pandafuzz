package model

// SortOrder represents the sort direction for API queries
type SortOrder string

const (
	// SortOrderAsc represents ascending sort order
	SortOrderAsc SortOrder = "asc"
	// SortOrderDesc represents descending sort order
	SortOrderDesc SortOrder = "desc"
)

// JobSortField represents the fields that can be used to sort jobs
type JobSortField string

const (
	// JobSortFieldCreatedAt sorts by job creation time
	JobSortFieldCreatedAt JobSortField = "created_at"
	// JobSortFieldStartedAt sorts by job start time
	JobSortFieldStartedAt JobSortField = "started_at"
	// JobSortFieldCompletedAt sorts by job completion time
	JobSortFieldCompletedAt JobSortField = "completed_at"
	// JobSortFieldName sorts by job name
	JobSortFieldName JobSortField = "name"
	// JobSortFieldStatus sorts by job status
	JobSortFieldStatus JobSortField = "status"
	// JobSortFieldProgress sorts by job progress percentage
	JobSortFieldProgress JobSortField = "progress"
)

// CrashSortField represents the fields that can be used to sort crashes
type CrashSortField string

const (
	// CrashSortFieldTimestamp sorts by crash discovery time
	CrashSortFieldTimestamp CrashSortField = "timestamp"
	// CrashSortFieldType sorts by crash type
	CrashSortFieldType CrashSortField = "type"
	// CrashSortFieldSignal sorts by signal number
	CrashSortFieldSignal CrashSortField = "signal"
	// CrashSortFieldSize sorts by crash input size
	CrashSortFieldSize CrashSortField = "size"
	// CrashSortFieldJobID sorts by job ID
	CrashSortFieldJobID CrashSortField = "job_id"
	// CrashSortFieldBotID sorts by bot ID
	CrashSortFieldBotID CrashSortField = "bot_id"
)

