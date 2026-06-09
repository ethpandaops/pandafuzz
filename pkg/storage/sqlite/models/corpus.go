package models

import (
	"database/sql"
	"time"
)

// DomainCorpusEntryRow represents a domain CorpusEntry as stored in the database.
type DomainCorpusEntryRow struct {
	ID             string         `db:"id"`
	Input          []byte         `db:"input"`
	Hash           string         `db:"hash"`
	Size           int            `db:"size"`
	CreatedAt      time.Time      `db:"created_at"`
	LastExecutedAt sql.NullTime   `db:"last_executed_at"`
	ExecutionCount int64          `db:"execution_count"`
	CoverageJSON   sql.NullString `db:"coverage"`
	MutationJSON   sql.NullString `db:"mutation_info"`
	TagsJSON       sql.NullString `db:"tags"`
	MetadataJSON   sql.NullString `db:"metadata"`
}

// DomainCorpusCollectionRow represents a corpus collection in the database.
type DomainCorpusCollectionRow struct {
	Name        string    `db:"name"`
	Description string    `db:"description"`
	MaxSize     int       `db:"max_size"`
	CreatedAt   time.Time `db:"created_at"`
}

// DomainCorpusCollectionEntryRow represents the mapping between collections and entries.
type DomainCorpusCollectionEntryRow struct {
	CollectionName string    `db:"collection_name"`
	EntryID        string    `db:"entry_id"`
	AddedAt        time.Time `db:"added_at"`
}
