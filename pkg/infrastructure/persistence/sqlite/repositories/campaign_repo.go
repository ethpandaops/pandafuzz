package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/cache"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/sqlite"
)

// CampaignRepository implements the campaign repository interface using SQLite
type CampaignRepository struct {
	conn  *sqlite.Connection
	cache cache.Cache
	log   logrus.FieldLogger
}

// NewCampaignRepository creates a new campaign repository
func NewCampaignRepository(conn *sqlite.Connection, cache cache.Cache, log logrus.FieldLogger) (*CampaignRepository, error) {
	if conn == nil {
		return nil, errors.New(errors.ErrorTypeDatabase, "new_campaign_repository", "connection is required")
	}
	if log == nil {
		log = logrus.New()
	}

	repo := &CampaignRepository{
		conn:  conn,
		cache: cache,
		log:   log.WithField("component", "campaign_repository"),
	}

	if err := repo.createSchema(); err != nil {
		return nil, err
	}

	return repo, nil
}

// createSchema creates the campaigns table if it doesn't exist
func (r *CampaignRepository) createSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS campaigns (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_campaigns_name ON campaigns(name);
		CREATE INDEX IF NOT EXISTS idx_campaigns_status ON campaigns(status);
		CREATE INDEX IF NOT EXISTS idx_campaigns_created_at ON campaigns(created_at);
	`

	_, err := r.conn.ExecContext(context.Background(), schema)
	if err != nil {
		return errors.NewDatabaseError("create_campaign_schema", err)
	}

	return nil
}

// Create creates a new campaign
func (r *CampaignRepository) Create(ctx context.Context, campaign *types.Campaign) error {
	if campaign == nil {
		return errors.NewValidationError("create_campaign", "campaign cannot be nil")
	}

	if err := campaign.Validate(); err != nil {
		return errors.NewValidationError("create_campaign", err.Error())
	}

	query := `
		INSERT INTO campaigns (id, name, description, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := r.conn.ExecContext(ctx, query,
		campaign.ID,
		campaign.Name,
		campaign.Description,
		campaign.Status,
		campaign.CreatedAt,
		campaign.UpdatedAt,
	)

	if err != nil {
		if r.isUniqueConstraintError(err) {
			return errors.NewConflictError("create_campaign", "campaign already exists").
				WithDetail("campaign_id", campaign.ID)
		}
		return errors.NewDatabaseError("create_campaign", err).
			WithDetail("campaign_id", campaign.ID)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.cacheKey(campaign.ID))
		r.cache.Delete(ctx, "campaigns:list")
		r.cache.Delete(ctx, fmt.Sprintf("campaigns:status:%s", campaign.Status))
	}

	r.log.WithField("campaign_id", campaign.ID).Debug("Campaign created")
	return nil
}

// Update updates an existing campaign
func (r *CampaignRepository) Update(ctx context.Context, campaign *types.Campaign) error {
	if campaign == nil {
		return errors.NewValidationError("update_campaign", "campaign cannot be nil")
	}

	if err := campaign.Validate(); err != nil {
		return errors.NewValidationError("update_campaign", err.Error())
	}

	query := `
		UPDATE campaigns 
		SET name = ?, description = ?, status = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := r.conn.ExecContext(ctx, query,
		campaign.Name,
		campaign.Description,
		campaign.Status,
		campaign.UpdatedAt,
		campaign.ID,
	)

	if err != nil {
		return errors.NewDatabaseError("update_campaign", err).
			WithDetail("campaign_id", campaign.ID)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_campaign_rows", err).
			WithDetail("campaign_id", campaign.ID)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_campaign", "campaign").
			WithDetail("campaign_id", campaign.ID)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.cacheKey(campaign.ID))
		r.cache.Delete(ctx, "campaigns:list")
		r.cache.Delete(ctx, fmt.Sprintf("campaigns:status:%s", campaign.Status))
	}

	r.log.WithField("campaign_id", campaign.ID).Debug("Campaign updated")
	return nil
}

// Delete deletes a campaign by ID
func (r *CampaignRepository) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("delete_campaign", "campaign ID cannot be empty")
	}

	query := `DELETE FROM campaigns WHERE id = ?`

	result, err := r.conn.ExecContext(ctx, query, id)
	if err != nil {
		return errors.NewDatabaseError("delete_campaign", err).
			WithDetail("campaign_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("delete_campaign_rows", err).
			WithDetail("campaign_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("delete_campaign", "campaign").
			WithDetail("campaign_id", id)
	}

	// Invalidate cache
	if r.cache != nil {
		r.cache.Delete(ctx, r.cacheKey(id))
		r.cache.Delete(ctx, "campaigns:list")
		r.cache.Clear(ctx) // Clear all status caches
	}

	r.log.WithField("campaign_id", id).Debug("Campaign deleted")
	return nil
}

// FindByID retrieves a campaign by its ID
func (r *CampaignRepository) FindByID(ctx context.Context, id string) (*types.Campaign, error) {
	if id == "" {
		return nil, errors.NewValidationError("find_campaign_by_id", "campaign ID cannot be empty")
	}

	// Check cache first
	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, r.cacheKey(id)); found {
			if campaign, ok := cached.(*types.Campaign); ok {
				return campaign, nil
			}
		}
	}

	query := `
		SELECT id, name, description, status, created_at, updated_at
		FROM campaigns
		WHERE id = ?
	`

	var campaign types.Campaign
	err := r.conn.QueryRowContext(ctx, query, id).Scan(
		&campaign.ID,
		&campaign.Name,
		&campaign.Description,
		&campaign.Status,
		&campaign.CreatedAt,
		&campaign.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewNotFoundError("find_campaign_by_id", "campaign").
				WithDetail("campaign_id", id)
		}
		return nil, errors.NewDatabaseError("find_campaign_by_id", err).
			WithDetail("campaign_id", id)
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, r.cacheKey(id), &campaign, 5*time.Minute)
	}

	return &campaign, nil
}

// FindByName retrieves campaigns by name (partial match)
func (r *CampaignRepository) FindByName(ctx context.Context, name string) ([]*types.Campaign, error) {
	if name == "" {
		return nil, errors.NewValidationError("find_campaign_by_name", "name cannot be empty")
	}

	query := `
		SELECT id, name, description, status, created_at, updated_at
		FROM campaigns
		WHERE name LIKE ?
		ORDER BY created_at DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, "%"+name+"%")
	if err != nil {
		return nil, errors.NewDatabaseError("find_campaign_by_name", err).
			WithDetail("name", name)
	}
	defer rows.Close()

	return r.scanCampaigns(rows)
}

