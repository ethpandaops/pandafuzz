package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	corpusrepo "github.com/ethpandaops/pandafuzz/pkg/domain/corpus/repository"
	corpustypes "github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/mappers"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/models"
	"github.com/sirupsen/logrus"
)

// Compile-time interface compliance checks
var _ corpusrepo.CorpusEntryRepository = (*CorpusRepository)(nil)
var _ corpusrepo.CorpusCollectionRepository = (*CorpusRepository)(nil)

// CorpusRepository implements corpus repositories using SQLite
type CorpusRepository struct {
	db     *sql.DB
	logger logrus.FieldLogger
}

// NewCorpusRepository creates a new SQLite-based corpus repository
func NewCorpusRepository(db *sql.DB, logger logrus.FieldLogger) *CorpusRepository {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	return &CorpusRepository{
		db:     db,
		logger: logger.WithField("component", "corpus_repository"),
	}
}

// CorpusEntryRepository implementation

// Create creates a new corpus entry
func (r *CorpusRepository) Create(ctx context.Context, entry *corpustypes.CorpusEntry) error {
	if entry == nil {
		return NewRepositoryError("create", "corpus_entry", "", fmt.Errorf("entry cannot be nil"))
	}

	row := mappers.DomainCorpusEntryToRow(entry)

	query := `
		INSERT INTO corpus_entries (
			id, input, hash, size, created_at, last_executed_at,
			execution_count, coverage, mutation_info, tags, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		row.ID, row.Input, row.Hash, row.Size, row.CreatedAt, row.LastExecutedAt,
		row.ExecutionCount, row.CoverageJSON, row.MutationJSON, row.TagsJSON, row.MetadataJSON,
	)

	if err != nil {
		return NewRepositoryError("create", "corpus_entry", entry.ID, err)
	}

	r.logger.WithField("entry_id", entry.ID).Debug("Corpus entry created")
	return nil
}

// Update updates an existing corpus entry
func (r *CorpusRepository) Update(ctx context.Context, entry *corpustypes.CorpusEntry) error {
	if entry == nil {
		return NewRepositoryError("update", "corpus_entry", "", fmt.Errorf("entry cannot be nil"))
	}

	row := mappers.DomainCorpusEntryToRow(entry)

	query := `
		UPDATE corpus_entries SET
			input = ?, hash = ?, size = ?, last_executed_at = ?,
			execution_count = ?, coverage = ?, mutation_info = ?, tags = ?, metadata = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query,
		row.Input, row.Hash, row.Size, row.LastExecutedAt,
		row.ExecutionCount, row.CoverageJSON, row.MutationJSON, row.TagsJSON, row.MetadataJSON,
		entry.ID,
	)

	if err != nil {
		return NewRepositoryError("update", "corpus_entry", entry.ID, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("update", "corpus_entry", entry.ID, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("entry_id", entry.ID).Debug("Corpus entry updated")
	return nil
}

// Delete deletes a corpus entry by ID
func (r *CorpusRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM corpus_entries WHERE id = ?", id)
	if err != nil {
		return NewRepositoryError("delete", "corpus_entry", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("delete", "corpus_entry", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("entry_id", id).Debug("Corpus entry deleted")
	return nil
}

// FindByID retrieves a corpus entry by its ID
func (r *CorpusRepository) FindByID(ctx context.Context, id string) (*corpustypes.CorpusEntry, error) {
	row, err := r.scanEntryRow(ctx, "SELECT "+entrySelectColumns()+" FROM corpus_entries WHERE id = ?", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, NewRepositoryError("find_by_id", "corpus_entry", id, err)
	}

	return mappers.CorpusEntryRowToDomain(row), nil
}

// FindByHash retrieves a corpus entry by its hash
func (r *CorpusRepository) FindByHash(ctx context.Context, hash string) (*corpustypes.CorpusEntry, error) {
	row, err := r.scanEntryRow(ctx, "SELECT "+entrySelectColumns()+" FROM corpus_entries WHERE hash = ?", hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, NewRepositoryError("find_by_hash", "corpus_entry", hash, err)
	}

	return mappers.CorpusEntryRowToDomain(row), nil
}

// FindByTag retrieves all entries with a specific tag
func (r *CorpusRepository) FindByTag(ctx context.Context, tag string) ([]*corpustypes.CorpusEntry, error) {
	query := "SELECT " + entrySelectColumns() + " FROM corpus_entries WHERE tags LIKE ? ORDER BY created_at DESC"
	return r.scanEntryRows(ctx, query, "%\""+tag+"\"%")
}

// FindInteresting retrieves all entries marked as interesting
func (r *CorpusRepository) FindInteresting(ctx context.Context) ([]*corpustypes.CorpusEntry, error) {
	// Interesting entries have new_coverage=true or coverage_gained>0 in their coverage JSON
	query := `
		SELECT ` + entrySelectColumns() + `
		FROM corpus_entries
		WHERE coverage LIKE '%"new_coverage":true%'
		   OR coverage LIKE '%"coverage_gained":%'
		ORDER BY created_at DESC
	`
	return r.scanEntryRows(ctx, query)
}

// FindByParent retrieves all entries derived from a parent
func (r *CorpusRepository) FindByParent(ctx context.Context, parentID string) ([]*corpustypes.CorpusEntry, error) {
	query := "SELECT " + entrySelectColumns() + " FROM corpus_entries WHERE mutation_info LIKE ? ORDER BY created_at DESC"
	return r.scanEntryRows(ctx, query, "%\"parent_id\":\""+parentID+"\"%")
}

// FindByCoverage retrieves entries with coverage above threshold
func (r *CorpusRepository) FindByCoverage(ctx context.Context, minCoverage float64) ([]*corpustypes.CorpusEntry, error) {
	// This is a simplified implementation - real implementation would need JSON functions
	query := "SELECT " + entrySelectColumns() + " FROM corpus_entries ORDER BY created_at DESC"
	entries, err := r.scanEntryRows(ctx, query)
	if err != nil {
		return nil, err
	}

	// Filter by coverage in memory
	filtered := make([]*corpustypes.CorpusEntry, 0)
	for _, entry := range entries {
		if entry.Coverage.CoverageScore >= minCoverage {
			filtered = append(filtered, entry)
		}
	}

	return filtered, nil
}

// List retrieves entries with pagination
func (r *CorpusRepository) List(ctx context.Context, offset, limit int) ([]*corpustypes.CorpusEntry, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM corpus_entries").Scan(&total)
	if err != nil {
		return nil, 0, NewRepositoryError("list", "corpus_entry", "", err)
	}

	query := "SELECT " + entrySelectColumns() + " FROM corpus_entries ORDER BY created_at DESC LIMIT ? OFFSET ?"
	entries, err := r.scanEntryRows(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	return entries, total, nil
}

// ListByExecutionCount retrieves entries ordered by execution count
func (r *CorpusRepository) ListByExecutionCount(ctx context.Context, offset, limit int, ascending bool) ([]*corpustypes.CorpusEntry, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM corpus_entries").Scan(&total)
	if err != nil {
		return nil, 0, NewRepositoryError("list_by_execution_count", "corpus_entry", "", err)
	}

	order := "DESC"
	if ascending {
		order = "ASC"
	}

	query := fmt.Sprintf("SELECT %s FROM corpus_entries ORDER BY execution_count %s LIMIT ? OFFSET ?", entrySelectColumns(), order)
	entries, err := r.scanEntryRows(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	return entries, total, nil
}

// UpdateExecutionStats updates execution count and last executed time
func (r *CorpusRepository) UpdateExecutionStats(ctx context.Context, id string) error {
	now := time.Now().UTC()
	query := `
		UPDATE corpus_entries
		SET execution_count = execution_count + 1, last_executed_at = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query, now, id)
	if err != nil {
		return NewRepositoryError("update_execution_stats", "corpus_entry", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("update_execution_stats", "corpus_entry", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// Exists checks if an entry exists by ID
func (r *CorpusRepository) Exists(ctx context.Context, id string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM corpus_entries WHERE id = ?)"
	err := r.db.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		return false, NewRepositoryError("exists", "corpus_entry", id, err)
	}
	return exists, nil
}

// ExistsByHash checks if an entry exists by hash
func (r *CorpusRepository) ExistsByHash(ctx context.Context, hash string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM corpus_entries WHERE hash = ?)"
	err := r.db.QueryRowContext(ctx, query, hash).Scan(&exists)
	if err != nil {
		return false, NewRepositoryError("exists_by_hash", "corpus_entry", "", err)
	}
	return exists, nil
}

// Count returns the total number of entries
func (r *CorpusRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM corpus_entries").Scan(&count)
	if err != nil {
		return 0, NewRepositoryError("count", "corpus_entry", "", err)
	}
	return count, nil
}

// CountInteresting returns the number of interesting entries
func (r *CorpusRepository) CountInteresting(ctx context.Context) (int, error) {
	var count int
	query := `
		SELECT COUNT(*) FROM corpus_entries
		WHERE coverage LIKE '%"new_coverage":true%'
		   OR coverage LIKE '%"coverage_gained":%'
	`
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, NewRepositoryError("count_interesting", "corpus_entry", "", err)
	}
	return count, nil
}

// GetStats retrieves aggregate statistics
func (r *CorpusRepository) GetStats(ctx context.Context) (*corpustypes.CollectionStats, error) {
	stats := &corpustypes.CollectionStats{
		LastUpdated: time.Now().UTC(),
	}

	// Total entries
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM corpus_entries").Scan(&stats.TotalEntries)
	if err != nil {
		return nil, NewRepositoryError("get_stats", "corpus_entry", "", err)
	}

	// Total size
	err = r.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(size), 0) FROM corpus_entries").Scan(&stats.TotalSize)
	if err != nil {
		return nil, NewRepositoryError("get_stats", "corpus_entry", "", err)
	}

	// Total executions
	err = r.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(execution_count), 0) FROM corpus_entries").Scan(&stats.TotalExecutions)
	if err != nil {
		return nil, NewRepositoryError("get_stats", "corpus_entry", "", err)
	}

	// Interesting inputs
	interesting, err := r.CountInteresting(ctx)
	if err != nil {
		return nil, err
	}
	stats.InterestingInputs = interesting

	return stats, nil
}

// CorpusCollectionRepository implementation

// CreateCollection creates a new corpus collection
func (r *CorpusRepository) CreateCollection(ctx context.Context, collection *corpustypes.CorpusCollection) error {
	if collection == nil {
		return NewRepositoryError("create_collection", "corpus_collection", "", fmt.Errorf("collection cannot be nil"))
	}

	query := `
		INSERT INTO corpus_collections (name, description, max_size, created_at)
		VALUES (?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		collection.Name(), collection.Description(), 0, collection.CreatedAt(),
	)

	if err != nil {
		return NewRepositoryError("create_collection", "corpus_collection", collection.Name(), err)
	}

	r.logger.WithField("collection_name", collection.Name()).Debug("Corpus collection created")
	return nil
}

// UpdateCollection updates an existing collection
func (r *CorpusRepository) UpdateCollection(ctx context.Context, collection *corpustypes.CorpusCollection) error {
	if collection == nil {
		return NewRepositoryError("update_collection", "corpus_collection", "", fmt.Errorf("collection cannot be nil"))
	}

	query := `UPDATE corpus_collections SET description = ? WHERE name = ?`

	result, err := r.db.ExecContext(ctx, query, collection.Description(), collection.Name())
	if err != nil {
		return NewRepositoryError("update_collection", "corpus_collection", collection.Name(), err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("update_collection", "corpus_collection", collection.Name(), err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteCollection deletes a collection by name
func (r *CorpusRepository) DeleteCollection(ctx context.Context, name string) error {
	// First delete all entries in the collection mapping
	_, err := r.db.ExecContext(ctx, "DELETE FROM corpus_collection_entries WHERE collection_name = ?", name)
	if err != nil {
		return NewRepositoryError("delete_collection", "corpus_collection", name, err)
	}

	// Then delete the collection itself
	result, err := r.db.ExecContext(ctx, "DELETE FROM corpus_collections WHERE name = ?", name)
	if err != nil {
		return NewRepositoryError("delete_collection", "corpus_collection", name, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("delete_collection", "corpus_collection", name, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("collection_name", name).Debug("Corpus collection deleted")
	return nil
}

// FindCollectionByName retrieves a collection by name
func (r *CorpusRepository) FindCollectionByName(ctx context.Context, name string) (*corpustypes.CorpusCollection, error) {
	row := &models.DomainCorpusCollectionRow{}
	query := "SELECT name, description, COALESCE(max_size, 0), created_at FROM corpus_collections WHERE name = ?"

	err := r.db.QueryRowContext(ctx, query, name).Scan(&row.Name, &row.Description, &row.MaxSize, &row.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, NewRepositoryError("find_collection_by_name", "corpus_collection", name, err)
	}

	return mappers.CorpusCollectionRowToDomain(row), nil
}

// ListCollections retrieves all collections
func (r *CorpusRepository) ListCollections(ctx context.Context) ([]*corpustypes.CorpusCollection, error) {
	query := "SELECT name, description, COALESCE(max_size, 0), created_at FROM corpus_collections ORDER BY name ASC"
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, NewRepositoryError("list_collections", "corpus_collection", "", err)
	}
	defer rows.Close()

	collections := make([]*corpustypes.CorpusCollection, 0)
	for rows.Next() {
		row := &models.DomainCorpusCollectionRow{}
		if err := rows.Scan(&row.Name, &row.Description, &row.MaxSize, &row.CreatedAt); err != nil {
			return nil, NewRepositoryError("list_collections", "corpus_collection", "", err)
		}
		if collection := mappers.CorpusCollectionRowToDomain(row); collection != nil {
			collections = append(collections, collection)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, NewRepositoryError("list_collections", "corpus_collection", "", err)
	}

	return collections, nil
}

// AddEntryToCollection adds an entry to a collection
func (r *CorpusRepository) AddEntryToCollection(ctx context.Context, collectionName string, entryID string) error {
	query := `
		INSERT INTO corpus_collection_entries (collection_name, entry_id, added_at)
		VALUES (?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query, collectionName, entryID, time.Now().UTC())
	if err != nil {
		return NewRepositoryError("add_entry_to_collection", "corpus_collection", collectionName, err)
	}

	return nil
}

// RemoveEntryFromCollection removes an entry from a collection
func (r *CorpusRepository) RemoveEntryFromCollection(ctx context.Context, collectionName string, entryID string) error {
	query := "DELETE FROM corpus_collection_entries WHERE collection_name = ? AND entry_id = ?"

	result, err := r.db.ExecContext(ctx, query, collectionName, entryID)
	if err != nil {
		return NewRepositoryError("remove_entry_from_collection", "corpus_collection", collectionName, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("remove_entry_from_collection", "corpus_collection", collectionName, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// GetCollectionEntries retrieves all entries in a collection
func (r *CorpusRepository) GetCollectionEntries(ctx context.Context, collectionName string) ([]*corpustypes.CorpusEntry, error) {
	query := `
		SELECT ` + entrySelectColumns() + `
		FROM corpus_entries e
		INNER JOIN corpus_collection_entries ce ON e.id = ce.entry_id
		WHERE ce.collection_name = ?
		ORDER BY ce.added_at DESC
	`
	return r.scanEntryRows(ctx, query, collectionName)
}

// CollectionExists checks if a collection exists
func (r *CorpusRepository) CollectionExists(ctx context.Context, name string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM corpus_collections WHERE name = ?)"
	err := r.db.QueryRowContext(ctx, query, name).Scan(&exists)
	if err != nil {
		return false, NewRepositoryError("collection_exists", "corpus_collection", name, err)
	}
	return exists, nil
}

// Helper functions

func entrySelectColumns() string {
	return "id, input, hash, size, created_at, last_executed_at, execution_count, coverage, mutation_info, tags, metadata"
}

func (r *CorpusRepository) scanEntryRow(ctx context.Context, query string, args ...interface{}) (*models.DomainCorpusEntryRow, error) {
	row := r.db.QueryRowContext(ctx, query, args...)
	return r.scanSingleEntryRow(row)
}

func (r *CorpusRepository) scanEntryRows(ctx context.Context, query string, args ...interface{}) ([]*corpustypes.CorpusEntry, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]*corpustypes.CorpusEntry, 0, 16)
	for rows.Next() {
		entryRow, err := r.scanRowsEntryRow(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, mappers.CorpusEntryRowToDomain(entryRow))
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func (r *CorpusRepository) scanSingleEntryRow(row *sql.Row) (*models.DomainCorpusEntryRow, error) {
	er := &models.DomainCorpusEntryRow{}

	err := row.Scan(
		&er.ID, &er.Input, &er.Hash, &er.Size, &er.CreatedAt, &er.LastExecutedAt,
		&er.ExecutionCount, &er.CoverageJSON, &er.MutationJSON, &er.TagsJSON, &er.MetadataJSON,
	)
	if err != nil {
		return nil, err
	}

	return er, nil
}

func (r *CorpusRepository) scanRowsEntryRow(rows *sql.Rows) (*models.DomainCorpusEntryRow, error) {
	er := &models.DomainCorpusEntryRow{}

	err := rows.Scan(
		&er.ID, &er.Input, &er.Hash, &er.Size, &er.CreatedAt, &er.LastExecutedAt,
		&er.ExecutionCount, &er.CoverageJSON, &er.MutationJSON, &er.TagsJSON, &er.MetadataJSON,
	)
	if err != nil {
		return nil, err
	}

	return er, nil
}
