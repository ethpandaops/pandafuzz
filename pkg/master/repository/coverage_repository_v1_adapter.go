package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/cache"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/sqlite"
)

// CoverageRepositoryV1Adapter adapts the v1 coverage table to the v3 repository interface
type CoverageRepositoryV1Adapter struct {
	conn  *sqlite.Connection
	cache cache.Cache
	log   logrus.FieldLogger
}

// NewCoverageRepositoryV1Adapter creates a new adapter for v1 coverage table
func NewCoverageRepositoryV1Adapter(conn *sqlite.Connection, cache cache.Cache, log logrus.FieldLogger) (*CoverageRepositoryV1Adapter, error) {
	if conn == nil {
		return nil, errors.New(errors.ErrorTypeDatabase, "new_coverage_repository_adapter", "connection is required")
	}
	if log == nil {
		log = logrus.New()
	}

	repo := &CoverageRepositoryV1Adapter{
		conn:  conn,
		cache: cache,
		log:   log.WithField("component", "coverage_repository_v1_adapter"),
	}

	// Don't create new schema - use existing v1 coverage table
	return repo, nil
}

// SaveReport saves a coverage report (not implemented for v1 adapter)
func (r *CoverageRepositoryV1Adapter) SaveReport(ctx context.Context, report *CoverageReport) error {
	// V1 API handles saving directly to coverage table
	return errors.New(errors.ErrorTypeValidation, "save_report", "SaveReport is not supported in v1 adapter")
}

// GetReportByID retrieves a coverage report by its ID from v1 coverage table
func (r *CoverageRepositoryV1Adapter) GetReportByID(ctx context.Context, reportID string) (*CoverageReport, error) {
	if reportID == "" {
		return nil, errors.NewValidationError("get_coverage_report", "report ID cannot be empty")
	}

	// Check cache first
	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, r.cacheKey(reportID)); found {
			if report, ok := cached.(*CoverageReport); ok {
				return report, nil
			}
		}
	}

	// First try coverage_reports table if it exists
	var tableName string
	checkTableQuery := `SELECT name FROM sqlite_master WHERE type='table' AND name='coverage_reports' LIMIT 1`
	err := r.conn.QueryRowContext(ctx, checkTableQuery).Scan(&tableName)
	hasReportsTable := err == nil && tableName == "coverage_reports"

	if hasReportsTable {
		// Try the new coverage_reports table first
		query := `
			SELECT id, job_id, format, storage_path, size, created_at
			FROM coverage_reports
			WHERE id = ?
		`

		var report CoverageReport
		err := r.conn.QueryRowContext(ctx, query, reportID).Scan(
			&report.ID,
			&report.JobID,
			&report.Format,
			&report.StoragePath,
			&report.Size,
			&report.CreatedAt,
		)

		if err == nil {
			// Cache the result
			if r.cache != nil {
				r.cache.SetWithTTL(ctx, r.cacheKey(reportID), &report, 5*time.Minute)
			}
			return &report, nil
		} else if err != sql.ErrNoRows {
			// If it's not a "not found" error, return the database error
			return nil, errors.NewDatabaseError("get_coverage_report", err).
				WithDetail("report_id", reportID)
		}
		// If not found in coverage_reports, fall through to try coverage table
	}

	// Query the v1 coverage table
	query := `
		SELECT id, job_id, bot_id, edges, new_edges, timestamp, exec_count, created_at
		FROM coverage
		WHERE id = ?
	`

	var (
		id        string
		jobID     string
		botID     string
		edges     int64
		newEdges  int64
		timestamp time.Time // DATETIME column
		execCount int64
		createdAt time.Time
	)

	err = r.conn.QueryRowContext(ctx, query, reportID).Scan(
		&id, &jobID, &botID, &edges, &newEdges, &timestamp, &execCount, &createdAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewNotFoundError("get_coverage_report", "coverage report").
				WithDetail("report_id", reportID)
		}
		return nil, errors.NewDatabaseError("get_coverage_report", err).
			WithDetail("report_id", reportID)
	}

	// Convert v1 data to v3 format
	report := &CoverageReport{
		ID:          id,
		JobID:       jobID,
		Format:      "afl++", // Default format for v1 data
		StoragePath: fmt.Sprintf("coverage/%s/%s", jobID, id),
		Size:        0, // Not stored in v1
		CreatedAt:   createdAt,
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, r.cacheKey(reportID), report, 5*time.Minute)
	}

	return report, nil
}

