package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/cache"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/sqlite"
)

// CorpusRepository implements both CorpusEntryRepository and CorpusCollectionRepository interfaces using SQLite
type CorpusRepository struct {
	conn  *sqlite.Connection
	cache cache.Cache
	log   logrus.FieldLogger
}

// NewCorpusRepository creates a new corpus repository
func NewCorpusRepository(conn *sqlite.Connection, cache cache.Cache, log logrus.FieldLogger) (*CorpusRepository, error) {
	if conn == nil {
		return nil, errors.New(errors.ErrorTypeDatabase, "new_corpus_repository", "connection is required")
	}
	if log == nil {
		log = logrus.New()
	}

	repo := &CorpusRepository{
		conn:  conn,
		cache: cache,
		log:   log.WithField("component", "corpus_repository"),
	}

	if err := repo.createSchema(); err != nil {
		return nil, err
	}

	return repo, nil
}

// createSchema creates the corpus tables if they don't exist
func (r *CorpusRepository) createSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS corpus_entries (
			id TEXT PRIMARY KEY,
			input BLOB NOT NULL,
			hash TEXT NOT NULL UNIQUE,
			size INTEGER NOT NULL,
			created_at DATETIME NOT NULL,
			last_executed_at DATETIME,
			execution_count INTEGER DEFAULT 0,
			coverage_total_blocks INTEGER DEFAULT 0,
			coverage_covered_blocks INTEGER DEFAULT 0,
			coverage_total_edges INTEGER DEFAULT 0,
			coverage_covered_edges INTEGER DEFAULT 0,
			coverage_score REAL DEFAULT 0.0,
			coverage_new_coverage INTEGER DEFAULT 0,
			coverage_gained INTEGER DEFAULT 0,
			mutation_parent_id TEXT,
			mutation_method TEXT,
			mutation_generation INTEGER DEFAULT 0,
			mutation_created_at DATETIME NOT NULL,
			tags TEXT,
			metadata TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_corpus_entries_hash ON corpus_entries(hash);
		CREATE INDEX IF NOT EXISTS idx_corpus_entries_parent ON corpus_entries(mutation_parent_id);
		CREATE INDEX IF NOT EXISTS idx_corpus_entries_coverage ON corpus_entries(coverage_score);
		CREATE INDEX IF NOT EXISTS idx_corpus_entries_execution ON corpus_entries(execution_count);
		CREATE INDEX IF NOT EXISTS idx_corpus_entries_created ON corpus_entries(created_at);
		CREATE INDEX IF NOT EXISTS idx_corpus_entries_last_executed ON corpus_entries(last_executed_at);

		CREATE TABLE IF NOT EXISTS corpus_collections (
			name TEXT PRIMARY KEY,
			description TEXT,
			created_at DATETIME NOT NULL,
			max_size INTEGER DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS corpus_collection_entries (
			collection_name TEXT NOT NULL,
			entry_id TEXT NOT NULL,
			added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (collection_name, entry_id),
			FOREIGN KEY (collection_name) REFERENCES corpus_collections(name) ON DELETE CASCADE,
			FOREIGN KEY (entry_id) REFERENCES corpus_entries(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_collection_entries_collection ON corpus_collection_entries(collection_name);
		CREATE INDEX IF NOT EXISTS idx_collection_entries_entry ON corpus_collection_entries(entry_id);
	`

	_, err := r.conn.ExecContext(context.Background(), schema)
	if err != nil {
		return errors.NewDatabaseError("create_corpus_schema", err)
	}

	return nil
}

// CorpusEntryRepository methods

// Create creates a new corpus entry
func (r *CorpusRepository) Create(ctx context.Context, entry *types.CorpusEntry) error {
	if entry == nil {
		return errors.NewValidationError("create_corpus_entry", "entry cannot be nil")
	}

	if err := entry.Validate(); err != nil {
		return errors.NewValidationError("create_corpus_entry", err.Error())
	}

	tags, err := json.Marshal(entry.Tags)
	if err != nil {
		return errors.NewSystemError("marshal_tags", err)
	}

	var metadata []byte
	if entry.Metadata != nil && len(entry.Metadata) > 0 {
		metadata, err = json.Marshal(entry.Metadata)
		if err != nil {
			return errors.NewSystemError("marshal_metadata", err)
		}
	}

	query := `
		INSERT INTO corpus_entries (
			id, input, hash, size, created_at, last_executed_at, execution_count,
			coverage_total_blocks, coverage_covered_blocks, coverage_total_edges,
			coverage_covered_edges, coverage_score, coverage_new_coverage, coverage_gained,
			mutation_parent_id, mutation_method, mutation_generation, mutation_created_at,
			tags, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.conn.ExecContext(ctx, query,
		entry.ID,
		entry.Input,
		entry.Hash,
		entry.Size,
		entry.CreatedAt,
		entry.LastExecutedAt,
		entry.ExecutionCount,
		entry.Coverage.TotalBlocks,
		entry.Coverage.CoveredBlocks,
		entry.Coverage.TotalEdges,
		entry.Coverage.CoveredEdges,
		entry.Coverage.CoverageScore,
		boolToInt(entry.Coverage.NewCoverage),
		entry.Coverage.CoverageGained,
		entry.MutationInfo.ParentID,
		entry.MutationInfo.MutationMethod,
		entry.MutationInfo.Generation,
		entry.MutationInfo.CreatedAt,
		string(tags),
		metadata,
	)

	if err != nil {
		if r.isUniqueConstraintError(err) {
			return errors.NewConflictError("create_corpus_entry", "corpus entry already exists").
				WithDetail("entry_id", entry.ID).
				WithDetail("hash", entry.Hash)
		}
		return errors.NewDatabaseError("create_corpus_entry", err).
			WithDetail("entry_id", entry.ID)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.entryCacheKey(entry.ID))
		r.cache.Delete(ctx, r.entryHashCacheKey(entry.Hash))
		r.cache.Delete(ctx, "corpus:stats")
	}

	r.log.WithField("entry_id", entry.ID).Debug("Corpus entry created")
	return nil
}

// Update updates an existing corpus entry
func (r *CorpusRepository) Update(ctx context.Context, entry *types.CorpusEntry) error {
	if entry == nil {
		return errors.NewValidationError("update_corpus_entry", "entry cannot be nil")
	}

	if err := entry.Validate(); err != nil {
		return errors.NewValidationError("update_corpus_entry", err.Error())
	}

	tags, err := json.Marshal(entry.Tags)
	if err != nil {
		return errors.NewSystemError("marshal_tags", err)
	}

	var metadata []byte
	if entry.Metadata != nil && len(entry.Metadata) > 0 {
		metadata, err = json.Marshal(entry.Metadata)
		if err != nil {
			return errors.NewSystemError("marshal_metadata", err)
		}
	}

	query := `
		UPDATE corpus_entries SET
			last_executed_at = ?, execution_count = ?,
			coverage_total_blocks = ?, coverage_covered_blocks = ?,
			coverage_total_edges = ?, coverage_covered_edges = ?,
			coverage_score = ?, coverage_new_coverage = ?, coverage_gained = ?,
			tags = ?, metadata = ?
		WHERE id = ?
	`

	result, err := r.conn.ExecContext(ctx, query,
		entry.LastExecutedAt,
		entry.ExecutionCount,
		entry.Coverage.TotalBlocks,
		entry.Coverage.CoveredBlocks,
		entry.Coverage.TotalEdges,
		entry.Coverage.CoveredEdges,
		entry.Coverage.CoverageScore,
		boolToInt(entry.Coverage.NewCoverage),
		entry.Coverage.CoverageGained,
		string(tags),
		metadata,
		entry.ID,
	)

	if err != nil {
		return errors.NewDatabaseError("update_corpus_entry", err).
			WithDetail("entry_id", entry.ID)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_corpus_entry_rows", err).
			WithDetail("entry_id", entry.ID)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_corpus_entry", "corpus entry").
			WithDetail("entry_id", entry.ID)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.entryCacheKey(entry.ID))
		r.cache.Delete(ctx, r.entryHashCacheKey(entry.Hash))
		r.cache.Delete(ctx, "corpus:stats")
		r.cache.Clear(ctx) // Clear tag and coverage caches
	}

	r.log.WithField("entry_id", entry.ID).Debug("Corpus entry updated")
	return nil
}

// Delete deletes a corpus entry by ID
func (r *CorpusRepository) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("delete_corpus_entry", "entry ID cannot be empty")
	}

	// Get the hash before deletion for cache invalidation
	var hash string
	err := r.conn.QueryRowContext(ctx, "SELECT hash FROM corpus_entries WHERE id = ?", id).Scan(&hash)
	if err != nil && err != sql.ErrNoRows {
		return errors.NewDatabaseError("get_hash_for_deletion", err).
			WithDetail("entry_id", id)
	}

	query := `DELETE FROM corpus_entries WHERE id = ?`

	result, err := r.conn.ExecContext(ctx, query, id)
	if err != nil {
		return errors.NewDatabaseError("delete_corpus_entry", err).
			WithDetail("entry_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("delete_corpus_entry_rows", err).
			WithDetail("entry_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("delete_corpus_entry", "corpus entry").
			WithDetail("entry_id", id)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.entryCacheKey(id))
		if hash != "" {
			r.cache.Delete(ctx, r.entryHashCacheKey(hash))
		}
		r.cache.Clear(ctx) // Clear all caches
	}

	r.log.WithField("entry_id", id).Debug("Corpus entry deleted")
	return nil
}

// FindByID retrieves a corpus entry by its ID
func (r *CorpusRepository) FindByID(ctx context.Context, id string) (*types.CorpusEntry, error) {
	if id == "" {
		return nil, errors.NewValidationError("find_corpus_entry_by_id", "entry ID cannot be empty")
	}

	// Check cache first
	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, r.entryCacheKey(id)); found {
			if entry, ok := cached.(*types.CorpusEntry); ok {
				return entry, nil
			}
		}
	}

	query := `
		SELECT id, input, hash, size, created_at, last_executed_at, execution_count,
			coverage_total_blocks, coverage_covered_blocks, coverage_total_edges,
			coverage_covered_edges, coverage_score, coverage_new_coverage, coverage_gained,
			mutation_parent_id, mutation_method, mutation_generation, mutation_created_at,
			tags, metadata
		FROM corpus_entries
		WHERE id = ?
	`

	entry, err := r.scanEntry(r.conn.QueryRowContext(ctx, query, id))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewNotFoundError("find_corpus_entry_by_id", "corpus entry").
				WithDetail("entry_id", id)
		}
		return nil, err
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, r.entryCacheKey(id), entry, 5*time.Minute)
		r.cache.SetWithTTL(ctx, r.entryHashCacheKey(entry.Hash), entry, 5*time.Minute)
	}

	return entry, nil
}

