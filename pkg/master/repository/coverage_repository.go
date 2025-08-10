package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/cache"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/sqlite"
)

// CoverageReport represents a coverage report in the database
type CoverageReport struct {
	ID          string    `json:"id"`
	JobID       string    `json:"job_id"`
	Format      string    `json:"format"`
	StoragePath string    `json:"storage_path"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
}

// CoverageMetadata represents coverage metadata in the database
type CoverageMetadata struct {
	ID               int64     `json:"id"`
	ReportID         string    `json:"report_id"`
	LineCoverage     *float64  `json:"line_coverage"`
	FunctionCoverage *float64  `json:"function_coverage"`
	BranchCoverage   *float64  `json:"branch_coverage"`
	TotalLines       *int      `json:"total_lines"`
	CoveredLines     *int      `json:"covered_lines"`
	TotalFunctions   *int      `json:"total_functions"`
	CoveredFunctions *int      `json:"covered_functions"`
	CollectedAt      time.Time `json:"collected_at"`
}

// CoverageReportFilter represents filters for coverage report queries
type CoverageReportFilter struct {
	JobID    string
	Format   string
	FromTime *time.Time
	ToTime   *time.Time
}

// CoverageRepository defines the interface for coverage operations
type CoverageRepository interface {
	// Report operations
	SaveReport(ctx context.Context, report *CoverageReport) error
	GetReportByID(ctx context.Context, reportID string) (*CoverageReport, error)
	GetReportsByJobID(ctx context.Context, jobID string, filter *CoverageReportFilter, offset, limit int) ([]*CoverageReport, int, error)
	DeleteReport(ctx context.Context, reportID string) error

	// Metadata operations
	SaveMetadata(ctx context.Context, metadata *CoverageMetadata) error
	GetMetadataByReportID(ctx context.Context, reportID string) (*CoverageMetadata, error)
	DeleteMetadata(ctx context.Context, reportID string) error

	// Utility operations
	Exists(ctx context.Context, reportID string) (bool, error)
}

// CoverageRepositoryImpl implements the coverage repository interface using SQLite
type CoverageRepositoryImpl struct {
	conn  *sqlite.Connection
	cache cache.Cache
	log   logrus.FieldLogger
}

// NewCoverageRepository creates a new coverage repository
func NewCoverageRepository(conn *sqlite.Connection, cache cache.Cache, log logrus.FieldLogger) (*CoverageRepositoryImpl, error) {
	if conn == nil {
		return nil, errors.New(errors.ErrorTypeDatabase, "new_coverage_repository", "connection is required")
	}
	if log == nil {
		log = logrus.New()
	}

	repo := &CoverageRepositoryImpl{
		conn:  conn,
		cache: cache,
		log:   log.WithField("component", "coverage_repository"),
	}

	if err := repo.createSchema(); err != nil {
		return nil, err
	}

	return repo, nil
}

// createSchema creates the coverage tables if they don't exist
func (r *CoverageRepositoryImpl) createSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS coverage_reports (
			id TEXT PRIMARY KEY,
			job_id TEXT NOT NULL,
			format TEXT NOT NULL,
			storage_path TEXT NOT NULL,
			size INTEGER NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS coverage_metadata (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			report_id TEXT NOT NULL,
			line_coverage REAL,
			function_coverage REAL,
			branch_coverage REAL,
			total_lines INTEGER,
			covered_lines INTEGER,
			total_functions INTEGER,
			covered_functions INTEGER,
			collected_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (report_id) REFERENCES coverage_reports(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_coverage_reports_job_id ON coverage_reports(job_id);
		CREATE INDEX IF NOT EXISTS idx_coverage_reports_created_at ON coverage_reports(created_at);
		CREATE INDEX IF NOT EXISTS idx_coverage_reports_format ON coverage_reports(format);
		CREATE INDEX IF NOT EXISTS idx_coverage_metadata_report_id ON coverage_metadata(report_id);
		CREATE INDEX IF NOT EXISTS idx_coverage_metadata_collected_at ON coverage_metadata(collected_at);
	`

	_, err := r.conn.ExecContext(context.Background(), schema)
	if err != nil {
		return errors.NewDatabaseError("create_coverage_schema", err)
	}

	return nil
}

