package coverage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/errors"
)

// CoverageRepository interface defines methods for coverage data storage operations
type CoverageRepository interface {
	// Store saves coverage data to filesystem
	Store(ctx context.Context, jobID string, coverageData []byte) error

	// Get retrieves coverage data by job ID
	Get(ctx context.Context, jobID string) (*CoverageReport, error)

	// List returns all coverage reports with optional prefix filtering
	List(ctx context.Context, prefix string) ([]*CoverageReport, error)

	// Delete removes coverage data by job ID
	Delete(ctx context.Context, jobID string) error

	// GetMetadata retrieves metadata for a coverage report
	GetMetadata(ctx context.Context, jobID string) (*CoverageMetadata, error)
}

// CoverageReport represents a coverage report with its data and metadata
type CoverageReport struct {
	JobID     string           `json:"job_id"`
	Data      []byte           `json:"data"`
	Metadata  CoverageMetadata `json:"metadata"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// CoverageMetadata contains metadata about a coverage report
type CoverageMetadata struct {
	JobID           string            `json:"job_id"`
	FuzzerType      string            `json:"fuzzer_type"`
	TotalBlocks     int64             `json:"total_blocks"`
	CoveredBlocks   int64             `json:"covered_blocks"`
	CoveragePercent float64           `json:"coverage_percent"`
	Size            int64             `json:"size"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	CustomMetadata  map[string]string `json:"custom_metadata,omitempty"`
}

// FilesystemCoverageRepository implements CoverageRepository using filesystem storage
type FilesystemCoverageRepository struct {
	basePath string
	fileMode os.FileMode
	dirMode  os.FileMode
	logger   logrus.FieldLogger
	mu       sync.RWMutex
}

// Ensure interface compliance
var _ CoverageRepository = (*FilesystemCoverageRepository)(nil)

// NewFilesystemCoverageRepository creates a new filesystem-based coverage repository
func NewFilesystemCoverageRepository(basePath string, logger logrus.FieldLogger) (*FilesystemCoverageRepository, error) {
	if basePath == "" {
		return nil, errors.NewValidationError("new_filesystem_coverage_repository", "base path cannot be empty")
	}

	if logger == nil {
		logger = logrus.New()
	}

	// Ensure base path is absolute
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, errors.NewSystemError("resolve_base_path", fmt.Errorf("failed to resolve absolute path: %w", err))
	}

	// Create coverage directory under base path
	coveragePath := filepath.Join(absPath, "coverage")
	if err := os.MkdirAll(coveragePath, 0o755); err != nil {
		return nil, errors.NewSystemError("create_coverage_directory", fmt.Errorf("failed to create coverage directory: %w", err))
	}

	// Test write permissions
	testFile := filepath.Join(coveragePath, ".write_test")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		return nil, errors.NewSystemError("test_write_permissions", fmt.Errorf("coverage directory is not writable: %w", err))
	}
	_ = os.Remove(testFile)

	repo := &FilesystemCoverageRepository{
		basePath: coveragePath,
		fileMode: 0o644,
		dirMode:  0o755,
		logger:   logger.WithField("component", "filesystem_coverage_repository"),
	}

	repo.logger.WithField("base_path", coveragePath).Info("Initialized filesystem coverage repository")

	return repo, nil
}

