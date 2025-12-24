// Package common provides shared types and utilities for the PandaFuzz system.
// Retry types are re-exported from pkg/retry for convenience.
package common

import (
	"github.com/ethpandaops/pandafuzz/pkg/retry"
)

// Re-export retry types
type (
	RetryManager         = retry.Manager
	CircuitBreaker       = retry.CircuitBreaker
	CircuitState         = retry.CircuitState
	ResilientClient      = retry.ResilientClient
	ResilientClientStats = retry.ResilientClientStats
)

// Re-export circuit states
const (
	CircuitClosed   = retry.CircuitClosed
	CircuitOpen     = retry.CircuitOpen
	CircuitHalfOpen = retry.CircuitHalfOpen
)

// Re-export constructor functions
var (
	NewRetryManager    = retry.NewManager
	NewCircuitBreaker  = retry.NewCircuitBreaker
	NewResilientClient = retry.NewResilientClient
)

// Re-export default policies
var (
	DefaultRetryPolicy  = retry.DefaultPolicy
	DatabaseRetryPolicy = retry.DatabasePolicy
	NetworkRetryPolicy  = retry.NetworkPolicy
	UpdateRetryPolicy   = retry.UpdatePolicy
)

// Re-export error detection functions
var (
	IsNetworkError   = retry.IsNetworkError
	IsTimeoutError   = retry.IsTimeoutError
	IsTemporaryError = retry.IsTemporaryError
	IsDatabaseError  = retry.IsDatabaseError
)