// SaveReport saves a coverage report to the database
func (r *CoverageRepositoryImpl) SaveReport(ctx context.Context, report *CoverageReport) error {
	if report == nil {
		return errors.NewValidationError("save_coverage_report", "report cannot be nil")
	}

	if report.ID == "" {
		return errors.NewValidationError("save_coverage_report", "report ID cannot be empty")
	}

	query := `
		INSERT INTO coverage_reports (id, job_id, format, storage_path, size, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := r.conn.ExecContext(ctx, query,
		report.ID,
		report.JobID,
		report.Format,
		report.StoragePath,
		report.Size,
		report.CreatedAt,
	)

	if err != nil {
		if r.isUniqueConstraintError(err) {
			return errors.NewConflictError("save_coverage_report", "coverage report already exists").
				WithDetail("report_id", report.ID)
		}
		return errors.NewDatabaseError("save_coverage_report", err).
			WithDetail("report_id", report.ID)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.cacheKey(report.ID))
		r.cache.Delete(ctx, r.jobCacheKey(report.JobID))
	}

	r.log.WithField("report_id", report.ID).Debug("Coverage report saved")
	return nil
}

// GetReportByID retrieves a coverage report by its ID
func (r *CoverageRepositoryImpl) GetReportByID(ctx context.Context, reportID string) (*CoverageReport, error) {
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

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewNotFoundError("get_coverage_report", "coverage report").
				WithDetail("report_id", reportID)
		}
		return nil, errors.NewDatabaseError("get_coverage_report", err).
			WithDetail("report_id", reportID)
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, r.cacheKey(reportID), &report, 5*time.Minute)
	}

	return &report, nil
}

// GetReportsByJobID retrieves coverage reports for a specific job with pagination
func (r *CoverageRepositoryImpl) GetReportsByJobID(ctx context.Context, jobID string, filter *CoverageReportFilter, offset, limit int) ([]*CoverageReport, int, error) {
	if jobID == "" {
		return nil, 0, errors.NewValidationError("get_coverage_reports_by_job", "job ID cannot be empty")
	}

	if offset < 0 {
		return nil, 0, errors.NewValidationError("get_coverage_reports_by_job", "offset cannot be negative")
	}
	if limit <= 0 {
		return nil, 0, errors.NewValidationError("get_coverage_reports_by_job", "limit must be positive")
	}

	// Build base query with filters
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

	// Get total count
	countQuery := "SELECT COUNT(*) FROM coverage_reports " + baseWhere
	var total int
	err := r.conn.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("count_coverage_reports", err).
			WithDetail("job_id", jobID)
	}

	// Get paginated results
	query := `
		SELECT id, job_id, format, storage_path, size, created_at
		FROM coverage_reports ` + baseWhere + `
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

	reports, err := r.scanReports(rows)
	if err != nil {
		return nil, 0, err
	}

	return reports, total, nil
}

// DeleteReport deletes a coverage report and its metadata
func (r *CoverageRepositoryImpl) DeleteReport(ctx context.Context, reportID string) error {
	if reportID == "" {
		return errors.NewValidationError("delete_coverage_report", "report ID cannot be empty")
	}

	// Start transaction to ensure consistency
	tx, err := r.conn.DB().BeginTx(ctx, nil)
	if err != nil {
		return errors.NewDatabaseError("delete_coverage_report_begin_tx", err).
			WithDetail("report_id", reportID)
	}
	defer tx.Rollback()

	// Delete metadata first (due to foreign key)
	_, err = tx.ExecContext(ctx, "DELETE FROM coverage_metadata WHERE report_id = ?", reportID)
	if err != nil {
		return errors.NewDatabaseError("delete_coverage_metadata", err).
			WithDetail("report_id", reportID)
	}

	// Delete report
	result, err := tx.ExecContext(ctx, "DELETE FROM coverage_reports WHERE id = ?", reportID)
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

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return errors.NewDatabaseError("delete_coverage_report_commit", err).
			WithDetail("report_id", reportID)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.cacheKey(reportID))
		r.cache.Delete(ctx, r.metadataCacheKey(reportID))
	}

	r.log.WithField("report_id", reportID).Debug("Coverage report deleted")
	return nil
}

