package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/cache"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/sqlite"
)

// CrashRepository implements the crash repository interface using SQLite
type CrashRepository struct {
	conn  *sqlite.Connection
	cache cache.Cache
	log   logrus.FieldLogger
}

// NewCrashRepository creates a new crash repository
func NewCrashRepository(conn *sqlite.Connection, cache cache.Cache, log logrus.FieldLogger) (*CrashRepository, error) {
	if conn == nil {
		return nil, errors.New(errors.ErrorTypeDatabase, "new_crash_repository", "connection is required")
	}
	if log == nil {
		log = logrus.New()
	}

	repo := &CrashRepository{
		conn:  conn,
		cache: cache,
		log:   log.WithField("component", "crash_repository"),
	}

	if err := repo.createSchema(); err != nil {
		return nil, err
	}

	return repo, nil
}

// createSchema creates the crashes table if it doesn't exist
func (r *CrashRepository) createSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS crashes (
			id TEXT PRIMARY KEY,
			signature_hash TEXT NOT NULL,
			signature_top_frames TEXT NOT NULL,
			signature_function_names TEXT NOT NULL,
			signature_library_names TEXT NOT NULL,
			signature_type TEXT NOT NULL,
			signature_confidence REAL NOT NULL,
			input BLOB NOT NULL,
			input_hash TEXT NOT NULL,
			stack_trace TEXT NOT NULL,
			severity TEXT NOT NULL,
			type TEXT NOT NULL,
			discovered_at DATETIME NOT NULL,
			last_seen_at DATETIME NOT NULL,
			occurrence_count INTEGER DEFAULT 1,
			corpus_entry_id TEXT,
			target_name TEXT NOT NULL,
			target_version TEXT,
			target_command TEXT,
			target_environment TEXT,
			metadata TEXT,
			reproducible INTEGER DEFAULT 1,
			fixed INTEGER DEFAULT 0,
			fixed_at DATETIME,
			tags TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_crashes_signature ON crashes(signature_hash);
		CREATE INDEX IF NOT EXISTS idx_crashes_severity ON crashes(severity);
		CREATE INDEX IF NOT EXISTS idx_crashes_type ON crashes(type);
		CREATE INDEX IF NOT EXISTS idx_crashes_target ON crashes(target_name);
		CREATE INDEX IF NOT EXISTS idx_crashes_corpus ON crashes(corpus_entry_id);
		CREATE INDEX IF NOT EXISTS idx_crashes_reproducible ON crashes(reproducible);
		CREATE INDEX IF NOT EXISTS idx_crashes_fixed ON crashes(fixed);
		CREATE INDEX IF NOT EXISTS idx_crashes_discovered ON crashes(discovered_at);
		CREATE INDEX IF NOT EXISTS idx_crashes_last_seen ON crashes(last_seen_at);
		CREATE INDEX IF NOT EXISTS idx_crashes_occurrence ON crashes(occurrence_count);
	`

	_, err := r.conn.ExecContext(context.Background(), schema)
	if err != nil {
		return errors.NewDatabaseError("create_crash_schema", err)
	}

	return nil
}

// Create creates a new crash
func (r *CrashRepository) Create(ctx context.Context, crash *types.Crash) error {
	if crash == nil {
		return errors.NewValidationError("create_crash", "crash cannot be nil")
	}

	if err := crash.Validate(); err != nil {
		return errors.NewValidationError("create_crash", err.Error())
	}

	topFrames, err := json.Marshal(crash.Signature.TopFrames)
	if err != nil {
		return errors.NewSystemError("marshal_top_frames", err)
	}

	functionNames, err := json.Marshal(crash.Signature.FunctionNames)
	if err != nil {
		return errors.NewSystemError("marshal_function_names", err)
	}

	libraryNames, err := json.Marshal(crash.Signature.LibraryNames)
	if err != nil {
		return errors.NewSystemError("marshal_library_names", err)
	}

	tags, err := json.Marshal(crash.Tags)
	if err != nil {
		return errors.NewSystemError("marshal_tags", err)
	}

	var metadata []byte
	if crash.Metadata != nil && len(crash.Metadata) > 0 {
		metadata, err = json.Marshal(crash.Metadata)
		if err != nil {
			return errors.NewSystemError("marshal_metadata", err)
		}
	}

	query := `
		INSERT INTO crashes (
			id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.conn.ExecContext(ctx, query,
		crash.ID,
		crash.Signature.Hash,
		string(topFrames),
		string(functionNames),
		string(libraryNames),
		crash.Signature.SignatureType,
		crash.Signature.Confidence,
		crash.Input,
		crash.InputHash,
		crash.StackTrace,
		crash.Severity,
		crash.Type,
		crash.DiscoveredAt,
		crash.LastSeenAt,
		crash.OccurrenceCount,
		crash.CorpusEntryID,
		crash.TargetInfo.Name,
		crash.TargetInfo.Version,
		crash.TargetInfo.Command,
		crash.TargetInfo.Environment,
		metadata,
		boolToInt(crash.Reproducible),
		boolToInt(crash.Fixed),
		crash.FixedAt,
		string(tags),
	)

	if err != nil {
		if r.isUniqueConstraintError(err) {
			return errors.NewConflictError("create_crash", "crash already exists").
				WithDetail("crash_id", crash.ID)
		}
		return errors.NewDatabaseError("create_crash", err).
			WithDetail("crash_id", crash.ID)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.crashCacheKey(crash.ID))
		r.cache.Delete(ctx, fmt.Sprintf("crashes:signature:%s", crash.Signature.Hash))
		r.cache.Delete(ctx, "crashes:stats")
	}

	r.log.WithField("crash_id", crash.ID).Debug("Crash created")
	return nil
}

