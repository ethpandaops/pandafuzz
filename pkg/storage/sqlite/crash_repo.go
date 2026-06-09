package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	crashrepo "github.com/ethpandaops/pandafuzz/pkg/domain/crash/repository"
	crashtypes "github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/mappers"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/models"
	"github.com/sirupsen/logrus"
)

// Compile-time interface compliance check
var _ crashrepo.CrashRepository = (*CrashRepository)(nil)

// CrashRepository implements crashrepo.CrashRepository using SQLite
type CrashRepository struct {
	db     *sql.DB
	logger logrus.FieldLogger
}

// NewCrashRepository creates a new SQLite-based crash repository
func NewCrashRepository(db *sql.DB, logger logrus.FieldLogger) *CrashRepository {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	return &CrashRepository{
		db:     db,
		logger: logger.WithField("component", "crash_repository"),
	}
}

// Create creates a new crash
func (r *CrashRepository) Create(ctx context.Context, crash *crashtypes.Crash) error {
	if crash == nil {
		return NewRepositoryError("create", "crash", "", fmt.Errorf("crash cannot be nil"))
	}

	row := mappers.DomainCrashToRow(crash)

	query := `
		INSERT INTO crashes (
			id, signature_hash, signature, input, input_hash, stack_trace,
			severity, type, discovered_at, last_seen_at, occurrence_count,
			corpus_entry_id, target_name, target_version, target_command, target_env,
			metadata, reproducible, fixed, fixed_at, tags
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		row.ID, row.SignatureHash, row.SignatureJSON, row.Input, row.InputHash, row.StackTrace,
		row.Severity, row.Type, row.DiscoveredAt, row.LastSeenAt, row.OccurrenceCount,
		row.CorpusEntryID, row.TargetName, row.TargetVersion, row.TargetCommand, row.TargetEnv,
		row.MetadataJSON, row.Reproducible, row.Fixed, row.FixedAt, row.TagsJSON,
	)

	if err != nil {
		return NewRepositoryError("create", "crash", crash.ID, err)
	}

	r.logger.WithField("crash_id", crash.ID).Debug("Crash created")
	return nil
}

// Update updates an existing crash
func (r *CrashRepository) Update(ctx context.Context, crash *crashtypes.Crash) error {
	if crash == nil {
		return NewRepositoryError("update", "crash", "", fmt.Errorf("crash cannot be nil"))
	}

	row := mappers.DomainCrashToRow(crash)

	query := `
		UPDATE crashes SET
			signature_hash = ?, signature = ?, input = ?, input_hash = ?, stack_trace = ?,
			severity = ?, type = ?, last_seen_at = ?, occurrence_count = ?,
			corpus_entry_id = ?, target_name = ?, target_version = ?, target_command = ?, target_env = ?,
			metadata = ?, reproducible = ?, fixed = ?, fixed_at = ?, tags = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query,
		row.SignatureHash, row.SignatureJSON, row.Input, row.InputHash, row.StackTrace,
		row.Severity, row.Type, row.LastSeenAt, row.OccurrenceCount,
		row.CorpusEntryID, row.TargetName, row.TargetVersion, row.TargetCommand, row.TargetEnv,
		row.MetadataJSON, row.Reproducible, row.Fixed, row.FixedAt, row.TagsJSON,
		crash.ID,
	)

	if err != nil {
		return NewRepositoryError("update", "crash", crash.ID, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("update", "crash", crash.ID, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("crash_id", crash.ID).Debug("Crash updated")
	return nil
}

// Delete deletes a crash by ID
func (r *CrashRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM crashes WHERE id = ?", id)
	if err != nil {
		return NewRepositoryError("delete", "crash", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("delete", "crash", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("crash_id", id).Debug("Crash deleted")
	return nil
}

// FindByID retrieves a crash by its ID
func (r *CrashRepository) FindByID(ctx context.Context, id string) (*crashtypes.Crash, error) {
	row, err := r.scanCrashRow(ctx, "SELECT "+crashSelectColumns()+" FROM crashes WHERE id = ?", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, NewRepositoryError("find_by_id", "crash", id, err)
	}

	return mappers.CrashRowToDomain(row), nil
}

// FindBySignature retrieves crashes by signature hash
func (r *CrashRepository) FindBySignature(ctx context.Context, signatureHash string) ([]*crashtypes.Crash, error) {
	query := "SELECT " + crashSelectColumns() + " FROM crashes WHERE signature_hash = ? ORDER BY discovered_at DESC"
	return r.scanCrashRows(ctx, query, signatureHash)
}

// FindBySeverity retrieves all crashes with a specific severity
func (r *CrashRepository) FindBySeverity(ctx context.Context, severity crashtypes.Severity) ([]*crashtypes.Crash, error) {
	query := "SELECT " + crashSelectColumns() + " FROM crashes WHERE severity = ? ORDER BY discovered_at DESC"
	return r.scanCrashRows(ctx, query, string(severity))
}

// FindByType retrieves all crashes of a specific type
func (r *CrashRepository) FindByType(ctx context.Context, crashType crashtypes.CrashType) ([]*crashtypes.Crash, error) {
	query := "SELECT " + crashSelectColumns() + " FROM crashes WHERE type = ? ORDER BY discovered_at DESC"
	return r.scanCrashRows(ctx, query, string(crashType))
}

// FindByTarget retrieves all crashes for a specific target
func (r *CrashRepository) FindByTarget(ctx context.Context, targetName string) ([]*crashtypes.Crash, error) {
	query := "SELECT " + crashSelectColumns() + " FROM crashes WHERE target_name = ? ORDER BY discovered_at DESC"
	return r.scanCrashRows(ctx, query, targetName)
}

// FindByCorpusEntry retrieves crashes associated with a corpus entry
func (r *CrashRepository) FindByCorpusEntry(ctx context.Context, corpusEntryID string) ([]*crashtypes.Crash, error) {
	query := "SELECT " + crashSelectColumns() + " FROM crashes WHERE corpus_entry_id = ? ORDER BY discovered_at DESC"
	return r.scanCrashRows(ctx, query, corpusEntryID)
}

// FindReproducible retrieves all reproducible crashes
func (r *CrashRepository) FindReproducible(ctx context.Context) ([]*crashtypes.Crash, error) {
	query := "SELECT " + crashSelectColumns() + " FROM crashes WHERE reproducible = 1 ORDER BY discovered_at DESC"
	return r.scanCrashRows(ctx, query)
}

// FindUnfixed retrieves all unfixed crashes
func (r *CrashRepository) FindUnfixed(ctx context.Context) ([]*crashtypes.Crash, error) {
	query := "SELECT " + crashSelectColumns() + " FROM crashes WHERE fixed = 0 ORDER BY severity DESC, discovered_at DESC"
	return r.scanCrashRows(ctx, query)
}

// FindByTag retrieves all crashes with a specific tag
func (r *CrashRepository) FindByTag(ctx context.Context, tag string) ([]*crashtypes.Crash, error) {
	query := "SELECT " + crashSelectColumns() + " FROM crashes WHERE tags LIKE ? ORDER BY discovered_at DESC"
	return r.scanCrashRows(ctx, query, "%\""+tag+"\"%")
}

// FindRecent retrieves crashes discovered within a time range
func (r *CrashRepository) FindRecent(ctx context.Context, since time.Time) ([]*crashtypes.Crash, error) {
	query := "SELECT " + crashSelectColumns() + " FROM crashes WHERE discovered_at >= ? ORDER BY discovered_at DESC"
	return r.scanCrashRows(ctx, query, since)
}

// FindSimilar finds crashes similar to the given signature
func (r *CrashRepository) FindSimilar(ctx context.Context, signature *crashtypes.CrashSignature, threshold float64) ([]*crashtypes.Crash, error) {
	if signature == nil {
		return nil, NewRepositoryError("find_similar", "crash", "", fmt.Errorf("signature cannot be nil"))
	}

	// For SQLite, we'll do a simple signature hash match and let the caller do fuzzy matching
	// A more sophisticated implementation could use full-text search or custom functions
	query := "SELECT " + crashSelectColumns() + " FROM crashes WHERE signature_hash = ? ORDER BY discovered_at DESC"
	return r.scanCrashRows(ctx, query, signature.Hash)
}

// List retrieves crashes with pagination
func (r *CrashRepository) List(ctx context.Context, offset, limit int) ([]*crashtypes.Crash, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crashes").Scan(&total)
	if err != nil {
		return nil, 0, NewRepositoryError("list", "crash", "", err)
	}

	query := "SELECT " + crashSelectColumns() + " FROM crashes ORDER BY discovered_at DESC LIMIT ? OFFSET ?"
	crashes, err := r.scanCrashRows(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	return crashes, total, nil
}

// ListBySeverity retrieves crashes ordered by severity
func (r *CrashRepository) ListBySeverity(ctx context.Context, offset, limit int) ([]*crashtypes.Crash, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crashes").Scan(&total)
	if err != nil {
		return nil, 0, NewRepositoryError("list_by_severity", "crash", "", err)
	}

	// Order by severity: critical > high > medium > low > unknown
	query := `
		SELECT ` + crashSelectColumns() + `
		FROM crashes
		ORDER BY
			CASE severity
				WHEN 'critical' THEN 1
				WHEN 'high' THEN 2
				WHEN 'medium' THEN 3
				WHEN 'low' THEN 4
				ELSE 5
			END,
			discovered_at DESC
		LIMIT ? OFFSET ?
	`
	crashes, err := r.scanCrashRows(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	return crashes, total, nil
}

// ListByOccurrence retrieves crashes ordered by occurrence count
func (r *CrashRepository) ListByOccurrence(ctx context.Context, offset, limit int, ascending bool) ([]*crashtypes.Crash, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crashes").Scan(&total)
	if err != nil {
		return nil, 0, NewRepositoryError("list_by_occurrence", "crash", "", err)
	}

	order := "DESC"
	if ascending {
		order = "ASC"
	}

	query := fmt.Sprintf("SELECT %s FROM crashes ORDER BY occurrence_count %s LIMIT ? OFFSET ?", crashSelectColumns(), order)
	crashes, err := r.scanCrashRows(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	return crashes, total, nil
}

// RecordOccurrence increments the occurrence count for a crash
func (r *CrashRepository) RecordOccurrence(ctx context.Context, id string) error {
	now := time.Now().UTC()
	query := `
		UPDATE crashes
		SET occurrence_count = occurrence_count + 1, last_seen_at = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query, now, id)
	if err != nil {
		return NewRepositoryError("record_occurrence", "crash", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("record_occurrence", "crash", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// MarkAsFixed marks a crash as fixed
func (r *CrashRepository) MarkAsFixed(ctx context.Context, id string) error {
	now := time.Now().UTC()
	query := `
		UPDATE crashes
		SET fixed = 1, fixed_at = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query, now, id)
	if err != nil {
		return NewRepositoryError("mark_as_fixed", "crash", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("mark_as_fixed", "crash", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("crash_id", id).Debug("Crash marked as fixed")
	return nil
}

// MarkAsNotReproducible marks a crash as not reproducible
func (r *CrashRepository) MarkAsNotReproducible(ctx context.Context, id string) error {
	query := `
		UPDATE crashes
		SET reproducible = 0
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return NewRepositoryError("mark_as_not_reproducible", "crash", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("mark_as_not_reproducible", "crash", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("crash_id", id).Debug("Crash marked as not reproducible")
	return nil
}

// Exists checks if a crash exists by ID
func (r *CrashRepository) Exists(ctx context.Context, id string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM crashes WHERE id = ?)"
	err := r.db.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		return false, NewRepositoryError("exists", "crash", id, err)
	}
	return exists, nil
}

// ExistsBySignature checks if a crash exists by signature
func (r *CrashRepository) ExistsBySignature(ctx context.Context, signatureHash string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM crashes WHERE signature_hash = ?)"
	err := r.db.QueryRowContext(ctx, query, signatureHash).Scan(&exists)
	if err != nil {
		return false, NewRepositoryError("exists_by_signature", "crash", "", err)
	}
	return exists, nil
}

// Count returns the total number of crashes
func (r *CrashRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crashes").Scan(&count)
	if err != nil {
		return 0, NewRepositoryError("count", "crash", "", err)
	}
	return count, nil
}

// CountBySeverity counts crashes by severity
func (r *CrashRepository) CountBySeverity(ctx context.Context, severity crashtypes.Severity) (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM crashes WHERE severity = ?"
	err := r.db.QueryRowContext(ctx, query, string(severity)).Scan(&count)
	if err != nil {
		return 0, NewRepositoryError("count_by_severity", "crash", "", err)
	}
	return count, nil
}

// CountByType counts crashes by type
func (r *CrashRepository) CountByType(ctx context.Context, crashType crashtypes.CrashType) (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM crashes WHERE type = ?"
	err := r.db.QueryRowContext(ctx, query, string(crashType)).Scan(&count)
	if err != nil {
		return 0, NewRepositoryError("count_by_type", "crash", "", err)
	}
	return count, nil
}

// CountUnfixed counts unfixed crashes
func (r *CrashRepository) CountUnfixed(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crashes WHERE fixed = 0").Scan(&count)
	if err != nil {
		return 0, NewRepositoryError("count_unfixed", "crash", "", err)
	}
	return count, nil
}

// GetStatsByTarget retrieves crash statistics grouped by target
func (r *CrashRepository) GetStatsByTarget(ctx context.Context) (map[string]crashrepo.CrashStats, error) {
	// First get all unique targets
	targetsQuery := "SELECT DISTINCT target_name FROM crashes"
	rows, err := r.db.QueryContext(ctx, targetsQuery)
	if err != nil {
		return nil, NewRepositoryError("get_stats_by_target", "crash", "", err)
	}
	defer rows.Close()

	stats := make(map[string]crashrepo.CrashStats)
	targets := make([]string, 0)

	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			return nil, NewRepositoryError("get_stats_by_target", "crash", "", err)
		}
		targets = append(targets, target)
	}

	if err := rows.Err(); err != nil {
		return nil, NewRepositoryError("get_stats_by_target", "crash", "", err)
	}

	// For each target, compute stats
	for _, target := range targets {
		targetStats := crashrepo.CrashStats{
			BySeverity: make(map[crashtypes.Severity]int),
			ByType:     make(map[crashtypes.CrashType]int),
		}

		// Total count
		err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crashes WHERE target_name = ?", target).Scan(&targetStats.Total)
		if err != nil {
			return nil, NewRepositoryError("get_stats_by_target", "crash", "", err)
		}

		// Reproducible count
		err = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crashes WHERE target_name = ? AND reproducible = 1", target).Scan(&targetStats.Reproducible)
		if err != nil {
			return nil, NewRepositoryError("get_stats_by_target", "crash", "", err)
		}

		// Fixed count
		err = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM crashes WHERE target_name = ? AND fixed = 1", target).Scan(&targetStats.Fixed)
		if err != nil {
			return nil, NewRepositoryError("get_stats_by_target", "crash", "", err)
		}

		// By severity
		severityRows, err := r.db.QueryContext(ctx, "SELECT severity, COUNT(*) FROM crashes WHERE target_name = ? GROUP BY severity", target)
		if err != nil {
			return nil, NewRepositoryError("get_stats_by_target", "crash", "", err)
		}
		for severityRows.Next() {
			var sev string
			var cnt int
			if err := severityRows.Scan(&sev, &cnt); err != nil {
				severityRows.Close()
				return nil, NewRepositoryError("get_stats_by_target", "crash", "", err)
			}
			targetStats.BySeverity[crashtypes.Severity(sev)] = cnt
		}
		severityRows.Close()

		// By type
		typeRows, err := r.db.QueryContext(ctx, "SELECT type, COUNT(*) FROM crashes WHERE target_name = ? GROUP BY type", target)
		if err != nil {
			return nil, NewRepositoryError("get_stats_by_target", "crash", "", err)
		}
		for typeRows.Next() {
			var typ string
			var cnt int
			if err := typeRows.Scan(&typ, &cnt); err != nil {
				typeRows.Close()
				return nil, NewRepositoryError("get_stats_by_target", "crash", "", err)
			}
			targetStats.ByType[crashtypes.CrashType(typ)] = cnt
		}
		typeRows.Close()

		stats[target] = targetStats
	}

	return stats, nil
}

// Helper functions

// crashSelectColumns returns the column list for SELECT queries
func crashSelectColumns() string {
	return `id, signature_hash, signature, input, input_hash, stack_trace,
		severity, type, discovered_at, last_seen_at, occurrence_count,
		corpus_entry_id, target_name, target_version, target_command, target_env,
		metadata, reproducible, fixed, fixed_at, tags`
}

// scanCrashRow scans a single crash row from a query
func (r *CrashRepository) scanCrashRow(ctx context.Context, query string, args ...interface{}) (*models.DomainCrashRow, error) {
	row := r.db.QueryRowContext(ctx, query, args...)
	return r.scanSingleCrashRow(row)
}

// scanCrashRows scans multiple crash rows from a query
func (r *CrashRepository) scanCrashRows(ctx context.Context, query string, args ...interface{}) ([]*crashtypes.Crash, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	crashes := make([]*crashtypes.Crash, 0, 16)
	for rows.Next() {
		crashRow, err := r.scanRowsCrashRow(rows)
		if err != nil {
			return nil, err
		}
		crashes = append(crashes, mappers.CrashRowToDomain(crashRow))
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return crashes, nil
}

// scanSingleCrashRow scans a single *sql.Row into a DomainCrashRow
func (r *CrashRepository) scanSingleCrashRow(row *sql.Row) (*models.DomainCrashRow, error) {
	cr := &models.DomainCrashRow{}

	err := row.Scan(
		&cr.ID, &cr.SignatureHash, &cr.SignatureJSON, &cr.Input, &cr.InputHash, &cr.StackTrace,
		&cr.Severity, &cr.Type, &cr.DiscoveredAt, &cr.LastSeenAt, &cr.OccurrenceCount,
		&cr.CorpusEntryID, &cr.TargetName, &cr.TargetVersion, &cr.TargetCommand, &cr.TargetEnv,
		&cr.MetadataJSON, &cr.Reproducible, &cr.Fixed, &cr.FixedAt, &cr.TagsJSON,
	)
	if err != nil {
		return nil, err
	}

	return cr, nil
}

// scanRowsCrashRow scans a single row from *sql.Rows into a DomainCrashRow
func (r *CrashRepository) scanRowsCrashRow(rows *sql.Rows) (*models.DomainCrashRow, error) {
	cr := &models.DomainCrashRow{}

	err := rows.Scan(
		&cr.ID, &cr.SignatureHash, &cr.SignatureJSON, &cr.Input, &cr.InputHash, &cr.StackTrace,
		&cr.Severity, &cr.Type, &cr.DiscoveredAt, &cr.LastSeenAt, &cr.OccurrenceCount,
		&cr.CorpusEntryID, &cr.TargetName, &cr.TargetVersion, &cr.TargetCommand, &cr.TargetEnv,
		&cr.MetadataJSON, &cr.Reproducible, &cr.Fixed, &cr.FixedAt, &cr.TagsJSON,
	)
	if err != nil {
		return nil, err
	}

	return cr, nil
}
