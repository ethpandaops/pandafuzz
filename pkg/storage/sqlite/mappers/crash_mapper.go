package mappers

import (
	"database/sql"
	"encoding/json"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	crashtypes "github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/models"
)

// Common crash type string constants
const (
	crashTypeSegfault      = "segfault"
	crashTypeHeapOverflow  = "heap-overflow"
	crashTypeStackOverflow = "stack-overflow"
	crashTypeAssertion     = "assertion"
	crashTypeTimeout       = "timeout"
	crashTypeMemoryLeak    = "memory-leak"
	crashTypeOther         = "other"
)

// DomainCrashToRow converts a domain Crash to a database row
func DomainCrashToRow(crash *crashtypes.Crash) *models.DomainCrashRow {
	if crash == nil {
		return nil
	}

	row := &models.DomainCrashRow{
		ID:              crash.ID,
		Input:           crash.Input,
		InputHash:       crash.InputHash,
		StackTrace:      crash.StackTrace,
		Severity:        string(crash.Severity),
		Type:            string(crash.Type),
		DiscoveredAt:    crash.DiscoveredAt,
		LastSeenAt:      crash.LastSeenAt,
		OccurrenceCount: safeUint64ToInt64(crash.OccurrenceCount),
		TargetName:      crash.TargetInfo.Name,
		Reproducible:    crash.Reproducible,
		Fixed:           crash.Fixed,
	}

	// Signature
	if crash.Signature != nil {
		row.SignatureHash = crash.Signature.Hash
		if sigData, err := json.Marshal(crash.Signature); err == nil {
			row.SignatureJSON = sql.NullString{String: string(sigData), Valid: true}
		}
	}

	// Corpus entry ID
	if crash.CorpusEntryID != "" {
		row.CorpusEntryID = sql.NullString{String: crash.CorpusEntryID, Valid: true}
	}

	// Target info
	if crash.TargetInfo.Version != "" {
		row.TargetVersion = sql.NullString{String: crash.TargetInfo.Version, Valid: true}
	}
	if crash.TargetInfo.Command != "" {
		row.TargetCommand = sql.NullString{String: crash.TargetInfo.Command, Valid: true}
	}
	if crash.TargetInfo.Environment != "" {
		row.TargetEnv = sql.NullString{String: crash.TargetInfo.Environment, Valid: true}
	}

	// Metadata
	if len(crash.Metadata) > 0 {
		if data, err := json.Marshal(crash.Metadata); err == nil {
			row.MetadataJSON = sql.NullString{String: string(data), Valid: true}
		}
	}

	// Fixed at
	if crash.FixedAt != nil {
		row.FixedAt = sql.NullTime{Time: *crash.FixedAt, Valid: true}
	}

	// Tags
	if len(crash.Tags) > 0 {
		if data, err := json.Marshal(crash.Tags); err == nil {
			row.TagsJSON = sql.NullString{String: string(data), Valid: true}
		}
	}

	return row
}

// CrashRowToDomain converts a database row to a domain Crash
func CrashRowToDomain(row *models.DomainCrashRow) *crashtypes.Crash {
	if row == nil {
		return nil
	}

	crash := &crashtypes.Crash{
		ID:              row.ID,
		Input:           row.Input,
		InputHash:       row.InputHash,
		StackTrace:      row.StackTrace,
		Severity:        crashtypes.Severity(row.Severity),
		Type:            crashtypes.CrashType(row.Type),
		DiscoveredAt:    row.DiscoveredAt,
		LastSeenAt:      row.LastSeenAt,
		OccurrenceCount: safeInt64ToUint64(row.OccurrenceCount),
		Reproducible:    row.Reproducible,
		Fixed:           row.Fixed,
		TargetInfo: crashtypes.TargetInfo{
			Name: row.TargetName,
		},
		Metadata: make(map[string]string),
		Tags:     make([]string, 0),
	}

	// Parse signature
	if row.SignatureJSON.Valid && row.SignatureJSON.String != "" {
		var sig crashtypes.CrashSignature
		if err := json.Unmarshal([]byte(row.SignatureJSON.String), &sig); err == nil {
			crash.Signature = &sig
		}
	}

	// Corpus entry ID
	if row.CorpusEntryID.Valid {
		crash.CorpusEntryID = row.CorpusEntryID.String
	}

	// Target info
	if row.TargetVersion.Valid {
		crash.TargetInfo.Version = row.TargetVersion.String
	}
	if row.TargetCommand.Valid {
		crash.TargetInfo.Command = row.TargetCommand.String
	}
	if row.TargetEnv.Valid {
		crash.TargetInfo.Environment = row.TargetEnv.String
	}

	// Metadata
	if row.MetadataJSON.Valid && row.MetadataJSON.String != "" {
		_ = json.Unmarshal([]byte(row.MetadataJSON.String), &crash.Metadata)
	}

	// Fixed at
	if row.FixedAt.Valid {
		crash.FixedAt = &row.FixedAt.Time
	}

	// Tags
	if row.TagsJSON.Valid && row.TagsJSON.String != "" {
		_ = json.Unmarshal([]byte(row.TagsJSON.String), &crash.Tags)
	}

	return crash
}