// FindByStatus retrieves all campaigns with a specific status
func (r *CampaignRepository) FindByStatus(ctx context.Context, status types.State) ([]*types.Campaign, error) {
	if err := status.Validate(); err != nil {
		return nil, errors.NewValidationError("find_campaign_by_status", err.Error())
	}

	// Check cache first
	cacheKey := fmt.Sprintf("campaigns:status:%s", status)
	if r.cache != nil {
		if cached, found := r.cache.Get(ctx, cacheKey); found {
			if campaigns, ok := cached.([]*types.Campaign); ok {
				return campaigns, nil
			}
		}
	}

	query := `
		SELECT id, name, description, status, created_at, updated_at
		FROM campaigns
		WHERE status = ?
		ORDER BY created_at DESC
	`

	rows, err := r.conn.QueryContext(ctx, query, status)
	if err != nil {
		return nil, errors.NewDatabaseError("find_campaign_by_status", err).
			WithDetail("status", string(status))
	}
	defer rows.Close()

	campaigns, err := r.scanCampaigns(rows)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if r.cache != nil {
		r.cache.SetWithTTL(ctx, cacheKey, campaigns, 2*time.Minute)
	}

	return campaigns, nil
}

// FindActive retrieves all active campaigns
func (r *CampaignRepository) FindActive(ctx context.Context) ([]*types.Campaign, error) {
	return r.FindByStatus(ctx, types.StateActive)
}

// List retrieves campaigns with pagination
func (r *CampaignRepository) List(ctx context.Context, offset, limit int) ([]*types.Campaign, int, error) {
	if offset < 0 {
		return nil, 0, errors.NewValidationError("list_campaigns", "offset cannot be negative")
	}
	if limit <= 0 {
		return nil, 0, errors.NewValidationError("list_campaigns", "limit must be positive")
	}

	// Get total count
	countQuery := `SELECT COUNT(*) FROM campaigns`
	var total int
	err := r.conn.QueryRowContext(ctx, countQuery).Scan(&total)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("count_campaigns", err)
	}

	// Get paginated results
	query := `
		SELECT id, name, description, status, created_at, updated_at
		FROM campaigns
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`

	rows, err := r.conn.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, errors.NewDatabaseError("list_campaigns", err)
	}
	defer rows.Close()

	campaigns, err := r.scanCampaigns(rows)
	if err != nil {
		return nil, 0, err
	}

	return campaigns, total, nil
}

// Exists checks if a campaign exists by ID
func (r *CampaignRepository) Exists(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, errors.NewValidationError("campaign_exists", "campaign ID cannot be empty")
	}

	query := `SELECT 1 FROM campaigns WHERE id = ? LIMIT 1`

	var exists int
	err := r.conn.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, errors.NewDatabaseError("campaign_exists", err).
			WithDetail("campaign_id", id)
	}

	return true, nil
}

// CountByStatus counts campaigns by status
func (r *CampaignRepository) CountByStatus(ctx context.Context, status types.State) (int, error) {
	if err := status.Validate(); err != nil {
		return 0, errors.NewValidationError("count_campaigns_by_status", err.Error())
	}

	query := `SELECT COUNT(*) FROM campaigns WHERE status = ?`

	var count int
	err := r.conn.QueryRowContext(ctx, query, status).Scan(&count)
	if err != nil {
		return 0, errors.NewDatabaseError("count_campaigns_by_status", err).
			WithDetail("status", string(status))
	}

	return count, nil
}

