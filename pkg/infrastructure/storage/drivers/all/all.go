// Package all imports all storage drivers to register them
package all

import (
	// Import drivers to register them
	_ "github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/drivers/filesystem"
	_ "github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/drivers/s3"
)
