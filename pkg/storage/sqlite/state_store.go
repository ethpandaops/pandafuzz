// Package sqlite provides SQLite-based repository implementations for PandaFuzz.
package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	botrepo "github.com/ethpandaops/pandafuzz/pkg/domain/bot/repository"
	bottypes "github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
	crashrepo "github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	jobrepo "github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/mappers"
)

// RepositoryStateStore implements service.StateStore using domain repositories.
// This provides a bridge between the legacy StateStore interface and the new
// repository-based architecture.
type RepositoryStateStore struct {
	db        *sql.DB
	jobRepo   jobrepo.JobRepository
	botRepo   botrepo.AgentRepository
	crashRepo crashrepo.CrashRepository
	logger    logrus.FieldLogger
	config    *common.MasterConfig
}

// Compile-time interface compliance check
var _ service.StateStore = (*RepositoryStateStore)(nil)

// NewRepositoryStateStore creates a new repository-backed StateStore.
func NewRepositoryStateStore(
	db *sql.DB,
	jobRepo jobrepo.JobRepository,
	botRepo botrepo.AgentRepository,
	crashRepo crashrepo.CrashRepository,
	config *common.MasterConfig,
	logger logrus.FieldLogger,
) *RepositoryStateStore {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	return &RepositoryStateStore{
		db:        db,
		jobRepo:   jobRepo,
		botRepo:   botRepo,
		crashRepo: crashRepo,
		config:    config,
		logger:    logger.WithField("component", "repository_state_store"),
	}
}

// Bot operations

func (s *RepositoryStateStore) SaveBotWithRetry(bot *common.Bot) error {
	if bot == nil {
		return errors.NewValidationError("save_bot", "bot cannot be nil")
	}

	// Convert common.Bot to domain Agent
	agent := mappers.CommonBotToDomainAgent(bot)
	if agent == nil {
		return errors.NewValidationError("save_bot", "failed to convert bot to agent")
	}

	ctx := context.Background()

	// Check if bot exists
	existing, err := s.botRepo.FindByID(ctx, bot.ID)
	if err != nil && !errors.IsNotFoundError(err) {
		return errors.Wrap(errors.ErrorTypeDatabase, "save_bot", "failed to check existing bot", err)
	}

	if existing != nil {
		// Update existing bot
		if err := s.botRepo.Update(ctx, agent); err != nil {
			return errors.Wrap(errors.ErrorTypeDatabase, "save_bot", "failed to update bot", err)
		}
	} else {
		// Create new bot
		if err := s.botRepo.Create(ctx, agent); err != nil {
			return errors.Wrap(errors.ErrorTypeDatabase, "save_bot", "failed to create bot", err)
		}
	}

	return nil
}

func (s *RepositoryStateStore) GetBot(botID string) (*common.Bot, error) {
	if botID == "" {
		return nil, errors.NewValidationError("get_bot", "bot ID cannot be empty")
	}

	agent, err := s.botRepo.FindByID(context.Background(), botID)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return nil, err
		}
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "get_bot", "failed to get bot", err)
	}

	return mappers.DomainAgentToCommonBot(agent), nil
}

func (s *RepositoryStateStore) DeleteBot(botID string) error {
	if botID == "" {
		return errors.NewValidationError("delete_bot", "bot ID cannot be empty")
	}

	return s.botRepo.Delete(context.Background(), botID)
}

func (s *RepositoryStateStore) ListBots() ([]*common.Bot, error) {
	agents, _, err := s.botRepo.List(context.Background(), 0, 1000)
	if err != nil {
		return nil, errors.Wrap(errors.ErrorTypeDatabase, "list_bots", "failed to list bots", err)
	}

	bots := make([]*common.Bot, len(agents))
	for i, agent := range agents {
		bots[i] = mappers.DomainAgentToCommonBot(agent)
	}

	return bots, nil
}

// Optimized bot operations

func (s *RepositoryStateStore) UpdateBotHeartbeat(ctx context.Context, botID string, _ common.BotStatus, _ *string) error {
	if botID == "" {
		return errors.NewValidationError("update_bot_heartbeat", "bot ID cannot be empty")
	}

	return s.botRepo.UpdateHeartbeat(ctx, botID)
}

func (s *RepositoryStateStore) GetAvailableBotWithCapabilities(ctx context.Context, requiredCapabilities []string) (*common.Bot, error) {
	// Get idle bots
	agents, err := s.botRepo.FindByStatus(ctx, bottypes.StatusIdle)
	if err != nil {
		return nil, err
	}

	for _, agent := range agents {
		// Check capabilities
		hasAllCaps := true
		for _, reqCap := range requiredCapabilities {
			found := false
			for _, agentCap := range agent.Capabilities {
				if string(agentCap) == reqCap {
					found = true
					break
				}
			}
			if !found {
				hasAllCaps = false
				break
			}
		}

		if hasAllCaps {
			return mappers.DomainAgentToCommonBot(agent), nil
		}
	}

	return nil, errors.NewNotFoundError("get_available_bot", "bot with required capabilities")
}

