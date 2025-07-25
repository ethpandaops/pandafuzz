package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/cache"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/sqlite"
)

// BotRepository implements the agent repository interface using SQLite
type BotRepository struct {
	conn  *sqlite.Connection
	cache cache.Cache
	log   logrus.FieldLogger
}

// NewBotRepository creates a new bot repository
func NewBotRepository(conn *sqlite.Connection, cache cache.Cache, log logrus.FieldLogger) (*BotRepository, error) {
	if conn == nil {
		return nil, errors.New(errors.ErrorTypeDatabase, "new_bot_repository", "connection is required")
	}
	if log == nil {
		log = logrus.New()
	}

	repo := &BotRepository{
		conn:  conn,
		cache: cache,
		log:   log.WithField("component", "bot_repository"),
	}

	if err := repo.createSchema(); err != nil {
		return nil, err
	}

	return repo, nil
}

// createSchema creates the bots table if it doesn't exist
func (r *BotRepository) createSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS bots (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			capabilities TEXT NOT NULL,
			last_heartbeat DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			metadata TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_bots_name ON bots(name);
		CREATE INDEX IF NOT EXISTS idx_bots_status ON bots(status);
		CREATE INDEX IF NOT EXISTS idx_bots_last_heartbeat ON bots(last_heartbeat);
		CREATE INDEX IF NOT EXISTS idx_bots_capabilities ON bots(capabilities);
	`

	_, err := r.conn.ExecContext(context.Background(), schema)
	if err != nil {
		return errors.NewDatabaseError("create_bot_schema", err)
	}

	return nil
}

// Create creates a new agent
func (r *BotRepository) Create(ctx context.Context, agent *types.Agent) error {
	if agent == nil {
		return errors.NewValidationError("create_agent", "agent cannot be nil")
	}

	if err := agent.Validate(); err != nil {
		return errors.NewValidationError("create_agent", err.Error())
	}

	capabilities, err := json.Marshal(agent.Capabilities)
	if err != nil {
		return errors.NewSystemError("marshal_capabilities", err)
	}

	var metadata []byte
	if agent.Metadata != nil && len(agent.Metadata) > 0 {
		metadata, err = json.Marshal(agent.Metadata)
		if err != nil {
			return errors.NewSystemError("marshal_metadata", err)
		}
	}

	query := `
		INSERT INTO bots (id, name, status, capabilities, last_heartbeat, created_at, updated_at, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.conn.ExecContext(ctx, query,
		agent.ID,
		agent.Name,
		agent.Status,
		string(capabilities),
		agent.LastHeartbeat,
		agent.CreatedAt,
		agent.UpdatedAt,
		metadata,
	)

	if err != nil {
		if r.isUniqueConstraintError(err) {
			return errors.NewConflictError("create_agent", "agent already exists").
				WithDetail("agent_id", agent.ID)
		}
		return errors.NewDatabaseError("create_agent", err).
			WithDetail("agent_id", agent.ID)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.cacheKey(agent.ID))
		r.cache.Delete(ctx, "bots:list")
		r.cache.Delete(ctx, fmt.Sprintf("bots:status:%s", agent.Status))
		for _, cap := range agent.Capabilities {
			r.cache.Delete(ctx, fmt.Sprintf("bots:capability:%s", cap))
		}
	}

	r.log.WithField("agent_id", agent.ID).Debug("Agent created")
	return nil
}

