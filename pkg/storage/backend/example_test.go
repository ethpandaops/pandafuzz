package backend_test

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/config"
	"github.com/ethpandaops/pandafuzz/pkg/storage/backend"
)

func ExampleNewStorageBackend_filesystem() {
	logger := logrus.New()

	// Configure filesystem storage
	cfg := config.StorageConfig{
		Type: config.StorageTypeFilesystem,
		Filesystem: config.FilesystemConfig{
			BasePath: "/tmp/pandafuzz-corpus",
		},
		MaxFileSize: 100 * 1024 * 1024, // 100MB
		EnableDedup: true,
	}

	// Create storage backend
	storage, err := backend.NewStorageBackend(cfg, logger)
	if err != nil {
		log.Fatal(err)
	}

	// Use the storage backend
	ctx := context.Background()
	key := "campaign1/corpus/test.bin"
	data := []byte("test corpus data")

	err = storage.Store(ctx, key, strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("File stored successfully")
}

func ExampleNewStorageBackend_minio() {
	logger := logrus.New()

	// Configure MinIO storage
	cfg := config.StorageConfig{
		Type: config.StorageTypeMinIO,
		MinIO: config.MinIOConfig{
			S3Config: config.S3Config{
				Endpoint:         "localhost:9000",
				AccessKeyID:      "pandafuzz",
				SecretAccessKey:  "pandafuzz123",
				CorpusBucket:     "corpus",
				QuarantineBucket: "quarantine",
				BackupBucket:     "backup",
				UseSSL:           false,
				UsePathStyle:     true,
				PartSize:         64 * 1024 * 1024, // 64MB parts
				Concurrency:      4,
			},
		},
	}

	// Create storage backend
	storage, err := backend.NewStorageBackend(cfg, logger)
	if err != nil {
		log.Fatal(err)
	}

	// Health check
	ctx := context.Background()
	if err := storage.HealthCheck(ctx); err != nil {
		log.Fatal("MinIO health check failed:", err)
	}

	fmt.Println("MinIO backend initialized successfully")
}

func ExampleNewStorageBackend_s3() {
	logger := logrus.New()

	// Configure AWS S3 storage
	cfg := config.StorageConfig{
		Type: config.StorageTypeS3,
		S3: config.S3Config{
			Region:           "us-east-1",
			CorpusBucket:     "pandafuzz-corpus-prod",
			QuarantineBucket: "pandafuzz-quarantine-prod",
			BackupBucket:     "pandafuzz-backup-prod",
			UseSSL:           true,
			// AWS credentials will be loaded from environment or IAM role
		},
	}

	// Create storage backend
	_, err := backend.NewStorageBackend(cfg, logger)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("S3 backend initialized successfully")
}
