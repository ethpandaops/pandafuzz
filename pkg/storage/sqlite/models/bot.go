// Package models provides database row types for SQLite storage.
package models

import (
	"database/sql"
	"time"
)

// DomainBotRow represents a domain Agent as stored in the database.
// This is for the domain bot/types.Agent, not the common.Bot.
type DomainBotRow struct {
	ID               string         `db:"id"`
	Name             string         `db:"name"`
	Status           string         `db:"status"`
	CapabilitiesJSON sql.NullString `db:"capabilities"`
	MetadataJSON     sql.NullString `db:"metadata"`
	LastHeartbeat    time.Time      `db:"last_heartbeat"`
	CreatedAt        time.Time      `db:"created_at"`
	UpdatedAt        time.Time      `db:"updated_at"`
}