// Update updates an existing agent
func (r *BotRepository) Update(ctx context.Context, agent *types.Agent) error {
	if agent == nil {
		return errors.NewValidationError("update_agent", "agent cannot be nil")
	}

	if err := agent.Validate(); err != nil {
		return errors.NewValidationError("update_agent", err.Error())
	}

	capabilities, err := json.Marshal(agent.Capabilities)
	if err != nil {
		return errors.NewSystemError("marshal_capabilities", err)
	}

	var metadata []byte
	if agent.Metadata != nil && len(agent.Metadata) > 0 {
		metadata, err = json.Marshal(agent.Metadata)
		if err != nil {
			return errors.NewSystemError("marshal_metadata", err)
		}
	}

	query := `
		UPDATE bots 
		SET name = ?, status = ?, capabilities = ?, last_heartbeat = ?, updated_at = ?, metadata = ?
		WHERE id = ?
	`

	result, err := r.conn.ExecContext(ctx, query,
		agent.Name,
		agent.Status,
		string(capabilities),
		agent.LastHeartbeat,
		agent.UpdatedAt,
		metadata,
		agent.ID,
	)

	if err != nil {
		return errors.NewDatabaseError("update_agent", err).
			WithDetail("agent_id", agent.ID)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_agent_rows", err).
			WithDetail("agent_id", agent.ID)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_agent", "agent").
			WithDetail("agent_id", agent.ID)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.cacheKey(agent.ID))
		r.cache.Delete(ctx, "bots:list")
		r.cache.Clear(ctx) // Clear all status and capability caches
	}

	r.log.WithField("agent_id", agent.ID).Debug("Agent updated")
	return nil
}

// Delete deletes an agent by ID
func (r *BotRepository) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("delete_agent", "agent ID cannot be empty")
	}

	query := `DELETE FROM bots WHERE id = ?`

	result, err := r.conn.ExecContext(ctx, query, id)
	if err != nil {
		return errors.NewDatabaseError("delete_agent", err).
			WithDetail("agent_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("delete_agent_rows", err).
			WithDetail("agent_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("delete_agent", "agent").
			WithDetail("agent_id", id)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.cacheKey(id))
		r.cache.Clear(ctx) // Clear all caches
	}

	r.log.WithField("agent_id", id).Debug("Agent deleted")
	return nil
}

// FindByID retrieves an agent by its ID
func (r *BotRepository) FindByID(ctx context.Context, id string) (*types.Agent, error) {
	if id == "" {
		return nil, errors.NewValidationError("find_agent_by_id", "agent ID cannot be empty")
	}

	// Check cache first
	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, r.cacheKey(id)); found {
			if agent, ok := cached.(*types.Agent); ok {
				return agent, nil
			}
		}
	}

	query := `
		SELECT id, name, status, capabilities, last_heartbeat, created_at, updated_at, metadata
		FROM bots
		WHERE id = ?
	`

	agent, err := r.scanAgent(r.conn.QueryRowContext(ctx, query, id))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewNotFoundError("find_agent_by_id", "agent").
				WithDetail("agent_id", id)
		}
		return nil, err
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, r.cacheKey(id), agent, 5*time.Minute)
	}

	return agent, nil
}

// FindByName retrieves agents by name (partial match)
func (r *BotRepository) FindByName(ctx context.Context, name string) ([]*types.Agent, error) {
	if name == "" {
		return nil, errors.NewValidationError("find_agent_by_name", "name cannot be empty")
	}

	query := `
		SELECT id, name, status, capabilities, last_heartbeat, created_at, updated_at, metadata
		FROM bots
		WHERE name LIKE ?
		ORDER BY created_at DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, "%"+name+"%")
	if err != nil {
		return nil, errors.NewDatabaseError("find_agent_by_name", err).
			WithDetail("name", name)
	}
	defer rows.Close()

	return r.scanAgents(rows)
}

// FindByStatus retrieves all agents with a specific status
func (r *BotRepository) FindByStatus(ctx context.Context, status types.Status) ([]*types.Agent, error) {
	if err := status.Validate(); err != nil {
		return nil, errors.NewValidationError("find_agent_by_status", err.Error())
	}

	// Check cache first
	cacheKey := fmt.Sprintf("bots:status:%s", status)
	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, cacheKey); found {
			if agents, ok := cached.([]*types.Agent); ok {
				return agents, nil
			}
		}
	}

	query := `
		SELECT id, name, status, capabilities, last_heartbeat, created_at, updated_at, metadata
		FROM bots
		WHERE status = ?
		ORDER BY last_heartbeat DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, status)
	if err != nil {
		return nil, errors.NewDatabaseError("find_agent_by_status", err).
			WithDetail("status", string(status))
	}
	defer rows.Close()

	agents, err := r.scanAgents(rows)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, cacheKey, agents, 1*time.Minute)
	}

	return agents, nil
}

