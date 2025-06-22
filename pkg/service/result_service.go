package service

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/sirupsen/logrus"
)

// resultService implements ResultService interface
type resultService struct {
	state  StateStore
	config *common.MasterConfig
	logger *logrus.Logger
}

// NewResultService creates a new result service
func NewResultService(
	state StateStore,
	config *common.MasterConfig,
	logger *logrus.Logger,
) ResultService {
	return &resultService{
		state:  state,
		config: config,
		logger: logger,
	}
}

// ProcessCrashResult processes a crash result
func (s *resultService) ProcessCrashResult(ctx context.Context, crash *common.CrashResult) error {
	// Validate crash result
	if crash.JobID == "" || crash.BotID == "" {
		return errors.NewValidationError("process_crash", "Job ID and Bot ID are required")
	}

	// Set timestamp if not provided
	if crash.Timestamp.IsZero() {
		crash.Timestamp = time.Now()
	}

	// Process crash result
	if err := s.state.ProcessCrashResultWithRetry(crash); err != nil {
		return errors.Wrap(errors.ErrorTypeDatabase, "process_crash", "Failed to process crash result", err)
	}

	s.logger.WithFields(logrus.Fields{
		"crash_id":  crash.ID,
		"job_id":    crash.JobID,
		"bot_id":    crash.BotID,
		"hash":      crash.Hash,
		"type":      crash.Type,
		"is_unique": crash.IsUnique,
	}).Info("Crash result processed")

	return nil
}

// ProcessCoverageResult processes coverage data
func (s *resultService) ProcessCoverageResult(ctx context.Context, coverage *common.CoverageResult) error {
	// Validate coverage result
	if coverage.JobID == "" || coverage.BotID == "" {
		return errors.NewValidationError("process_coverage", "Job ID and Bot ID are required")
	}

	// Set timestamp if not provided
	if coverage.Timestamp.IsZero() {
		coverage.Timestamp = time.Now()
	}

	// Process coverage result
	if err := s.state.ProcessCoverageResultWithRetry(coverage); err != nil {
		return errors.Wrap(errors.ErrorTypeDatabase, "process_coverage", "Failed to process coverage result", err)
	}

	s.logger.WithFields(logrus.Fields{
		"coverage_id": coverage.ID,
		"job_id":      coverage.JobID,
		"bot_id":      coverage.BotID,
		"edges":       coverage.Edges,
		"new_edges":   coverage.NewEdges,
	}).Debug("Coverage result processed")

	return nil
}

// ProcessCorpusUpdate processes corpus updates
func (s *resultService) ProcessCorpusUpdate(ctx context.Context, corpus *common.CorpusUpdate) error {
	// Validate corpus update
	if corpus.JobID == "" || corpus.BotID == "" {
		return errors.NewValidationError("process_corpus", "Job ID and Bot ID are required")
	}

	// Set timestamp if not provided
	if corpus.Timestamp.IsZero() {
		corpus.Timestamp = time.Now()
	}

	// Process corpus update
	if err := s.state.ProcessCorpusUpdateWithRetry(corpus); err != nil {
		return errors.Wrap(errors.ErrorTypeDatabase, "process_corpus", "Failed to process corpus update", err)
	}

	s.logger.WithFields(logrus.Fields{
		"corpus_id":  corpus.ID,
		"job_id":     corpus.JobID,
		"bot_id":     corpus.BotID,
		"file_count": len(corpus.Files),
		"total_size": corpus.TotalSize,
	}).Debug("Corpus update processed")

	return nil
}

// GetCrashResults retrieves crash results for a job
func (s *resultService) GetCrashResults(ctx context.Context, jobID string) ([]*common.CrashResult, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_crash_results", "Job ID is required")
	}

	// TODO: Implement actual crash result retrieval from database
	// For now, return empty list
	return []*common.CrashResult{}, nil
}

// GetCoverageHistory retrieves coverage history
func (s *resultService) GetCoverageHistory(ctx context.Context, jobID string) ([]*common.CoverageResult, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_coverage_history", "Job ID is required")
	}

	// TODO: Implement actual coverage history retrieval from database
	// For now, return empty list
	return []*common.CoverageResult{}, nil
}