package models

import (
	"database/sql"
	"time"
)

// DomainCrashRow represents a domain Crash as stored in the database.
// This is for the domain crash/types.Crash, not the common crash types.
type DomainCrashRow struct {
	ID              string         `db:"id"`
	SignatureHash   string         `db:"signature_hash"`
	SignatureJSON   sql.NullString `db:"signature"`
	Input           []byte         `db:"input"`
	InputHash       string         `db:"input_hash"`
	StackTrace      string         `db:"stack_trace"`
	Severity        string         `db:"severity"`
	Type            string         `db:"type"`
	DiscoveredAt    time.Time      `db:"discovered_at"`
	LastSeenAt      time.Time      `db:"last_seen_at"`
	OccurrenceCount int64          `db:"occurrence_count"`
	CorpusEntryID   sql.NullString `db:"corpus_entry_id"`
	TargetName      string         `db:"target_name"`
	TargetVersion   sql.NullString `db:"target_version"`
	TargetCommand   sql.NullString `db:"target_command"`
	TargetEnv       sql.NullString `db:"target_env"`
	MetadataJSON    sql.NullString `db:"metadata"`
	Reproducible    bool           `db:"reproducible"`
	Fixed           bool           `db:"fixed"`
	FixedAt         sql.NullTime   `db:"fixed_at"`
	TagsJSON        sql.NullString `db:"tags"`
}