// Update updates an existing crash
func (r *CrashRepository) Update(ctx context.Context, crash *types.Crash) error {
	if crash == nil {
		return errors.NewValidationError("update_crash", "crash cannot be nil")
	}

	if err := crash.Validate(); err != nil {
		return errors.NewValidationError("update_crash", err.Error())
	}

	topFrames, err := json.Marshal(crash.Signature.TopFrames)
	if err != nil {
		return errors.NewSystemError("marshal_top_frames", err)
	}

	functionNames, err := json.Marshal(crash.Signature.FunctionNames)
	if err != nil {
		return errors.NewSystemError("marshal_function_names", err)
	}

	libraryNames, err := json.Marshal(crash.Signature.LibraryNames)
	if err != nil {
		return errors.NewSystemError("marshal_library_names", err)
	}

	tags, err := json.Marshal(crash.Tags)
	if err != nil {
		return errors.NewSystemError("marshal_tags", err)
	}

	var metadata []byte
	if crash.Metadata != nil && len(crash.Metadata) > 0 {
		metadata, err = json.Marshal(crash.Metadata)
		if err != nil {
			return errors.NewSystemError("marshal_metadata", err)
		}
	}

	query := `
		UPDATE crashes SET
			signature_hash = ?, signature_top_frames = ?, signature_function_names = ?,
			signature_library_names = ?, signature_type = ?, signature_confidence = ?,
			input = ?, input_hash = ?, stack_trace = ?, severity = ?, type = ?,
			last_seen_at = ?, occurrence_count = ?, corpus_entry_id = ?,
			target_name = ?, target_version = ?, target_command = ?, target_environment = ?,
			metadata = ?, reproducible = ?, fixed = ?, fixed_at = ?, tags = ?
		WHERE id = ?
	`

	result, err := r.conn.ExecContext(ctx, query,
		crash.Signature.Hash,
		string(topFrames),
		string(functionNames),
		string(libraryNames),
		crash.Signature.SignatureType,
		crash.Signature.Confidence,
		crash.Input,
		crash.InputHash,
		crash.StackTrace,
		crash.Severity,
		crash.Type,
		crash.LastSeenAt,
		crash.OccurrenceCount,
		crash.CorpusEntryID,
		crash.TargetInfo.Name,
		crash.TargetInfo.Version,
		crash.TargetInfo.Command,
		crash.TargetInfo.Environment,
		metadata,
		boolToInt(crash.Reproducible),
		boolToInt(crash.Fixed),
		crash.FixedAt,
		string(tags),
		crash.ID,
	)

	if err != nil {
		return errors.NewDatabaseError("update_crash", err).
			WithDetail("crash_id", crash.ID)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_crash_rows", err).
			WithDetail("crash_id", crash.ID)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_crash", "crash").
			WithDetail("crash_id", crash.ID)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.crashCacheKey(crash.ID))
		r.cache.Delete(ctx, fmt.Sprintf("crashes:signature:%s", crash.Signature.Hash))
		r.cache.Clear(ctx) // Clear all caches
	}

	r.log.WithField("crash_id", crash.ID).Debug("Crash updated")
	return nil
}

// Delete deletes a crash by ID
func (r *CrashRepository) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("delete_crash", "crash ID cannot be empty")
	}

	// Get the signature hash before deletion for cache invalidation
	var signatureHash string
	err := r.conn.QueryRowContext(ctx, "SELECT signature_hash FROM crashes WHERE id = ?", id).Scan(&signatureHash)
	if err != nil && err != sql.ErrNoRows {
		return errors.NewDatabaseError("get_signature_for_deletion", err).
			WithDetail("crash_id", id)
	}

	query := `DELETE FROM crashes WHERE id = ?`

	result, err := r.conn.ExecContext(ctx, query, id)
	if err != nil {
		return errors.NewDatabaseError("delete_crash", err).
			WithDetail("crash_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("delete_crash_rows", err).
			WithDetail("crash_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("delete_crash", "crash").
			WithDetail("crash_id", id)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.crashCacheKey(id))
		if signatureHash != "" {
			r.cache.Delete(ctx, fmt.Sprintf("crashes:signature:%s", signatureHash))
		}
		r.cache.Clear(ctx) // Clear all caches
	}

	r.log.WithField("crash_id", id).Debug("Crash deleted")
	return nil
}

// FindByID retrieves a crash by its ID
func (r *CrashRepository) FindByID(ctx context.Context, id string) (*types.Crash, error) {
	if id == "" {
		return nil, errors.NewValidationError("find_crash_by_id", "crash ID cannot be empty")
	}

	// Check cache first
	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, r.crashCacheKey(id)); found {
			if crash, ok := cached.(*types.Crash); ok {
				return crash, nil
			}
		}
	}

	query := `
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		WHERE id = ?
	`

	crash, err := r.scanCrash(r.conn.QueryRowContext(ctx, query, id))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewNotFoundError("find_crash_by_id", "crash").
				WithDetail("crash_id", id)
		}
		return nil, err
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, r.crashCacheKey(id), crash, 5*time.Minute)
	}

	return crash, nil
}

// FindBySignature retrieves crashes by signature hash
func (r *CrashRepository) FindBySignature(ctx context.Context, signatureHash string) ([]*types.Crash, error) {
	if signatureHash == "" {
		return nil, errors.NewValidationError("find_crash_by_signature", "signature hash cannot be empty")
	}

	// Check cache first
	cacheKey := fmt.Sprintf("crashes:signature:%s", signatureHash)
	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, cacheKey); found {
			if crashes, ok := cached.([]*types.Crash); ok {
				return crashes, nil
			}
		}
	}

	query := `
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		WHERE signature_hash = ?
		ORDER BY occurrence_count DESC, last_seen_at DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, signatureHash)
	if err != nil {
		return nil, errors.NewDatabaseError("find_crash_by_signature", err).
			WithDetail("signature_hash", signatureHash)
	}
	defer rows.Close()

	crashes, err := r.scanCrashes(rows)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, cacheKey, crashes, 2*time.Minute)
	}

	return crashes, nil
}

// FindBySeverity retrieves all crashes with a specific severity
func (r *CrashRepository) FindBySeverity(ctx context.Context, severity types.Severity) ([]*types.Crash, error) {
	query := `
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		WHERE severity = ?
		ORDER BY occurrence_count DESC, last_seen_at DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, severity)
	if err != nil {
		return nil, errors.NewDatabaseError("find_crash_by_severity", err).
			WithDetail("severity", string(severity))
	}
	defer rows.Close()

	return r.scanCrashes(rows)
}