// GetReportsByJobID retrieves coverage reports for a specific job from v1 coverage table
func (r *CoverageRepositoryV1Adapter) GetReportsByJobID(ctx context.Context, jobID string, filter *CoverageReportFilter, offset, limit int) ([]*CoverageReport, int, error) {
	r.log.WithFields(logrus.Fields{
		"job_id": jobID,
		"offset": offset,
		"limit":  limit,
	}).Debug("GetReportsByJobID called")

	if jobID == "" {
		return nil, 0, errors.NewValidationError("get_coverage_reports_by_job", "job ID cannot be empty")
	}

	if offset < 0 {
		return nil, 0, errors.NewValidationError("get_coverage_reports_by_job", "offset cannot be negative")
	}
	if limit <= 0 {
		return nil, 0, errors.NewValidationError("get_coverage_reports_by_job", "limit must be positive")
	}

	// First, check if coverage_reports table exists and has data
	var tableName string
	checkTableQuery := `SELECT name FROM sqlite_master WHERE type='table' AND name='coverage_reports' LIMIT 1`
	err := r.conn.QueryRowContext(ctx, checkTableQuery).Scan(&tableName)
	hasReportsTable := err == nil && tableName == "coverage_reports"

	if hasReportsTable {
		// Use the new coverage_reports table if it exists
		r.log.Debug("Using coverage_reports table")

		baseWhere := "WHERE job_id = ?"
		args := []any{jobID}

		if filter != nil {
			if filter.Format != "" {
				baseWhere += " AND format = ?"
				args = append(args, filter.Format)
			}
			if filter.FromTime != nil {
				baseWhere += " AND created_at >= ?"
				args = append(args, *filter.FromTime)
			}
			if filter.ToTime != nil {
				baseWhere += " AND created_at <= ?"
				args = append(args, *filter.ToTime)
			}
		}

		// Get total count from coverage_reports
		countQuery := "SELECT COUNT(*) FROM coverage_reports " + baseWhere
		var total int
		err := r.conn.QueryRowContext(ctx, countQuery, args...).Scan(&total)
		if err != nil {
			r.log.WithError(err).Debug("Failed to count from coverage_reports, falling back to coverage table")
			// Fall back to the old method if this fails
			return r.getReportsFromCoverageTable(ctx, jobID, filter, offset, limit)
		}

		// Get paginated results from coverage_reports table
		query := `
			SELECT id, job_id, format, storage_path, size, created_at
			FROM coverage_reports ` + baseWhere + `
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?
		`

		paginationArgs := append(args, limit, offset)
		rows, err := r.conn.QueryContext(ctx, query, paginationArgs...)
		if err != nil {
			r.log.WithError(err).Debug("Failed to query coverage_reports, falling back to coverage table")
			return r.getReportsFromCoverageTable(ctx, jobID, filter, offset, limit)
		}
		defer rows.Close()

		var reports []*CoverageReport
		for rows.Next() {
			var report CoverageReport
			err := rows.Scan(
				&report.ID,
				&report.JobID,
				&report.Format,
				&report.StoragePath,
				&report.Size,
				&report.CreatedAt,
			)
			if err != nil {
				return nil, 0, errors.NewDatabaseError("scan_coverage_report", err)
			}
			reports = append(reports, &report)
		}

		if err := rows.Err(); err != nil {
			return nil, 0, errors.NewDatabaseError("scan_coverage_reports_rows", err)
		}

		r.log.WithFields(logrus.Fields{
			"job_id":        jobID,
			"reports_count": len(reports),
			"total":         total,
		}).Debug("Reports retrieved from coverage_reports table")

		return reports, total, nil
	}

	// Fall back to the old coverage table method
	return r.getReportsFromCoverageTable(ctx, jobID, filter, offset, limit)
}