// CommonCrashToDomain converts common.CrashResult to domain Crash
func CommonCrashToDomain(cr *common.CrashResult) *crashtypes.Crash {
	if cr == nil {
		return nil
	}

	crash := &crashtypes.Crash{
		ID:           cr.ID,
		Input:        cr.Input,
		InputHash:    cr.Hash,
		StackTrace:   cr.StackTrace,
		Severity:     commonCrashTypeToDomainSeverity(cr.Type),
		Type:         commonCrashTypeToDomainType(cr.Type),
		DiscoveredAt: cr.Timestamp,
		LastSeenAt:   cr.Timestamp,
		Reproducible: cr.Reproducible,
		Fixed:        false,
		TargetInfo: crashtypes.TargetInfo{
			Name: cr.JobID, // Use JobID as target name
		},
		Metadata: make(map[string]string),
		Tags:     make([]string, 0),
	}

	// Convert metadata
	if cr.Metadata != nil {
		for k, v := range cr.Metadata {
			if s, ok := v.(string); ok {
				crash.Metadata[k] = s
			}
		}
	}

	return crash
}

// DomainCrashToCommon converts domain Crash to common.CrashResult
func DomainCrashToCommon(crash *crashtypes.Crash) *common.CrashResult {
	if crash == nil {
		return nil
	}

	cr := &common.CrashResult{
		ID:           crash.ID,
		Hash:         crash.InputHash,
		Type:         domainCrashTypeToCommon(crash.Type),
		Timestamp:    crash.DiscoveredAt,
		StackTrace:   crash.StackTrace,
		Input:        crash.Input,
		Reproducible: crash.Reproducible,
		IsUnique:     true, // Assume unique since it's in domain
		Metadata:     make(map[string]interface{}),
	}

	// Convert metadata
	if crash.Metadata != nil {
		for k, v := range crash.Metadata {
			cr.Metadata[k] = v
		}
	}

	// Add job ID from target info if available
	if crash.TargetInfo.Name != "" {
		cr.JobID = crash.TargetInfo.Name
	}

	return cr
}

// commonCrashTypeToDomainSeverity maps common crash type to domain severity
func commonCrashTypeToDomainSeverity(t string) crashtypes.Severity {
	switch t {
	case crashTypeSegfault, crashTypeHeapOverflow, crashTypeStackOverflow:
		return crashtypes.SeverityHigh
	case crashTypeAssertion:
		return crashtypes.SeverityMedium
	case crashTypeTimeout:
		return crashtypes.SeverityLow
	default:
		return crashtypes.SeverityUnknown
	}
}

// commonCrashTypeToDomainType maps common crash type string to domain CrashType
func commonCrashTypeToDomainType(t string) crashtypes.CrashType {
	switch t {
	case crashTypeSegfault:
		return crashtypes.CrashTypeSegmentationFault
	case crashTypeHeapOverflow:
		return crashtypes.CrashTypeHeapOverflow
	case crashTypeStackOverflow:
		return crashtypes.CrashTypeStackOverflow
	case crashTypeAssertion:
		return crashtypes.CrashTypeAssertion
	case crashTypeTimeout:
		return crashtypes.CrashTypeTimeout
	default:
		return crashtypes.CrashTypeOther
	}
}

// domainCrashTypeToCommon maps domain CrashType to common crash type string
func domainCrashTypeToCommon(t crashtypes.CrashType) string {
	switch t {
	case crashtypes.CrashTypeSegmentationFault:
		return crashTypeSegfault
	case crashtypes.CrashTypeHeapOverflow:
		return crashTypeHeapOverflow
	case crashtypes.CrashTypeStackOverflow:
		return crashTypeStackOverflow
	case crashtypes.CrashTypeAssertion:
		return crashTypeAssertion
	case crashtypes.CrashTypeTimeout:
		return crashTypeTimeout
	case crashtypes.CrashTypeMemoryLeak:
		return crashTypeMemoryLeak
	default:
		return crashTypeOther
	}
}
