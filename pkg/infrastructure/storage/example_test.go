package storage_test

import (
	"context"
	"fmt"
	"log"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/drivers/filesystem"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/drivers/s3"
)

func ExampleFilesystemDriver() {
	// Create logger
	logger := logrus.New()

	// Configure filesystem driver
	config := filesystem.Config{
		BasePath: "/var/lib/pandafuzz/storage",
		FileMode: 0o644,
		DirMode:  0o755,
		Config:   storage.DefaultConfig(),
	}

	// Create driver
	driver, err := filesystem.NewDriver(config, logger)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Store data
	data := []byte("Hello, World!")
	if err := driver.Put(ctx, "greetings/hello.txt", data); err != nil {
		log.Fatal(err)
	}

	// Retrieve data
	retrieved, err := driver.Get(ctx, "greetings/hello.txt")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Retrieved: %s\n", retrieved)

	// List files
	keys, err := driver.List(ctx, "greetings/")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Files: %v\n", keys)
}

func ExampleS3Driver() {
	// Create logger
	logger := logrus.New()

	// Configure S3 driver for AWS
	config := s3.Config{
		Bucket:          "my-pandafuzz-bucket",
		Region:          "us-west-2",
		AccessKeyID:     "your-access-key",
		SecretAccessKey: "your-secret-key",
		Config:          storage.DefaultConfig(),
	}

	// Create driver
	driver, err := s3.NewDriver(config, logger)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Store data
	data := []byte("Important fuzzing results")
	if err := driver.Put(ctx, "results/campaign-123/crash-001.bin", data); err != nil {
		log.Fatal(err)
	}

	// Check if file exists
	exists, err := driver.Exists(ctx, "results/campaign-123/crash-001.bin")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("File exists: %v\n", exists)

	// Get presigned URL for download
	url, err := driver.GetURL(ctx, "results/campaign-123/crash-001.bin")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Download URL: %s\n", url)
}

func ExampleS3Driver_minio() {
	// Create logger
	logger := logrus.New()

	// Configure S3 driver for MinIO
	config := s3.Config{
		Bucket:          "pandafuzz",
		Region:          "us-east-1",
		Endpoint:        "localhost:9000",
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "minioadmin",
		DisableSSL:      true,
		UsePathStyle:    true,
		Config:          storage.DefaultConfig(),
	}

	// Create driver
	driver, err := s3.NewDriver(config, logger)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Store multiple files
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("corpus/input-%03d.bin", i)
		data := []byte(fmt.Sprintf("Input data %d", i))

		if err := driver.Put(ctx, key, data); err != nil {
			log.Fatal(err)
		}
	}

	// List all corpus files
	keys, err := driver.List(ctx, "corpus/")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Corpus files: %v\n", keys)

	// Clean up
	for _, key := range keys {
		if err := driver.Delete(ctx, key); err != nil {
			log.Fatal(err)
		}
	}
}
