// Package crash provides crash handling, analysis, and deduplication for the PandaFuzz system.
// This file contains converter functions between fuzzer-internal crash types and storage/API types.
package crash

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	fuzzertypes "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
)

// FromFuzzerCrashInfo converts a fuzzer's internal CrashInfo to the storage/API CrashResult type.
// This function bridges the gap between fuzzer-internal crash representation and the
// canonical storage format used throughout the system.
//
// Parameters:
//   - info: The fuzzer's internal crash information
//   - jobID: The job ID this crash belongs to
//   - botID: The bot ID that discovered this crash
//   - campaignID: The campaign ID this crash belongs to (can be empty)
//
// Returns a CrashResult ready for storage/API use.
func FromFuzzerCrashInfo(info *fuzzertypes.CrashInfo, jobID, botID, campaignID string) *common.CrashResult {
	if info == nil {
		return nil
	}

	// Calculate hash for deduplication
	hash := calculateInputHash(info.Input)

	// Convert metadata from map[string]string to map[string]interface{}
	metadata := make(map[string]interface{}, len(info.Metadata))
	for k, v := range info.Metadata {
		metadata[k] = v
	}

	// Add fuzzer type to metadata
	if info.FuzzerType != "" {
		metadata["fuzzer_type"] = info.FuzzerType
	}

	return &common.CrashResult{
		ID:           info.ID,
		JobID:        jobID,
		BotID:        botID,
		CampaignID:   campaignID,
		Hash:         hash,
		Type:         classifyCrashType(info.Signal),
		Signal:       info.Signal,
		ExitCode:     signalToExitCode(info.Signal),
		Timestamp:    info.DiscoveredAt,
		Size:         int64(len(info.Input)),
		IsUnique:     true, // Default to unique; deduplication service will update
		Input:        info.Input,
		StackTrace:   info.StackTrace,
		Reproducible: true, // Default to reproducible; reproduction service will verify
		Minimized:    false,
		Metadata:     metadata,
	}
}

// calculateInputHash computes the SHA256 hash of crash input data for deduplication.
func calculateInputHash(input []byte) string {
	if len(input) == 0 {
		return ""
	}
	hash := sha256.Sum256(input)
	return hex.EncodeToString(hash[:])
}

// classifyCrashType determines the crash type based on the signal number.
// This provides a human-readable classification for crash analysis.
func classifyCrashType(signal int) string {
	switch signal {
	case 4: // SIGILL
		return "illegal_instruction"
	case 6: // SIGABRT
		return "abort"
	case 7: // SIGBUS
		return "bus_error"
	case 8: // SIGFPE
		return "floating_point_exception"
	case 11: // SIGSEGV
		return "segfault"
	case 14: // SIGALRM
		return "timeout"
	case 15: // SIGTERM
		return "terminated"
	case 31: // SIGSYS
		return "system_call_error"
	default:
		if signal > 0 {
			return "signal"
		}
		return "unknown"
	}
}

// signalToExitCode converts a signal number to the expected exit code.
// On Unix systems, programs killed by signals typically exit with 128 + signal.
func signalToExitCode(signal int) int {
	if signal > 0 {
		return 128 + signal
	}
	return 0
}

// ToCrashInfo converts a storage CrashResult back to a fuzzer CrashInfo type.
// This is useful when a stored crash needs to be passed to fuzzer components.
func ToCrashInfo(result *common.CrashResult) *fuzzertypes.CrashInfo {
	if result == nil {
		return nil
	}

	// Convert metadata from map[string]interface{} to map[string]string
	metadata := make(map[string]string, len(result.Metadata))
	for k, v := range result.Metadata {
		if str, ok := v.(string); ok {
			metadata[k] = str
		}
	}

	// Extract fuzzer type from metadata if present
	fuzzerType := ""
	if ft, ok := result.Metadata["fuzzer_type"].(string); ok {
		fuzzerType = ft
	}

	return &fuzzertypes.CrashInfo{
		ID:           result.ID,
		Input:        result.Input,
		StackTrace:   result.StackTrace,
		Signal:       result.Signal,
		DiscoveredAt: result.Timestamp,
		FuzzerType:   fuzzerType,
		Metadata:     metadata,
	}
}

// FromCrashResultWithTimestamp creates a CrashResult from CrashInfo with a custom timestamp.
// This is useful for testing or when the discovery time differs from storage time.
func FromCrashInfoWithTimestamp(info *fuzzertypes.CrashInfo, jobID, botID, campaignID string, timestamp time.Time) *common.CrashResult {
	result := FromFuzzerCrashInfo(info, jobID, botID, campaignID)
	if result != nil {
		result.Timestamp = timestamp
	}
	return result
}