// FindByCapability retrieves all agents with a specific capability
func (r *BotRepository) FindByCapability(ctx context.Context, capability types.Capability) ([]*types.Agent, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("bots:capability:%s", capability)
	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, cacheKey); found {
			if agents, ok := cached.([]*types.Agent); ok {
				return agents, nil
			}
		}
	}

	// SQLite doesn't have native JSON support, so we use LIKE
	query := `
		SELECT id, name, status, capabilities, last_heartbeat, created_at, updated_at, metadata
		FROM bots
		WHERE capabilities LIKE ?
		ORDER BY last_heartbeat DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, fmt.Sprintf(`%%"%s"%%`, capability))
	if err != nil {
		return nil, errors.NewDatabaseError("find_agent_by_capability", err).
			WithDetail("capability", string(capability))
	}
	defer rows.Close()

	agents, err := r.scanAgents(rows)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, cacheKey, agents, 2*time.Minute)
	}

	return agents, nil
}

// FindOnline retrieves all online agents
func (r *BotRepository) FindOnline(ctx context.Context) ([]*types.Agent, error) {
	// Online means not offline and heartbeat within last 5 minutes
	query := `
		SELECT id, name, status, capabilities, last_heartbeat, created_at, updated_at, metadata
		FROM bots
		WHERE status != ? AND last_heartbeat > ?
		ORDER BY last_heartbeat DESC
	`

	threshold := time.Now().Add(-5 * time.Minute)
	rows, err := r.conn.QueryContext(ctx, query, types.StatusOffline, threshold)
	if err != nil {
		return nil, errors.NewDatabaseError("find_online_agents", err)
	}
	defer rows.Close()

	return r.scanAgents(rows)
}

// FindAvailable retrieves all available agents (online and idle)
func (r *BotRepository) FindAvailable(ctx context.Context) ([]*types.Agent, error) {
	query := `
		SELECT id, name, status, capabilities, last_heartbeat, created_at, updated_at, metadata
		FROM bots
		WHERE status = ? AND last_heartbeat > ?
		ORDER BY last_heartbeat DESC
	`

	threshold := time.Now().Add(-5 * time.Minute)
	rows, err := r.conn.QueryContext(ctx, query, types.StatusIdle, threshold)
	if err != nil {
		return nil, errors.NewDatabaseError("find_available_agents", err)
	}
	defer rows.Close()

	return r.scanAgents(rows)
}

// FindStale retrieves agents that haven't sent heartbeat within duration
func (r *BotRepository) FindStale(ctx context.Context, staleThreshold time.Duration) ([]*types.Agent, error) {
	query := `
		SELECT id, name, status, capabilities, last_heartbeat, created_at, updated_at, metadata
		FROM bots
		WHERE last_heartbeat < ?
		ORDER BY last_heartbeat ASC
	`

	threshold := time.Now().Add(-staleThreshold)
	rows, err := r.conn.QueryContext(ctx, query, threshold)
	if err != nil {
		return nil, errors.NewDatabaseError("find_stale_agents", err)
	}
	defer rows.Close()

	return r.scanAgents(rows)
}

// UpdateHeartbeat updates the last heartbeat time for an agent
func (r *BotRepository) UpdateHeartbeat(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("update_heartbeat", "agent ID cannot be empty")
	}

	query := `
		UPDATE bots 
		SET last_heartbeat = ?, updated_at = ?
		WHERE id = ?
	`

	now := time.Now()
	result, err := r.conn.ExecContext(ctx, query, now, now, id)
	if err != nil {
		return errors.NewDatabaseError("update_heartbeat", err).
			WithDetail("agent_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_heartbeat_rows", err).
			WithDetail("agent_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_heartbeat", "agent").
			WithDetail("agent_id", id)
	}

	// Invalidate cache for this agent
	if r.cache != nil {
		r.cache.Delete(ctx, r.cacheKey(id))
	}

	return nil
}

// UpdateStatus updates only the status of an agent
func (r *BotRepository) UpdateStatus(ctx context.Context, id string, status types.Status) error {
	if id == "" {
		return errors.NewValidationError("update_status", "agent ID cannot be empty")
	}

	if err := status.Validate(); err != nil {
		return errors.NewValidationError("update_status", err.Error())
	}

	query := `
		UPDATE bots 
		SET status = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := r.conn.ExecContext(ctx, query, status, time.Now(), id)
	if err != nil {
		return errors.NewDatabaseError("update_status", err).
			WithDetail("agent_id", id).
			WithDetail("status", string(status))
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_status_rows", err).
			WithDetail("agent_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_status", "agent").
			WithDetail("agent_id", id)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.cacheKey(id))
		r.cache.Clear(ctx) // Clear all status caches
	}

	return nil
}