// FindByType retrieves all crashes of a specific type
func (r *CrashRepository) FindByType(ctx context.Context, crashType types.CrashType) ([]*types.Crash, error) {
	query := `
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		WHERE type = ?
		ORDER BY severity DESC, occurrence_count DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, crashType)
	if err != nil {
		return nil, errors.NewDatabaseError("find_crash_by_type", err).
			WithDetail("type", string(crashType))
	}
	defer rows.Close()

	return r.scanCrashes(rows)
}

// FindByTarget retrieves all crashes for a specific target
func (r *CrashRepository) FindByTarget(ctx context.Context, targetName string) ([]*types.Crash, error) {
	if targetName == "" {
		return nil, errors.NewValidationError("find_crash_by_target", "target name cannot be empty")
	}

	query := `
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		WHERE target_name = ?
		ORDER BY severity DESC, discovered_at DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, targetName)
	if err != nil {
		return nil, errors.NewDatabaseError("find_crash_by_target", err).
			WithDetail("target_name", targetName)
	}
	defer rows.Close()

	return r.scanCrashes(rows)
}

// FindByCorpusEntry retrieves crashes associated with a corpus entry
func (r *CrashRepository) FindByCorpusEntry(ctx context.Context, corpusEntryID string) ([]*types.Crash, error) {
	if corpusEntryID == "" {
		return nil, errors.NewValidationError("find_crash_by_corpus_entry", "corpus entry ID cannot be empty")
	}

	query := `
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		WHERE corpus_entry_id = ?
		ORDER BY discovered_at DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, corpusEntryID)
	if err != nil {
		return nil, errors.NewDatabaseError("find_crash_by_corpus_entry", err).
			WithDetail("corpus_entry_id", corpusEntryID)
	}
	defer rows.Close()

	return r.scanCrashes(rows)
}

// FindReproducible retrieves all reproducible crashes
func (r *CrashRepository) FindReproducible(ctx context.Context) ([]*types.Crash, error) {
	query := `
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		WHERE reproducible = 1
		ORDER BY severity DESC, occurrence_count DESC
	`

	rows, err := r.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, errors.NewDatabaseError("find_reproducible_crashes", err)
	}
	defer rows.Close()

	return r.scanCrashes(rows)
}

// FindUnfixed retrieves all unfixed crashes
func (r *CrashRepository) FindUnfixed(ctx context.Context) ([]*types.Crash, error) {
	query := `
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		WHERE fixed = 0
		ORDER BY severity DESC, occurrence_count DESC
	`

	rows, err := r.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, errors.NewDatabaseError("find_unfixed_crashes", err)
	}
	defer rows.Close()

	return r.scanCrashes(rows)
}

// FindByTag retrieves all crashes with a specific tag
func (r *CrashRepository) FindByTag(ctx context.Context, tag string) ([]*types.Crash, error) {
	if tag == "" {
		return nil, errors.NewValidationError("find_crash_by_tag", "tag cannot be empty")
	}

	// SQLite doesn't have native JSON support, so we use LIKE
	query := `
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		WHERE tags LIKE ?
		ORDER BY discovered_at DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, fmt.Sprintf(`%%"%s"%%`, tag))
	if err != nil {
		return nil, errors.NewDatabaseError("find_crash_by_tag", err).
			WithDetail("tag", tag)
	}
	defer rows.Close()

	return r.scanCrashes(rows)
}

// FindRecent retrieves crashes discovered within a time range
func (r *CrashRepository) FindRecent(ctx context.Context, since time.Time) ([]*types.Crash, error) {
	query := `
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		WHERE discovered_at >= ?
		ORDER BY discovered_at DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, since)
	if err != nil {
		return nil, errors.NewDatabaseError("find_recent_crashes", err).
			WithDetail("since", since.String())
	}
	defer rows.Close()

	return r.scanCrashes(rows)
}

// FindSimilar finds crashes similar to the given signature
func (r *CrashRepository) FindSimilar(ctx context.Context, signature *types.CrashSignature, threshold float64) ([]*types.Crash, error) {
	if signature == nil {
		return nil, errors.NewValidationError("find_similar_crashes", "signature cannot be nil")
	}

	if threshold < 0 || threshold > 1 {
		return nil, errors.NewValidationError("find_similar_crashes", "threshold must be between 0 and 1")
	}

	// First, get all crashes
	query := `
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		ORDER BY occurrence_count DESC
		LIMIT 1000
	`

	rows, err := r.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, errors.NewDatabaseError("find_similar_crashes", err)
	}
	defer rows.Close()

	allCrashes, err := r.scanCrashes(rows)
	if err != nil {
		return nil, err
	}

	// Filter by similarity
	var similarCrashes []*types.Crash
	for _, crash := range allCrashes {
		if crash.Signature != nil && crash.Signature.IsSimilar(signature, threshold) {
			similarCrashes = append(similarCrashes, crash)
		}
	}

	return similarCrashes, nil
}

// List retrieves crashes with pagination
func (r *CrashRepository) List(ctx context.Context, offset, limit int) ([]*types.Crash, int, error) {
	if offset < 0 {
		return nil, 0, errors.NewValidationError("list_crashes", "offset cannot be negative")
	}
	if limit <= 0 {
		return nil, 0, errors.NewValidationError("list_crashes", "limit must be positive")
	}

	// Get total count
	countQuery := `SELECT COUNT(*) FROM crashes`
	var total int
	err := r.conn.QueryRowContext(ctx, countQuery).Scan(&total)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("count_crashes", err)
	}

	// Get paginated results
	query := `
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		ORDER BY discovered_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.conn.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("list_crashes", err)
	}
	defer rows.Close()

	crashes, err := r.scanCrashes(rows)
	if err != nil {
		return nil, 0, err
	}

	return crashes, total, nil
}

