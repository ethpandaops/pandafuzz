package types

import (
	"errors"
	"time"
)

// Campaign represents a fuzzing campaign entity
type Campaign struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      State     `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NewCampaign creates a new Campaign with validation
func NewCampaign(id, name, description string) (*Campaign, error) {
	if id == "" {
		return nil, errors.New("campaign ID cannot be empty")
	}
	if name == "" {
		return nil, errors.New("campaign name cannot be empty")
	}

	now := time.Now()
	return &Campaign{
		ID:          id,
		Name:        name,
		Description: description,
		Status:      StateDraft,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// UpdateStatus updates the campaign status and UpdatedAt timestamp
func (c *Campaign) UpdateStatus(newStatus State) error {
	if err := newStatus.Validate(); err != nil {
		return err
	}

	if !c.Status.CanTransitionTo(newStatus) {
		return errors.New("invalid status transition")
	}

	c.Status = newStatus
	c.UpdatedAt = time.Now()
	return nil
}

// IsActive returns true if the campaign is in an active state
func (c *Campaign) IsActive() bool {
	return c.Status == StateActive
}

// CanBeModified returns true if the campaign can be modified
func (c *Campaign) CanBeModified() bool {
	return c.Status == StateDraft || c.Status == StatePaused
}

// Validate ensures the campaign has valid data
func (c *Campaign) Validate() error {
	if c.ID == "" {
		return errors.New("campaign ID cannot be empty")
	}
	if c.Name == "" {
		return errors.New("campaign name cannot be empty")
	}
	if err := c.Status.Validate(); err != nil {
		return err
	}
	return nil
}