// List retrieves agents with pagination
func (r *BotRepository) List(ctx context.Context, offset, limit int) ([]*types.Agent, int, error) {
	if offset < 0 {
		return nil, 0, errors.NewValidationError("list_agents", "offset cannot be negative")
	}
	if limit <= 0 {
		return nil, 0, errors.NewValidationError("list_agents", "limit must be positive")
	}

	// Get total count
	countQuery := `SELECT COUNT(*) FROM bots`
	var total int
	err := r.conn.QueryRowContext(ctx, countQuery).Scan(&total)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("count_agents", err)
	}

	// Get paginated results
	query := `
		SELECT id, name, status, capabilities, last_heartbeat, created_at, updated_at, metadata
		FROM bots
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.conn.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("list_agents", err)
	}
	defer rows.Close()

	agents, err := r.scanAgents(rows)
	if err != nil {
		return nil, 0, err
	}

	return agents, total, nil
}

// Exists checks if an agent exists by ID
func (r *BotRepository) Exists(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, errors.NewValidationError("agent_exists", "agent ID cannot be empty")
	}

	query := `SELECT 1 FROM bots WHERE id = ? LIMIT 1`

	var exists int
	err := r.conn.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, errors.NewDatabaseError("agent_exists", err).
			WithDetail("agent_id", id)
	}

	return true, nil
}

// CountByStatus counts agents by status
func (r *BotRepository) CountByStatus(ctx context.Context, status types.Status) (int, error) {
	if err := status.Validate(); err != nil {
		return 0, errors.NewValidationError("count_agents_by_status", err.Error())
	}

	query := `SELECT COUNT(*) FROM bots WHERE status = ?`

	var count int
	err := r.conn.QueryRowContext(ctx, query, status).Scan(&count)
	if err != nil {
		return 0, errors.NewDatabaseError("count_agents_by_status", err).
			WithDetail("status", string(status))
	}

	return count, nil
}

// CountByCapability counts agents by capability
func (r *BotRepository) CountByCapability(ctx context.Context, capability types.Capability) (int, error) {
	query := `SELECT COUNT(*) FROM bots WHERE capabilities LIKE ?`

	var count int
	err := r.conn.QueryRowContext(ctx, query, fmt.Sprintf(`%%"%s"%%`, capability)).Scan(&count)
	if err != nil {
		return 0, errors.NewDatabaseError("count_agents_by_capability", err).
			WithDetail("capability", string(capability))
	}

	return count, nil
}

// BeginTransaction starts a new transaction
func (r *BotRepository) BeginTransaction(ctx context.Context) (repository.AgentTransaction, error) {
	tx, err := r.conn.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.NewDatabaseError("begin_transaction", err)
	}

	return &agentTransaction{
		tx:   tx,
		repo: r,
		log:  r.log,
	}, nil
}

// scanAgent scans a single agent row
func (r *BotRepository) scanAgent(row *sql.Row) (*types.Agent, error) {
	var agent types.Agent
	var capabilitiesJSON string
	var metadataJSON sql.NullString

	err := row.Scan(
		&agent.ID,
		&agent.Name,
		&agent.Status,
		&capabilitiesJSON,
		&agent.LastHeartbeat,
		&agent.CreatedAt,
		&agent.UpdatedAt,
		&metadataJSON,
	)

	if err != nil {
		return nil, errors.NewDatabaseError("scan_agent", err)
	}

	// Unmarshal capabilities
	if err := json.Unmarshal([]byte(capabilitiesJSON), &agent.Capabilities); err != nil {
		return nil, errors.NewSystemError("unmarshal_capabilities", err)
	}

	// Unmarshal metadata if present
	if metadataJSON.Valid && metadataJSON.String != "" {
		agent.Metadata = make(map[string]interface{})
		if err := json.Unmarshal([]byte(metadataJSON.String), &agent.Metadata); err != nil {
			return nil, errors.NewSystemError("unmarshal_metadata", err)
		}
	}

	return &agent, nil
}

// scanAgents scans multiple agent rows
func (r *BotRepository) scanAgents(rows *sql.Rows) ([]*types.Agent, error) {
	var agents []*types.Agent

	for rows.Next() {
		var agent types.Agent
		var capabilitiesJSON string
		var metadataJSON sql.NullString

		err := rows.Scan(
			&agent.ID,
			&agent.Name,
			&agent.Status,
			&capabilitiesJSON,
			&agent.LastHeartbeat,
			&agent.CreatedAt,
			&agent.UpdatedAt,
			&metadataJSON,
		)
		if err != nil {
			return nil, errors.NewDatabaseError("scan_agent", err)
		}

		// Unmarshal capabilities
		if err := json.Unmarshal([]byte(capabilitiesJSON), &agent.Capabilities); err != nil {
			return nil, errors.NewSystemError("unmarshal_capabilities", err)
		}

		// Unmarshal metadata if present
		if metadataJSON.Valid && metadataJSON.String != "" {
			agent.Metadata = make(map[string]interface{})
			if err := json.Unmarshal([]byte(metadataJSON.String), &agent.Metadata); err != nil {
				return nil, errors.NewSystemError("unmarshal_metadata", err)
			}
		}

		agents = append(agents, &agent)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.NewDatabaseError("scan_agents_rows", err)
	}

	return agents, nil
}

// isUniqueConstraintError checks if an error is a unique constraint violation
func (r *BotRepository) isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	// SQLite unique constraint error contains "UNIQUE constraint failed"
	return contains(err.Error(), "UNIQUE constraint failed")
}

// cacheKey generates a cache key for an agent
func (r *BotRepository) cacheKey(id string) string {
	return fmt.Sprintf("bot:%s", id)
}

// agentTransaction implements AgentTransaction
type agentTransaction struct {
	tx   *sql.Tx
	repo *BotRepository
	log  logrus.FieldLogger
}

// Commit commits the transaction
func (t *agentTransaction) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction
func (t *agentTransaction) Rollback() error {
	return t.tx.Rollback()
}

// CreateTx creates an agent within a transaction
func (t *agentTransaction) CreateTx(ctx context.Context, agent *types.Agent) error {
	if agent == nil {
		return errors.NewValidationError("create_agent_tx", "agent cannot be nil")
	}

	if err := agent.Validate(); err != nil {
		return errors.NewValidationError("create_agent_tx", err.Error())
	}

	capabilities, err := json.Marshal(agent.Capabilities)
	if err != nil {
		return errors.NewSystemError("marshal_capabilities", err)
	}

	var metadata []byte
	if agent.Metadata != nil && len(agent.Metadata) > 0 {
		metadata, err = json.Marshal(agent.Metadata)
		if err != nil {
			return errors.NewSystemError("marshal_metadata", err)
		}
	}

	query := `
		INSERT INTO bots (id, name, status, capabilities, last_heartbeat, created_at, updated_at, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = t.tx.ExecContext(ctx, query,
		agent.ID,
		agent.Name,
		agent.Status,
		string(capabilities),
		agent.LastHeartbeat,
		agent.CreatedAt,
		agent.UpdatedAt,
		metadata,
	)

	if err != nil {
		if t.repo.isUniqueConstraintError(err) {
			return errors.NewConflictError("create_agent_tx", "agent already exists").
				WithDetail("agent_id", agent.ID)
		}
		return errors.NewDatabaseError("create_agent_tx", err).
			WithDetail("agent_id", agent.ID)
	}

	return nil
}

