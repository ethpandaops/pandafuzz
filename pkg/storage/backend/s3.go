package backend

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/config"
	"github.com/ethpandaops/pandafuzz/pkg/monitoring"
)

// S3Backend implements StorageBackend using S3-compatible storage.
type S3Backend struct {
	client            *minio.Client
	config            config.S3Config
	buckets           map[string]string // corpus/quarantine/backup bucket names
	logger            logrus.FieldLogger
	metrics           *monitoring.StorageMetrics
	uploadPartSize    int64
	uploadConcurrency int
}

// Ensure S3Backend implements StorageBackend interface
var _ StorageBackend = (*S3Backend)(nil)

// NewS3Backend creates a new S3 storage backend
func NewS3Backend(cfg config.S3Config, logger logrus.FieldLogger, metrics *monitoring.StorageMetrics) (*S3Backend, error) {
	// Initialize MinIO client (works with S3, MinIO, GCS)
	var creds *credentials.Credentials

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		creds = credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken)
	} else {
		// Use IAM role or default credentials
		creds = credentials.NewIAM("")
	}

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  creds,
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	// Set default values
	partSize := cfg.PartSize
	if partSize == 0 {
		partSize = 64 * 1024 * 1024 // 64MB default
	}

	concurrency := cfg.Concurrency
	if concurrency == 0 {
		concurrency = 4
	}

	s3 := &S3Backend{
		client: client,
		config: cfg,
		buckets: map[string]string{
			"corpus":     cfg.CorpusBucket,
			"quarantine": cfg.QuarantineBucket,
			"backup":     cfg.BackupBucket,
		},
		logger:            logger.WithField("backend", "s3"),
		metrics:           metrics,
		uploadPartSize:    partSize,
		uploadConcurrency: concurrency,
	}

	// Verify buckets exist
	ctx := context.Background()
	for bucketType, bucketName := range s3.buckets {
		if bucketName == "" {
			continue
		}

		exists, err := client.BucketExists(ctx, bucketName)
		if err != nil {
			return nil, fmt.Errorf("failed to check %s bucket: %w", bucketType, err)
		}

		if !exists {
			// Attempt to create bucket
			err = client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{
				Region: cfg.Region,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create %s bucket: %w", bucketType, err)
			}
			logger.WithField("bucket", bucketName).Info("Created S3 bucket")
		}
	}

	return s3, nil
}

// Store uploads an object to S3.
// It supports multipart uploads for large files based on configured part size.
func (s *S3Backend) Store(ctx context.Context, key string, reader io.Reader, size int64) error {
	startTime := time.Now()
	if s.metrics != nil {
		defer s.metrics.TrackActiveOperation("s3", "store")()
	}

	logger := monitoring.WithRequestID(ctx, s.logger)
	bucket := s.buckets["corpus"]

	logger = logger.WithFields(logrus.Fields{
		"key":           key,
		"bucket":        bucket,
		"expected_size": size,
		"multipart":     size > s.uploadPartSize,
	})

	logger.Debug("Starting S3 store operation")

	opts := minio.PutObjectOptions{
		ContentType: "application/octet-stream",
		UserMetadata: map[string]string{
			"uploaded-by": "pandafuzz",
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
		},
	}

	// Use multipart upload for large files
	if size > s.uploadPartSize {
		opts.PartSize = uint64(s.uploadPartSize)
		opts.NumThreads = uint(s.uploadConcurrency)
		logger.WithFields(logrus.Fields{
			"part_size":   s.uploadPartSize,
			"concurrency": s.uploadConcurrency,
		}).Debug("Using multipart upload")
	}

	info, err := s.client.PutObject(ctx, bucket, key, reader, size, opts)
	if err != nil {
		err = fmt.Errorf("failed to store object: %w", err)
		if s.metrics != nil {
			s.metrics.RecordOperation("s3", "store", bucket, size, err, time.Since(startTime))
		}
		logger.WithError(err).Error("Failed to store object in S3")
		return err
	}

	// Record success metrics
	if s.metrics != nil {
		s.metrics.RecordOperation("s3", "store", bucket, info.Size, nil, time.Since(startTime))
	}

	logger.WithFields(logrus.Fields{
		"size":     info.Size,
		"etag":     info.ETag,
		"duration": time.Since(startTime).Seconds(),
	}).Info("Successfully stored object in S3")

	return nil
}

