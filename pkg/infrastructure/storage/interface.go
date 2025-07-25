// Package storage provides storage drivers for the infrastructure layer.
// This file provides compatibility exports from the abstraction package.
package storage

import (
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/storage/abstraction"
)

// Re-export core types from abstraction package for backward compatibility

// Driver defines the interface for storage backends.
type Driver = abstraction.Driver

// Config provides common configuration options for storage drivers.
type Config = abstraction.Config

// Common errors returned by storage drivers
var (
	ErrNotFound           = abstraction.ErrNotFound
	ErrInvalidKey         = abstraction.ErrInvalidKey
	ErrStorageUnavailable = abstraction.ErrStorageUnavailable
)

// DefaultConfig returns default configuration values.
func DefaultConfig() Config {
	return abstraction.DefaultConfig()
}