// FindByHash retrieves a corpus entry by its hash
func (r *CorpusRepository) FindByHash(ctx context.Context, hash string) (*types.CorpusEntry, error) {
	if hash == "" {
		return nil, errors.NewValidationError("find_corpus_entry_by_hash", "hash cannot be empty")
	}

	// Check cache first
	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, r.entryHashCacheKey(hash)); found {
			if entry, ok := cached.(*types.CorpusEntry); ok {
				return entry, nil
			}
		}
	}

	query := `
		SELECT id, input, hash, size, created_at, last_executed_at, execution_count,
			coverage_total_blocks, coverage_covered_blocks, coverage_total_edges,
			coverage_covered_edges, coverage_score, coverage_new_coverage, coverage_gained,
			mutation_parent_id, mutation_method, mutation_generation, mutation_created_at,
			tags, metadata
		FROM corpus_entries
		WHERE hash = ?
	`

	entry, err := r.scanEntry(r.conn.QueryRowContext(ctx, query, hash))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewNotFoundError("find_corpus_entry_by_hash", "corpus entry").
				WithDetail("hash", hash)
		}
		return nil, err
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, r.entryCacheKey(entry.ID), entry, 5*time.Minute)
		r.cache.SetWithTTL(ctx, r.entryHashCacheKey(hash), entry, 5*time.Minute)
	}

	return entry, nil
}

