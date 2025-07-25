package abstraction

import (
	"time"

	"github.com/sirupsen/logrus"
)

// FilesystemConfig represents filesystem driver configuration
type FilesystemConfig struct {
	BasePath string
	FileMode uint32
	DirMode  uint32
}

// S3Config represents S3 driver configuration
type S3Config struct {
	Bucket             string
	Region             string
	AccessKeyID        string
	SecretAccessKey    string
	Endpoint           string
	DisableSSL         bool
	UsePathStyle       bool
	PresignedURLExpiry time.Duration
}

// NewFilesystemDriver is a convenience function to create a filesystem driver
func NewFilesystemDriver(basePath string, logger logrus.FieldLogger) (Driver, error) {
	factory := NewFactory(logger)
	return factory.NewDriver(FactoryConfig{
		Type: TypeFilesystem,
		Config: &FilesystemConfig{
			BasePath: basePath,
			FileMode: 0o644,
			DirMode:  0o755,
		},
	})
}

// NewS3Driver is a convenience function to create an S3 driver
func NewS3Driver(bucket, region, accessKey, secretKey string, logger logrus.FieldLogger) (Driver, error) {
	factory := NewFactory(logger)
	return factory.NewDriver(FactoryConfig{
		Type: TypeS3,
		Config: &S3Config{
			Bucket:             bucket,
			Region:             region,
			AccessKeyID:        accessKey,
			SecretAccessKey:    secretKey,
			PresignedURLExpiry: 1 * time.Hour,
		},
	})
}
