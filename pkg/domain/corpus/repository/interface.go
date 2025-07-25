package repository

import (
	"context"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
)

// CorpusEntryRepository defines the interface for corpus entry persistence
type CorpusEntryRepository interface {
	// Create creates a new corpus entry
	Create(ctx context.Context, entry *types.CorpusEntry) error

	// Update updates an existing corpus entry
	Update(ctx context.Context, entry *types.CorpusEntry) error

	// Delete deletes a corpus entry by ID
	Delete(ctx context.Context, id string) error

	// FindByID retrieves a corpus entry by its ID
	FindByID(ctx context.Context, id string) (*types.CorpusEntry, error)

	// FindByHash retrieves a corpus entry by its hash
	FindByHash(ctx context.Context, hash string) (*types.CorpusEntry, error)

	// FindByTag retrieves all entries with a specific tag
	FindByTag(ctx context.Context, tag string) ([]*types.CorpusEntry, error)

	// FindInteresting retrieves all entries marked as interesting
	FindInteresting(ctx context.Context) ([]*types.CorpusEntry, error)

	// FindByParent retrieves all entries derived from a parent
	FindByParent(ctx context.Context, parentID string) ([]*types.CorpusEntry, error)

	// FindByCoverage retrieves entries with coverage above threshold
	FindByCoverage(ctx context.Context, minCoverage float64) ([]*types.CorpusEntry, error)

	// List retrieves entries with pagination
	List(ctx context.Context, offset, limit int) ([]*types.CorpusEntry, int, error)

	// ListByExecutionCount retrieves entries ordered by execution count
	ListByExecutionCount(ctx context.Context, offset, limit int, ascending bool) ([]*types.CorpusEntry, int, error)

	// UpdateExecutionStats updates execution count and last executed time
	UpdateExecutionStats(ctx context.Context, id string) error

	// Exists checks if an entry exists by ID
	Exists(ctx context.Context, id string) (bool, error)

	// ExistsByHash checks if an entry exists by hash
	ExistsByHash(ctx context.Context, hash string) (bool, error)

	// Count returns the total number of entries
	Count(ctx context.Context) (int, error)

	// CountInteresting returns the number of interesting entries
	CountInteresting(ctx context.Context) (int, error)

	// GetStats retrieves aggregate statistics
	GetStats(ctx context.Context) (*types.CollectionStats, error)
}

// CorpusCollectionRepository defines the interface for corpus collection persistence
type CorpusCollectionRepository interface {
	// CreateCollection creates a new corpus collection
	CreateCollection(ctx context.Context, collection *types.CorpusCollection) error

	// UpdateCollection updates an existing collection
	UpdateCollection(ctx context.Context, collection *types.CorpusCollection) error

	// DeleteCollection deletes a collection by name
	DeleteCollection(ctx context.Context, name string) error

	// FindCollectionByName retrieves a collection by name
	FindCollectionByName(ctx context.Context, name string) (*types.CorpusCollection, error)

	// ListCollections retrieves all collections
	ListCollections(ctx context.Context) ([]*types.CorpusCollection, error)

	// AddEntryToCollection adds an entry to a collection
	AddEntryToCollection(ctx context.Context, collectionName string, entryID string) error

	// RemoveEntryFromCollection removes an entry from a collection
	RemoveEntryFromCollection(ctx context.Context, collectionName string, entryID string) error

	// GetCollectionEntries retrieves all entries in a collection
	GetCollectionEntries(ctx context.Context, collectionName string) ([]*types.CorpusEntry, error)

	// CollectionExists checks if a collection exists
	CollectionExists(ctx context.Context, name string) (bool, error)
}

// CorpusTransactionRepository extends corpus repositories with transaction support
type CorpusTransactionRepository interface {
	CorpusEntryRepository
	CorpusCollectionRepository

	// BeginTransaction starts a new transaction
	BeginTransaction(ctx context.Context) (CorpusTransaction, error)
}

// CorpusTransaction represents a corpus repository transaction
type CorpusTransaction interface {
	// Commit commits the transaction
	Commit() error

	// Rollback rolls back the transaction
	Rollback() error

	// Entry operations within transaction
	CreateEntryTx(ctx context.Context, entry *types.CorpusEntry) error
	UpdateEntryTx(ctx context.Context, entry *types.CorpusEntry) error
	DeleteEntryTx(ctx context.Context, id string) error

	// Collection operations within transaction
	CreateCollectionTx(ctx context.Context, collection *types.CorpusCollection) error
	UpdateCollectionTx(ctx context.Context, collection *types.CorpusCollection) error
	DeleteCollectionTx(ctx context.Context, name string) error
	AddEntryToCollectionTx(ctx context.Context, collectionName string, entryID string) error
	RemoveEntryFromCollectionTx(ctx context.Context, collectionName string, entryID string) error
}
