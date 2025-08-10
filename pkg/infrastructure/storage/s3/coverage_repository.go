package s3

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/errors"
)

// CoverageRepository interface defines methods for coverage data storage operations
type CoverageRepository interface {
	// Store saves coverage data to S3
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

// S3CoverageRepository implements CoverageRepository using S3-compatible storage
type S3CoverageRepository struct {
	bucket string
	prefix string
	client *minio.Client
	logger logrus.FieldLogger
	mu     sync.RWMutex
}

// S3Config contains S3-specific configuration for coverage repository
type S3Config struct {
	Bucket          string
	Region          string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	UsePathStyle    bool
	DisableSSL      bool
	Prefix          string // Optional prefix for all coverage objects
}

// Ensure interface compliance
var _ CoverageRepository = (*S3CoverageRepository)(nil)

// NewS3CoverageRepository creates a new S3-based coverage repository
func NewS3CoverageRepository(config S3Config, logger logrus.FieldLogger) (*S3CoverageRepository, error) {
	if config.Bucket == "" {
		return nil, errors.NewValidationError("new_s3_coverage_repository", "bucket name cannot be empty")
	}

	if logger == nil {
		logger = logrus.New()
	}

	// Set default endpoint if not provided
	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}

	// Create credentials
	var creds *credentials.Credentials
	if config.AccessKeyID != "" && config.SecretAccessKey != "" {
		creds = credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, config.SessionToken)
	} else {
		// Use IAM role or default credentials
		creds = credentials.NewIAM("")
	}

	// Create MinIO client
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  creds,
		Secure: !config.DisableSSL,
		Region: config.Region,
	})
	if err != nil {
		return nil, errors.NewSystemError("create_s3_client", fmt.Errorf("failed to create S3 client: %w", err))
	}

	// Set path style if needed
	if config.UsePathStyle {
		client.SetAppInfo("pandafuzz", "1.0.0")
	}

	// Set default prefix
	prefix := "coverage/"
	if config.Prefix != "" {
		prefix = strings.TrimSuffix(config.Prefix, "/") + "/coverage/"
	}

	repo := &S3CoverageRepository{
		bucket: config.Bucket,
		prefix: prefix,
		client: client,
		logger: logger.WithField("component", "s3_coverage_repository"),
	}

	// Test bucket access
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := repo.verifyBucketAccess(ctx); err != nil {
		return nil, errors.NewSystemError("verify_bucket_access", fmt.Errorf("failed to verify bucket access: %w", err))
	}

	repo.logger.WithFields(logrus.Fields{
		"bucket":   config.Bucket,
		"region":   config.Region,
		"endpoint": config.Endpoint,
		"prefix":   prefix,
	}).Info("Initialized S3 coverage repository")

	return repo, nil
}

