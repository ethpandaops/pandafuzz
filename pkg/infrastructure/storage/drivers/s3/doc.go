// Package s3 provides a storage driver implementation using AWS S3 or S3-compatible services.
//
// The S3 driver stores data as objects in an S3 bucket, supporting both AWS S3 and
// S3-compatible services like MinIO, DigitalOcean Spaces, or Google Cloud Storage
// in S3-compatibility mode. It provides efficient object storage with built-in
// redundancy and scalability.
//
// # Features
//
//   - Support for AWS S3 and S3-compatible services
//   - Automatic bucket creation if it doesn't exist
//   - Presigned URL generation for secure, time-limited access
//   - Efficient pagination for listing large numbers of objects
//   - Support for custom endpoints and path-style URLs
//   - Configurable authentication methods (static credentials or IAM roles)
//
// # Configuration
//
// The driver is configured using the Config struct:
//
//	config := s3.Config{
//	    Bucket:          "pandafuzz-storage",
//	    Region:          "us-west-2",
//	    AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
//	    SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
//	    Config: storage.Config{
//	        MaxKeyLength: 1024,
//	        MaxValueSize: 5 * 1024 * 1024 * 1024, // 5GB (S3 limit)
//	    },
//	}
//
// For S3-compatible services like MinIO:
//
//	config := s3.Config{
//	    Bucket:       "pandafuzz-storage",
//	    Region:       "us-east-1",
//	    Endpoint:     "http://localhost:9000",
//	    UsePathStyle: true,
//	    DisableSSL:   true, // For local testing only
//	    AccessKeyID:  "minioadmin",
//	    SecretAccessKey: "minioadmin",
//	}
//
// # Authentication
//
// The driver supports multiple authentication methods:
//   - Static credentials (AccessKeyID and SecretAccessKey)
//   - IAM roles (when running on AWS infrastructure)
//   - Default credential chain (environment variables, shared config, etc.)
//
// If no credentials are provided, the driver will use the AWS SDK's default
// credential provider chain.
//
// # Key Format
//
// Keys are used directly as S3 object keys. They must:
//   - Be non-empty
//   - Not start with a forward slash
//   - Not contain consecutive slashes
//   - Not exceed the configured maximum length
//
// # Example Usage
//
//	// Create a new S3 driver
//	driver, err := s3.NewDriver(config, logger)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Store data
//	err = driver.Put(ctx, "data/reports/2024/january.pdf", reportData)
//
//	// Retrieve data
//	data, err := driver.Get(ctx, "data/reports/2024/january.pdf")
//
//	// Generate a presigned URL for download
//	url, err := driver.GetURL(ctx, "data/reports/2024/january.pdf")
//
//	// List all reports for 2024
//	keys, err := driver.List(ctx, "data/reports/2024/")
//
// # Thread Safety
//
// All operations are thread-safe and can be called concurrently.
// The AWS SDK handles connection pooling and request retries internally.
//
// # Error Handling
//
// The driver returns storage.ErrNotFound when attempting to retrieve
// non-existent objects, and storage.ErrInvalidKey for invalid key formats.
// Network errors and AWS service errors are wrapped with additional context.
//
// # Performance Considerations
//
// For optimal performance:
//   - Use appropriate object sizes (larger objects for better throughput)
//   - Consider multipart uploads for very large objects (not implemented in this basic driver)
//   - Use presigned URLs for direct client uploads/downloads to reduce server load
//   - Enable S3 Transfer Acceleration for geographically distributed access
package s3
