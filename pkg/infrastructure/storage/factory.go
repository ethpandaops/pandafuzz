package storage

import (
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/abstraction"
)

// NewFilesystemDriver is a convenience function to create a filesystem driver
// Deprecated: Use abstraction.NewFilesystemDriver instead
func NewFilesystemDriver(basePath string, logger logrus.FieldLogger) (Driver, error) {
	return abstraction.NewFilesystemDriver(basePath, logger)
}

// NewS3Driver is a convenience function to create an S3 driver
// Deprecated: Use abstraction.NewS3Driver instead
func NewS3Driver(bucket, region, accessKey, secretKey string, logger logrus.FieldLogger) (Driver, error) {
	return abstraction.NewS3Driver(bucket, region, accessKey, secretKey, logger)
}