// Store saves coverage data to S3
func (r *S3CoverageRepository) Store(ctx context.Context, jobID string, coverageData []byte) error {
	if jobID == "" {
		return errors.NewValidationError("store_coverage", "job ID cannot be empty")
	}

	if len(coverageData) == 0 {
		return errors.NewValidationError("store_coverage", "coverage data cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Extract metadata from coverage data
	metadata, err := r.extractMetadata(jobID, coverageData)
	if err != nil {
		return errors.NewSystemError("extract_metadata", fmt.Errorf("failed to extract metadata: %w", err))
	}

	// Store coverage data
	dataKey := r.getDataKey(jobID)
	dataReader := bytes.NewReader(coverageData)

	_, err = r.client.PutObject(ctx, r.bucket, dataKey, dataReader, int64(len(coverageData)),
		minio.PutObjectOptions{
			ContentType: "application/octet-stream",
			UserMetadata: map[string]string{
				"job-id":      jobID,
				"fuzzer-type": metadata.FuzzerType,
				"created-at":  metadata.CreatedAt.Format(time.RFC3339),
			},
		})
	if err != nil {
		return errors.NewSystemError("store_coverage_data", fmt.Errorf("failed to store coverage data: %w", err))
	}

	// Store metadata
	metadataKey := r.getMetadataKey(jobID)
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return errors.NewSystemError("marshal_metadata", fmt.Errorf("failed to marshal metadata: %w", err))
	}

	metadataReader := bytes.NewReader(metadataJSON)
	_, err = r.client.PutObject(ctx, r.bucket, metadataKey, metadataReader, int64(len(metadataJSON)),
		minio.PutObjectOptions{
			ContentType: "application/json",
			UserMetadata: map[string]string{
				"job-id":     jobID,
				"created-at": metadata.CreatedAt.Format(time.RFC3339),
			},
		})
	if err != nil {
		return errors.NewSystemError("store_metadata", fmt.Errorf("failed to store metadata: %w", err))
	}

	r.logger.WithFields(logrus.Fields{
		"job_id":   jobID,
		"size":     len(coverageData),
		"data_key": dataKey,
	}).Debug("Stored coverage data to S3")

	return nil
}

// Get retrieves coverage data by job ID
func (r *S3CoverageRepository) Get(ctx context.Context, jobID string) (*CoverageReport, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_coverage", "job ID cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get coverage data
	dataKey := r.getDataKey(jobID)
	dataObj, err := r.client.GetObject(ctx, r.bucket, dataKey, minio.GetObjectOptions{})
	if err != nil {
		if r.isNotFoundError(err) {
			return nil, errors.NewNotFoundError("get_coverage_data", "coverage report").
				WithDetail("job_id", jobID)
		}
		return nil, errors.NewSystemError("get_coverage_data", fmt.Errorf("failed to get coverage data: %w", err))
	}
	defer dataObj.Close()

	data, err := io.ReadAll(dataObj)
	if err != nil {
		return nil, errors.NewSystemError("read_coverage_data", fmt.Errorf("failed to read coverage data: %w", err))
	}

	// Get data object info for timestamps
	dataInfo, err := r.client.StatObject(ctx, r.bucket, dataKey, minio.StatObjectOptions{})
	if err != nil {
		return nil, errors.NewSystemError("stat_coverage_data", fmt.Errorf("failed to get coverage data info: %w", err))
	}

	// Get metadata
	metadata, err := r.getMetadataFromS3(ctx, jobID)
	if err != nil {
		return nil, err
	}

	report := &CoverageReport{
		JobID:     jobID,
		Data:      data,
		Metadata:  *metadata,
		CreatedAt: metadata.CreatedAt,
		UpdatedAt: dataInfo.LastModified,
	}

	r.logger.WithFields(logrus.Fields{
		"job_id": jobID,
		"size":   len(data),
	}).Debug("Retrieved coverage data from S3")

	return report, nil
}

// List returns all coverage reports with optional prefix filtering
func (r *S3CoverageRepository) List(ctx context.Context, prefix string) ([]*CoverageReport, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Build S3 prefix for listing
	s3Prefix := r.prefix
	if prefix != "" {
		s3Prefix = r.prefix + prefix
	}

	var reports []*CoverageReport
	seenJobIDs := make(map[string]bool)

	// List objects with the prefix
	objectCh := r.client.ListObjects(ctx, r.bucket, minio.ListObjectsOptions{
		Prefix:    s3Prefix,
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			return nil, errors.NewSystemError("list_s3_objects", fmt.Errorf("failed to list S3 objects: %w", object.Err))
		}

		// Extract job ID from object key
		jobID, err := r.extractJobIDFromKey(object.Key)
		if err != nil {
			continue // Skip invalid keys
		}

		// Skip if we've already processed this job ID
		if seenJobIDs[jobID] {
			continue
		}
		seenJobIDs[jobID] = true

		// Try to get the full coverage report
		report, err := r.Get(ctx, jobID)
		if err != nil {
			r.logger.WithError(err).WithField("job_id", jobID).Warn("Failed to get coverage report during list operation")
			continue
		}

		reports = append(reports, report)
	}

	// Sort reports by creation time (newest first)
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].CreatedAt.After(reports[j].CreatedAt)
	})

	r.logger.WithFields(logrus.Fields{
		"count":  len(reports),
		"prefix": prefix,
	}).Debug("Listed coverage reports from S3")

	return reports, nil
}