// Store saves coverage data to filesystem
func (r *FilesystemCoverageRepository) Store(ctx context.Context, jobID string, coverageData []byte) error {
	if jobID == "" {
		return errors.NewValidationError("store_coverage", "job ID cannot be empty")
	}

	if len(coverageData) == 0 {
		return errors.NewValidationError("store_coverage", "coverage data cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Create job-specific directory
	jobDir := filepath.Join(r.basePath, jobID)
	if err := os.MkdirAll(jobDir, r.dirMode); err != nil {
		return errors.NewSystemError("create_job_directory", fmt.Errorf("failed to create job directory: %w", err))
	}

	// Parse coverage data to extract metadata
	metadata, err := r.extractMetadata(jobID, coverageData)
	if err != nil {
		return errors.NewSystemError("extract_metadata", fmt.Errorf("failed to extract metadata: %w", err))
	}

	// Store coverage data
	dataPath := filepath.Join(jobDir, "coverage.dat")
	if err := r.writeFileAtomic(dataPath, coverageData); err != nil {
		return errors.NewSystemError("write_coverage_data", fmt.Errorf("failed to write coverage data: %w", err))
	}

	// Store metadata
	metadataPath := filepath.Join(jobDir, "metadata.json")
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return errors.NewSystemError("marshal_metadata", fmt.Errorf("failed to marshal metadata: %w", err))
	}

	if err := r.writeFileAtomic(metadataPath, metadataJSON); err != nil {
		return errors.NewSystemError("write_metadata", fmt.Errorf("failed to write metadata: %w", err))
	}

	r.logger.WithFields(logrus.Fields{
		"job_id": jobID,
		"size":   len(coverageData),
		"path":   jobDir,
	}).Debug("Stored coverage data")

	return nil
}

// Get retrieves coverage data by job ID
func (r *FilesystemCoverageRepository) Get(ctx context.Context, jobID string) (*CoverageReport, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_coverage", "job ID cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	jobDir := filepath.Join(r.basePath, jobID)

	// Check if job directory exists
	if _, err := os.Stat(jobDir); os.IsNotExist(err) {
		return nil, errors.NewNotFoundError("get_coverage", "coverage report").
			WithDetail("job_id", jobID)
	}

	// Read coverage data
	dataPath := filepath.Join(jobDir, "coverage.dat")
	data, err := os.ReadFile(dataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NewNotFoundError("get_coverage_data", "coverage data file").
				WithDetail("job_id", jobID)
		}
		return nil, errors.NewSystemError("read_coverage_data", fmt.Errorf("failed to read coverage data: %w", err))
	}

	// Read metadata
	metadata, err := r.readMetadata(jobDir)
	if err != nil {
		return nil, err
	}

	// Get file stats for timestamps
	dataInfo, err := os.Stat(dataPath)
	if err != nil {
		return nil, errors.NewSystemError("stat_coverage_file", fmt.Errorf("failed to stat coverage file: %w", err))
	}

	report := &CoverageReport{
		JobID:     jobID,
		Data:      data,
		Metadata:  *metadata,
		CreatedAt: metadata.CreatedAt,
		UpdatedAt: dataInfo.ModTime(),
	}

	r.logger.WithFields(logrus.Fields{
		"job_id": jobID,
		"size":   len(data),
	}).Debug("Retrieved coverage data")

	return report, nil
}

// List returns all coverage reports with optional prefix filtering
func (r *FilesystemCoverageRepository) List(ctx context.Context, prefix string) ([]*CoverageReport, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var reports []*CoverageReport

	err := filepath.Walk(r.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip if not a directory or if it's the base path
		if !info.IsDir() || path == r.basePath {
			return nil
		}

		// Get job ID from directory name
		jobID := filepath.Base(path)

		// Apply prefix filter if specified
		if prefix != "" && !strings.HasPrefix(jobID, prefix) {
			return filepath.SkipDir
		}

		// Check if this directory contains coverage data
		dataPath := filepath.Join(path, "coverage.dat")
		if _, err := os.Stat(dataPath); os.IsNotExist(err) {
			return filepath.SkipDir
		}

		// Try to read the coverage report
		report, err := r.Get(ctx, jobID)
		if err != nil {
			r.logger.WithError(err).WithField("job_id", jobID).Warn("Failed to read coverage report during list operation")
			return filepath.SkipDir
		}

		reports = append(reports, report)
		return filepath.SkipDir // Don't descend into subdirectories
	})

	if err != nil {
		return nil, errors.NewSystemError("list_coverage_reports", fmt.Errorf("failed to walk coverage directory: %w", err))
	}

	// Sort reports by creation time (newest first)
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].CreatedAt.After(reports[j].CreatedAt)
	})

	r.logger.WithFields(logrus.Fields{
		"count":  len(reports),
		"prefix": prefix,
	}).Debug("Listed coverage reports")

	return reports, nil
}

