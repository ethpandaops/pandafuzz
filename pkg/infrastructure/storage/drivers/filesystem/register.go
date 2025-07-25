package filesystem

import (
	"os"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/abstraction"
)

func init() {
	abstraction.RegisterDriver(abstraction.TypeFilesystem, func(config interface{}, logger logrus.FieldLogger) (abstraction.Driver, error) {
		// Try to convert from helper config first
		if helperCfg, ok := config.(*abstraction.FilesystemConfig); ok {
			cfg := Config{
				BasePath: helperCfg.BasePath,
				FileMode: os.FileMode(helperCfg.FileMode),
				DirMode:  os.FileMode(helperCfg.DirMode),
				Config:   abstraction.DefaultConfig(),
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