// Delete removes coverage data by job ID
func (r *S3CoverageRepository) Delete(ctx context.Context, jobID string) error {
	if jobID == "" {
		return errors.NewValidationError("delete_coverage", "job ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Delete coverage data
	dataKey := r.getDataKey(jobID)
	if err := r.client.RemoveObject(ctx, r.bucket, dataKey, minio.RemoveObjectOptions{}); err != nil {
		if !r.isNotFoundError(err) {
			return errors.NewSystemError("delete_coverage_data", fmt.Errorf("failed to delete coverage data: %w", err))
		}
	}

	// Delete metadata
	metadataKey := r.getMetadataKey(jobID)
	if err := r.client.RemoveObject(ctx, r.bucket, metadataKey, minio.RemoveObjectOptions{}); err != nil {
		if !r.isNotFoundError(err) {
			return errors.NewSystemError("delete_metadata", fmt.Errorf("failed to delete metadata: %w", err))
		}
	}

	r.logger.WithField("job_id", jobID).Debug("Deleted coverage data from S3")
	return nil
}

// GetMetadata retrieves metadata for a coverage report
func (r *S3CoverageRepository) GetMetadata(ctx context.Context, jobID string) (*CoverageMetadata, error) {
	if jobID == "" {
		return nil, errors.NewValidationError("get_coverage_metadata", "job ID cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getMetadataFromS3(ctx, jobID)
}

// Helper methods

// getDataKey returns the S3 key for coverage data
func (r *S3CoverageRepository) getDataKey(jobID string) string {
	return fmt.Sprintf("%s%s/coverage.dat", r.prefix, jobID)
}

// getMetadataKey returns the S3 key for metadata
func (r *S3CoverageRepository) getMetadataKey(jobID string) string {
	return fmt.Sprintf("%s%s/metadata.json", r.prefix, jobID)
}

// extractJobIDFromKey extracts job ID from an S3 object key
func (r *S3CoverageRepository) extractJobIDFromKey(key string) (string, error) {
	// Remove prefix
	if !strings.HasPrefix(key, r.prefix) {
		return "", fmt.Errorf("key does not have expected prefix")
	}

	keyWithoutPrefix := strings.TrimPrefix(key, r.prefix)
	parts := strings.Split(keyWithoutPrefix, "/")

	if len(parts) < 2 {
		return "", fmt.Errorf("invalid key format")
	}

	return parts[0], nil
}

// getMetadataFromS3 retrieves and parses metadata from S3
func (r *S3CoverageRepository) getMetadataFromS3(ctx context.Context, jobID string) (*CoverageMetadata, error) {
	metadataKey := r.getMetadataKey(jobID)

	metadataObj, err := r.client.GetObject(ctx, r.bucket, metadataKey, minio.GetObjectOptions{})
	if err != nil {
		if r.isNotFoundError(err) {
			return nil, errors.NewNotFoundError("get_metadata", "metadata").
				WithDetail("job_id", jobID)
		}
		return nil, errors.NewSystemError("get_metadata_object", fmt.Errorf("failed to get metadata object: %w", err))
	}
	defer metadataObj.Close()

	metadataJSON, err := io.ReadAll(metadataObj)
	if err != nil {
		return nil, errors.NewSystemError("read_metadata", fmt.Errorf("failed to read metadata: %w", err))
	}

	var metadata CoverageMetadata
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return nil, errors.NewSystemError("unmarshal_metadata", fmt.Errorf("failed to unmarshal metadata: %w", err))
	}

	return &metadata, nil
}

// extractMetadata extracts metadata from coverage data
func (r *S3CoverageRepository) extractMetadata(jobID string, data []byte) (*CoverageMetadata, error) {
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
func (r *S3CoverageRepository) parseCoverageData(data []byte, metadata *CoverageMetadata) error {
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

// verifyBucketAccess checks if the bucket is accessible
func (r *S3CoverageRepository) verifyBucketAccess(ctx context.Context) error {
	exists, err := r.client.BucketExists(ctx, r.bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		// Try to create the bucket
		err = r.client.MakeBucket(ctx, r.bucket, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("bucket does not exist and could not be created: %w", err)
		}

		r.logger.WithField("bucket", r.bucket).Info("Created S3 bucket")
	}

	return nil
}

// isNotFoundError checks if an error indicates that an object was not found
func (r *S3CoverageRepository) isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	// Check for MinIO error response
	errResp := minio.ToErrorResponse(err)
	return errResp.Code == "NoSuchKey" ||
		errResp.Code == "NoSuchBucket" ||
		strings.Contains(err.Error(), "key does not exist")
}
