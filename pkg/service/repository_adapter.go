package service

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// StateStoreBotRepository adapts StateStore to the narrow BotRepository interface.
// This provides a migration path from the god interface to focused repositories.
type StateStoreBotRepository struct {
	state StateStore
}

// Compile-time interface compliance check
var _ BotRepository = (*StateStoreBotRepository)(nil)

// NewStateStoreBotRepository creates a new bot repository backed by StateStore.
func NewStateStoreBotRepository(state StateStore) *StateStoreBotRepository {
	return &StateStoreBotRepository{state: state}
}

// Create persists a new bot.
func (r *StateStoreBotRepository) Create(ctx context.Context, bot *common.Bot) error {
	return r.state.SaveBotWithRetry(bot)
}

// Get retrieves a bot by ID.
func (r *StateStoreBotRepository) Get(_ context.Context, botID string) (*common.Bot, error) {
	return r.state.GetBot(botID)
}

// Update updates an existing bot.
func (r *StateStoreBotRepository) Update(ctx context.Context, bot *common.Bot) error {
	return r.state.SaveBotWithRetry(bot)
}

// Delete removes a bot by ID.
func (r *StateStoreBotRepository) Delete(_ context.Context, botID string) error {
	return r.state.DeleteBot(botID)
}

// List returns all bots.
func (r *StateStoreBotRepository) List(_ context.Context) ([]*common.Bot, error) {
	return r.state.ListBots()
}

// ListByStatus returns bots filtered by status.
func (r *StateStoreBotRepository) ListByStatus(_ context.Context, status common.BotStatus) ([]*common.Bot, error) {
	bots, err := r.state.ListBots()
	if err != nil {
		return nil, err
	}

	result := make([]*common.Bot, 0, len(bots))
	for _, bot := range bots {
		if bot.Status == status {
			result = append(result, bot)
		}
	}
	return result, nil
}

// UpdateHeartbeat updates the heartbeat timestamp and status for a bot.
func (r *StateStoreBotRepository) UpdateHeartbeat(ctx context.Context, botID string, status common.BotStatus, currentJob *string) error {
	return r.state.UpdateBotHeartbeat(ctx, botID, status, currentJob)
}

// FindAvailableWithCapabilities finds an available bot with the required capabilities.
func (r *StateStoreBotRepository) FindAvailableWithCapabilities(ctx context.Context, capabilities []string) (*common.Bot, error) {
	return r.state.GetAvailableBotWithCapabilities(ctx, capabilities)
}

// BatchUpdateStatus updates status for multiple bots.
func (r *StateStoreBotRepository) BatchUpdateStatus(ctx context.Context, botIDs []string, status common.BotStatus) error {
	return r.state.BatchUpdateBotStatus(ctx, botIDs, status)
}

// StateStoreJobRepository adapts StateStore to the narrow JobRepository interface.
type StateStoreJobRepository struct {
	state StateStore
}

// Compile-time interface compliance check
var _ JobRepository = (*StateStoreJobRepository)(nil)

// NewStateStoreJobRepository creates a new job repository backed by StateStore.
func NewStateStoreJobRepository(state StateStore) *StateStoreJobRepository {
	return &StateStoreJobRepository{state: state}
}

// Create persists a new job.
func (r *StateStoreJobRepository) Create(ctx context.Context, job *common.Job) error {
	return r.state.SaveJobWithRetry(job)
}

// Get retrieves a job by ID.
func (r *StateStoreJobRepository) Get(_ context.Context, jobID string) (*common.Job, error) {
	return r.state.GetJob(jobID)
}

// Update updates an existing job.
func (r *StateStoreJobRepository) Update(ctx context.Context, job *common.Job) error {
	return r.state.UpdateJob(ctx, job)
}

// List returns jobs with optional status filter.
func (r *StateStoreJobRepository) List(ctx context.Context, status *common.JobStatus, fuzzer *string, limit, offset int) ([]*common.Job, error) {
	// Guard against divide-by-zero
	if limit <= 0 {
		limit = 50 // default limit
	}
	// Convert offset to page number (1-indexed)
	page := (offset / limit) + 1
	return r.state.ListJobsFiltered(ctx, status, fuzzer, limit, page)
}

// ListAll returns all jobs.
func (r *StateStoreJobRepository) ListAll(_ context.Context) ([]*common.Job, error) {
	return r.state.ListJobs()
}

// AtomicAssign atomically assigns a pending job to a bot.
func (r *StateStoreJobRepository) AtomicAssign(ctx context.Context, botID string) (*common.Job, error) {
	// Try optimized version first
	job, err := r.state.AtomicJobAssignmentOptimized(ctx, botID)
	if err == nil {
		return job, nil
	}
	// Fall back to retry version
	return r.state.AtomicJobAssignmentWithRetry(botID)
}

// Complete marks a job as completed.
func (r *StateStoreJobRepository) Complete(ctx context.Context, jobID, botID string, success bool) error {
	// Try optimized version first
	err := r.state.CompleteJobOptimized(ctx, jobID, botID, success)
	if err == nil {
		return nil
	}
	// Fall back to retry version
	return r.state.CompleteJobWithRetry(jobID, botID, success)
}

// GetLogs retrieves logs for a job.
func (r *StateStoreJobRepository) GetLogs(ctx context.Context, jobID string, limit, offset int) ([]string, int, error) {
	return r.state.GetJobLogs(ctx, jobID, limit, offset)
}

// StoreLogs stores logs for a job.
func (r *StateStoreJobRepository) StoreLogs(ctx context.Context, jobID string, logs []string) error {
	return r.state.StoreJobLogs(ctx, jobID, logs)
}

