package repository

import (
	"context"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
)

// CampaignRepository defines the interface for campaign persistence
type CampaignRepository interface {
	// Create creates a new campaign
	Create(ctx context.Context, campaign *types.Campaign) error

	// Update updates an existing campaign
	Update(ctx context.Context, campaign *types.Campaign) error

	// Delete deletes a campaign by ID
	Delete(ctx context.Context, id string) error

	// FindByID retrieves a campaign by its ID
	FindByID(ctx context.Context, id string) (*types.Campaign, error)

	// FindByName retrieves campaigns by name (partial match)
	FindByName(ctx context.Context, name string) ([]*types.Campaign, error)

	// FindByStatus retrieves all campaigns with a specific status
	FindByStatus(ctx context.Context, status types.State) ([]*types.Campaign, error)

	// FindActive retrieves all active campaigns
	FindActive(ctx context.Context) ([]*types.Campaign, error)

	// List retrieves campaigns with pagination
	List(ctx context.Context, offset, limit int) ([]*types.Campaign, int, error)

	// Exists checks if a campaign exists by ID
	Exists(ctx context.Context, id string) (bool, error)

	// CountByStatus counts campaigns by status
	CountByStatus(ctx context.Context, status types.State) (int, error)
}

// CampaignTransactionRepository extends CampaignRepository with transaction support
type CampaignTransactionRepository interface {
	CampaignRepository

	// BeginTransaction starts a new transaction
	BeginTransaction(ctx context.Context) (CampaignTransaction, error)
}

// CampaignTransaction represents a campaign repository transaction
type CampaignTransaction interface {
	// Commit commits the transaction
	Commit() error

	// Rollback rolls back the transaction
	Rollback() error

	// Campaign operations within transaction
	CreateTx(ctx context.Context, campaign *types.Campaign) error
	UpdateTx(ctx context.Context, campaign *types.Campaign) error
	DeleteTx(ctx context.Context, id string) error
}