// UpdateTx updates an agent within a transaction
func (t *agentTransaction) UpdateTx(ctx context.Context, agent *types.Agent) error {
	if agent == nil {
		return errors.NewValidationError("update_agent_tx", "agent cannot be nil")
	}

	if err := agent.Validate(); err != nil {
		return errors.NewValidationError("update_agent_tx", err.Error())
	}

	capabilities, err := json.Marshal(agent.Capabilities)
	if err != nil {
		return errors.NewSystemError("marshal_capabilities", err)
	}

	var metadata []byte
	if agent.Metadata != nil && len(agent.Metadata) > 0 {
		metadata, err = json.Marshal(agent.Metadata)
		if err != nil {
			return errors.NewSystemError("marshal_metadata", err)
		}
	}

	query := `
		UPDATE bots 
		SET name = ?, status = ?, capabilities = ?, last_heartbeat = ?, updated_at = ?, metadata = ?
		WHERE id = ?
	`

	result, err := t.tx.ExecContext(ctx, query,
		agent.Name,
		agent.Status,
		string(capabilities),
		agent.LastHeartbeat,
		agent.UpdatedAt,
		metadata,
		agent.ID,
	)

	if err != nil {
		return errors.NewDatabaseError("update_agent_tx", err).
			WithDetail("agent_id", agent.ID)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_agent_tx_rows", err).
			WithDetail("agent_id", agent.ID)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_agent_tx", "agent").
			WithDetail("agent_id", agent.ID)
	}

	return nil
}