// ListBySeverity retrieves crashes ordered by severity
func (r *CrashRepository) ListBySeverity(ctx context.Context, offset, limit int) ([]*types.Crash, int, error) {
	if offset < 0 {
		return nil, 0, errors.NewValidationError("list_crashes_by_severity", "offset cannot be negative")
	}
	if limit <= 0 {
		return nil, 0, errors.NewValidationError("list_crashes_by_severity", "limit must be positive")
	}

	// Get total count
	countQuery := `SELECT COUNT(*) FROM crashes`
	var total int
	err := r.conn.QueryRowContext(ctx, countQuery).Scan(&total)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("count_crashes", err)
	}

	// Get paginated results ordered by severity
	query := `
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		ORDER BY 
			CASE severity
				WHEN 'critical' THEN 1
				WHEN 'high' THEN 2
				WHEN 'medium' THEN 3
				WHEN 'low' THEN 4
				ELSE 5
			END,
			occurrence_count DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.conn.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("list_crashes_by_severity", err)
	}
	defer rows.Close()

	crashes, err := r.scanCrashes(rows)
	if err != nil {
		return nil, 0, err
	}

	return crashes, total, nil
}

// ListByOccurrence retrieves crashes ordered by occurrence count
func (r *CrashRepository) ListByOccurrence(ctx context.Context, offset, limit int, ascending bool) ([]*types.Crash, int, error) {
	if offset < 0 {
		return nil, 0, errors.NewValidationError("list_crashes_by_occurrence", "offset cannot be negative")
	}
	if limit <= 0 {
		return nil, 0, errors.NewValidationError("list_crashes_by_occurrence", "limit must be positive")
	}

	// Get total count
	countQuery := `SELECT COUNT(*) FROM crashes`
	var total int
	err := r.conn.QueryRowContext(ctx, countQuery).Scan(&total)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("count_crashes", err)
	}

	order := "DESC"
	if ascending {
		order = "ASC"
	}

	// Get paginated results
	query := fmt.Sprintf(`
		SELECT id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		FROM crashes
		ORDER BY occurrence_count %s, last_seen_at DESC
		LIMIT ? OFFSET ?
	`, order)

	rows, err := r.conn.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("list_crashes_by_occurrence", err)
	}
	defer rows.Close()

	crashes, err := r.scanCrashes(rows)
	if err != nil {
		return nil, 0, err
	}

	return crashes, total, nil
}

// RecordOccurrence increments the occurrence count for a crash
func (r *CrashRepository) RecordOccurrence(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("record_occurrence", "crash ID cannot be empty")
	}

	query := `
		UPDATE crashes
		SET occurrence_count = occurrence_count + 1,
			last_seen_at = ?
		WHERE id = ?
	`

	result, err := r.conn.ExecContext(ctx, query, time.Now(), id)
	if err != nil {
		return errors.NewDatabaseError("record_occurrence", err).
			WithDetail("crash_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("record_occurrence_rows", err).
			WithDetail("crash_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("record_occurrence", "crash").
			WithDetail("crash_id", id)
	}

	// Invalidate cache for this crash
	if r.cache != nil {
		r.cache.Delete(ctx, r.crashCacheKey(id))
	}

	return nil
}

// MarkAsFixed marks a crash as fixed
func (r *CrashRepository) MarkAsFixed(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("mark_as_fixed", "crash ID cannot be empty")
	}

	query := `
		UPDATE crashes
		SET fixed = 1,
			fixed_at = ?
		WHERE id = ?
	`

	result, err := r.conn.ExecContext(ctx, query, time.Now(), id)
	if err != nil {
		return errors.NewDatabaseError("mark_as_fixed", err).
			WithDetail("crash_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("mark_as_fixed_rows", err).
			WithDetail("crash_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("mark_as_fixed", "crash").
			WithDetail("crash_id", id)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.crashCacheKey(id))
		r.cache.Clear(ctx) // Clear stats caches
	}

	return nil
}

// MarkAsNotReproducible marks a crash as not reproducible
func (r *CrashRepository) MarkAsNotReproducible(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("mark_as_not_reproducible", "crash ID cannot be empty")
	}

	query := `
		UPDATE crashes
		SET reproducible = 0
		WHERE id = ?
	`

	result, err := r.conn.ExecContext(ctx, query, id)
	if err != nil {
		return errors.NewDatabaseError("mark_as_not_reproducible", err).
			WithDetail("crash_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("mark_as_not_reproducible_rows", err).
			WithDetail("crash_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("mark_as_not_reproducible", "crash").
			WithDetail("crash_id", id)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.crashCacheKey(id))
	}

	return nil
}

// Exists checks if a crash exists by ID
func (r *CrashRepository) Exists(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, errors.NewValidationError("crash_exists", "crash ID cannot be empty")
	}

	query := `SELECT 1 FROM crashes WHERE id = ? LIMIT 1`

	var exists int
	err := r.conn.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, errors.NewDatabaseError("crash_exists", err).
			WithDetail("crash_id", id)
	}

	return true, nil
}

// ExistsBySignature checks if a crash exists by signature
func (r *CrashRepository) ExistsBySignature(ctx context.Context, signatureHash string) (bool, error) {
	if signatureHash == "" {
		return false, errors.NewValidationError("crash_exists_by_signature", "signature hash cannot be empty")
	}

	query := `SELECT 1 FROM crashes WHERE signature_hash = ? LIMIT 1`

	var exists int
	err := r.conn.QueryRowContext(ctx, query, signatureHash).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, errors.NewDatabaseError("crash_exists_by_signature", err).
			WithDetail("signature_hash", signatureHash)
	}

	return true, nil
}

// Count returns the total number of crashes
func (r *CrashRepository) Count(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM crashes`

	var count int
	err := r.conn.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, errors.NewDatabaseError("count_crashes", err)
	}

	return count, nil
}