// FindByTag retrieves all entries with a specific tag
func (r *CorpusRepository) FindByTag(ctx context.Context, tag string) ([]*types.CorpusEntry, error) {
	if tag == "" {
		return nil, errors.NewValidationError("find_corpus_entry_by_tag", "tag cannot be empty")
	}

	// SQLite doesn't have native JSON support, so we use LIKE
	query := `
		SELECT id, input, hash, size, created_at, last_executed_at, execution_count,
			coverage_total_blocks, coverage_covered_blocks, coverage_total_edges,
			coverage_covered_edges, coverage_score, coverage_new_coverage, coverage_gained,
			mutation_parent_id, mutation_method, mutation_generation, mutation_created_at,
			tags, metadata
		FROM corpus_entries
		WHERE tags LIKE ?
		ORDER BY created_at DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, fmt.Sprintf(`%%"%s"%%`, tag))
	if err != nil {
		return nil, errors.NewDatabaseError("find_corpus_entry_by_tag", err).
			WithDetail("tag", tag)
	}
	defer rows.Close()

	return r.scanEntries(rows)
}

// FindInteresting retrieves all entries marked as interesting
func (r *CorpusRepository) FindInteresting(ctx context.Context) ([]*types.CorpusEntry, error) {
	query := `
		SELECT id, input, hash, size, created_at, last_executed_at, execution_count,
			coverage_total_blocks, coverage_covered_blocks, coverage_total_edges,
			coverage_covered_edges, coverage_score, coverage_new_coverage, coverage_gained,
			mutation_parent_id, mutation_method, mutation_generation, mutation_created_at,
			tags, metadata
		FROM corpus_entries
		WHERE coverage_new_coverage = 1 OR coverage_gained > 0
		ORDER BY coverage_score DESC
	`

	rows, err := r.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, errors.NewDatabaseError("find_interesting_corpus_entries", err)
	}
	defer rows.Close()

	return r.scanEntries(rows)
}

// FindByParent retrieves all entries derived from a parent
func (r *CorpusRepository) FindByParent(ctx context.Context, parentID string) ([]*types.CorpusEntry, error) {
	if parentID == "" {
		return nil, errors.NewValidationError("find_corpus_entry_by_parent", "parent ID cannot be empty")
	}

	query := `
		SELECT id, input, hash, size, created_at, last_executed_at, execution_count,
			coverage_total_blocks, coverage_covered_blocks, coverage_total_edges,
			coverage_covered_edges, coverage_score, coverage_new_coverage, coverage_gained,
			mutation_parent_id, mutation_method, mutation_generation, mutation_created_at,
			tags, metadata
		FROM corpus_entries
		WHERE mutation_parent_id = ?
		ORDER BY mutation_generation, created_at DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, parentID)
	if err != nil {
		return nil, errors.NewDatabaseError("find_corpus_entry_by_parent", err).
			WithDetail("parent_id", parentID)
	}
	defer rows.Close()

	return r.scanEntries(rows)
}

// FindByCoverage retrieves entries with coverage above threshold
func (r *CorpusRepository) FindByCoverage(ctx context.Context, minCoverage float64) ([]*types.CorpusEntry, error) {
	if minCoverage < 0 || minCoverage > 1 {
		return nil, errors.NewValidationError("find_corpus_entry_by_coverage", "minCoverage must be between 0 and 1")
	}

	query := `
		SELECT id, input, hash, size, created_at, last_executed_at, execution_count,
			coverage_total_blocks, coverage_covered_blocks, coverage_total_edges,
			coverage_covered_edges, coverage_score, coverage_new_coverage, coverage_gained,
			mutation_parent_id, mutation_method, mutation_generation, mutation_created_at,
			tags, metadata
		FROM corpus_entries
		WHERE coverage_score >= ?
		ORDER BY coverage_score DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, minCoverage)
	if err != nil {
		return nil, errors.NewDatabaseError("find_corpus_entry_by_coverage", err).
			WithDetail("min_coverage", minCoverage)
	}
	defer rows.Close()

	return r.scanEntries(rows)
}

// List retrieves entries with pagination
func (r *CorpusRepository) List(ctx context.Context, offset, limit int) ([]*types.CorpusEntry, int, error) {
	if offset < 0 {
		return nil, 0, errors.NewValidationError("list_corpus_entries", "offset cannot be negative")
	}
	if limit <= 0 {
		return nil, 0, errors.NewValidationError("list_corpus_entries", "limit must be positive")
	}

	// Get total count
	countQuery := `SELECT COUNT(*) FROM corpus_entries`
	var total int
	err := r.conn.QueryRowContext(ctx, countQuery).Scan(&total)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("count_corpus_entries", err)
	}

	// Get paginated results
	query := `
		SELECT id, input, hash, size, created_at, last_executed_at, execution_count,
			coverage_total_blocks, coverage_covered_blocks, coverage_total_edges,
			coverage_covered_edges, coverage_score, coverage_new_coverage, coverage_gained,
			mutation_parent_id, mutation_method, mutation_generation, mutation_created_at,
			tags, metadata
		FROM corpus_entries
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.conn.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("list_corpus_entries", err)
	}
	defer rows.Close()

	entries, err := r.scanEntries(rows)
	if err != nil {
		return nil, 0, err
	}

	return entries, total, nil
}

// ListByExecutionCount retrieves entries ordered by execution count
func (r *CorpusRepository) ListByExecutionCount(ctx context.Context, offset, limit int, ascending bool) ([]*types.CorpusEntry, int, error) {
	if offset < 0 {
		return nil, 0, errors.NewValidationError("list_corpus_entries_by_execution", "offset cannot be negative")
	}
	if limit <= 0 {
		return nil, 0, errors.NewValidationError("list_corpus_entries_by_execution", "limit must be positive")
	}

	// Get total count
	countQuery := `SELECT COUNT(*) FROM corpus_entries`
	var total int
	err := r.conn.QueryRowContext(ctx, countQuery).Scan(&total)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("count_corpus_entries", err)
	}

	order := "DESC"
	if ascending {
		order = "ASC"
	}

	// Get paginated results
	query := fmt.Sprintf(`
		SELECT id, input, hash, size, created_at, last_executed_at, execution_count,
			coverage_total_blocks, coverage_covered_blocks, coverage_total_edges,
			coverage_covered_edges, coverage_score, coverage_new_coverage, coverage_gained,
			mutation_parent_id, mutation_method, mutation_generation, mutation_created_at,
			tags, metadata
		FROM corpus_entries
		ORDER BY execution_count %s
		LIMIT ? OFFSET ?
	`, order)

	rows, err := r.conn.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("list_corpus_entries_by_execution", err)
	}
	defer rows.Close()

	entries, err := r.scanEntries(rows)
	if err != nil {
		return nil, 0, err
	}

	return entries, total, nil
}

