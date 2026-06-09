package models

import (
	"database/sql"
	"time"
)

// DomainCoverageResultRow represents a coverage result in the database.
type DomainCoverageResultRow struct {
	ID           string         `db:"id"`
	JobID        string         `db:"job_id"`
	BotID        sql.NullString `db:"bot_id"`
	Edges        int            `db:"edges"`
	NewEdges     int            `db:"new_edges"`
	ExecCount    int64          `db:"exec_count"`
	Timestamp    time.Time      `db:"timestamp"`
	MetadataJSON sql.NullString `db:"metadata"`
	CreatedAt    time.Time      `db:"created_at"`
}
