package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	campaignrepo "github.com/ethpandaops/pandafuzz/pkg/domain/campaign/repository"
	campaigntypes "github.com/ethpandaops/pandafuzz/pkg/domain/campaign/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/mappers"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/models"
	"github.com/sirupsen/logrus"
)

// Compile-time interface compliance check
var _ campaignrepo.CampaignRepository = (*CampaignRepository)(nil)

// CampaignRepository implements campaignrepo.CampaignRepository using SQLite
type CampaignRepository struct {
	db     *sql.DB
	logger logrus.FieldLogger
}

// NewCampaignRepository creates a new SQLite-based campaign repository
func NewCampaignRepository(db *sql.DB, logger logrus.FieldLogger) *CampaignRepository {
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}
	return &CampaignRepository{
		db:     db,
		logger: logger.WithField("component", "campaign_repository"),
	}
}

// Create creates a new campaign
func (r *CampaignRepository) Create(ctx context.Context, campaign *campaigntypes.Campaign) error {
	if campaign == nil {
		return NewRepositoryError("create", "campaign", "", fmt.Errorf("campaign cannot be nil"))
	}

	row := mappers.DomainCampaignToRow(campaign)

	query := `
		INSERT INTO campaigns (
			id, name, description, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		row.ID, row.Name, row.Description, row.Status, row.CreatedAt, row.UpdatedAt,
	)

	if err != nil {
		return NewRepositoryError("create", "campaign", campaign.ID, err)
	}

	r.logger.WithField("campaign_id", campaign.ID).Debug("Campaign created")
	return nil
}

// Update updates an existing campaign
func (r *CampaignRepository) Update(ctx context.Context, campaign *campaigntypes.Campaign) error {
	if campaign == nil {
		return NewRepositoryError("update", "campaign", "", fmt.Errorf("campaign cannot be nil"))
	}

	row := mappers.DomainCampaignToRow(campaign)

	query := `
		UPDATE campaigns SET
			name = ?, description = ?, status = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query,
		row.Name, row.Description, row.Status, time.Now().UTC(),
		campaign.ID,
	)

	if err != nil {
		return NewRepositoryError("update", "campaign", campaign.ID, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("update", "campaign", campaign.ID, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("campaign_id", campaign.ID).Debug("Campaign updated")
	return nil
}

// Delete deletes a campaign by ID
func (r *CampaignRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM campaigns WHERE id = ?", id)
	if err != nil {
		return NewRepositoryError("delete", "campaign", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return NewRepositoryError("delete", "campaign", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	r.logger.WithField("campaign_id", id).Debug("Campaign deleted")
	return nil
}

// FindByID retrieves a campaign by its ID
func (r *CampaignRepository) FindByID(ctx context.Context, id string) (*campaigntypes.Campaign, error) {
	row, err := r.scanCampaignRow(ctx, "SELECT "+campaignSelectColumns()+" FROM campaigns WHERE id = ?", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, NewRepositoryError("find_by_id", "campaign", id, err)
	}

	return mappers.CampaignRowToDomain(row), nil
}

// FindByName retrieves campaigns by name (partial match)
func (r *CampaignRepository) FindByName(ctx context.Context, name string) ([]*campaigntypes.Campaign, error) {
	query := "SELECT " + campaignSelectColumns() + " FROM campaigns WHERE name LIKE ? ORDER BY name ASC"
	return r.scanCampaignRows(ctx, query, "%"+name+"%")
}

// FindByStatus retrieves all campaigns with a specific status
func (r *CampaignRepository) FindByStatus(ctx context.Context, status campaigntypes.State) ([]*campaigntypes.Campaign, error) {
	query := "SELECT " + campaignSelectColumns() + " FROM campaigns WHERE status = ? ORDER BY created_at DESC"
	return r.scanCampaignRows(ctx, query, string(status))
}

// FindActive retrieves all active campaigns
func (r *CampaignRepository) FindActive(ctx context.Context) ([]*campaigntypes.Campaign, error) {
	return r.FindByStatus(ctx, campaigntypes.StateActive)
}

// List retrieves campaigns with pagination
func (r *CampaignRepository) List(ctx context.Context, offset, limit int) ([]*campaigntypes.Campaign, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM campaigns").Scan(&total)
	if err != nil {
		return nil, 0, NewRepositoryError("list", "campaign", "", err)
	}

	query := "SELECT " + campaignSelectColumns() + " FROM campaigns ORDER BY created_at DESC LIMIT ? OFFSET ?"
	campaigns, err := r.scanCampaignRows(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	return campaigns, total, nil
}

// Exists checks if a campaign exists by ID
func (r *CampaignRepository) Exists(ctx context.Context, id string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM campaigns WHERE id = ?)"
	err := r.db.QueryRowContext(ctx, query, id).Scan(&exists)
	if err != nil {
		return false, NewRepositoryError("exists", "campaign", id, err)
	}
	return exists, nil
}

// CountByStatus counts campaigns by status
func (r *CampaignRepository) CountByStatus(ctx context.Context, status campaigntypes.State) (int, error) {
	var count int
	query := "SELECT COUNT(*) FROM campaigns WHERE status = ?"
	err := r.db.QueryRowContext(ctx, query, string(status)).Scan(&count)
	if err != nil {
		return 0, NewRepositoryError("count_by_status", "campaign", "", err)
	}
	return count, nil
}

// Helper functions

func campaignSelectColumns() string {
	return "id, name, description, status, created_at, updated_at"
}

func (r *CampaignRepository) scanCampaignRow(ctx context.Context, query string, args ...interface{}) (*models.DomainCampaignRow, error) {
	row := r.db.QueryRowContext(ctx, query, args...)
	return r.scanSingleCampaignRow(row)
}

func (r *CampaignRepository) scanCampaignRows(ctx context.Context, query string, args ...interface{}) ([]*campaigntypes.Campaign, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	campaigns := make([]*campaigntypes.Campaign, 0, 16)
	for rows.Next() {
		campaignRow, err := r.scanRowsCampaignRow(rows)
		if err != nil {
			return nil, err
		}
		campaigns = append(campaigns, mappers.CampaignRowToDomain(campaignRow))
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return campaigns, nil
}

func (r *CampaignRepository) scanSingleCampaignRow(row *sql.Row) (*models.DomainCampaignRow, error) {
	cr := &models.DomainCampaignRow{}

	err := row.Scan(
		&cr.ID, &cr.Name, &cr.Description, &cr.Status, &cr.CreatedAt, &cr.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return cr, nil
}

func (r *CampaignRepository) scanRowsCampaignRow(rows *sql.Rows) (*models.DomainCampaignRow, error) {
	cr := &models.DomainCampaignRow{}

	err := rows.Scan(
		&cr.ID, &cr.Name, &cr.Description, &cr.Status, &cr.CreatedAt, &cr.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return cr, nil
}