// UpdateExecutionStats updates execution count and last executed time
func (r *CorpusRepository) UpdateExecutionStats(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("update_execution_stats", "entry ID cannot be empty")
	}

	query := `
		UPDATE corpus_entries
		SET execution_count = execution_count + 1,
			last_executed_at = ?
		WHERE id = ?
	`

	result, err := r.conn.ExecContext(ctx, query, time.Now(), id)
	if err != nil {
		return errors.NewDatabaseError("update_execution_stats", err).
			WithDetail("entry_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_execution_stats_rows", err).
			WithDetail("entry_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_execution_stats", "corpus entry").
			WithDetail("entry_id", id)
	}

	// Invalidate cache for this entry
	if r.cache != nil {
		r.cache.Delete(ctx, r.entryCacheKey(id))
	}

	return nil
}

// Exists checks if an entry exists by ID
func (r *CorpusRepository) Exists(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, errors.NewValidationError("corpus_entry_exists", "entry ID cannot be empty")
	}

	query := `SELECT 1 FROM corpus_entries WHERE id = ? LIMIT 1`

	var exists int
	err := r.conn.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, errors.NewDatabaseError("corpus_entry_exists", err).
			WithDetail("entry_id", id)
	}

	return true, nil
}

// ExistsByHash checks if an entry exists by hash
func (r *CorpusRepository) ExistsByHash(ctx context.Context, hash string) (bool, error) {
	if hash == "" {
		return false, errors.NewValidationError("corpus_entry_exists_by_hash", "hash cannot be empty")
	}

	query := `SELECT 1 FROM corpus_entries WHERE hash = ? LIMIT 1`

	var exists int
	err := r.conn.QueryRowContext(ctx, query, hash).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, errors.NewDatabaseError("corpus_entry_exists_by_hash", err).
			WithDetail("hash", hash)
	}

	return true, nil
}

// Count returns the total number of entries
func (r *CorpusRepository) Count(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM corpus_entries`

	var count int
	err := r.conn.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, errors.NewDatabaseError("count_corpus_entries", err)
	}

	return count, nil
}

// CountInteresting returns the number of interesting entries
func (r *CorpusRepository) CountInteresting(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM corpus_entries WHERE coverage_new_coverage = 1 OR coverage_gained > 0`

	var count int
	err := r.conn.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, errors.NewDatabaseError("count_interesting_corpus_entries", err)
	}

	return count, nil
}

// GetStats retrieves aggregate statistics
func (r *CorpusRepository) GetStats(ctx context.Context) (*types.CollectionStats, error) {
	// Check cache first
	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, "corpus:stats"); found {
			if stats, ok := cached.(*types.CollectionStats); ok {
				return stats, nil
			}
		}
	}

	query := `
		SELECT 
			COUNT(*) as total_entries,
			COALESCE(SUM(size), 0) as total_size,
			COALESCE(SUM(execution_count), 0) as total_executions,
			COALESCE(AVG(coverage_score), 0.0) as average_coverage,
			COALESCE(MAX(coverage_covered_edges), 0) as unique_edges,
			COALESCE(SUM(CASE WHEN coverage_new_coverage = 1 OR coverage_gained > 0 THEN 1 ELSE 0 END), 0) as interesting_inputs
		FROM corpus_entries
	`

	stats := &types.CollectionStats{
		LastUpdated: time.Now(),
	}

	err := r.conn.QueryRowContext(ctx, query).Scan(
		&stats.TotalEntries,
		&stats.TotalSize,
		&stats.TotalExecutions,
		&stats.AverageCoverage,
		&stats.UniqueEdges,
		&stats.InterestingInputs,
	)

	if err != nil {
		return nil, errors.NewDatabaseError("get_corpus_stats", err)
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, "corpus:stats", stats, 1*time.Minute)
	}

	return stats, nil
}

// CorpusCollectionRepository methods

// CreateCollection creates a new corpus collection
func (r *CorpusRepository) CreateCollection(ctx context.Context, collection *types.CorpusCollection) error {
	if collection == nil {
		return errors.NewValidationError("create_collection", "collection cannot be nil")
	}

	if collection.Name() == "" {
		return errors.NewValidationError("create_collection", "collection name cannot be empty")
	}

	query := `
		INSERT INTO corpus_collections (name, description, created_at, max_size)
		VALUES (?, ?, ?, ?)
	`

	_, err := r.conn.ExecContext(ctx, query,
		collection.Name(),
		collection.Description(),
		collection.CreatedAt(),
		collection.Size(), // Using current size as max_size
	)

	if err != nil {
		if r.isUniqueConstraintError(err) {
			return errors.NewConflictError("create_collection", "collection already exists").
				WithDetail("collection_name", collection.Name())
		}
		return errors.NewDatabaseError("create_collection", err).
			WithDetail("collection_name", collection.Name())
	}

	r.log.WithField("collection_name", collection.Name()).Debug("Collection created")
	return nil
}

// UpdateCollection updates an existing collection
func (r *CorpusRepository) UpdateCollection(ctx context.Context, collection *types.CorpusCollection) error {
	if collection == nil {
		return errors.NewValidationError("update_collection", "collection cannot be nil")
	}

	query := `
		UPDATE corpus_collections
		SET description = ?, max_size = ?
		WHERE name = ?
	`

	result, err := r.conn.ExecContext(ctx, query,
		collection.Description(),
		collection.Size(),
		collection.Name(),
	)

	if err != nil {
		return errors.NewDatabaseError("update_collection", err).
			WithDetail("collection_name", collection.Name())
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_collection_rows", err).
			WithDetail("collection_name", collection.Name())
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_collection", "collection").
			WithDetail("collection_name", collection.Name())
	}

	r.log.WithField("collection_name", collection.Name()).Debug("Collection updated")
	return nil
}