func (s *RepositoryStateStore) BatchUpdateBotStatus(ctx context.Context, botIDs []string, status common.BotStatus) error {
	for _, botID := range botIDs {
		if err := s.botRepo.UpdateStatus(ctx, botID, mappers.CommonBotStatusToDomain(status)); err != nil {
			return err
		}
	}
	return nil
}

// Job operations

func (s *RepositoryStateStore) SaveJobWithRetry(job *common.Job) error {
	if job == nil {
		return errors.NewValidationError("save_job", "job cannot be nil")
	}

	domainJob := mappers.CommonJobToDomain(job)
	if domainJob == nil {
		return errors.NewValidationError("save_job", "failed to convert job")
	}

	ctx := context.Background()

	// Check if job exists
	existing, err := s.jobRepo.Get(ctx, job.ID)
	if err != nil && !errors.IsNotFoundError(err) {
		return errors.Wrap(errors.ErrorTypeDatabase, "save_job", "failed to check existing job", err)
	}

	if existing != nil {
		// Update existing job
		if err := s.jobRepo.Update(ctx, domainJob); err != nil {
			return errors.Wrap(errors.ErrorTypeDatabase, "save_job", "failed to update job", err)
		}
	} else {
		// Create new job
		if err := s.jobRepo.Create(ctx, domainJob); err != nil {
			return errors.Wrap(errors.ErrorTypeDatabase, "save_job", "failed to create job", err)
		}
	}

	return nil
}

func (s *RepositoryStateStore) GetJob(jobID string) (*common.Job, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_job", "job ID cannot be empty")
	}

	domainJob, err := s.jobRepo.Get(context.Background(), jobID)
	if err != nil {
		return nil, err
	}

	return mappers.DomainJobToCommon(domainJob), nil
}

func (s *RepositoryStateStore) ListJobs() ([]*common.Job, error) {
	domainJobs, err := s.jobRepo.List(context.Background(), jobrepo.JobFilter{Limit: 1000})
	if err != nil {
		return nil, err
	}

	jobs := make([]*common.Job, len(domainJobs))
	for i, dj := range domainJobs {
		jobs[i] = mappers.DomainJobToCommon(dj)
	}

	return jobs, nil
}

func (s *RepositoryStateStore) AtomicJobAssignmentWithRetry(botID string) (*common.Job, error) {
	if botID == "" {
		return nil, errors.NewValidationError("atomic_job_assignment", "bot ID cannot be empty")
	}

	ctx := context.Background()

	// Get pending jobs
	pendingJobs, err := s.jobRepo.ListPending(ctx, 1)
	if err != nil {
		return nil, err
	}
	if len(pendingJobs) == 0 {
		return nil, errors.NewNotFoundError("atomic_job_assignment", "pending job")
	}

	// Lock the first pending job for processing
	domainJob, err := s.jobRepo.LockForProcessing(ctx, pendingJobs[0].ID, botID, 45*time.Second)
	if err != nil {
		return nil, err
	}

	return mappers.DomainJobToCommon(domainJob), nil
}

func (s *RepositoryStateStore) CompleteJobWithRetry(jobID, botID string, success bool) error {
	if jobID == "" {
		return errors.NewValidationError("complete_job", "job ID cannot be empty")
	}

	ctx := context.Background()

	// Unlock the job
	if err := s.jobRepo.UnlockJob(ctx, jobID, botID); err != nil {
		s.logger.WithError(err).WithField("job_id", jobID).Warn("Failed to unlock job")
	}

	// Determine target status
	var toStatus jobtypes.JobStatus
	if success {
		toStatus = jobtypes.StatusCompleted
	} else {
		toStatus = jobtypes.StatusFailed
	}

	// Update status from running to completed/failed
	return s.jobRepo.UpdateStatus(ctx, jobID, jobtypes.StatusRunning, toStatus)
}

// Optimized job operations

func (s *RepositoryStateStore) ListJobsFiltered(ctx context.Context, status *common.JobStatus, fuzzer *string, limit, page int) ([]*common.Job, error) {
	filter := jobrepo.JobFilter{
		Limit:  limit,
		Offset: (page - 1) * limit,
	}

	if status != nil {
		domainStatus := mappers.CommonStatusToDomain(*status)
		filter.Status = &domainStatus
	}

	if fuzzer != nil {
		filter.FuzzerType = fuzzer
	}

	domainJobs, err := s.jobRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	jobs := make([]*common.Job, len(domainJobs))
	for i, dj := range domainJobs {
		jobs[i] = mappers.DomainJobToCommon(dj)
	}

	return jobs, nil
}

func (s *RepositoryStateStore) AtomicJobAssignmentOptimized(ctx context.Context, botID string) (*common.Job, error) {
	return s.AtomicJobAssignmentWithRetry(botID)
}