// Retrieve downloads an object from S3.
// The caller must close the returned reader when done.
func (s *S3Backend) Retrieve(ctx context.Context, key string) (io.ReadCloser, error) {
	bucket := s.buckets["corpus"]

	object, err := s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve object: %w", err)
	}

	// Verify object exists by checking stat
	_, err = object.Stat()
	if err != nil {
		object.Close()
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, fmt.Errorf("object not found: %s", key)
		}
		return nil, fmt.Errorf("failed to stat object: %w", err)
	}

	return object, nil
}

// Delete removes an object from S3.
func (s *S3Backend) Delete(ctx context.Context, key string) error {
	bucket := s.buckets["corpus"]

	err := s.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	s.logger.WithField("key", key).Debug("Deleted object from S3")
	return nil
}

// Exists checks if an object exists in S3.
func (s *S3Backend) Exists(ctx context.Context, key string) (bool, error) {
	bucket := s.buckets["corpus"]

	_, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check object existence: %w", err)
	}

	return true, nil
}

// List returns objects with the given prefix.
// It performs a recursive listing to include all objects under the prefix.
func (s *S3Backend) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	bucket := s.buckets["corpus"]

	var objects []ObjectInfo

	opts := minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}

	for object := range s.client.ListObjects(ctx, bucket, opts) {
		if object.Err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", object.Err)
		}

		objects = append(objects, ObjectInfo{
			Key:          object.Key,
			Size:         object.Size,
			LastModified: object.LastModified,
			ETag:         object.ETag,
		})
	}

	return objects, nil
}

// DeleteMany removes multiple objects from S3.
// It uses batch deletion for efficiency.
func (s *S3Backend) DeleteMany(ctx context.Context, keys []string) error {
	bucket := s.buckets["corpus"]

	objectsCh := make(chan minio.ObjectInfo)
	go func() {
		defer close(objectsCh)
		for _, key := range keys {
			objectsCh <- minio.ObjectInfo{Key: key}
		}
	}()

	errorCh := s.client.RemoveObjects(ctx, bucket, objectsCh, minio.RemoveObjectsOptions{})

	var errs []error
	for e := range errorCh {
		if e.Err != nil {
			errs = append(errs, fmt.Errorf("failed to delete %s: %w", e.ObjectName, e.Err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to delete %d objects: %v", len(errs), errs)
	}

	s.logger.WithField("count", len(keys)).Debug("Deleted multiple objects from S3")
	return nil
}

// GetMetadata retrieves object metadata from S3.
func (s *S3Backend) GetMetadata(ctx context.Context, key string) (*ObjectMetadata, error) {
	bucket := s.buckets["corpus"]

	stat, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, fmt.Errorf("object not found: %s", key)
		}
		return nil, fmt.Errorf("failed to get object metadata: %w", err)
	}

	return &ObjectMetadata{
		ContentType:  stat.ContentType,
		Size:         stat.Size,
		ETag:         stat.ETag,
		LastModified: stat.LastModified,
		UserMetadata: stat.UserMetadata,
	}, nil
}

// SetMetadata updates object metadata by copying the object.
// S3 requires copying the object to update metadata.
func (s *S3Backend) SetMetadata(ctx context.Context, key string, metadata map[string]string) error {
	bucket := s.buckets["corpus"]

	// Get current object info
	stat, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to get object info: %w", err)
	}

	// Merge with existing metadata
	userMeta := make(map[string]string)
	for k, v := range stat.UserMetadata {
		userMeta[k] = v
	}
	for k, v := range metadata {
		userMeta[k] = v
	}

	// Copy object with new metadata
	src := minio.CopySrcOptions{
		Bucket: bucket,
		Object: key,
	}

	dst := minio.CopyDestOptions{
		Bucket:       bucket,
		Object:       key,
		UserMetadata: userMeta,
	}

	_, err = s.client.CopyObject(ctx, dst, src)
	if err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	return nil
}