// getReportsFromCoverageTable is the fallback method using the old coverage table
func (r *CoverageRepositoryV1Adapter) getReportsFromCoverageTable(ctx context.Context, jobID string, filter *CoverageReportFilter, offset, limit int) ([]*CoverageReport, int, error) {
	// Build base query for v1 coverage table
	baseWhere := "WHERE job_id = ?"
	args := []any{jobID}

	if filter != nil {
		if filter.FromTime != nil {
			baseWhere += " AND created_at >= ?"
			args = append(args, *filter.FromTime)
		}
		if filter.ToTime != nil {
			baseWhere += " AND created_at <= ?"
			args = append(args, *filter.ToTime)
		}
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM coverage " + baseWhere
	var total int
	err := r.conn.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("count_coverage_reports", err).
			WithDetail("job_id", jobID)
	}

	r.log.WithFields(logrus.Fields{
		"job_id":      jobID,
		"total_count": total,
		"query":       countQuery,
		"args":        args,
	}).Debug("Count query executed on coverage table")

	// Get paginated results from v1 coverage table
	query := `
		SELECT id, job_id, bot_id, edges, new_edges, timestamp, exec_count, created_at
		FROM coverage ` + baseWhere + `
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	paginationArgs := append(args, limit, offset)
	rows, err := r.conn.QueryContext(ctx, query, paginationArgs...)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("get_coverage_reports_by_job", err).
			WithDetail("job_id", jobID)
	}
	defer rows.Close()

	reports, err := r.scanV1Reports(rows)
	if err != nil {
		return nil, 0, err
	}

	r.log.WithFields(logrus.Fields{
		"job_id":        jobID,
		"reports_count": len(reports),
		"total":         total,
	}).Debug("Reports retrieved from coverage table")

	// Also check for full coverage reports stored in metadata
	// Try to fetch additional data from coverage:report:* keys if available
	for _, report := range reports {
		r.enrichReportFromMetadata(ctx, report)
	}

	return reports, total, nil
}

// enrichReportFromMetadata tries to fetch additional coverage data from metadata storage
func (r *CoverageRepositoryV1Adapter) enrichReportFromMetadata(ctx context.Context, report *CoverageReport) {
	// Query metadata table for full report data
	metaQuery := `
		SELECT value FROM metadata WHERE key = ?
	`

	fullReportKey := fmt.Sprintf("coverage:report:%s", report.ID)
	var metadataJSON string
	err := r.conn.QueryRowContext(ctx, metaQuery, fullReportKey).Scan(&metadataJSON)
	if err == nil && metadataJSON != "" {
		// Try to parse the JSON data to get format info
		var fullReport struct {
			Format string `json:"format"`
		}
		if json.Unmarshal([]byte(metadataJSON), &fullReport) == nil && fullReport.Format != "" {
			report.Format = fullReport.Format
		}
	}
}

// DeleteReport deletes a coverage report from v1 table
func (r *CoverageRepositoryV1Adapter) DeleteReport(ctx context.Context, reportID string) error {
	if reportID == "" {
		return errors.NewValidationError("delete_coverage_report", "report ID cannot be empty")
	}

	// Delete from v1 coverage table
	result, err := r.conn.ExecContext(ctx, "DELETE FROM coverage WHERE id = ?", reportID)
	if err != nil {
		return errors.NewDatabaseError("delete_coverage_report", err).
			WithDetail("report_id", reportID)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("delete_coverage_report_rows", err).
			WithDetail("report_id", reportID)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("delete_coverage_report", "coverage report").
			WithDetail("report_id", reportID)
	}

	// Also try to delete from metadata
	r.conn.ExecContext(ctx, "DELETE FROM metadata WHERE key LIKE ?", fmt.Sprintf("coverage:%%:%s", reportID))

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.cacheKey(reportID))
	}

	r.log.WithField("report_id", reportID).Debug("Coverage report deleted")
	return nil
}

// SaveMetadata saves coverage metadata (not implemented for v1 adapter)
func (r *CoverageRepositoryV1Adapter) SaveMetadata(ctx context.Context, metadata *CoverageMetadata) error {
	// V1 doesn't have separate metadata table
	return errors.New(errors.ErrorTypeValidation, "save_metadata", "SaveMetadata is not supported in v1 adapter")
}

// GetMetadataByReportID retrieves coverage metadata from v1 data
func (r *CoverageRepositoryV1Adapter) GetMetadataByReportID(ctx context.Context, reportID string) (*CoverageMetadata, error) {
	if reportID == "" {
		return nil, errors.NewValidationError("get_coverage_metadata", "report ID cannot be empty")
	}

	// Try to get full report from metadata
	metaQuery := `
		SELECT value FROM metadata WHERE key = ?
	`

	fullReportKey := fmt.Sprintf("coverage:report:%s", reportID)
	var metadataJSON string
	err := r.conn.QueryRowContext(ctx, metaQuery, fullReportKey).Scan(&metadataJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			// No metadata available, return minimal data
			return &CoverageMetadata{
				ReportID:    reportID,
				CollectedAt: time.Now(),
			}, nil
		}
		return nil, errors.NewDatabaseError("get_coverage_metadata", err).
			WithDetail("report_id", reportID)
	}

	// Parse the JSON to extract coverage percentages
	var fullReport struct {
		LineCoverage     float64   `json:"line_coverage"`
		FunctionCoverage float64   `json:"function_coverage"`
		BranchCoverage   float64   `json:"branch_coverage"`
		CollectedAt      time.Time `json:"collected_at"`
	}

	if err := json.Unmarshal([]byte(metadataJSON), &fullReport); err != nil {
		// Return minimal metadata if parsing fails
		return &CoverageMetadata{
			ReportID:    reportID,
			CollectedAt: time.Now(),
		}, nil
	}

	metadata := &CoverageMetadata{
		ReportID:         reportID,
		LineCoverage:     &fullReport.LineCoverage,
		FunctionCoverage: &fullReport.FunctionCoverage,
		BranchCoverage:   &fullReport.BranchCoverage,
		CollectedAt:      fullReport.CollectedAt,
	}

	if metadata.CollectedAt.IsZero() {
		metadata.CollectedAt = time.Now()
	}

	return metadata, nil
}

// DeleteMetadata deletes coverage metadata (not implemented for v1 adapter)
func (r *CoverageRepositoryV1Adapter) DeleteMetadata(ctx context.Context, reportID string) error {
	// V1 doesn't have separate metadata table
	return nil
}

// Exists checks if a coverage report exists in v1 table
func (r *CoverageRepositoryV1Adapter) Exists(ctx context.Context, reportID string) (bool, error) {
	if reportID == "" {
		return false, errors.NewValidationError("coverage_report_exists", "report ID cannot be empty")
	}

	query := "SELECT 1 FROM coverage WHERE id = ? LIMIT 1"

	var exists int
	err := r.conn.QueryRowContext(ctx, query, reportID).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, errors.NewDatabaseError("coverage_report_exists", err).
			WithDetail("report_id", reportID)
	}

	return true, nil
}

// scanV1Reports scans multiple coverage report rows from v1 table
func (r *CoverageRepositoryV1Adapter) scanV1Reports(rows *sql.Rows) ([]*CoverageReport, error) {
	var reports []*CoverageReport

	for rows.Next() {
		var (
			id        string
			jobID     string
			botID     string
			edges     int64
			newEdges  int64
			timestamp time.Time // DATETIME column
			execCount int64
			createdAt time.Time
		)

		err := rows.Scan(&id, &jobID, &botID, &edges, &newEdges, &timestamp, &execCount, &createdAt)
		if err != nil {
			return nil, errors.NewDatabaseError("scan_coverage_report", err)
		}

		report := &CoverageReport{
			ID:          id,
			JobID:       jobID,
			Format:      "afl++", // Default format for v1 data
			StoragePath: fmt.Sprintf("coverage/%s/%s", jobID, id),
			Size:        0, // Not stored in v1
			CreatedAt:   createdAt,
		}
		reports = append(reports, report)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.NewDatabaseError("scan_coverage_reports_rows", err)
	}

	return reports, nil
}

// cacheKey generates a cache key for a coverage report
func (r *CoverageRepositoryV1Adapter) cacheKey(reportID string) string {
	return "coverage_report_v1:" + reportID
}

// Ensure interface is implemented
var _ CoverageRepository = (*CoverageRepositoryV1Adapter)(nil)
