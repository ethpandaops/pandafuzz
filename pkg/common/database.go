// Package common provides shared types and utilities for the PandaFuzz system.
// Database types are re-exported from pkg/database for convenience.
package common

import (
	"github.com/ethpandaops/pandafuzz/pkg/database"
)

// Re-export database types
type (
	Database               = database.Database
	Transaction            = database.Transaction
	DatabaseStats          = database.Stats
	DatabaseConfig         = database.Config
	Query                  = database.Query
	AdvancedDatabase       = database.Advanced
	DatabaseFactory        = database.Factory
	DatabaseMiddleware     = database.Middleware
	DatabaseWithMiddleware = database.WithMiddleware
)

// Re-export constructor functions
var NewDatabaseWithMiddleware = database.NewWithMiddleware

// Re-export helper functions
var (
	IsTransactionError = database.IsTransactionError
	IsDatabaseClosed   = database.IsDatabaseClosed
)
