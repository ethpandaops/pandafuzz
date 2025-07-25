package repository

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
)

// AgentRepository defines the interface for bot agent persistence
type AgentRepository interface {
	// Create creates a new agent
	Create(ctx context.Context, agent *types.Agent) error

	// Update updates an existing agent
	Update(ctx context.Context, agent *types.Agent) error

	// Delete deletes an agent by ID
	Delete(ctx context.Context, id string) error

	// FindByID retrieves an agent by its ID
	FindByID(ctx context.Context, id string) (*types.Agent, error)

	// FindByName retrieves agents by name (partial match)
	FindByName(ctx context.Context, name string) ([]*types.Agent, error)

	// FindByStatus retrieves all agents with a specific status
	FindByStatus(ctx context.Context, status types.Status) ([]*types.Agent, error)

	// FindByCapability retrieves all agents with a specific capability
	FindByCapability(ctx context.Context, capability types.Capability) ([]*types.Agent, error)

	// FindOnline retrieves all online agents
	FindOnline(ctx context.Context) ([]*types.Agent, error)

	// FindAvailable retrieves all available agents (online and idle)
	FindAvailable(ctx context.Context) ([]*types.Agent, error)

	// FindStale retrieves agents that haven't sent heartbeat within duration
	FindStale(ctx context.Context, staleThreshold time.Duration) ([]*types.Agent, error)

	// UpdateHeartbeat updates the last heartbeat time for an agent
	UpdateHeartbeat(ctx context.Context, id string) error

	// UpdateStatus updates only the status of an agent
	UpdateStatus(ctx context.Context, id string, status types.Status) error

	// List retrieves agents with pagination
	List(ctx context.Context, offset, limit int) ([]*types.Agent, int, error)

	// Exists checks if an agent exists by ID
	Exists(ctx context.Context, id string) (bool, error)

	// CountByStatus counts agents by status
	CountByStatus(ctx context.Context, status types.Status) (int, error)

	// CountByCapability counts agents by capability
	CountByCapability(ctx context.Context, capability types.Capability) (int, error)
}

// AgentTransactionRepository extends AgentRepository with transaction support
type AgentTransactionRepository interface {
	AgentRepository

	// BeginTransaction starts a new transaction
	BeginTransaction(ctx context.Context) (AgentTransaction, error)
}

// AgentTransaction represents an agent repository transaction
type AgentTransaction interface {
	// Commit commits the transaction
	Commit() error

	// Rollback rolls back the transaction
	Rollback() error

	// Agent operations within transaction
	CreateTx(ctx context.Context, agent *types.Agent) error
	UpdateTx(ctx context.Context, agent *types.Agent) error
	DeleteTx(ctx context.Context, id string) error
	UpdateHeartbeatTx(ctx context.Context, id string) error
	UpdateStatusTx(ctx context.Context, id string, status types.Status) error
}