// GetPresignedURL generates a presigned URL for downloading an object.
func (s *S3Backend) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	startTime := time.Now()
	logger := monitoring.WithRequestID(ctx, s.logger)
	bucket := s.buckets["corpus"]

	logger = logger.WithFields(logrus.Fields{
		"key":    key,
		"bucket": bucket,
		"expiry": expiry.String(),
	})

	logger.Debug("Generating presigned GET URL for S3")

	url, err := s.client.PresignedGetObject(ctx, bucket, key, expiry, nil)
	if err != nil {
		err = fmt.Errorf("failed to generate presigned URL: %w", err)
		if s.metrics != nil {
			s.metrics.RecordPresignedURL("s3", "get", bucket, err, time.Since(startTime))
		}
		logger.WithError(err).Error("Failed to generate presigned GET URL")
		return "", err
	}

	// Record success metrics
	if s.metrics != nil {
		s.metrics.RecordPresignedURL("s3", "get", bucket, nil, time.Since(startTime))
	}

	urlStr := url.String()
	logger.WithFields(logrus.Fields{
		"url":      urlStr[:20] + "...", // Log only prefix for security
		"duration": time.Since(startTime).Seconds(),
	}).Debug("Generated presigned GET URL")

	return urlStr, nil
}

// PutPresignedURL generates a presigned URL for uploading an object.
func (s *S3Backend) PutPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	startTime := time.Now()
	logger := monitoring.WithRequestID(ctx, s.logger)
	bucket := s.buckets["corpus"]

	logger = logger.WithFields(logrus.Fields{
		"key":    key,
		"bucket": bucket,
		"expiry": expiry.String(),
	})

	logger.Debug("Generating presigned PUT URL for S3")

	url, err := s.client.PresignedPutObject(ctx, bucket, key, expiry)
	if err != nil {
		err = fmt.Errorf("failed to generate presigned PUT URL: %w", err)
		if s.metrics != nil {
			s.metrics.RecordPresignedURL("s3", "put", bucket, err, time.Since(startTime))
		}
		logger.WithError(err).Error("Failed to generate presigned PUT URL")
		return "", err
	}

	// Record success metrics
	if s.metrics != nil {
		s.metrics.RecordPresignedURL("s3", "put", bucket, nil, time.Since(startTime))
	}

	urlStr := url.String()
	logger.WithFields(logrus.Fields{
		"url":      urlStr[:20] + "...", // Log only prefix for security
		"duration": time.Since(startTime).Seconds(),
	}).Debug("Generated presigned PUT URL")

	return urlStr, nil
}

// HealthCheck verifies S3 connectivity by listing buckets.
func (s *S3Backend) HealthCheck(ctx context.Context) error {
	startTime := time.Now()
	logger := monitoring.WithRequestID(ctx, s.logger)

	logger.Debug("Starting S3 health check")

	// Try to list buckets as a health check
	_, err := s.client.ListBuckets(ctx)
	if err != nil {
		err = fmt.Errorf("S3 health check failed: %w", err)
		if s.metrics != nil {
			s.metrics.RecordHealthCheck("s3", err, time.Since(startTime))
		}
		logger.WithError(err).Error("S3 health check failed")
		return err
	}

	// Record success
	if s.metrics != nil {
		s.metrics.RecordHealthCheck("s3", nil, time.Since(startTime))
	}

	logger.WithField("duration", time.Since(startTime).Seconds()).Info("S3 health check passed")
	return nil
}

