package common

import (
	"context"
)

// CampaignService defines the interface for campaign management
type CampaignService interface {
	Create(ctx context.Context, campaign *Campaign) error
	Get(ctx context.Context, id string) (*Campaign, error)
	List(ctx context.Context, filters CampaignFilters) ([]*Campaign, error)
	Update(ctx context.Context, id string, updates CampaignUpdates) error
	Delete(ctx context.Context, id string) error
	GetStatistics(ctx context.Context, id string) (*CampaignStats, error)
	RestartCampaign(ctx context.Context, id string) error
}

// DeduplicationService defines the interface for crash deduplication
type DeduplicationService interface {
	ProcessCrash(ctx context.Context, crash *CrashResult) (*CrashGroup, bool, error)
	GetCrashGroups(ctx context.Context, campaignID string) ([]*CrashGroup, error)
	GetStackTrace(ctx context.Context, crashID string) (*StackTrace, error)
}

// CorpusService defines the interface for corpus management
type CorpusService interface {
	AddFile(ctx context.Context, file *CorpusFile) error
	GetEvolution(ctx context.Context, campaignID string) ([]*CorpusEvolution, error)
	SyncCorpus(ctx context.Context, campaignID string, botID string) ([]*CorpusFile, error)
	ShareCorpus(ctx context.Context, fromCampaign, toCampaign string) error
	PromoteCrashToCorpus(ctx context.Context, crashID, campaignID string) (*CorpusFile, error)
	GetCorpusForJob(ctx context.Context, jobID string) ([]*CorpusFile, error)
	LinkJobCorpus(ctx context.Context, jobID, campaignID string) error
}

// FileStorage defines the interface for file storage operations
type FileStorage interface {
	SaveFile(ctx context.Context, path string, data []byte) error
	ReadFile(ctx context.Context, path string) ([]byte, error)
	DeleteFile(ctx context.Context, path string) error
	ListFiles(ctx context.Context, prefix string) ([]string, error)
	FileExists(ctx context.Context, path string) (bool, error)
}

// FuzzerEventHandler defines the interface for handling fuzzer events
type FuzzerEventHandler interface {
	HandleEvent(ctx context.Context, event FuzzerEvent) error
}

// WSHub defines the interface for WebSocket hub
type WSHub interface {
	BroadcastCampaignUpdate(campaignID string, update interface{})
	BroadcastCrashFound(crash *CrashResult)
	BroadcastCorpusUpdate(campaignID string, update *CorpusEvolution)
	BroadcastBotStatus(bot *Bot)
	BroadcastJobStatus(job *Job)
	BroadcastMetrics(metrics interface{})
}

// JobService defines the interface for job management (existing, but adding for clarity)
type JobService interface {
	CreateJob(ctx context.Context, job *Job) error
	GetJob(ctx context.Context, id string) (*Job, error)
	UpdateJob(ctx context.Context, id string, updates map[string]interface{}) error
	DeleteJob(ctx context.Context, id string) error
	AssignJob(ctx context.Context, jobID, botID string) error
	CompleteJob(ctx context.Context, jobID string) error
	GetJobsByStatus(ctx context.Context, status JobStatus) ([]*Job, error)
}

// ReproducibilityService defines the interface for crash reproducibility management
type ReproducibilityService interface {
	// Lifecycle methods
	Start(ctx context.Context) error
	Stop() error

	// QueueReproduction adds a crash to the reproduction queue
	QueueReproduction(ctx context.Context, crashID string, priority int) error

	// QueueBatchReproduction queues multiple crashes for reproduction testing
	QueueBatchReproduction(ctx context.Context, crashIDs []string, priority int) error

	// GetReproductionStatus gets the current status of a reproduction task
	GetReproductionStatus(ctx context.Context, crashID string) (*ReproductionRequest, error)

	// RecordReproductionResult records the result of a reproduction attempt
	RecordReproductionResult(ctx context.Context, result *ReproductionResult) error

	// GetReproductionResults gets all reproduction results for a crash
	GetReproductionResults(ctx context.Context, crashID string) ([]*ReproductionResult, error)

	// CalculateReproducibilityScore calculates the reproducibility score for a crash
	CalculateReproducibilityScore(ctx context.Context, crashID string) (float64, error)

	// GetDetailedScore returns the full reproducibility score with all components
	GetDetailedScore(ctx context.Context, crashID string) (interface{}, error)

	// GetPlatformAnalysis returns platform-specific reproduction analysis
	GetPlatformAnalysis(ctx context.Context, crashID string) (map[string]interface{}, error)

	// GetTrendAnalysis returns reproduction trend analysis over time
	GetTrendAnalysis(ctx context.Context, crashID string) (map[string]interface{}, error)

	// VerifyFix triggers verification of a fix for a crash
	VerifyFix(ctx context.Context, crashID, fixCommit string) error

	// GetQueueStatus returns the current queue status
	GetQueueStatus() map[string]interface{}
}