func (s *RepositoryStateStore) CompleteJobOptimized(ctx context.Context, jobID, botID string, success bool) error {
	return s.CompleteJobWithRetry(jobID, botID, success)
}

// Context-based job operations

func (s *RepositoryStateStore) UpdateJob(_ context.Context, job *common.Job) error {
	return s.SaveJobWithRetry(job)
}

func (s *RepositoryStateStore) GetJobLogs(_ context.Context, _ string, _, _ int) ([]string, int, error) {
	// Job logs are stored separately - for now return empty
	// This would need a dedicated log repository
	return []string{}, 0, nil
}

func (s *RepositoryStateStore) StoreJobLogs(_ context.Context, _ string, _ []string) error {
	// Job logs storage - for now no-op
	return nil
}

func (s *RepositoryStateStore) ListCrashes(ctx context.Context, jobID string, limit, offset int) ([]*common.CrashResult, error) {
	// Use FindByTarget since crashes are associated with targets (job IDs)
	crashes, err := s.crashRepo.FindByTarget(ctx, jobID)
	if err != nil {
		return nil, err
	}

	results := make([]*common.CrashResult, len(crashes))
	for i, crash := range crashes {
		results[i] = mappers.DomainCrashToCommon(crash)
	}

	// Apply pagination
	start := offset
	if start > len(results) {
		return []*common.CrashResult{}, nil
	}
	end := start + limit
	if end > len(results) {
		end = len(results)
	}

	return results[start:end], nil
}

func (s *RepositoryStateStore) GetCoverageData(_ context.Context, jobID string) (*common.CoverageData, error) {
	// Coverage data would need a dedicated repository
	return &common.CoverageData{
		JobID:     jobID,
		UpdatedAt: time.Now(),
	}, nil
}

// Result processing

func (s *RepositoryStateStore) ProcessCrashResultWithRetry(crash *common.CrashResult) error {
	if crash == nil {
		return errors.NewValidationError("process_crash", "crash cannot be nil")
	}

	domainCrash := mappers.CommonCrashToDomain(crash)
	if domainCrash == nil {
		return errors.NewValidationError("process_crash", "failed to convert crash")
	}

	return s.crashRepo.Create(context.Background(), domainCrash)
}

func (s *RepositoryStateStore) ProcessCoverageResultWithRetry(_ *common.CoverageResult) error {
	// Coverage would need a dedicated repository
	return nil
}

func (s *RepositoryStateStore) ProcessCorpusUpdateWithRetry(_ *common.CorpusUpdate) error {
	// Corpus updates would need the corpus repository
	return nil
}

// Stats and health

func (s *RepositoryStateStore) GetStats() any {
	ctx := context.Background()

	jobs, _ := s.jobRepo.List(ctx, jobrepo.JobFilter{Limit: 10000})
	bots, _, _ := s.botRepo.List(ctx, 0, 10000)

	return map[string]any{
		"total_jobs": len(jobs),
		"total_bots": len(bots),
	}
}

func (s *RepositoryStateStore) GetDatabaseStats() any {
	return map[string]any{
		"type": "sqlite",
	}
}

func (s *RepositoryStateStore) HealthCheck() error {
	return s.db.PingContext(context.Background())
}

// Analytics operations

func (s *RepositoryStateStore) GetCampaignJobs(ctx context.Context, _ string) ([]*common.Job, error) {
	// Campaign filtering would need to be added to job repository
	// For now, return all jobs (limited)
	domainJobs, err := s.jobRepo.List(ctx, jobrepo.JobFilter{Limit: 1000})
	if err != nil {
		return nil, err
	}

	jobs := make([]*common.Job, len(domainJobs))
	for i, dj := range domainJobs {
		jobs[i] = mappers.DomainJobToCommon(dj)
	}

	return jobs, nil
}

func (s *RepositoryStateStore) GetJobCrashes(ctx context.Context, jobID string) ([]*common.CrashResult, error) {
	crashes, err := s.crashRepo.FindByTarget(ctx, jobID)
	if err != nil {
		return nil, err
	}

	results := make([]*common.CrashResult, len(crashes))
	for i, crash := range crashes {
		results[i] = mappers.DomainCrashToCommon(crash)
	}

	return results, nil
}

func (s *RepositoryStateStore) GetCrashesInTimeRange(ctx context.Context, startTime, _ time.Time) ([]*common.CrashResult, error) {
	// Use FindRecent with the start time
	crashes, err := s.crashRepo.FindRecent(ctx, startTime)
	if err != nil {
		return nil, err
	}

	results := make([]*common.CrashResult, len(crashes))
	for i, crash := range crashes {
		results[i] = mappers.DomainCrashToCommon(crash)
	}

	return results, nil
}

func (s *RepositoryStateStore) GetJobCoverageHistory(_ context.Context, _ string, _, _ time.Time) ([]*common.CoverageResult, error) {
	// Would need a coverage repository
	return []*common.CoverageResult{}, nil
}
