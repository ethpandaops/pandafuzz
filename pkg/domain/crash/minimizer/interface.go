package minimizer

import (
	"context"
)

// Minimizer defines the interface for crash input minimization
type Minimizer interface {
	// MinimizeCrash minimizes a crash input while preserving reproducibility
	MinimizeCrash(ctx context.Context, crashID string, options *MinimizationOptions) (*MinimizationResult, error)

	// GetProgress returns the current progress of a minimization job
	GetProgress(crashID string) (*MinimizationProgress, error)

	// CancelMinimization cancels an active minimization job
	CancelMinimization(crashID string) error

	// GetMinimalInput retrieves the current best minimal input for a crash
	GetMinimalInput(crashID string) ([]byte, error)

	// ResumeMinimization resumes a previously paused minimization job
	ResumeMinimization(ctx context.Context, crashID string, progress *MinimizationProgress, options *MinimizationOptions) (*MinimizationResult, error)

	// ExportProgress exports the current minimization progress for persistence
	ExportProgress(crashID string) (*MinimizationProgress, error)

	// ListActiveJobs returns a list of active minimization job IDs
	ListActiveJobs() []string

	// RegisterStrategy registers a new minimization strategy
	RegisterStrategy(name string, strategy MinimizationStrategy)
}

// Verify interface compliance at compile time
var _ Minimizer = (*Service)(nil)