// SaveMetadata saves coverage metadata to the database
func (r *CoverageRepositoryImpl) SaveMetadata(ctx context.Context, metadata *CoverageMetadata) error {
	if metadata == nil {
		return errors.NewValidationError("save_coverage_metadata", "metadata cannot be nil")
	}

	if metadata.ReportID == "" {
		return errors.NewValidationError("save_coverage_metadata", "report ID cannot be empty")
	}

	query := `
		INSERT INTO coverage_metadata 
		(report_id, line_coverage, function_coverage, branch_coverage, 
		 total_lines, covered_lines, total_functions, covered_functions, collected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.conn.ExecContext(ctx, query,
		metadata.ReportID,
		metadata.LineCoverage,
		metadata.FunctionCoverage,
		metadata.BranchCoverage,
		metadata.TotalLines,
		metadata.CoveredLines,
		metadata.TotalFunctions,
		metadata.CoveredFunctions,
		metadata.CollectedAt,
	)

	if err != nil {
		return errors.NewDatabaseError("save_coverage_metadata", err).
			WithDetail("report_id", metadata.ReportID)
	}

	// Get the auto-generated ID
	id, err := result.LastInsertId()
	if err != nil {
		return errors.NewDatabaseError("save_coverage_metadata_id", err).
			WithDetail("report_id", metadata.ReportID)
	}
	metadata.ID = id

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.metadataCacheKey(metadata.ReportID))
	}

	r.log.WithField("report_id", metadata.ReportID).Debug("Coverage metadata saved")
	return nil
}

// GetMetadataByReportID retrieves coverage metadata for a specific report
func (r *CoverageRepositoryImpl) GetMetadataByReportID(ctx context.Context, reportID string) (*CoverageMetadata, error) {
	if reportID == "" {
		return nil, errors.NewValidationError("get_coverage_metadata", "report ID cannot be empty")
	}

	// Check cache first
	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, r.metadataCacheKey(reportID)); found {
			if metadata, ok := cached.(*CoverageMetadata); ok {
				return metadata, nil
			}
		}
	}

	query := `
		SELECT id, report_id, line_coverage, function_coverage, branch_coverage,
		       total_lines, covered_lines, total_functions, covered_functions, collected_at
		FROM coverage_metadata
		WHERE report_id = ?
	`

	var metadata CoverageMetadata
	err := r.conn.QueryRowContext(ctx, query, reportID).Scan(
		&metadata.ID,
		&metadata.ReportID,
		&metadata.LineCoverage,
		&metadata.FunctionCoverage,
		&metadata.BranchCoverage,
		&metadata.TotalLines,
		&metadata.CoveredLines,
		&metadata.TotalFunctions,
		&metadata.CoveredFunctions,
		&metadata.CollectedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewNotFoundError("get_coverage_metadata", "coverage metadata").
				WithDetail("report_id", reportID)
		}
		return nil, errors.NewDatabaseError("get_coverage_metadata", err).
			WithDetail("report_id", reportID)
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, r.metadataCacheKey(reportID), &metadata, 5*time.Minute)
	}

	return &metadata, nil
}

// DeleteMetadata deletes coverage metadata for a specific report
func (r *CoverageRepositoryImpl) DeleteMetadata(ctx context.Context, reportID string) error {
	if reportID == "" {
		return errors.NewValidationError("delete_coverage_metadata", "report ID cannot be empty")
	}

	query := "DELETE FROM coverage_metadata WHERE report_id = ?"

	result, err := r.conn.ExecContext(ctx, query, reportID)
	if err != nil {
		return errors.NewDatabaseError("delete_coverage_metadata", err).
			WithDetail("report_id", reportID)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("delete_coverage_metadata_rows", err).
			WithDetail("report_id", reportID)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("delete_coverage_metadata", "coverage metadata").
			WithDetail("report_id", reportID)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.metadataCacheKey(reportID))
	}

	r.log.WithField("report_id", reportID).Debug("Coverage metadata deleted")
	return nil
}

// Exists checks if a coverage report exists by ID
func (r *CoverageRepositoryImpl) Exists(ctx context.Context, reportID string) (bool, error) {
	if reportID == "" {
		return false, errors.NewValidationError("coverage_report_exists", "report ID cannot be empty")
	}

	query := "SELECT 1 FROM coverage_reports WHERE id = ? LIMIT 1"

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

// Helper methods

// scanReports scans multiple coverage report rows
func (r *CoverageRepositoryImpl) scanReports(rows *sql.Rows) ([]*CoverageReport, error) {
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
			return nil, errors.NewDatabaseError("scan_coverage_report", err)
		}
		reports = append(reports, &report)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.NewDatabaseError("scan_coverage_reports_rows", err)
	}

	return reports, nil
}

// isUniqueConstraintError checks if an error is a unique constraint violation
func (r *CoverageRepositoryImpl) isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	// SQLite unique constraint error contains "UNIQUE constraint failed"
	return contains(err.Error(), "UNIQUE constraint failed")
}

// cacheKey generates a cache key for a coverage report
func (r *CoverageRepositoryImpl) cacheKey(reportID string) string {
	return "coverage_report:" + reportID
}

// metadataCacheKey generates a cache key for coverage metadata
func (r *CoverageRepositoryImpl) metadataCacheKey(reportID string) string {
	return "coverage_metadata:" + reportID
}

// jobCacheKey generates a cache key for job coverage reports
func (r *CoverageRepositoryImpl) jobCacheKey(jobID string) string {
	return "coverage_job:" + jobID
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Ensure interface is implemented
var _ CoverageRepository = (*CoverageRepositoryImpl)(nil)