// CrashMinimizerService defines the interface for crash test case minimization
type CrashMinimizerService interface {
	// Lifecycle methods
	Start(ctx context.Context) error
	Stop() error

	// MinimizeCrash minimizes a crash input using the specified strategy
	MinimizeCrash(ctx context.Context, crashID string, strategy string) (*MinimizationResult, error)

	// GetMinimizationResult retrieves a previous minimization result
	GetMinimizationResult(ctx context.Context, resultID string) (*MinimizationResult, error)

	// ListMinimizationResults lists minimization results for a crash
	ListMinimizationResults(ctx context.Context, crashID string) ([]*MinimizationResult, error)

	// GetBestMinimization returns the best (smallest) minimization for a crash
	GetBestMinimization(ctx context.Context, crashID string) (*MinimizationResult, error)
}

// Storage defines the main storage interface (extending the existing one)
type Storage interface {
	// Campaign operations
	CreateCampaign(ctx context.Context, campaign *Campaign) error
	GetCampaign(ctx context.Context, id string) (*Campaign, error)
	ListCampaigns(ctx context.Context, limit, offset int, status string) ([]*Campaign, error)
	UpdateCampaign(ctx context.Context, id string, updates map[string]interface{}) error
	DeleteCampaign(ctx context.Context, id string) error
	GetCampaignJobs(ctx context.Context, campaignID string) ([]*Job, error)
	LinkJobToCampaign(ctx context.Context, campaignID, jobID string) error
	GetCampaignStatistics(ctx context.Context, campaignID string) (*CampaignStats, error)

	// Crash group operations
	CreateCrashGroup(ctx context.Context, cg *CrashGroup) error
	GetCrashGroup(ctx context.Context, campaignID, stackHash string) (*CrashGroup, error)
	UpdateCrashGroupCount(ctx context.Context, id string) error
	ListCrashGroups(ctx context.Context, campaignID string) ([]*CrashGroup, error)
	CreateStackTrace(ctx context.Context, crashID string, st *StackTrace) error
	GetStackTrace(ctx context.Context, crashID string) (*StackTrace, error)
	LinkCrashToGroup(ctx context.Context, crashID, groupID string) error

	// Corpus operations
	AddCorpusFile(ctx context.Context, cf *CorpusFile) error
	GetCorpusFiles(ctx context.Context, campaignID string) ([]*CorpusFile, error)
	GetCorpusFile(ctx context.Context, fileID string) (*CorpusFile, error)
	GetCorpusFileByHash(ctx context.Context, hash string) (*CorpusFile, error)
	UpdateCorpusFile(ctx context.Context, fileID string, updates map[string]interface{}) error
	DeleteCorpusFile(ctx context.Context, fileID string) error
	UpdateCorpusCoverage(ctx context.Context, id string, coverage, newCoverage int64) error
	RecordCorpusEvolution(ctx context.Context, ce *CorpusEvolution) error
	GetCorpusEvolution(ctx context.Context, campaignID string, limit int) ([]*CorpusEvolution, error)
	GetUnsyncedCorpusFiles(ctx context.Context, campaignID, botID string) ([]*CorpusFile, error)
	MarkCorpusFilesSynced(ctx context.Context, fileIDs []string, botID string) error

	// Quarantine operations
	AddQuarantinedFile(ctx context.Context, qf *QuarantinedFile) error
	GetQuarantinedFile(ctx context.Context, fileID string) (*QuarantinedFile, error)
	GetQuarantinedFiles(ctx context.Context, campaignID string) ([]*QuarantinedFile, error)
	UpdateQuarantinedFile(ctx context.Context, id string, updates map[string]interface{}) error

	// Corpus metrics operations
	GetCorpusFileMetrics(ctx context.Context, fileID string) (*CorpusFileMetrics, error)
	UpdateCorpusFileMetrics(ctx context.Context, fileID string, metrics *CorpusFileMetrics) error

	// Existing operations (to ensure compatibility)
	CreateBot(ctx context.Context, bot *Bot) error
	GetBot(ctx context.Context, id string) (*Bot, error)
	UpdateBot(ctx context.Context, id string, updates map[string]interface{}) error
	ListBots(ctx context.Context) ([]*Bot, error)
	DeleteBot(ctx context.Context, id string) error

	CreateJob(ctx context.Context, job *Job) error
	GetJob(ctx context.Context, id string) (*Job, error)
	UpdateJob(ctx context.Context, id string, updates map[string]interface{}) error
	ListJobs(ctx context.Context, limit, offset int, status string) ([]*Job, error)
	DeleteJob(ctx context.Context, id string) error

	CreateCrash(ctx context.Context, crash *CrashResult) error
	GetCrash(ctx context.Context, id string) (*CrashResult, error)
	ListCrashes(ctx context.Context, jobID string, limit, offset int) ([]*CrashResult, error)
	GetCrashCount(ctx context.Context, jobID string) (int, error)
	GetCrashesByCampaign(ctx context.Context, campaignID string) ([]*CrashResult, error)
	UpdateCrashWithCampaign(ctx context.Context, crashID, campaignID string) error
	StoreCrashInput(ctx context.Context, crashID string, input []byte) error
	GetCrashInput(ctx context.Context, crashID string) ([]byte, error)

	CreateCoverage(ctx context.Context, coverage *CoverageResult) error
	GetLatestCoverage(ctx context.Context, jobID string) (*CoverageResult, error)

	RecordCorpusUpdate(ctx context.Context, update *CorpusUpdate) error

	GetSystemStats(ctx context.Context) (map[string]interface{}, error)

	// Transaction support
	BeginTx(ctx context.Context) (Transaction, error)

	// Maintenance operations
	Cleanup(ctx context.Context) error
	Backup(ctx context.Context, path string) error

	// Health check
	Ping(ctx context.Context) error
	Close(ctx context.Context) error

	// Reproduction operations
	CreateReproductionResult(ctx context.Context, result *ReproductionResult) error
	GetReproductionResults(ctx context.Context, crashID string) ([]*ReproductionResult, error)

	// Minimization operations
	CreateMinimizationResult(ctx context.Context, result *MinimizationResult) error
	GetMinimizationResult(ctx context.Context, resultID string) (*MinimizationResult, error)
	ListMinimizationResults(ctx context.Context, crashID string) ([]*MinimizationResult, error)
	GetMinimizationStats(ctx context.Context, campaignID string) (map[string]interface{}, error)

	// Corpus collection operations
	CreateCorpusCollection(ctx context.Context, collection *CorpusCollection) error
	GetCorpusCollection(ctx context.Context, collectionID string) (*CorpusCollection, error)
	GetCorpusCollections(ctx context.Context) ([]*CorpusCollection, error)
	UpdateCorpusCollection(ctx context.Context, collection *CorpusCollection) error
	DeleteCorpusCollection(ctx context.Context, collectionID string) error

	// Corpus collection file operations
	AddCorpusCollectionFile(ctx context.Context, file *CorpusCollectionFile) error
	GetCorpusCollectionFiles(ctx context.Context, collectionID string) ([]*CorpusCollectionFile, error)
	DeleteCorpusCollectionFile(ctx context.Context, fileID string) error

	// Job log operations
	StoreJobLogs(ctx context.Context, jobID string, logs []*JobLog) error
	GetJobLogs(ctx context.Context, jobID string, limit, offset int) ([]*JobLog, int, error)
}
