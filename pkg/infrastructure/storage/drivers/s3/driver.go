package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/abstraction"
)

var (
	// ErrInvalidConfig is returned when configuration is invalid
	ErrInvalidConfig = errors.New("invalid configuration")
)

// Config contains S3-specific configuration
type Config struct {
	// Bucket is the S3 bucket name
	Bucket string

	// Region is the AWS region
	Region string

	// Endpoint is the S3 endpoint URL (for S3-compatible services)
	Endpoint string

	// AccessKeyID is the AWS access key ID
	AccessKeyID string

	// SecretAccessKey is the AWS secret access key
	SecretAccessKey string

	// SessionToken is the optional AWS session token
	SessionToken string

	// UsePathStyle forces path-style URLs (for S3-compatible services)
	UsePathStyle bool

	// DisableSSL disables SSL/TLS (for local testing only)
	DisableSSL bool

	// PresignedURLExpiry is the duration for which presigned URLs are valid
	PresignedURLExpiry time.Duration

	// Common configuration options
	abstraction.Config
}

// DefaultConfig returns default S3 configuration
func DefaultConfig() Config {
	return Config{
		Region:             "us-east-1",
		PresignedURLExpiry: 1 * time.Hour,
		Config:             abstraction.DefaultConfig(),
	}
}

// driver implements the abstraction.Driver interface using S3-compatible storage
type driver struct {
	config Config
	client *minio.Client
	logger logrus.FieldLogger

	// mu protects concurrent operations
	mu sync.RWMutex
}

// Ensure driver implements abstraction.Driver interface
var _ abstraction.Driver = (*driver)(nil)

// NewDriver creates a new S3 storage driver
func NewDriver(cfg Config, logger logrus.FieldLogger) (abstraction.Driver, error) {
	// Validate configuration
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("bucket name cannot be empty")
	}

	// Set default endpoint if not provided
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}

	// Create credentials
	var creds *credentials.Credentials
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		creds = credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken)
	} else {
		// Use IAM role or default credentials
		creds = credentials.NewIAM("")
	}

	// Create MinIO client
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  creds,
		Secure: !cfg.DisableSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	// Set path style if needed
	if cfg.UsePathStyle {
		client.SetAppInfo("pandafuzz", "1.0.0")
	}

	d := &driver{
		config: cfg,
		client: client,
		logger: logger.WithField("driver", "s3"),
	}

	// Test bucket access
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := d.verifyBucketAccess(ctx); err != nil {
		return nil, fmt.Errorf("failed to verify bucket access: %w", err)
	}

	d.logger.WithFields(logrus.Fields{
		"bucket":   cfg.Bucket,
		"region":   cfg.Region,
		"endpoint": cfg.Endpoint,
	}).Info("Initialized S3 storage driver")

	return d, nil
}

// Put stores data with the given key
func (d *driver) Put(ctx context.Context, key string, data []byte) error {
	// Validate key
	if err := d.validateKey(key); err != nil {
		return err
	}

	// Check data size
	if int64(len(data)) > d.config.MaxValueSize {
		return fmt.Errorf("data size %d exceeds maximum allowed size %d", len(data), d.config.MaxValueSize)
	}

	d.logger.WithFields(logrus.Fields{
		"key":    key,
		"bucket": d.config.Bucket,
		"size":   len(data),
	}).Debug("Storing data")

	_, err := d.client.PutObject(ctx, d.config.Bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to put object: %w", err)
	}

	return nil
}

// Get retrieves data for the given key
func (d *driver) Get(ctx context.Context, key string) ([]byte, error) {
	// Validate key
	if err := d.validateKey(key); err != nil {
		return nil, err
	}

	d.logger.WithFields(logrus.Fields{
		"key":    key,
		"bucket": d.config.Bucket,
	}).Debug("Retrieving data")

	object, err := d.client.GetObject(ctx, d.config.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		if isNotFoundError(err) {
			return nil, abstraction.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	defer object.Close()

	// Read the data
	data, err := io.ReadAll(object)
	if err != nil {
		return nil, fmt.Errorf("failed to read object body: %w", err)
	}

	return data, nil
}

// Delete removes data for the given key
func (d *driver) Delete(ctx context.Context, key string) error {
	// Validate key
	if err := d.validateKey(key); err != nil {
		return err
	}

	d.logger.WithFields(logrus.Fields{
		"key":    key,
		"bucket": d.config.Bucket,
	}).Debug("Deleting data")

	err := d.client.RemoveObject(ctx, d.config.Bucket, key, minio.RemoveObjectOptions{})
	if err != nil && !isNotFoundError(err) {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	return nil
}

// List returns all keys with the given prefix
func (d *driver) List(ctx context.Context, prefix string) ([]string, error) {
	d.logger.WithFields(logrus.Fields{
		"prefix": prefix,
		"bucket": d.config.Bucket,
	}).Debug("Listing keys")

	var keys []string

	// Create done channel for context
	doneCh := make(chan struct{})
	defer close(doneCh)

	// List objects
	objectCh := d.client.ListObjects(ctx, d.config.Bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", object.Err)
		}
		keys = append(keys, object.Key)
	}

	return keys, nil
}

// Exists checks if a key exists in storage
func (d *driver) Exists(ctx context.Context, key string) (bool, error) {
	// Validate key
	if err := d.validateKey(key); err != nil {
		return false, err
	}

	_, err := d.client.StatObject(ctx, d.config.Bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check object existence: %w", err)
	}

	return true, nil
}

// GetURL returns a presigned URL for downloading the object
func (d *driver) GetURL(ctx context.Context, key string) (string, error) {
	// Validate key
	if err := d.validateKey(key); err != nil {
		return "", err
	}

	// Check if object exists
	exists, err := d.Exists(ctx, key)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", abstraction.ErrNotFound
	}

	// Generate presigned URL
	url, err := d.client.PresignedGetObject(ctx, d.config.Bucket, key, d.config.PresignedURLExpiry, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create presigned URL: %w", err)
	}

	return url.String(), nil
}

// validateKey checks if a key is valid
func (d *driver) validateKey(key string) error {
	if key == "" {
		return abstraction.ErrInvalidKey
	}

	if len(key) > d.config.MaxKeyLength {
		return fmt.Errorf("key length %d exceeds maximum allowed length %d", len(key), d.config.MaxKeyLength)
	}

	// S3 key validation
	if strings.HasPrefix(key, "/") {
		return fmt.Errorf("%w: key cannot start with '/'", abstraction.ErrInvalidKey)
	}

	if strings.Contains(key, "//") {
		return fmt.Errorf("%w: key cannot contain consecutive slashes", abstraction.ErrInvalidKey)
	}

	return nil
}

// verifyBucketAccess checks if the bucket is accessible
func (d *driver) verifyBucketAccess(ctx context.Context) error {
	exists, err := d.client.BucketExists(ctx, d.config.Bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		// Try to create the bucket
		err = d.client.MakeBucket(ctx, d.config.Bucket, minio.MakeBucketOptions{
			Region: d.config.Region,
		})
		if err != nil {
			return fmt.Errorf("bucket does not exist and could not be created: %w", err)
		}

		d.logger.WithField("bucket", d.config.Bucket).Info("Created S3 bucket")
	}

	return nil
}

// isNotFoundError checks if an error indicates that an object was not found
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	// Check for MinIO error response
	errResp := minio.ToErrorResponse(err)
	return errResp.Code == "NoSuchKey" ||
		errResp.Code == "NoSuchBucket" ||
		strings.Contains(err.Error(), "key does not exist")
}