// DeleteCollection deletes a collection by name
func (r *CorpusRepository) DeleteCollection(ctx context.Context, name string) error {
	if name == "" {
		return errors.NewValidationError("delete_collection", "collection name cannot be empty")
	}

	query := `DELETE FROM corpus_collections WHERE name = ?`

	result, err := r.conn.ExecContext(ctx, query, name)
	if err != nil {
		return errors.NewDatabaseError("delete_collection", err).
			WithDetail("collection_name", name)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("delete_collection_rows", err).
			WithDetail("collection_name", name)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("delete_collection", "collection").
			WithDetail("collection_name", name)
	}

	r.log.WithField("collection_name", name).Debug("Collection deleted")
	return nil
}

// FindCollectionByName retrieves a collection by name
func (r *CorpusRepository) FindCollectionByName(ctx context.Context, name string) (*types.CorpusCollection, error) {
	if name == "" {
		return nil, errors.NewValidationError("find_collection_by_name", "collection name cannot be empty")
	}

	// First get collection info
	query := `
		SELECT name, description, created_at, max_size
		FROM corpus_collections
		WHERE name = ?
	`

	var desc, maxSize sql.NullString
	var createdAt time.Time

	err := r.conn.QueryRowContext(ctx, query, name).Scan(&name, &desc, &createdAt, &maxSize)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewNotFoundError("find_collection_by_name", "collection").
				WithDetail("collection_name", name)
		}
		return nil, errors.NewDatabaseError("find_collection_by_name", err).
			WithDetail("collection_name", name)
	}

	// Create collection (we can't reconstruct the full in-memory state, but we can create a basic one)
	maxSizeInt := 0
	if maxSize.Valid {
		fmt.Sscanf(maxSize.String, "%d", &maxSizeInt)
	}

	collection, err := types.NewCorpusCollection(name, maxSizeInt)
	if err != nil {
		return nil, errors.NewSystemError("create_collection_instance", err)
	}

	if desc.Valid {
		collection.SetDescription(desc.String)
	}

	// Get all entries in this collection
	entries, err := r.GetCollectionEntries(ctx, name)
	if err != nil {
		return nil, err
	}

	// Add entries to collection
	for _, entry := range entries {
		if err := collection.Add(entry); err != nil {
			r.log.WithError(err).WithField("entry_id", entry.ID).Warn("Failed to add entry to collection")
		}
	}

	return collection, nil
}

// ListCollections retrieves all collections
func (r *CorpusRepository) ListCollections(ctx context.Context) ([]*types.CorpusCollection, error) {
	query := `
		SELECT name, description, created_at, max_size
		FROM corpus_collections
		ORDER BY created_at DESC
	`

	rows, err := r.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, errors.NewDatabaseError("list_collections", err)
	}
	defer rows.Close()

	var collections []*types.CorpusCollection

	for rows.Next() {
		var name string
		var desc, maxSize sql.NullString
		var createdAt time.Time

		err := rows.Scan(&name, &desc, &createdAt, &maxSize)
		if err != nil {
			return nil, errors.NewDatabaseError("scan_collection", err)
		}

		maxSizeInt := 0
		if maxSize.Valid {
			fmt.Sscanf(maxSize.String, "%d", &maxSizeInt)
		}

		collection, err := types.NewCorpusCollection(name, maxSizeInt)
		if err != nil {
			r.log.WithError(err).WithField("collection_name", name).Warn("Failed to create collection instance")
			continue
		}

		if desc.Valid {
			collection.SetDescription(desc.String)
		}

		collections = append(collections, collection)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.NewDatabaseError("list_collections_rows", err)
	}

	return collections, nil
}

// AddEntryToCollection adds an entry to a collection
func (r *CorpusRepository) AddEntryToCollection(ctx context.Context, collectionName string, entryID string) error {
	if collectionName == "" {
		return errors.NewValidationError("add_entry_to_collection", "collection name cannot be empty")
	}
	if entryID == "" {
		return errors.NewValidationError("add_entry_to_collection", "entry ID cannot be empty")
	}

	query := `
		INSERT INTO corpus_collection_entries (collection_name, entry_id)
		VALUES (?, ?)
	`

	_, err := r.conn.ExecContext(ctx, query, collectionName, entryID)
	if err != nil {
		if r.isUniqueConstraintError(err) {
			return errors.NewConflictError("add_entry_to_collection", "entry already in collection").
				WithDetail("collection_name", collectionName).
				WithDetail("entry_id", entryID)
		}
		if r.isForeignKeyError(err) {
			return errors.NewNotFoundError("add_entry_to_collection", "collection or entry").
				WithDetail("collection_name", collectionName).
				WithDetail("entry_id", entryID)
		}
		return errors.NewDatabaseError("add_entry_to_collection", err).
			WithDetail("collection_name", collectionName).
			WithDetail("entry_id", entryID)
	}

	return nil
}

// RemoveEntryFromCollection removes an entry from a collection
func (r *CorpusRepository) RemoveEntryFromCollection(ctx context.Context, collectionName string, entryID string) error {
	if collectionName == "" {
		return errors.NewValidationError("remove_entry_from_collection", "collection name cannot be empty")
	}
	if entryID == "" {
		return errors.NewValidationError("remove_entry_from_collection", "entry ID cannot be empty")
	}

	query := `
		DELETE FROM corpus_collection_entries
		WHERE collection_name = ? AND entry_id = ?
	`

	result, err := r.conn.ExecContext(ctx, query, collectionName, entryID)
	if err != nil {
		return errors.NewDatabaseError("remove_entry_from_collection", err).
			WithDetail("collection_name", collectionName).
			WithDetail("entry_id", entryID)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("remove_entry_from_collection_rows", err).
			WithDetail("collection_name", collectionName).
			WithDetail("entry_id", entryID)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("remove_entry_from_collection", "entry in collection").
			WithDetail("collection_name", collectionName).
			WithDetail("entry_id", entryID)
	}

	return nil
}

