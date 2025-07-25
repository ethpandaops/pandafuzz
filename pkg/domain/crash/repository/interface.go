package repository

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
)

// CrashRepository defines the interface for crash persistence
type CrashRepository interface {
	// Create creates a new crash
	Create(ctx context.Context, crash *types.Crash) error

	// Update updates an existing crash
	Update(ctx context.Context, crash *types.Crash) error

	// Delete deletes a crash by ID
	Delete(ctx context.Context, id string) error

	// FindByID retrieves a crash by its ID
	FindByID(ctx context.Context, id string) (*types.Crash, error)

	// FindBySignature retrieves crashes by signature hash
	FindBySignature(ctx context.Context, signatureHash string) ([]*types.Crash, error)

	// FindBySeverity retrieves all crashes with a specific severity
	FindBySeverity(ctx context.Context, severity types.Severity) ([]*types.Crash, error)

	// FindByType retrieves all crashes of a specific type
	FindByType(ctx context.Context, crashType types.CrashType) ([]*types.Crash, error)

	// FindByTarget retrieves all crashes for a specific target
	FindByTarget(ctx context.Context, targetName string) ([]*types.Crash, error)

	// FindByCorpusEntry retrieves crashes associated with a corpus entry
	FindByCorpusEntry(ctx context.Context, corpusEntryID string) ([]*types.Crash, error)

	// FindReproducible retrieves all reproducible crashes
	FindReproducible(ctx context.Context) ([]*types.Crash, error)

	// FindUnfixed retrieves all unfixed crashes
	FindUnfixed(ctx context.Context) ([]*types.Crash, error)

	// FindByTag retrieves all crashes with a specific tag
	FindByTag(ctx context.Context, tag string) ([]*types.Crash, error)

	// FindRecent retrieves crashes discovered within a time range
	FindRecent(ctx context.Context, since time.Time) ([]*types.Crash, error)

	// FindSimilar finds crashes similar to the given signature
	FindSimilar(ctx context.Context, signature *types.CrashSignature, threshold float64) ([]*types.Crash, error)

	// List retrieves crashes with pagination
	List(ctx context.Context, offset, limit int) ([]*types.Crash, int, error)

	// ListBySeverity retrieves crashes ordered by severity
	ListBySeverity(ctx context.Context, offset, limit int) ([]*types.Crash, int, error)

	// ListByOccurrence retrieves crashes ordered by occurrence count
	ListByOccurrence(ctx context.Context, offset, limit int, ascending bool) ([]*types.Crash, int, error)

	// RecordOccurrence increments the occurrence count for a crash
	RecordOccurrence(ctx context.Context, id string) error

	// MarkAsFixed marks a crash as fixed
	MarkAsFixed(ctx context.Context, id string) error

	// MarkAsNotReproducible marks a crash as not reproducible
	MarkAsNotReproducible(ctx context.Context, id string) error

	// Exists checks if a crash exists by ID
	Exists(ctx context.Context, id string) (bool, error)

	// ExistsBySignature checks if a crash exists by signature
	ExistsBySignature(ctx context.Context, signatureHash string) (bool, error)

	// Count returns the total number of crashes
	Count(ctx context.Context) (int, error)

	// CountBySeverity counts crashes by severity
	CountBySeverity(ctx context.Context, severity types.Severity) (int, error)

	// CountByType counts crashes by type
	CountByType(ctx context.Context, crashType types.CrashType) (int, error)

	// CountUnfixed counts unfixed crashes
	CountUnfixed(ctx context.Context) (int, error)

	// GetStatsByTarget retrieves crash statistics grouped by target
	GetStatsByTarget(ctx context.Context) (map[string]CrashStats, error)
}

// CrashStats represents aggregate statistics for crashes
type CrashStats struct {
	Total        int                     `json:"total"`
	BySeverity   map[types.Severity]int  `json:"by_severity"`
	ByType       map[types.CrashType]int `json:"by_type"`
	Reproducible int                     `json:"reproducible"`
	Fixed        int                     `json:"fixed"`
	AverageAge   time.Duration           `json:"average_age"`
	OldestCrash  *types.Crash            `json:"oldest_crash,omitempty"`
	MostFrequent *types.Crash            `json:"most_frequent,omitempty"`
}

// CrashTransactionRepository extends CrashRepository with transaction support
type CrashTransactionRepository interface {
	CrashRepository

	// BeginTransaction starts a new transaction
	BeginTransaction(ctx context.Context) (CrashTransaction, error)
}

// CrashTransaction represents a crash repository transaction
type CrashTransaction interface {
	// Commit commits the transaction
	Commit() error

	// Rollback rolls back the transaction
	Rollback() error

	// Crash operations within transaction
	CreateTx(ctx context.Context, crash *types.Crash) error
	UpdateTx(ctx context.Context, crash *types.Crash) error
	DeleteTx(ctx context.Context, id string) error
	RecordOccurrenceTx(ctx context.Context, id string) error
	MarkAsFixedTx(ctx context.Context, id string) error
	MarkAsNotReproducibleTx(ctx context.Context, id string) error
}