// CountBySeverity counts crashes by severity
func (r *CrashRepository) CountBySeverity(ctx context.Context, severity types.Severity) (int, error) {
	query := `SELECT COUNT(*) FROM crashes WHERE severity = ?`

	var count int
	err := r.conn.QueryRowContext(ctx, query, severity).Scan(&count)
	if err != nil {
		return 0, errors.NewDatabaseError("count_crashes_by_severity", err).
			WithDetail("severity", string(severity))
	}

	return count, nil
}

// CountByType counts crashes by type
func (r *CrashRepository) CountByType(ctx context.Context, crashType types.CrashType) (int, error) {
	query := `SELECT COUNT(*) FROM crashes WHERE type = ?`

	var count int
	err := r.conn.QueryRowContext(ctx, query, crashType).Scan(&count)
	if err != nil {
		return 0, errors.NewDatabaseError("count_crashes_by_type", err).
			WithDetail("type", string(crashType))
	}

	return count, nil
}

// CountUnfixed counts unfixed crashes
func (r *CrashRepository) CountUnfixed(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM crashes WHERE fixed = 0`

	var count int
	err := r.conn.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, errors.NewDatabaseError("count_unfixed_crashes", err)
	}

	return count, nil
}

// GetStatsByTarget retrieves crash statistics grouped by target
func (r *CrashRepository) GetStatsByTarget(ctx context.Context) (map[string]repository.CrashStats, error) {
	// Check cache first
	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, "crashes:stats:by_target"); found {
			if stats, ok := cached.(map[string]repository.CrashStats); ok {
				return stats, nil
			}
		}
	}

	// Get unique targets
	targetsQuery := `SELECT DISTINCT target_name FROM crashes`
	rows, err := r.conn.QueryContext(ctx, targetsQuery)
	if err != nil {
		return nil, errors.NewDatabaseError("get_crash_targets", err)
	}

	var targets []string
	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			rows.Close()
			return nil, errors.NewDatabaseError("scan_crash_target", err)
		}
		targets = append(targets, target)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		return nil, errors.NewDatabaseError("get_crash_targets_rows", err)
	}

	// Get stats for each target
	statsByTarget := make(map[string]repository.CrashStats)

	for _, target := range targets {
		stats := repository.CrashStats{
			BySeverity: make(map[types.Severity]int),
			ByType:     make(map[types.CrashType]int),
		}

		// Total count
		countQuery := `SELECT COUNT(*) FROM crashes WHERE target_name = ?`
		if err := r.conn.QueryRowContext(ctx, countQuery, target).Scan(&stats.Total); err != nil {
			return nil, errors.NewDatabaseError("count_crashes_for_target", err).
				WithDetail("target", target)
		}

		// Count by severity
		severityQuery := `
			SELECT severity, COUNT(*) 
			FROM crashes 
			WHERE target_name = ? 
			GROUP BY severity
		`
		severityRows, err := r.conn.QueryContext(ctx, severityQuery, target)
		if err != nil {
			return nil, errors.NewDatabaseError("count_crashes_by_severity_for_target", err).
				WithDetail("target", target)
		}

		for severityRows.Next() {
			var severity types.Severity
			var count int
			if err := severityRows.Scan(&severity, &count); err != nil {
				severityRows.Close()
				return nil, errors.NewDatabaseError("scan_severity_count", err)
			}
			stats.BySeverity[severity] = count
		}
		severityRows.Close()

		// Count by type
		typeQuery := `
			SELECT type, COUNT(*) 
			FROM crashes 
			WHERE target_name = ? 
			GROUP BY type
		`
		typeRows, err := r.conn.QueryContext(ctx, typeQuery, target)
		if err != nil {
			return nil, errors.NewDatabaseError("count_crashes_by_type_for_target", err).
				WithDetail("target", target)
		}

		for typeRows.Next() {
			var crashType types.CrashType
			var count int
			if err := typeRows.Scan(&crashType, &count); err != nil {
				typeRows.Close()
				return nil, errors.NewDatabaseError("scan_type_count", err)
			}
			stats.ByType[crashType] = count
		}
		typeRows.Close()

		// Reproducible count
		reproducibleQuery := `SELECT COUNT(*) FROM crashes WHERE target_name = ? AND reproducible = 1`
		if err := r.conn.QueryRowContext(ctx, reproducibleQuery, target).Scan(&stats.Reproducible); err != nil {
			return nil, errors.NewDatabaseError("count_reproducible_crashes_for_target", err).
				WithDetail("target", target)
		}

		// Fixed count
		fixedQuery := `SELECT COUNT(*) FROM crashes WHERE target_name = ? AND fixed = 1`
		if err := r.conn.QueryRowContext(ctx, fixedQuery, target).Scan(&stats.Fixed); err != nil {
			return nil, errors.NewDatabaseError("count_fixed_crashes_for_target", err).
				WithDetail("target", target)
		}

		// Average age
		ageQuery := `
			SELECT AVG(JULIANDAY('now') - JULIANDAY(discovered_at)) * 24 * 60 * 60 * 1000000000
			FROM crashes 
			WHERE target_name = ?
		`
		var avgAgeNanos sql.NullFloat64
		if err := r.conn.QueryRowContext(ctx, ageQuery, target).Scan(&avgAgeNanos); err != nil {
			return nil, errors.NewDatabaseError("get_average_crash_age_for_target", err).
				WithDetail("target", target)
		}
		if avgAgeNanos.Valid {
			stats.AverageAge = time.Duration(avgAgeNanos.Float64)
		}

		// Oldest crash
		oldestQuery := `
			SELECT id, signature_hash, signature_top_frames, signature_function_names,
				signature_library_names, signature_type, signature_confidence,
				input, input_hash, stack_trace, severity, type,
				discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
				target_name, target_version, target_command, target_environment,
				metadata, reproducible, fixed, fixed_at, tags
			FROM crashes
			WHERE target_name = ?
			ORDER BY discovered_at ASC
			LIMIT 1
		`
		oldestCrash, err := r.scanCrash(r.conn.QueryRowContext(ctx, oldestQuery, target))
		if err != nil && err != sql.ErrNoRows {
			return nil, errors.NewDatabaseError("get_oldest_crash_for_target", err).
				WithDetail("target", target)
		}
		if oldestCrash != nil {
			stats.OldestCrash = oldestCrash
		}

		// Most frequent crash
		frequentQuery := `
			SELECT id, signature_hash, signature_top_frames, signature_function_names,
				signature_library_names, signature_type, signature_confidence,
				input, input_hash, stack_trace, severity, type,
				discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
				target_name, target_version, target_command, target_environment,
				metadata, reproducible, fixed, fixed_at, tags
			FROM crashes
			WHERE target_name = ?
			ORDER BY occurrence_count DESC
			LIMIT 1
		`
		frequentCrash, err := r.scanCrash(r.conn.QueryRowContext(ctx, frequentQuery, target))
		if err != nil && err != sql.ErrNoRows {
			return nil, errors.NewDatabaseError("get_most_frequent_crash_for_target", err).
				WithDetail("target", target)
		}
		if frequentCrash != nil {
			stats.MostFrequent = frequentCrash
		}

		statsByTarget[target] = stats
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, "crashes:stats:by_target", statsByTarget, 5*time.Minute)
	}

	return statsByTarget, nil
}

// BeginTransaction starts a new transaction
func (r *CrashRepository) BeginTransaction(ctx context.Context) (repository.CrashTransaction, error) {
	tx, err := r.conn.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.NewDatabaseError("begin_transaction", err)
	}

	return &crashTransaction{
		tx:   tx,
		repo: r,
		log:  r.log,
	}, nil
}

// Helper methods

func (r *CrashRepository) scanCrash(row *sql.Row) (*types.Crash, error) {
	var crash types.Crash
	var topFramesJSON, functionNamesJSON, libraryNamesJSON string
	var tagsJSON string
	var metadataJSON sql.NullString
	var corpusEntryID, targetVersion, targetCommand, targetEnvironment sql.NullString
	var fixedAt sql.NullTime
	var reproducible, fixed int

	err := row.Scan(
		&crash.ID,
		&crash.Signature.Hash,
		&topFramesJSON,
		&functionNamesJSON,
		&libraryNamesJSON,
		&crash.Signature.SignatureType,
		&crash.Signature.Confidence,
		&crash.Input,
		&crash.InputHash,
		&crash.StackTrace,
		&crash.Severity,
		&crash.Type,
		&crash.DiscoveredAt,
		&crash.LastSeenAt,
		&crash.OccurrenceCount,
		&corpusEntryID,
		&crash.TargetInfo.Name,
		&targetVersion,
		&targetCommand,
		&targetEnvironment,
		&metadataJSON,
		&reproducible,
		&fixed,
		&fixedAt,
		&tagsJSON,
	)

	if err != nil {
		return nil, errors.NewDatabaseError("scan_crash", err)
	}

	// Unmarshal signature fields
	if err := json.Unmarshal([]byte(topFramesJSON), &crash.Signature.TopFrames); err != nil {
		return nil, errors.NewSystemError("unmarshal_top_frames", err)
	}

	if err := json.Unmarshal([]byte(functionNamesJSON), &crash.Signature.FunctionNames); err != nil {
		return nil, errors.NewSystemError("unmarshal_function_names", err)
	}

	if err := json.Unmarshal([]byte(libraryNamesJSON), &crash.Signature.LibraryNames); err != nil {
		return nil, errors.NewSystemError("unmarshal_library_names", err)
	}

	// Handle nullable fields
	if corpusEntryID.Valid {
		crash.CorpusEntryID = corpusEntryID.String
	}

	if targetVersion.Valid {
		crash.TargetInfo.Version = targetVersion.String
	}
	if targetCommand.Valid {
		crash.TargetInfo.Command = targetCommand.String
	}
	if targetEnvironment.Valid {
		crash.TargetInfo.Environment = targetEnvironment.String
	}

	crash.Reproducible = intToBool(reproducible)
	crash.Fixed = intToBool(fixed)

	if fixedAt.Valid {
		crash.FixedAt = &fixedAt.Time
	}

	// Unmarshal tags
	if tagsJSON != "" && tagsJSON != "null" {
		if err := json.Unmarshal([]byte(tagsJSON), &crash.Tags); err != nil {
			return nil, errors.NewSystemError("unmarshal_tags", err)
		}
	} else {
		crash.Tags = []string{}
	}

	// Unmarshal metadata if present
	if metadataJSON.Valid && metadataJSON.String != "" && metadataJSON.String != "null" {
		crash.Metadata = make(map[string]string)
		if err := json.Unmarshal([]byte(metadataJSON.String), &crash.Metadata); err != nil {
			return nil, errors.NewSystemError("unmarshal_metadata", err)
		}
	} else {
		crash.Metadata = make(map[string]string)
	}

	return &crash, nil
}

func (r *CrashRepository) scanCrashes(rows *sql.Rows) ([]*types.Crash, error) {
	var crashes []*types.Crash

	for rows.Next() {
		var crash types.Crash
		crash.Signature = &types.CrashSignature{}
		var topFramesJSON, functionNamesJSON, libraryNamesJSON string
		var tagsJSON string
		var metadataJSON sql.NullString
		var corpusEntryID, targetVersion, targetCommand, targetEnvironment sql.NullString
		var fixedAt sql.NullTime
		var reproducible, fixed int

		err := rows.Scan(
			&crash.ID,
			&crash.Signature.Hash,
			&topFramesJSON,
			&functionNamesJSON,
			&libraryNamesJSON,
			&crash.Signature.SignatureType,
			&crash.Signature.Confidence,
			&crash.Input,
			&crash.InputHash,
			&crash.StackTrace,
			&crash.Severity,
			&crash.Type,
			&crash.DiscoveredAt,
			&crash.LastSeenAt,
			&crash.OccurrenceCount,
			&corpusEntryID,
			&crash.TargetInfo.Name,
			&targetVersion,
			&targetCommand,
			&targetEnvironment,
			&metadataJSON,
			&reproducible,
			&fixed,
			&fixedAt,
			&tagsJSON,
		)
		if err != nil {
			return nil, errors.NewDatabaseError("scan_crash", err)
		}

		// Unmarshal signature fields
		if err := json.Unmarshal([]byte(topFramesJSON), &crash.Signature.TopFrames); err != nil {
			return nil, errors.NewSystemError("unmarshal_top_frames", err)
		}

		if err := json.Unmarshal([]byte(functionNamesJSON), &crash.Signature.FunctionNames); err != nil {
			return nil, errors.NewSystemError("unmarshal_function_names", err)
		}

		if err := json.Unmarshal([]byte(libraryNamesJSON), &crash.Signature.LibraryNames); err != nil {
			return nil, errors.NewSystemError("unmarshal_library_names", err)
		}

		// Handle nullable fields
		if corpusEntryID.Valid {
			crash.CorpusEntryID = corpusEntryID.String
		}

		if targetVersion.Valid {
			crash.TargetInfo.Version = targetVersion.String
		}
		if targetCommand.Valid {
			crash.TargetInfo.Command = targetCommand.String
		}
		if targetEnvironment.Valid {
			crash.TargetInfo.Environment = targetEnvironment.String
		}

		crash.Reproducible = intToBool(reproducible)
		crash.Fixed = intToBool(fixed)

		if fixedAt.Valid {
			crash.FixedAt = &fixedAt.Time
		}

		// Unmarshal tags
		if tagsJSON != "" && tagsJSON != "null" {
			if err := json.Unmarshal([]byte(tagsJSON), &crash.Tags); err != nil {
				return nil, errors.NewSystemError("unmarshal_tags", err)
			}
		} else {
			crash.Tags = []string{}
		}

		// Unmarshal metadata if present
		if metadataJSON.Valid && metadataJSON.String != "" && metadataJSON.String != "null" {
			crash.Metadata = make(map[string]string)
			if err := json.Unmarshal([]byte(metadataJSON.String), &crash.Metadata); err != nil {
				return nil, errors.NewSystemError("unmarshal_metadata", err)
			}
		} else {
			crash.Metadata = make(map[string]string)
		}

		crashes = append(crashes, &crash)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.NewDatabaseError("scan_crashes_rows", err)
	}

	return crashes, nil
}

func (r *CrashRepository) isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "UNIQUE constraint failed")
}

func (r *CrashRepository) crashCacheKey(id string) string {
	return fmt.Sprintf("crash:%s", id)
}

// crashTransaction implements CrashTransaction
type crashTransaction struct {
	tx   *sql.Tx
	repo *CrashRepository
	log  logrus.FieldLogger
}

// Commit commits the transaction
func (t *crashTransaction) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction
func (t *crashTransaction) Rollback() error {
	return t.tx.Rollback()
}

// CreateTx creates a crash within a transaction
func (t *crashTransaction) CreateTx(ctx context.Context, crash *types.Crash) error {
	if crash == nil {
		return errors.NewValidationError("create_crash_tx", "crash cannot be nil")
	}

	if err := crash.Validate(); err != nil {
		return errors.NewValidationError("create_crash_tx", err.Error())
	}

	topFrames, err := json.Marshal(crash.Signature.TopFrames)
	if err != nil {
		return errors.NewSystemError("marshal_top_frames", err)
	}

	functionNames, err := json.Marshal(crash.Signature.FunctionNames)
	if err != nil {
		return errors.NewSystemError("marshal_function_names", err)
	}

	libraryNames, err := json.Marshal(crash.Signature.LibraryNames)
	if err != nil {
		return errors.NewSystemError("marshal_library_names", err)
	}

	tags, err := json.Marshal(crash.Tags)
	if err != nil {
		return errors.NewSystemError("marshal_tags", err)
	}

	var metadata []byte
	if crash.Metadata != nil && len(crash.Metadata) > 0 {
		metadata, err = json.Marshal(crash.Metadata)
		if err != nil {
			return errors.NewSystemError("marshal_metadata", err)
		}
	}

	query := `
		INSERT INTO crashes (
			id, signature_hash, signature_top_frames, signature_function_names,
			signature_library_names, signature_type, signature_confidence,
			input, input_hash, stack_trace, severity, type,
			discovered_at, last_seen_at, occurrence_count, corpus_entry_id,
			target_name, target_version, target_command, target_environment,
			metadata, reproducible, fixed, fixed_at, tags
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = t.tx.ExecContext(ctx, query,
		crash.ID,
		crash.Signature.Hash,
		string(topFrames),
		string(functionNames),
		string(libraryNames),
		crash.Signature.SignatureType,
		crash.Signature.Confidence,
		crash.Input,
		crash.InputHash,
		crash.StackTrace,
		crash.Severity,
		crash.Type,
		crash.DiscoveredAt,
		crash.LastSeenAt,
		crash.OccurrenceCount,
		crash.CorpusEntryID,
		crash.TargetInfo.Name,
		crash.TargetInfo.Version,
		crash.TargetInfo.Command,
		crash.TargetInfo.Environment,
		metadata,
		boolToInt(crash.Reproducible),
		boolToInt(crash.Fixed),
		crash.FixedAt,
		string(tags),
	)

	if err != nil {
		if t.repo.isUniqueConstraintError(err) {
			return errors.NewConflictError("create_crash_tx", "crash already exists").
				WithDetail("crash_id", crash.ID)
		}
		return errors.NewDatabaseError("create_crash_tx", err).
			WithDetail("crash_id", crash.ID)
	}

	return nil
}

// UpdateTx updates a crash within a transaction
func (t *crashTransaction) UpdateTx(ctx context.Context, crash *types.Crash) error {
	if crash == nil {
		return errors.NewValidationError("update_crash_tx", "crash cannot be nil")
	}

	if err := crash.Validate(); err != nil {
		return errors.NewValidationError("update_crash_tx", err.Error())
	}

	topFrames, err := json.Marshal(crash.Signature.TopFrames)
	if err != nil {
		return errors.NewSystemError("marshal_top_frames", err)
	}

	functionNames, err := json.Marshal(crash.Signature.FunctionNames)
	if err != nil {
		return errors.NewSystemError("marshal_function_names", err)
	}

	libraryNames, err := json.Marshal(crash.Signature.LibraryNames)
	if err != nil {
		return errors.NewSystemError("marshal_library_names", err)
	}

	tags, err := json.Marshal(crash.Tags)
	if err != nil {
		return errors.NewSystemError("marshal_tags", err)
	}

	var metadata []byte
	if crash.Metadata != nil && len(crash.Metadata) > 0 {
		metadata, err = json.Marshal(crash.Metadata)
		if err != nil {
			return errors.NewSystemError("marshal_metadata", err)
		}
	}

	query := `
		UPDATE crashes SET
			signature_hash = ?, signature_top_frames = ?, signature_function_names = ?,
			signature_library_names = ?, signature_type = ?, signature_confidence = ?,
			input = ?, input_hash = ?, stack_trace = ?, severity = ?, type = ?,
			last_seen_at = ?, occurrence_count = ?, corpus_entry_id = ?,
			target_name = ?, target_version = ?, target_command = ?, target_environment = ?,
			metadata = ?, reproducible = ?, fixed = ?, fixed_at = ?, tags = ?
		WHERE id = ?
	`

	result, err := t.tx.ExecContext(ctx, query,
		crash.Signature.Hash,
		string(topFrames),
		string(functionNames),
		string(libraryNames),
		crash.Signature.SignatureType,
		crash.Signature.Confidence,
		crash.Input,
		crash.InputHash,
		crash.StackTrace,
		crash.Severity,
		crash.Type,
		crash.LastSeenAt,
		crash.OccurrenceCount,
		crash.CorpusEntryID,
		crash.TargetInfo.Name,
		crash.TargetInfo.Version,
		crash.TargetInfo.Command,
		crash.TargetInfo.Environment,
		metadata,
		boolToInt(crash.Reproducible),
		boolToInt(crash.Fixed),
		crash.FixedAt,
		string(tags),
		crash.ID,
	)

	if err != nil {
		return errors.NewDatabaseError("update_crash_tx", err).
			WithDetail("crash_id", crash.ID)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_crash_tx_rows", err).
			WithDetail("crash_id", crash.ID)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_crash_tx", "crash").
			WithDetail("crash_id", crash.ID)
	}

	return nil
}

// DeleteTx deletes a crash within a transaction
func (t *crashTransaction) DeleteTx(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("delete_crash_tx", "crash ID cannot be empty")
	}

	query := `DELETE FROM crashes WHERE id = ?`

	result, err := t.tx.ExecContext(ctx, query, id)
	if err != nil {
		return errors.NewDatabaseError("delete_crash_tx", err).
			WithDetail("crash_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("delete_crash_tx_rows", err).
			WithDetail("crash_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("delete_crash_tx", "crash").
			WithDetail("crash_id", id)
	}

	return nil
}

// RecordOccurrenceTx increments the occurrence count within a transaction
func (t *crashTransaction) RecordOccurrenceTx(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("record_occurrence_tx", "crash ID cannot be empty")
	}

	query := `
		UPDATE crashes
		SET occurrence_count = occurrence_count + 1,
			last_seen_at = ?
		WHERE id = ?
	`

	result, err := t.tx.ExecContext(ctx, query, time.Now(), id)
	if err != nil {
		return errors.NewDatabaseError("record_occurrence_tx", err).
			WithDetail("crash_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("record_occurrence_tx_rows", err).
			WithDetail("crash_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("record_occurrence_tx", "crash").
			WithDetail("crash_id", id)
	}

	return nil
}

// MarkAsFixedTx marks a crash as fixed within a transaction
func (t *crashTransaction) MarkAsFixedTx(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("mark_as_fixed_tx", "crash ID cannot be empty")
	}

	query := `
		UPDATE crashes
		SET fixed = 1,
			fixed_at = ?
		WHERE id = ?
	`

	result, err := t.tx.ExecContext(ctx, query, time.Now(), id)
	if err != nil {
		return errors.NewDatabaseError("mark_as_fixed_tx", err).
			WithDetail("crash_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("mark_as_fixed_tx_rows", err).
			WithDetail("crash_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("mark_as_fixed_tx", "crash").
			WithDetail("crash_id", id)
	}

	return nil
}

// MarkAsNotReproducibleTx marks a crash as not reproducible within a transaction
func (t *crashTransaction) MarkAsNotReproducibleTx(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("mark_as_not_reproducible_tx", "crash ID cannot be empty")
	}

	query := `
		UPDATE crashes
		SET reproducible = 0
		WHERE id = ?
	`

	result, err := t.tx.ExecContext(ctx, query, id)
	if err != nil {
		return errors.NewDatabaseError("mark_as_not_reproducible_tx", err).
			WithDetail("crash_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("mark_as_not_reproducible_tx_rows", err).
			WithDetail("crash_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("mark_as_not_reproducible_tx", "crash").
			WithDetail("crash_id", id)
	}

	return nil
}

// Ensure interfaces are implemented
var (
	_ repository.CrashRepository            = (*CrashRepository)(nil)
	_ repository.CrashTransactionRepository = (*CrashRepository)(nil)
	_ repository.CrashTransaction           = (*crashTransaction)(nil)
)