// GetCollectionEntries retrieves all entries in a collection
func (r *CorpusRepository) GetCollectionEntries(ctx context.Context, collectionName string) ([]*types.CorpusEntry, error) {
	if collectionName == "" {
		return nil, errors.NewValidationError("get_collection_entries", "collection name cannot be empty")
	}

	query := `
		SELECT e.id, e.input, e.hash, e.size, e.created_at, e.last_executed_at, e.execution_count,
			e.coverage_total_blocks, e.coverage_covered_blocks, e.coverage_total_edges,
			e.coverage_covered_edges, e.coverage_score, e.coverage_new_coverage, e.coverage_gained,
			e.mutation_parent_id, e.mutation_method, e.mutation_generation, e.mutation_created_at,
			e.tags, e.metadata
		FROM corpus_entries e
		INNER JOIN corpus_collection_entries ce ON e.id = ce.entry_id
		WHERE ce.collection_name = ?
		ORDER BY ce.added_at DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, collectionName)
	if err != nil {
		return nil, errors.NewDatabaseError("get_collection_entries", err).
			WithDetail("collection_name", collectionName)
	}
	defer rows.Close()

	return r.scanEntries(rows)
}

// CollectionExists checks if a collection exists
func (r *CorpusRepository) CollectionExists(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, errors.NewValidationError("collection_exists", "collection name cannot be empty")
	}

	query := `SELECT 1 FROM corpus_collections WHERE name = ? LIMIT 1`

	var exists int
	err := r.conn.QueryRowContext(ctx, query, name).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, errors.NewDatabaseError("collection_exists", err).
			WithDetail("collection_name", name)
	}

	return true, nil
}

// BeginTransaction starts a new transaction
func (r *CorpusRepository) BeginTransaction(ctx context.Context) (repository.CorpusTransaction, error) {
	tx, err := r.conn.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.NewDatabaseError("begin_transaction", err)
	}

	return &corpusTransaction{
		tx:   tx,
		repo: r,
		log:  r.log,
	}, nil
}

// Helper methods

func (r *CorpusRepository) scanEntry(row *sql.Row) (*types.CorpusEntry, error) {
	var entry types.CorpusEntry
	var lastExecutedAt sql.NullTime
	var tagsJSON string
	var metadataJSON sql.NullString
	var parentID, mutationMethod sql.NullString
	var newCoverage int

	err := row.Scan(
		&entry.ID,
		&entry.Input,
		&entry.Hash,
		&entry.Size,
		&entry.CreatedAt,
		&lastExecutedAt,
		&entry.ExecutionCount,
		&entry.Coverage.TotalBlocks,
		&entry.Coverage.CoveredBlocks,
		&entry.Coverage.TotalEdges,
		&entry.Coverage.CoveredEdges,
		&entry.Coverage.CoverageScore,
		&newCoverage,
		&entry.Coverage.CoverageGained,
		&parentID,
		&mutationMethod,
		&entry.MutationInfo.Generation,
		&entry.MutationInfo.CreatedAt,
		&tagsJSON,
		&metadataJSON,
	)

	if err != nil {
		return nil, errors.NewDatabaseError("scan_corpus_entry", err)
	}

	// Handle nullable fields
	if lastExecutedAt.Valid {
		entry.LastExecutedAt = &lastExecutedAt.Time
	}

	entry.Coverage.NewCoverage = intToBool(newCoverage)

	if parentID.Valid {
		entry.MutationInfo.ParentID = parentID.String
	}
	if mutationMethod.Valid {
		entry.MutationInfo.MutationMethod = mutationMethod.String
	}

	// Unmarshal tags
	if tagsJSON != "" && tagsJSON != "null" {
		if err := json.Unmarshal([]byte(tagsJSON), &entry.Tags); err != nil {
			return nil, errors.NewSystemError("unmarshal_tags", err)
		}
	} else {
		entry.Tags = []string{}
	}

	// Unmarshal metadata if present
	if metadataJSON.Valid && metadataJSON.String != "" && metadataJSON.String != "null" {
		entry.Metadata = make(map[string]string)
		if err := json.Unmarshal([]byte(metadataJSON.String), &entry.Metadata); err != nil {
			return nil, errors.NewSystemError("unmarshal_metadata", err)
		}
	} else {
		entry.Metadata = make(map[string]string)
	}

	return &entry, nil
}

func (r *CorpusRepository) scanEntries(rows *sql.Rows) ([]*types.CorpusEntry, error) {
	var entries []*types.CorpusEntry

	for rows.Next() {
		var entry types.CorpusEntry
		var lastExecutedAt sql.NullTime
		var tagsJSON string
		var metadataJSON sql.NullString
		var parentID, mutationMethod sql.NullString
		var newCoverage int

		err := rows.Scan(
			&entry.ID,
			&entry.Input,
			&entry.Hash,
			&entry.Size,
			&entry.CreatedAt,
			&lastExecutedAt,
			&entry.ExecutionCount,
			&entry.Coverage.TotalBlocks,
			&entry.Coverage.CoveredBlocks,
			&entry.Coverage.TotalEdges,
			&entry.Coverage.CoveredEdges,
			&entry.Coverage.CoverageScore,
			&newCoverage,
			&entry.Coverage.CoverageGained,
			&parentID,
			&mutationMethod,
			&entry.MutationInfo.Generation,
			&entry.MutationInfo.CreatedAt,
			&tagsJSON,
			&metadataJSON,
		)
		if err != nil {
			return nil, errors.NewDatabaseError("scan_corpus_entry", err)
		}

		// Handle nullable fields
		if lastExecutedAt.Valid {
			entry.LastExecutedAt = &lastExecutedAt.Time
		}

		entry.Coverage.NewCoverage = intToBool(newCoverage)

		if parentID.Valid {
			entry.MutationInfo.ParentID = parentID.String
		}
		if mutationMethod.Valid {
			entry.MutationInfo.MutationMethod = mutationMethod.String
		}

		// Unmarshal tags
		if tagsJSON != "" && tagsJSON != "null" {
			if err := json.Unmarshal([]byte(tagsJSON), &entry.Tags); err != nil {
				return nil, errors.NewSystemError("unmarshal_tags", err)
			}
		} else {
			entry.Tags = []string{}
		}

		// Unmarshal metadata if present
		if metadataJSON.Valid && metadataJSON.String != "" && metadataJSON.String != "null" {
			entry.Metadata = make(map[string]string)
			if err := json.Unmarshal([]byte(metadataJSON.String), &entry.Metadata); err != nil {
				return nil, errors.NewSystemError("unmarshal_metadata", err)
			}
		} else {
			entry.Metadata = make(map[string]string)
		}

		entries = append(entries, &entry)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.NewDatabaseError("scan_corpus_entries_rows", err)
	}

	return entries, nil
}

func (r *CorpusRepository) isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "UNIQUE constraint failed")
}

func (r *CorpusRepository) isForeignKeyError(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "FOREIGN KEY constraint failed")
}

func (r *CorpusRepository) entryCacheKey(id string) string {
	return fmt.Sprintf("corpus:entry:%s", id)
}

func (r *CorpusRepository) entryHashCacheKey(hash string) string {
	return fmt.Sprintf("corpus:hash:%s", hash)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(i int) bool {
	return i != 0
}

// corpusTransaction implements CorpusTransaction
type corpusTransaction struct {
	tx   *sql.Tx
	repo *CorpusRepository
	log  logrus.FieldLogger
}

// Commit commits the transaction
func (t *corpusTransaction) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction
func (t *corpusTransaction) Rollback() error {
	return t.tx.Rollback()
}

// CreateEntryTx creates an entry within a transaction
func (t *corpusTransaction) CreateEntryTx(ctx context.Context, entry *types.CorpusEntry) error {
	if entry == nil {
		return errors.NewValidationError("create_corpus_entry_tx", "entry cannot be nil")
	}

	if err := entry.Validate(); err != nil {
		return errors.NewValidationError("create_corpus_entry_tx", err.Error())
	}

	tags, err := json.Marshal(entry.Tags)
	if err != nil {
		return errors.NewSystemError("marshal_tags", err)
	}

	var metadata []byte
	if entry.Metadata != nil && len(entry.Metadata) > 0 {
		metadata, err = json.Marshal(entry.Metadata)
		if err != nil {
			return errors.NewSystemError("marshal_metadata", err)
		}
	}

	query := `
		INSERT INTO corpus_entries (
			id, input, hash, size, created_at, last_executed_at, execution_count,
			coverage_total_blocks, coverage_covered_blocks, coverage_total_edges,
			coverage_covered_edges, coverage_score, coverage_new_coverage, coverage_gained,
			mutation_parent_id, mutation_method, mutation_generation, mutation_created_at,
			tags, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = t.tx.ExecContext(ctx, query,
		entry.ID,
		entry.Input,
		entry.Hash,
		entry.Size,
		entry.CreatedAt,
		entry.LastExecutedAt,
		entry.ExecutionCount,
		entry.Coverage.TotalBlocks,
		entry.Coverage.CoveredBlocks,
		entry.Coverage.TotalEdges,
		entry.Coverage.CoveredEdges,
		entry.Coverage.CoverageScore,
		boolToInt(entry.Coverage.NewCoverage),
		entry.Coverage.CoverageGained,
		entry.MutationInfo.ParentID,
		entry.MutationInfo.MutationMethod,
		entry.MutationInfo.Generation,
		entry.MutationInfo.CreatedAt,
		string(tags),
		metadata,
	)

	if err != nil {
		if t.repo.isUniqueConstraintError(err) {
			return errors.NewConflictError("create_corpus_entry_tx", "corpus entry already exists").
				WithDetail("entry_id", entry.ID).
				WithDetail("hash", entry.Hash)
		}
		return errors.NewDatabaseError("create_corpus_entry_tx", err).
			WithDetail("entry_id", entry.ID)
	}

	return nil
}

// UpdateEntryTx updates an entry within a transaction
func (t *corpusTransaction) UpdateEntryTx(ctx context.Context, entry *types.CorpusEntry) error {
	if entry == nil {
		return errors.NewValidationError("update_corpus_entry_tx", "entry cannot be nil")
	}

	if err := entry.Validate(); err != nil {
		return errors.NewValidationError("update_corpus_entry_tx", err.Error())
	}

	tags, err := json.Marshal(entry.Tags)
	if err != nil {
		return errors.NewSystemError("marshal_tags", err)
	}

	var metadata []byte
	if entry.Metadata != nil && len(entry.Metadata) > 0 {
		metadata, err = json.Marshal(entry.Metadata)
		if err != nil {
			return errors.NewSystemError("marshal_metadata", err)
		}
	}

	query := `
		UPDATE corpus_entries SET
			last_executed_at = ?, execution_count = ?,
			coverage_total_blocks = ?, coverage_covered_blocks = ?,
			coverage_total_edges = ?, coverage_covered_edges = ?,
			coverage_score = ?, coverage_new_coverage = ?, coverage_gained = ?,
			tags = ?, metadata = ?
		WHERE id = ?
	`

	result, err := t.tx.ExecContext(ctx, query,
		entry.LastExecutedAt,
		entry.ExecutionCount,
		entry.Coverage.TotalBlocks,
		entry.Coverage.CoveredBlocks,
		entry.Coverage.TotalEdges,
		entry.Coverage.CoveredEdges,
		entry.Coverage.CoverageScore,
		boolToInt(entry.Coverage.NewCoverage),
		entry.Coverage.CoverageGained,
		string(tags),
		metadata,
		entry.ID,
	)

	if err != nil {
		return errors.NewDatabaseError("update_corpus_entry_tx", err).
			WithDetail("entry_id", entry.ID)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_corpus_entry_tx_rows", err).
			WithDetail("entry_id", entry.ID)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_corpus_entry_tx", "corpus entry").
			WithDetail("entry_id", entry.ID)
	}

	return nil
}

// DeleteEntryTx deletes an entry within a transaction
func (t *corpusTransaction) DeleteEntryTx(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("delete_corpus_entry_tx", "entry ID cannot be empty")
	}

	query := `DELETE FROM corpus_entries WHERE id = ?`

	result, err := t.tx.ExecContext(ctx, query, id)
	if err != nil {
		return errors.NewDatabaseError("delete_corpus_entry_tx", err).
			WithDetail("entry_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("delete_corpus_entry_tx_rows", err).
			WithDetail("entry_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("delete_corpus_entry_tx", "corpus entry").
			WithDetail("entry_id", id)
	}

	return nil
}

// CreateCollectionTx creates a collection within a transaction
func (t *corpusTransaction) CreateCollectionTx(ctx context.Context, collection *types.CorpusCollection) error {
	if collection == nil {
		return errors.NewValidationError("create_collection_tx", "collection cannot be nil")
	}

	if collection.Name() == "" {
		return errors.NewValidationError("create_collection_tx", "collection name cannot be empty")
	}

	query := `
		INSERT INTO corpus_collections (name, description, created_at, max_size)
		VALUES (?, ?, ?, ?)
	`

	_, err := t.tx.ExecContext(ctx, query,
		collection.Name(),
		collection.Description(),
		collection.CreatedAt(),
		collection.Size(),
	)

	if err != nil {
		if t.repo.isUniqueConstraintError(err) {
			return errors.NewConflictError("create_collection_tx", "collection already exists").
				WithDetail("collection_name", collection.Name())
		}
		return errors.NewDatabaseError("create_collection_tx", err).
			WithDetail("collection_name", collection.Name())
	}

	return nil
}

// UpdateCollectionTx updates a collection within a transaction
func (t *corpusTransaction) UpdateCollectionTx(ctx context.Context, collection *types.CorpusCollection) error {
	if collection == nil {
		return errors.NewValidationError("update_collection_tx", "collection cannot be nil")
	}

	query := `
		UPDATE corpus_collections
		SET description = ?, max_size = ?
		WHERE name = ?
	`

	result, err := t.tx.ExecContext(ctx, query,
		collection.Description(),
		collection.Size(),
		collection.Name(),
	)

	if err != nil {
		return errors.NewDatabaseError("update_collection_tx", err).
			WithDetail("collection_name", collection.Name())
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_collection_tx_rows", err).
			WithDetail("collection_name", collection.Name())
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_collection_tx", "collection").
			WithDetail("collection_name", collection.Name())
	}

	return nil
}

// DeleteCollectionTx deletes a collection within a transaction
func (t *corpusTransaction) DeleteCollectionTx(ctx context.Context, name string) error {
	if name == "" {
		return errors.NewValidationError("delete_collection_tx", "collection name cannot be empty")
	}

	query := `DELETE FROM corpus_collections WHERE name = ?`

	result, err := t.tx.ExecContext(ctx, query, name)
	if err != nil {
		return errors.NewDatabaseError("delete_collection_tx", err).
			WithDetail("collection_name", name)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("delete_collection_tx_rows", err).
			WithDetail("collection_name", name)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("delete_collection_tx", "collection").
			WithDetail("collection_name", name)
	}

	return nil
}

// AddEntryToCollectionTx adds an entry to a collection within a transaction
func (t *corpusTransaction) AddEntryToCollectionTx(ctx context.Context, collectionName string, entryID string) error {
	if collectionName == "" {
		return errors.NewValidationError("add_entry_to_collection_tx", "collection name cannot be empty")
	}
	if entryID == "" {
		return errors.NewValidationError("add_entry_to_collection_tx", "entry ID cannot be empty")
	}

	query := `
		INSERT INTO corpus_collection_entries (collection_name, entry_id)
		VALUES (?, ?)
	`

	_, err := t.tx.ExecContext(ctx, query, collectionName, entryID)
	if err != nil {
		if t.repo.isUniqueConstraintError(err) {
			return errors.NewConflictError("add_entry_to_collection_tx", "entry already in collection").
				WithDetail("collection_name", collectionName).
				WithDetail("entry_id", entryID)
		}
		if t.repo.isForeignKeyError(err) {
			return errors.NewNotFoundError("add_entry_to_collection_tx", "collection or entry").
				WithDetail("collection_name", collectionName).
				WithDetail("entry_id", entryID)
		}
		return errors.NewDatabaseError("add_entry_to_collection_tx", err).
			WithDetail("collection_name", collectionName).
			WithDetail("entry_id", entryID)
	}

	return nil
}

// RemoveEntryFromCollectionTx removes an entry from a collection within a transaction
func (t *corpusTransaction) RemoveEntryFromCollectionTx(ctx context.Context, collectionName string, entryID string) error {
	if collectionName == "" {
		return errors.NewValidationError("remove_entry_from_collection_tx", "collection name cannot be empty")
	}
	if entryID == "" {
		return errors.NewValidationError("remove_entry_from_collection_tx", "entry ID cannot be empty")
	}

	query := `
		DELETE FROM corpus_collection_entries
		WHERE collection_name = ? AND entry_id = ?
	`

	result, err := t.tx.ExecContext(ctx, query, collectionName, entryID)
	if err != nil {
		return errors.NewDatabaseError("remove_entry_from_collection_tx", err).
			WithDetail("collection_name", collectionName).
			WithDetail("entry_id", entryID)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("remove_entry_from_collection_tx_rows", err).
			WithDetail("collection_name", collectionName).
			WithDetail("entry_id", entryID)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("remove_entry_from_collection_tx", "entry in collection").
			WithDetail("collection_name", collectionName).
			WithDetail("entry_id", entryID)
	}

	return nil
}

// Ensure interfaces are implemented
var (
	_ repository.CorpusEntryRepository       = (*CorpusRepository)(nil)
	_ repository.CorpusCollectionRepository  = (*CorpusRepository)(nil)
	_ repository.CorpusTransactionRepository = (*CorpusRepository)(nil)
	_ repository.CorpusTransaction           = (*corpusTransaction)(nil)
)
