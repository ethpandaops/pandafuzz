// Package common provides shared types and utilities for the PandaFuzz system.
// Error types are re-exported from pkg/errors for convenience.
package common

import (
	pkgerrors "github.com/ethpandaops/pandafuzz/pkg/errors"
)

// Re-export error types
type (
	ErrorCode           = pkgerrors.ErrorCode
	CodedError          = pkgerrors.CodedError
	Error               = pkgerrors.CodedError
	RetryExhaustedError = pkgerrors.RetryExhaustedError
	TimeoutError        = pkgerrors.TimeoutErr
)

// Re-export error code constants
const (
	ErrCodeInternal          = pkgerrors.ErrCodeInternal
	ErrCodeInvalidInput      = pkgerrors.ErrCodeInvalidInput
	ErrCodeNotFound          = pkgerrors.ErrCodeNotFound
	ErrCodeAlreadyExists     = pkgerrors.ErrCodeAlreadyExists
	ErrCodeUnauthorized      = pkgerrors.ErrCodeUnauthorized
	ErrCodeForbidden         = pkgerrors.ErrCodeForbidden
	ErrCodeFuzzerInit        = pkgerrors.ErrCodeFuzzerInit
	ErrCodeFuzzerExec        = pkgerrors.ErrCodeFuzzerExec
	ErrCodeFuzzerTimeout     = pkgerrors.ErrCodeFuzzerTimeout
	ErrCodeCorpusSync        = pkgerrors.ErrCodeCorpusSync
	ErrCodeCorpusInvalid     = pkgerrors.ErrCodeCorpusInvalid
	ErrCodeJobInvalid        = pkgerrors.ErrCodeJobInvalid
	ErrCodeJobNotFound       = pkgerrors.ErrCodeJobNotFound
	ErrCodeBinaryNotFound    = pkgerrors.ErrCodeBinaryNotFound
	ErrCodeStorageRead       = pkgerrors.ErrCodeStorageRead
	ErrCodeStorageWrite      = pkgerrors.ErrCodeStorageWrite
	ErrCodeStorageFull       = pkgerrors.ErrCodeStorageFull
	ErrCodeNetworkTimeout    = pkgerrors.ErrCodeNetworkTimeout
	ErrCodeNetworkConnection = pkgerrors.ErrCodeNetworkConnection
)

// Re-export sentinel errors
var (
	ErrCampaignNotFound     = pkgerrors.ErrCampaignNotFound
	ErrCampaignRunning      = pkgerrors.ErrCampaignRunning
	ErrInvalidStackTrace    = pkgerrors.ErrInvalidStackTrace
	ErrCorpusFileTooLarge   = pkgerrors.ErrCorpusFileTooLarge
	ErrDuplicateCorpusFile  = pkgerrors.ErrDuplicateCorpusFile
	ErrCampaignCompleted    = pkgerrors.ErrCampaignCompleted
	ErrCampaignPaused       = pkgerrors.ErrCampaignPaused
	ErrNoCampaignJobs       = pkgerrors.ErrNoCampaignJobs
	ErrInvalidCampaignState = pkgerrors.ErrInvalidCampaignState
	ErrCrashGroupNotFound   = pkgerrors.ErrCrashGroupNotFound
	ErrCorpusFileNotFound   = pkgerrors.ErrCorpusFileNotFound
	ErrBinaryHashMismatch   = pkgerrors.ErrBinaryHashMismatch
	ErrKeyNotFound          = pkgerrors.ErrKeyNotFound
	ErrTransactionFail      = pkgerrors.ErrTransactionFail
	ErrDatabaseClosed       = pkgerrors.ErrDatabaseClosed
	ErrInvalidConfig        = pkgerrors.ErrInvalidConfig
	ErrMigrationFailed      = pkgerrors.ErrMigrationFailed
	ErrBackupFailed         = pkgerrors.ErrBackupFailed
	ErrRestoreFailed        = pkgerrors.ErrRestoreFailed
)

// Re-export constructor functions
var (
	NewError               = pkgerrors.NewCodedError
	NewRetryExhaustedError = pkgerrors.NewRetryExhaustedError
)