// GetCoverageData retrieves coverage data for a job.
func (r *StateStoreJobRepository) GetCoverageData(ctx context.Context, jobID string) (*common.CoverageData, error) {
	return r.state.GetCoverageData(ctx, jobID)
}

// GetCoverageHistory retrieves coverage history for a job.
func (r *StateStoreJobRepository) GetCoverageHistory(ctx context.Context, jobID string, start, end time.Time) ([]*common.CoverageResult, error) {
	return r.state.GetJobCoverageHistory(ctx, jobID, start, end)
}

// StateStoreCrashRepository adapts StateStore to the narrow CrashRepository interface.
type StateStoreCrashRepository struct {
	state StateStore
}

// Compile-time interface compliance check
var _ CrashRepository = (*StateStoreCrashRepository)(nil)

// NewStateStoreCrashRepository creates a new crash repository backed by StateStore.
func NewStateStoreCrashRepository(state StateStore) *StateStoreCrashRepository {
	return &StateStoreCrashRepository{state: state}
}

// Create persists a new crash.
func (r *StateStoreCrashRepository) Create(ctx context.Context, crash *common.CrashResult) error {
	return r.state.ProcessCrashResultWithRetry(crash)
}

// List returns crashes with pagination.
func (r *StateStoreCrashRepository) List(ctx context.Context, jobID string, limit, offset int) ([]*common.CrashResult, error) {
	return r.state.ListCrashes(ctx, jobID, limit, offset)
}

// ListByJob returns all crashes for a job.
func (r *StateStoreCrashRepository) ListByJob(ctx context.Context, jobID string) ([]*common.CrashResult, error) {
	return r.state.GetJobCrashes(ctx, jobID)
}

// ListInTimeRange returns crashes in a time range.
func (r *StateStoreCrashRepository) ListInTimeRange(ctx context.Context, start, end time.Time) ([]*common.CrashResult, error) {
	return r.state.GetCrashesInTimeRange(ctx, start, end)
}

// StateStoreResultRepository adapts StateStore to the narrow ResultRepository interface.
type StateStoreResultRepository struct {
	state StateStore
}

// Compile-time interface compliance check
var _ ResultRepository = (*StateStoreResultRepository)(nil)

// NewStateStoreResultRepository creates a new result repository backed by StateStore.
func NewStateStoreResultRepository(state StateStore) *StateStoreResultRepository {
	return &StateStoreResultRepository{state: state}
}

// ProcessCrash stores and processes a crash result.
func (r *StateStoreResultRepository) ProcessCrash(_ context.Context, crash *common.CrashResult) error {
	return r.state.ProcessCrashResultWithRetry(crash)
}

// ProcessCoverage stores and processes coverage data.
func (r *StateStoreResultRepository) ProcessCoverage(_ context.Context, coverage *common.CoverageResult) error {
	return r.state.ProcessCoverageResultWithRetry(coverage)
}

// ProcessCorpusUpdate stores and processes a corpus update.
func (r *StateStoreResultRepository) ProcessCorpusUpdate(_ context.Context, update *common.CorpusUpdate) error {
	return r.state.ProcessCorpusUpdateWithRetry(update)
}

// StateStoreAnalyticsRepository adapts StateStore to the narrow AnalyticsRepository interface.
type StateStoreAnalyticsRepository struct {
	state StateStore
}

// Compile-time interface compliance check
var _ AnalyticsRepository = (*StateStoreAnalyticsRepository)(nil)

// NewStateStoreAnalyticsRepository creates a new analytics repository backed by StateStore.
func NewStateStoreAnalyticsRepository(state StateStore) *StateStoreAnalyticsRepository {
	return &StateStoreAnalyticsRepository{state: state}
}

// GetCampaignJobs returns all jobs for a campaign.
func (r *StateStoreAnalyticsRepository) GetCampaignJobs(ctx context.Context, campaignID string) ([]*common.Job, error) {
	return r.state.GetCampaignJobs(ctx, campaignID)
}

// GetJobCrashes returns crashes for a job.
func (r *StateStoreAnalyticsRepository) GetJobCrashes(ctx context.Context, jobID string) ([]*common.CrashResult, error) {
	return r.state.GetJobCrashes(ctx, jobID)
}

// GetCrashesInTimeRange returns crashes in a time range.
func (r *StateStoreAnalyticsRepository) GetCrashesInTimeRange(ctx context.Context, start, end time.Time) ([]*common.CrashResult, error) {
	return r.state.GetCrashesInTimeRange(ctx, start, end)
}

// GetCoverageHistory returns coverage history for a job.
func (r *StateStoreAnalyticsRepository) GetCoverageHistory(ctx context.Context, jobID string, start, end time.Time) ([]*common.CoverageResult, error) {
	return r.state.GetJobCoverageHistory(ctx, jobID, start, end)
}

// StateStoreSystemRepository adapts StateStore to the narrow SystemRepository interface.
type StateStoreSystemRepository struct {
	state StateStore
}

// Compile-time interface compliance check
var _ SystemRepository = (*StateStoreSystemRepository)(nil)

// NewStateStoreSystemRepository creates a new system repository backed by StateStore.
func NewStateStoreSystemRepository(state StateStore) *StateStoreSystemRepository {
	return &StateStoreSystemRepository{state: state}
}

// GetStats returns general statistics.
func (r *StateStoreSystemRepository) GetStats() any {
	return r.state.GetStats()
}

// GetDatabaseStats returns database statistics.
func (r *StateStoreSystemRepository) GetDatabaseStats() any {
	return r.state.GetDatabaseStats()
}

// HealthCheck performs a health check.
func (r *StateStoreSystemRepository) HealthCheck() error {
	return r.state.HealthCheck()
}