// GetBucket returns the bucket name for a given type (corpus/quarantine/backup).
func (s *S3Backend) GetBucket(bucketType string) string {
	return s.buckets[bucketType]
}

// MoveToQuarantine moves a file from corpus to quarantine bucket.
// It copies the object and then deletes it from the source bucket.
func (s *S3Backend) MoveToQuarantine(ctx context.Context, key string) error {
	corpusBucket := s.buckets["corpus"]
	quarantineBucket := s.buckets["quarantine"]

	if quarantineBucket == "" {
		return fmt.Errorf("quarantine bucket not configured")
	}

	// Copy to quarantine
	src := minio.CopySrcOptions{
		Bucket: corpusBucket,
		Object: key,
	}

	dst := minio.CopyDestOptions{
		Bucket: quarantineBucket,
		Object: key,
	}

	_, err := s.client.CopyObject(ctx, dst, src)
	if err != nil {
		return fmt.Errorf("failed to copy to quarantine: %w", err)
	}

	// Delete from corpus
	err = s.client.RemoveObject(ctx, corpusBucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		// Try to clean up the quarantine copy
		_ = s.client.RemoveObject(ctx, quarantineBucket, key, minio.RemoveObjectOptions{})
		return fmt.Errorf("failed to delete from corpus: %w", err)
	}

	s.logger.WithField("key", key).Info("Moved object to quarantine")
	return nil
}

// RestoreFromQuarantine moves a file from quarantine back to corpus.
// It copies the object and then deletes it from the source bucket.
func (s *S3Backend) RestoreFromQuarantine(ctx context.Context, key string) error {
	corpusBucket := s.buckets["corpus"]
	quarantineBucket := s.buckets["quarantine"]

	if quarantineBucket == "" {
		return fmt.Errorf("quarantine bucket not configured")
	}

	// Copy back to corpus
	src := minio.CopySrcOptions{
		Bucket: quarantineBucket,
		Object: key,
	}

	dst := minio.CopyDestOptions{
		Bucket: corpusBucket,
		Object: key,
	}

	_, err := s.client.CopyObject(ctx, dst, src)
	if err != nil {
		return fmt.Errorf("failed to restore from quarantine: %w", err)
	}

	// Delete from quarantine
	err = s.client.RemoveObject(ctx, quarantineBucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		// Try to clean up the corpus copy
		_ = s.client.RemoveObject(ctx, corpusBucket, key, minio.RemoveObjectOptions{})
		return fmt.Errorf("failed to delete from quarantine: %w", err)
	}

	s.logger.WithField("key", key).Info("Restored object from quarantine")
	return nil
}

// BackupObject copies an object to the backup bucket with a timestamp suffix.
func (s *S3Backend) BackupObject(ctx context.Context, key string) error {
	corpusBucket := s.buckets["corpus"]
	backupBucket := s.buckets["backup"]

	if backupBucket == "" {
		return fmt.Errorf("backup bucket not configured")
	}

	// Create backup key with timestamp
	backupKey := fmt.Sprintf("%s.%d", key, time.Now().Unix())

	src := minio.CopySrcOptions{
		Bucket: corpusBucket,
		Object: key,
	}

	dst := minio.CopyDestOptions{
		Bucket: backupBucket,
		Object: backupKey,
	}

	_, err := s.client.CopyObject(ctx, dst, src)
	if err != nil {
		return fmt.Errorf("failed to backup object: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"key":       key,
		"backupKey": backupKey,
	}).Debug("Backed up object")

	return nil
}

// StoreWithReader is a helper that accepts []byte data for storage.
// It converts the byte slice to an io.Reader and calls Store.
func (s *S3Backend) StoreWithReader(ctx context.Context, key string, data []byte) error {
	reader := bytes.NewReader(data)
	return s.Store(ctx, key, reader, int64(len(data)))
}