// DeleteTx deletes an agent within a transaction
func (t *agentTransaction) DeleteTx(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("delete_agent_tx", "agent ID cannot be empty")
	}

	query := `DELETE FROM bots WHERE id = ?`

	result, err := t.tx.ExecContext(ctx, query, id)
	if err != nil {
		return errors.NewDatabaseError("delete_agent_tx", err).
			WithDetail("agent_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("delete_agent_tx_rows", err).
			WithDetail("agent_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("delete_agent_tx", "agent").
			WithDetail("agent_id", id)
	}

	return nil
}

// UpdateHeartbeatTx updates heartbeat within a transaction
func (t *agentTransaction) UpdateHeartbeatTx(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("update_heartbeat_tx", "agent ID cannot be empty")
	}

	query := `
		UPDATE bots 
		SET last_heartbeat = ?, updated_at = ?
		WHERE id = ?
	`

	now := time.Now()
	result, err := t.tx.ExecContext(ctx, query, now, now, id)
	if err != nil {
		return errors.NewDatabaseError("update_heartbeat_tx", err).
			WithDetail("agent_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_heartbeat_tx_rows", err).
			WithDetail("agent_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_heartbeat_tx", "agent").
			WithDetail("agent_id", id)
	}

	return nil
}

// UpdateStatusTx updates status within a transaction
func (t *agentTransaction) UpdateStatusTx(ctx context.Context, id string, status types.Status) error {
	if id == "" {
		return errors.NewValidationError("update_status_tx", "agent ID cannot be empty")
	}

	if err := status.Validate(); err != nil {
		return errors.NewValidationError("update_status_tx", err.Error())
	}

	query := `
		UPDATE bots 
		SET status = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := t.tx.ExecContext(ctx, query, status, time.Now(), id)
	if err != nil {
		return errors.NewDatabaseError("update_status_tx", err).
			WithDetail("agent_id", id).
			WithDetail("status", string(status))
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_status_tx_rows", err).
			WithDetail("agent_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_status_tx", "agent").
			WithDetail("agent_id", id)
	}

	return nil
}

// Ensure interfaces are implemented
var (
	_ repository.AgentRepository            = (*BotRepository)(nil)
	_ repository.AgentTransactionRepository = (*BotRepository)(nil)
	_ repository.AgentTransaction           = (*agentTransaction)(nil)
)