// Delete removes coverage data by job ID
func (r *FilesystemCoverageRepository) Delete(ctx context.Context, jobID string) error {
	if jobID == "" {
		return errors.NewValidationError("delete_coverage", "job ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	jobDir := filepath.Join(r.basePath, jobID)

	// Check if job directory exists
	if _, err := os.Stat(jobDir); os.IsNotExist(err) {
		return errors.NewNotFoundError("delete_coverage", "coverage report").
			WithDetail("job_id", jobID)
	}

	// Remove the entire job directory
	if err := os.RemoveAll(jobDir); err != nil {
		return errors.NewSystemError("remove_job_directory", fmt.Errorf("failed to remove job directory: %w", err))
	}

	r.logger.WithField("job_id", jobID).Debug("Deleted coverage data")
	return nil
}

// GetMetadata retrieves metadata for a coverage report
func (r *FilesystemCoverageRepository) GetMetadata(ctx context.Context, jobID string) (*CoverageMetadata, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_coverage_metadata", "job ID cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	jobDir := filepath.Join(r.basePath, jobID)

	// Check if job directory exists
	if _, err := os.Stat(jobDir); os.IsNotExist(err) {
		return nil, errors.NewNotFoundError("get_coverage_metadata", "coverage report").
			WithDetail("job_id", jobID)
	}

	return r.readMetadata(jobDir)
}

// Helper methods

// writeFileAtomic writes data to a file atomically using a temporary file
func (r *FilesystemCoverageRepository) writeFileAtomic(filePath string, data []byte) error {
	tempPath := filePath + ".tmp"

	// Write to temporary file
	if err := os.WriteFile(tempPath, data, r.fileMode); err != nil {
		return err
	}

	// Ensure data is flushed to disk
	file, err := os.Open(tempPath)
	if err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return err
	}
	_ = file.Close()

	// Atomic rename
	if err := os.Rename(tempPath, filePath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	return nil
}

// readMetadata reads and parses metadata from a job directory
func (r *FilesystemCoverageRepository) readMetadata(jobDir string) (*CoverageMetadata, error) {
	metadataPath := filepath.Join(jobDir, "metadata.json")

	metadataJSON, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NewNotFoundError("read_metadata", "metadata file")
		}
		return nil, errors.NewSystemError("read_metadata_file", fmt.Errorf("failed to read metadata file: %w", err))
	}

	var metadata CoverageMetadata
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return nil, errors.NewSystemError("unmarshal_metadata", fmt.Errorf("failed to unmarshal metadata: %w", err))
	}

	return &metadata, nil
}

// extractMetadata extracts metadata from coverage data
func (r *FilesystemCoverageRepository) extractMetadata(jobID string, data []byte) (*CoverageMetadata, error) {
	now := time.Now()

	// Basic metadata extraction - in a real implementation, this would parse
	// the actual coverage format (e.g., LLVM profdata, gcov, etc.)
	metadata := &CoverageMetadata{
		JobID:           jobID,
		FuzzerType:      "unknown", // Would be determined from coverage data format
		TotalBlocks:     0,         // Would be parsed from coverage data
		CoveredBlocks:   0,         // Would be parsed from coverage data
		CoveragePercent: 0.0,       // Would be calculated
		Size:            int64(len(data)),
		CreatedAt:       now,
		UpdatedAt:       now,
		CustomMetadata:  make(map[string]string),
	}

	// Try to detect fuzzer type and extract basic metrics from coverage data
	if err := r.parseCoverageData(data, metadata); err != nil {
		r.logger.WithError(err).Warn("Failed to parse coverage data, using basic metadata")
	}

	return metadata, nil
}

// parseCoverageData attempts to parse coverage data and extract metrics
func (r *FilesystemCoverageRepository) parseCoverageData(data []byte, metadata *CoverageMetadata) error {
	// This would implement actual coverage data parsing based on format
	// For now, we'll do basic analysis

	// Detect format based on data patterns
	dataStr := string(data)

	if strings.Contains(dataStr, "LLVM") || strings.Contains(dataStr, "profdata") {
		metadata.FuzzerType = "libfuzzer"
	} else if strings.Contains(dataStr, "AFL") || strings.Contains(dataStr, "afl-") {
		metadata.FuzzerType = "aflplusplus"
	} else if strings.Contains(dataStr, "honggfuzz") || strings.Contains(dataStr, "hfuzz") {
		metadata.FuzzerType = "honggfuzz"
	}

	// For demonstration, set some mock values
	// In a real implementation, these would be parsed from the actual coverage format
	if len(data) > 100 {
		metadata.TotalBlocks = int64(len(data) / 10)   // Mock calculation
		metadata.CoveredBlocks = int64(len(data) / 20) // Mock calculation
		if metadata.TotalBlocks > 0 {
			metadata.CoveragePercent = float64(metadata.CoveredBlocks) / float64(metadata.TotalBlocks) * 100.0
		}
	}

	return nil
}