// BeginTransaction starts a new transaction
func (r *CampaignRepository) BeginTransaction(ctx context.Context) (repository.CampaignTransaction, error) {
	tx, err := r.conn.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.NewDatabaseError("begin_transaction", err)
	}

	return &campaignTransaction{
		tx:   tx,
		repo: r,
		log:  r.log,
	}, nil
}

// scanCampaigns scans multiple campaign rows
func (r *CampaignRepository) scanCampaigns(rows *sql.Rows) ([]*types.Campaign, error) {
	var campaigns []*types.Campaign

	for rows.Next() {
		var campaign types.Campaign
		err := rows.Scan(
			&campaign.ID,
			&campaign.Name,
			&campaign.Description,
			&campaign.Status,
			&campaign.CreatedAt,
			&campaign.UpdatedAt,
		)
		if err != nil {
			return nil, errors.NewDatabaseError("scan_campaign", err)
		}
		campaigns = append(campaigns, &campaign)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.NewDatabaseError("scan_campaigns_rows", err)
	}

	return campaigns, nil
}

// isUniqueConstraintError checks if an error is a unique constraint violation
func (r *CampaignRepository) isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	// SQLite unique constraint error contains "UNIQUE constraint failed"
	return contains(err.Error(), "UNIQUE constraint failed")
}

// cacheKey generates a cache key for a campaign
func (r *CampaignRepository) cacheKey(id string) string {
	return fmt.Sprintf("campaign:%s", id)
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// campaignTransaction implements CampaignTransaction
type campaignTransaction struct {
	tx   *sql.Tx
	repo *CampaignRepository
	log  logrus.FieldLogger
}

// Commit commits the transaction
func (t *campaignTransaction) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction
func (t *campaignTransaction) Rollback() error {
	return t.tx.Rollback()
}

// CreateTx creates a campaign within a transaction
func (t *campaignTransaction) CreateTx(ctx context.Context, campaign *types.Campaign) error {
	if campaign == nil {
		return errors.NewValidationError("create_campaign_tx", "campaign cannot be nil")
	}

	if err := campaign.Validate(); err != nil {
		return errors.NewValidationError("create_campaign_tx", err.Error())
	}

	query := `
		INSERT INTO campaigns (id, name, description, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := t.tx.ExecContext(ctx, query,
		campaign.ID,
		campaign.Name,
		campaign.Description,
		campaign.Status,
		campaign.CreatedAt,
		campaign.UpdatedAt,
	)

	if err != nil {
		if t.repo.isUniqueConstraintError(err) {
			return errors.NewConflictError("create_campaign_tx", "campaign already exists").
				WithDetail("campaign_id", campaign.ID)
		}
		return errors.NewDatabaseError("create_campaign_tx", err).
			WithDetail("campaign_id", campaign.ID)
	}

	return nil
}

// UpdateTx updates a campaign within a transaction
func (t *campaignTransaction) UpdateTx(ctx context.Context, campaign *types.Campaign) error {
	if campaign == nil {
		return errors.NewValidationError("update_campaign_tx", "campaign cannot be nil")
	}

	if err := campaign.Validate(); err != nil {
		return errors.NewValidationError("update_campaign_tx", err.Error())
	}

	query := `
		UPDATE campaigns 
		SET name = ?, description = ?, status = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := t.tx.ExecContext(ctx, query,
		campaign.Name,
		campaign.Description,
		campaign.Status,
		campaign.UpdatedAt,
		campaign.ID,
	)

	if err != nil {
		return errors.NewDatabaseError("update_campaign_tx", err).
			WithDetail("campaign_id", campaign.ID)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("update_campaign_tx_rows", err).
			WithDetail("campaign_id", campaign.ID)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("update_campaign_tx", "campaign").
			WithDetail("campaign_id", campaign.ID)
	}

	return nil
}

// DeleteTx deletes a campaign within a transaction
func (t *campaignTransaction) DeleteTx(ctx context.Context, id string) error {
	if id == "" {
		return errors.NewValidationError("delete_campaign_tx", "campaign ID cannot be empty")
	}

	query := `DELETE FROM campaigns WHERE id = ?`

	result, err := t.tx.ExecContext(ctx, query, id)
	if err != nil {
		return errors.NewDatabaseError("delete_campaign_tx", err).
			WithDetail("campaign_id", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return errors.NewDatabaseError("delete_campaign_tx_rows", err).
			WithDetail("campaign_id", id)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError("delete_campaign_tx", "campaign").
			WithDetail("campaign_id", id)
	}

	return nil
}

// Ensure interfaces are implemented
var (
	_ repository.CampaignRepository            = (*CampaignRepository)(nil)
	_ repository.CampaignTransactionRepository = (*CampaignRepository)(nil)
	_ repository.CampaignTransaction           = (*campaignTransaction)(nil)
)
