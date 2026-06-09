package mappers

import (
	"database/sql"
	"encoding/json"
	"time"

	corpustypes "github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/models"
)

// DomainCorpusEntryToRow converts a domain CorpusEntry to a database row
func DomainCorpusEntryToRow(entry *corpustypes.CorpusEntry) *models.DomainCorpusEntryRow {
	if entry == nil {
		return nil
	}

	row := &models.DomainCorpusEntryRow{
		ID:             entry.ID,
		Input:          entry.Input,
		Hash:           entry.Hash,
		Size:           entry.Size,
		CreatedAt:      entry.CreatedAt,
		ExecutionCount: safeUint64ToInt64(entry.ExecutionCount),
	}

	// Last executed at
	if entry.LastExecutedAt != nil {
		row.LastExecutedAt = sql.NullTime{Time: *entry.LastExecutedAt, Valid: true}
	}

	// Coverage
	if covData, err := json.Marshal(entry.Coverage); err == nil {
		row.CoverageJSON = sql.NullString{String: string(covData), Valid: true}
	}

	// Mutation info
	if mutData, err := json.Marshal(entry.MutationInfo); err == nil {
		row.MutationJSON = sql.NullString{String: string(mutData), Valid: true}
	}

	// Tags
	if len(entry.Tags) > 0 {
		if tagsData, err := json.Marshal(entry.Tags); err == nil {
			row.TagsJSON = sql.NullString{String: string(tagsData), Valid: true}
		}
	}

	// Metadata
	if len(entry.Metadata) > 0 {
		if metaData, err := json.Marshal(entry.Metadata); err == nil {
			row.MetadataJSON = sql.NullString{String: string(metaData), Valid: true}
		}
	}

	return row
}

// CorpusEntryRowToDomain converts a database row to a domain CorpusEntry
func CorpusEntryRowToDomain(row *models.DomainCorpusEntryRow) *corpustypes.CorpusEntry {
	if row == nil {
		return nil
	}

	entry := &corpustypes.CorpusEntry{
		ID:             row.ID,
		Input:          row.Input,
		Hash:           row.Hash,
		Size:           row.Size,
		CreatedAt:      row.CreatedAt,
		ExecutionCount: safeInt64ToUint64(row.ExecutionCount),
		Tags:           make([]string, 0),
		Metadata:       make(map[string]string),
	}

	// Last executed at
	if row.LastExecutedAt.Valid {
		entry.LastExecutedAt = &row.LastExecutedAt.Time
	}

	// Coverage
	if row.CoverageJSON.Valid && row.CoverageJSON.String != "" {
		_ = json.Unmarshal([]byte(row.CoverageJSON.String), &entry.Coverage)
	}

	// Mutation info
	if row.MutationJSON.Valid && row.MutationJSON.String != "" {
		_ = json.Unmarshal([]byte(row.MutationJSON.String), &entry.MutationInfo)
	}

	// Tags
	if row.TagsJSON.Valid && row.TagsJSON.String != "" {
		_ = json.Unmarshal([]byte(row.TagsJSON.String), &entry.Tags)
	}

	// Metadata
	if row.MetadataJSON.Valid && row.MetadataJSON.String != "" {
		_ = json.Unmarshal([]byte(row.MetadataJSON.String), &entry.Metadata)
	}

	return entry
}

// DomainCorpusCollectionToRow converts collection metadata to a row
func DomainCorpusCollectionToRow(collection *corpustypes.CorpusCollection) *models.DomainCorpusCollectionRow {
	if collection == nil {
		return nil
	}

	return &models.DomainCorpusCollectionRow{
		Name:        collection.Name(),
		Description: collection.Description(),
		CreatedAt:   collection.CreatedAt(),
	}
}

// CorpusCollectionRowToDomain creates a CorpusCollection from a row (without entries)
func CorpusCollectionRowToDomain(row *models.DomainCorpusCollectionRow) *corpustypes.CorpusCollection {
	if row == nil {
		return nil
	}

	collection, err := corpustypes.NewCorpusCollection(row.Name, row.MaxSize)
	if err != nil {
		return nil
	}
	collection.SetDescription(row.Description)
	return collection
}

// NowUTCTime returns the current UTC time
func NowUTCTime() time.Time {
	return time.Now().UTC()
}
