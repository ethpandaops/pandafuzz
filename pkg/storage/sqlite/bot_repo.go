// Package sqlite provides SQLite-based repository implementations for PandaFuzz.
// It implements domain repository interfaces using sqlite database storage.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	botrepo "github.com/ethpandaops/pandafuzz/pkg/domain/bot/repository"
	bottypes "github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/mappers"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/models"
	"github.com/sirupsen/logrus"
)

// Compile-time interface compliance check
var _ botrepo.AgentRepository = (*BotRepository)(nil)

// BotRepository implements botrepo.AgentRepository using SQLite
type BotRepository struct {
	db     *sql.DB
	logger logrus.FieldLogger
}

// NewBotRepository creates a new SQLite-based bot repository
func NewBotRepository(db *sql.DB, logger logrus.FieldLogger) *BotRepository {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	return &BotRepository{
		db:     db,
		logger: logger.WithField("component", "bot_repository"),
	}
}

// Create creates a new agent
func (r *BotRepository) Create(ctx context.Context, agent *bottypes.Agent) error {
	if agent == nil {
		return NewRepositoryError("create", "bot", "", fmt.Errorf("agent cannot be nil"))
	}

	row := mappers.DomainBotToRow(agent)

	query := `
		INSERT INTO bots (
			id, name, status, capabilities, metadata,
			last_heartbeat, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		row.ID, row.Name, row.Status, row.CapabilitiesJSON, row.MetadataJSON,
		row.LastHeartbeat, row.CreatedAt, row.UpdatedAt,
	)

	if err != nil {
		return NewRepositoryError("create", "bot", agent.ID, err)
	}

	r.logger.WithField("bot_id", agent.ID).Debug("Bot created")
	return nil
}

// Update updates an existing agent
func (r *BotRepository) Update(ctx context.Context, agent *bottypes.Agent) error {
	if agent == nil {
		return NewRepositoryError("update", "bot", "", fmt.Errorf("agent cannot be nil"))
	}

	row := mappers.DomainBotToRow(agent)

	query := `
		UPDATE bots SET
			name = ?, status = ?, capabilities = ?, metadata = ?,
			last_heartbeat = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query,
		row.Name, row.Status, row.CapabilitiesJSON, row.MetadataJSON,
		row.LastHeartbeat, time.Now().UTC(),
		agent.ID,
	)

	if err != nil {
		return NewRepositoryError("update", "bot", agent.ID, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("update", "bot", agent.ID, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("bot_id", agent.ID).Debug("Bot updated")
	return nil
}

// Delete deletes an agent by ID
func (r *BotRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM bots WHERE id = ?", id)
	if err != nil {
		return NewRepositoryError("delete", "bot", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("delete", "bot", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("bot_id", id).Debug("Bot deleted")
	return nil
}

// FindByID retrieves an agent by its ID
func (r *BotRepository) FindByID(ctx context.Context, id string) (*bottypes.Agent, error) {
	row, err := r.scanBotRow(ctx, "SELECT "+botSelectColumns()+" FROM bots WHERE id = ?", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, NewRepositoryError("find_by_id", "bot", id, err)
	}

	return mappers.BotRowToDomain(row), nil
}

// FindByName retrieves agents by name (partial match)
func (r *BotRepository) FindByName(ctx context.Context, name string) ([]*bottypes.Agent, error) {
	query := "SELECT " + botSelectColumns() + " FROM bots WHERE name LIKE ? ORDER BY name ASC"
	return r.scanBotRows(ctx, query, "%"+name+"%")
}

// FindByStatus retrieves all agents with a specific status
func (r *BotRepository) FindByStatus(ctx context.Context, status bottypes.Status) ([]*bottypes.Agent, error) {
	query := "SELECT " + botSelectColumns() + " FROM bots WHERE status = ? ORDER BY name ASC"
	return r.scanBotRows(ctx, query, string(status))
}

// FindByCapability retrieves all agents with a specific capability
func (r *BotRepository) FindByCapability(ctx context.Context, capability bottypes.Capability) ([]*bottypes.Agent, error) {
	// Capabilities are stored as JSON array, use LIKE for searching
	query := "SELECT " + botSelectColumns() + " FROM bots WHERE capabilities LIKE ? ORDER BY name ASC"
	return r.scanBotRows(ctx, query, "%\""+string(capability)+"\"%")
}

// FindOnline retrieves all online agents
func (r *BotRepository) FindOnline(ctx context.Context) ([]*bottypes.Agent, error) {
	// Consider an agent online if status is not offline and heartbeat is recent (5 minutes)
	threshold := time.Now().UTC().Add(-5 * time.Minute)
	query := `
		SELECT ` + botSelectColumns() + `
		FROM bots
		WHERE status != 'offline'
		  AND last_heartbeat > ?
		ORDER BY name ASC
	`
	return r.scanBotRows(ctx, query, threshold)
}

// FindAvailable retrieves all available agents (online and idle)
func (r *BotRepository) FindAvailable(ctx context.Context) ([]*bottypes.Agent, error) {
	threshold := time.Now().UTC().Add(-5 * time.Minute)
	query := `
		SELECT ` + botSelectColumns() + `
		FROM bots
		WHERE status = 'idle'
		  AND last_heartbeat > ?
		ORDER BY name ASC
	`
	return r.scanBotRows(ctx, query, threshold)
}

// FindStale retrieves agents that haven't sent heartbeat within duration
func (r *BotRepository) FindStale(ctx context.Context, staleThreshold time.Duration) ([]*bottypes.Agent, error) {
	threshold := time.Now().UTC().Add(-staleThreshold)
	query := `
		SELECT ` + botSelectColumns() + `
		FROM bots
		WHERE status != 'offline'
		  AND last_heartbeat < ?
		ORDER BY last_heartbeat ASC
	`
	return r.scanBotRows(ctx, query, threshold)
}

// UpdateHeartbeat updates the last heartbeat time for an agent
func (r *BotRepository) UpdateHeartbeat(ctx context.Context, id string) error {
	now := time.Now().UTC()
	query := `
		UPDATE bots
		SET last_heartbeat = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query, now, now, id)
	if err != nil {
		return NewRepositoryError("update_heartbeat", "bot", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("update_heartbeat", "bot", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("bot_id", id).Debug("Bot heartbeat updated")
	return nil
}

// UpdateStatus updates only the status of an agent
func (r *BotRepository) UpdateStatus(ctx context.Context, id string, status bottypes.Status) error {
	now := time.Now().UTC()
	query := `
		UPDATE bots
		SET status = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query, string(status), now, id)
	if err != nil {
		return NewRepositoryError("update_status", "bot", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("update_status", "bot", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithFields(logrus.Fields{
		"bot_id": id,
		"status": status,
	}).Debug("Bot status updated")
	return nil
}

// List retrieves agents with pagination
func (r *BotRepository) List(ctx context.Context, offset, limit int) ([]*bottypes.Agent, int, error) {
	// Get total count
	var total int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM bots").Scan(&total)
	if err != nil {
		return nil, 0, NewRepositoryError("list", "bot", "", err)
	}

	// Get paginated results
	query := "SELECT " + botSelectColumns() + " FROM bots ORDER BY name ASC LIMIT ? OFFSET ?"
	agents, err := r.scanBotRows(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	return agents, total, nil
}

// Exists checks if an agent exists by ID
func (r *BotRepository) Exists(ctx context.Context, id string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM bots WHERE id = ?)"
	err := r.db.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		return false, NewRepositoryError("exists", "bot", id, err)
	}
	return exists, nil
}

// CountByStatus counts agents by status
func (r *BotRepository) CountByStatus(ctx context.Context, status bottypes.Status) (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM bots WHERE status = ?"
	err := r.db.QueryRowContext(ctx, query, string(status)).Scan(&count)
	if err != nil {
		return 0, NewRepositoryError("count_by_status", "bot", "", err)
	}
	return count, nil
}

// CountByCapability counts agents by capability
func (r *BotRepository) CountByCapability(ctx context.Context, capability bottypes.Capability) (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM bots WHERE capabilities LIKE ?"
	err := r.db.QueryRowContext(ctx, query, "%\""+string(capability)+"\"%").Scan(&count)
	if err != nil {
		return 0, NewRepositoryError("count_by_capability", "bot", "", err)
	}
	return count, nil
}

// Helper functions

// botSelectColumns returns the column list for SELECT queries
func botSelectColumns() string {
	return "id, name, status, capabilities, metadata, last_heartbeat, created_at, updated_at"
}

// scanBotRow scans a single bot row from a query
func (r *BotRepository) scanBotRow(ctx context.Context, query string, args ...interface{}) (*models.DomainBotRow, error) {
	row := r.db.QueryRowContext(ctx, query, args...)
	return r.scanSingleBotRow(row)
}

// scanBotRows scans multiple bot rows from a query
func (r *BotRepository) scanBotRows(ctx context.Context, query string, args ...interface{}) ([]*bottypes.Agent, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := make([]*bottypes.Agent, 0, 16)
	for rows.Next() {
		botRow, err := r.scanRowsBotRow(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, mappers.BotRowToDomain(botRow))
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return agents, nil
}

// scanSingleBotRow scans a single *sql.Row into a DomainBotRow
func (r *BotRepository) scanSingleBotRow(row *sql.Row) (*models.DomainBotRow, error) {
	br := &models.DomainBotRow{}

	err := row.Scan(
		&br.ID, &br.Name, &br.Status, &br.CapabilitiesJSON, &br.MetadataJSON,
		&br.LastHeartbeat, &br.CreatedAt, &br.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return br, nil
}

// scanRowsBotRow scans a single row from *sql.Rows into a DomainBotRow
func (r *BotRepository) scanRowsBotRow(rows *sql.Rows) (*models.DomainBotRow, error) {
	br := &models.DomainBotRow{}

	err := rows.Scan(
		&br.ID, &br.Name, &br.Status, &br.CapabilitiesJSON, &br.MetadataJSON,
		&br.LastHeartbeat, &br.CreatedAt, &br.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return br, nil
}
