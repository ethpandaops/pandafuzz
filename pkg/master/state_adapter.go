package master

import (
	"context"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
)

// StateStoreAdapter adapts PersistentState to implement service.StateStore interface
type StateStoreAdapter struct {
	ps *PersistentState
}

// Compile-time interface compliance check
var _ service.StateStore = (*StateStoreAdapter)(nil)

// NewStateStoreAdapter creates a new adapter for PersistentState
func NewStateStoreAdapter(ps *PersistentState) service.StateStore {
	return &StateStoreAdapter{ps: ps}
}

// Bot operations
func (a *StateStoreAdapter) SaveBotWithRetry(bot *common.Bot) error {
	return a.ps.SaveBotWithRetry(context.Background(), bot)
}

func (a *StateStoreAdapter) GetBot(botID string) (*common.Bot, error) {
	return a.ps.GetBot(context.Background(), botID)
}

func (a *StateStoreAdapter) DeleteBot(botID string) error {
	return a.ps.DeleteBot(context.Background(), botID)
}

func (a *StateStoreAdapter) ListBots() ([]*common.Bot, error) {
	return a.ps.ListBots()
}

// Job operations
func (a *StateStoreAdapter) SaveJobWithRetry(job *common.Job) error {
	return a.ps.SaveJobWithRetry(context.Background(), job)
}

func (a *StateStoreAdapter) GetJob(jobID string) (*common.Job, error) {
	return a.ps.GetJob(context.Background(), jobID)
}

func (a *StateStoreAdapter) ListJobs() ([]*common.Job, error) {
	return a.ps.ListJobs()
}

func (a *StateStoreAdapter) AtomicJobAssignmentWithRetry(botID string) (*common.Job, error) {
	return a.ps.AtomicJobAssignmentWithRetry(context.Background(), botID)
}

func (a *StateStoreAdapter) CompleteJobWithRetry(jobID, botID string, success bool) error {
	return a.ps.CompleteJobWithRetry(context.Background(), jobID, botID, success)
}

// Result processing
func (a *StateStoreAdapter) ProcessCrashResultWithRetry(crash *common.CrashResult) error {
	return a.ps.ProcessCrashResultWithRetry(context.Background(), crash)
}

func (a *StateStoreAdapter) ProcessCoverageResultWithRetry(coverage *common.CoverageResult) error {
	return a.ps.ProcessCoverageResultWithRetry(context.Background(), coverage)
}

func (a *StateStoreAdapter) ProcessCorpusUpdateWithRetry(corpus *common.CorpusUpdate) error {
	return a.ps.ProcessCorpusUpdateWithRetry(context.Background(), corpus)
}

// Stats and health
func (a *StateStoreAdapter) GetStats() any {
	return a.ps.GetStats()
}

func (a *StateStoreAdapter) GetDatabaseStats() any {
	return a.ps.GetDatabaseStats(context.Background())
}

func (a *StateStoreAdapter) HealthCheck() error {
	return a.ps.HealthCheck(context.Background())
}