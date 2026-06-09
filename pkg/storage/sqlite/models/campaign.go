package models

import (
	"database/sql"
	"time"
)

// DomainCampaignRow represents a domain Campaign as stored in the database.
type DomainCampaignRow struct {
	ID          string       `db:"id"`
	Name        string       `db:"name"`
	Description string       `db:"description"`
	Status      string       `db:"status"`
	CreatedAt   time.Time    `db:"created_at"`
	UpdatedAt   time.Time    `db:"updated_at"`
	CompletedAt sql.NullTime `db:"completed_at"`
}
