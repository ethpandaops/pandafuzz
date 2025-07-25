package s3

import (
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/abstraction"
)

func init() {
	abstraction.RegisterDriver(abstraction.TypeS3, func(config interface{}, logger logrus.FieldLogger) (abstraction.Driver, error) {
		// Try to convert from helper config first
		if helperCfg, ok := config.(*abstraction.S3Config); ok {
			cfg := Config{
				Bucket:             helperCfg.Bucket,
				Region:             helperCfg.Region,
				AccessKeyID:        helperCfg.AccessKeyID,
				SecretAccessKey:    helperCfg.SecretAccessKey,
				Endpoint:           helperCfg.Endpoint,
				DisableSSL:         helperCfg.DisableSSL,
				UsePathStyle:       helperCfg.UsePathStyle,
				PresignedURLExpiry: helperCfg.PresignedURLExpiry,
				Config:             abstraction.DefaultConfig(),
			}
			if cfg.PresignedURLExpiry == 0 {
				cfg.PresignedURLExpiry = 1 * time.Hour
			}
			return NewDriver(cfg, logger)
		}

		// Otherwise expect the internal config type
		cfg, ok := config.(*Config)
		if !ok {
			return nil, ErrInvalidConfig
		}
		return NewDriver(*cfg, logger)
	})
}
