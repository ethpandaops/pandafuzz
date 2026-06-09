// Package sql provides SQL-based repository implementations for domain entities.
package sql

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
)

// BotRepository implements repository.AgentRepository using SQLiteStorage
type BotRepository struct {
	storage *storage.SQLiteStorage
}

// Compile-time interface compliance check
var _ repository.AgentRepository = (*BotRepository)(nil)

// NewBotRepository creates a new bot repository adapter
func NewBotRepository(storage *storage.SQLiteStorage) *BotRepository {
	return &BotRepository{storage: storage}
}

// Create creates a new agent
func (r *BotRepository) Create(ctx context.Context, agent *types.Agent) error {
	bot := agentToBot(agent)
	return r.storage.CreateBot(ctx, bot)
}

// Update updates an existing agent
func (r *BotRepository) Update(ctx context.Context, agent *types.Agent) error {
	updates := map[string]interface{}{
		"name":      agent.Name,
		"status":    statusToCommon(agent.Status),
		"last_seen": agent.LastHeartbeat,
		"is_online": agent.IsOnline(),
	}
	return r.storage.UpdateBot(ctx, agent.ID, updates)
}

// Delete deletes an agent by ID
func (r *BotRepository) Delete(ctx context.Context, id string) error {
	return r.storage.DeleteBot(ctx, id)
}

// FindByID retrieves an agent by its ID
func (r *BotRepository) FindByID(ctx context.Context, id string) (*types.Agent, error) {
	bot, err := r.storage.GetBot(ctx, id)
	if err != nil {
		return nil, err
	}
	return botToAgent(bot), nil
}

// FindByName retrieves agents by name (partial match)
func (r *BotRepository) FindByName(ctx context.Context, name string) ([]*types.Agent, error) {
	bots, err := r.storage.ListBots(ctx)
	if err != nil {
		return nil, err
	}

	var result []*types.Agent
	for _, bot := range bots {
		if bot.Name == name || containsSubstring(bot.Name, name) {
			result = append(result, botToAgent(bot))
		}
	}
	return result, nil
}

// FindByStatus retrieves all agents with a specific status
func (r *BotRepository) FindByStatus(ctx context.Context, status types.Status) ([]*types.Agent, error) {
	bots, err := r.storage.ListBots(ctx)
	if err != nil {
		return nil, err
	}

	var result []*types.Agent
	for _, bot := range bots {
		if statusFromCommon(bot.Status) == status {
			result = append(result, botToAgent(bot))
		}
	}
	return result, nil
}

// FindByCapability retrieves all agents with a specific capability
func (r *BotRepository) FindByCapability(ctx context.Context, capability types.Capability) ([]*types.Agent, error) {
	bots, err := r.storage.ListBots(ctx)
	if err != nil {
		return nil, err
	}

	var result []*types.Agent
	for _, bot := range bots {
		agent := botToAgent(bot)
		if agent.HasCapability(capability) {
			result = append(result, agent)
		}
	}
	return result, nil
}

// FindOnline retrieves all online agents
func (r *BotRepository) FindOnline(ctx context.Context) ([]*types.Agent, error) {
	bots, err := r.storage.ListBots(ctx)
	if err != nil {
		return nil, err
	}

	var result []*types.Agent
	for _, bot := range bots {
		if bot.IsOnline {
			result = append(result, botToAgent(bot))
		}
	}
	return result, nil
}

// FindAvailable retrieves all available agents (online and idle)
func (r *BotRepository) FindAvailable(ctx context.Context) ([]*types.Agent, error) {
	bots, err := r.storage.ListBots(ctx)
	if err != nil {
		return nil, err
	}

	var result []*types.Agent
	for _, bot := range bots {
		if bot.IsOnline && bot.Status == common.BotStatusIdle {
			result = append(result, botToAgent(bot))
		}
	}
	return result, nil
}

// FindStale retrieves agents that haven't sent heartbeat within duration
func (r *BotRepository) FindStale(ctx context.Context, staleThreshold time.Duration) ([]*types.Agent, error) {
	bots, err := r.storage.ListBots(ctx)
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-staleThreshold)
	var result []*types.Agent
	for _, bot := range bots {
		if bot.LastSeen.Before(cutoff) {
			result = append(result, botToAgent(bot))
		}
	}
	return result, nil
}

// UpdateHeartbeat updates the last heartbeat time for an agent
func (r *BotRepository) UpdateHeartbeat(ctx context.Context, id string) error {
	updates := map[string]interface{}{
		"last_seen": time.Now(),
		"is_online": true,
	}
	return r.storage.UpdateBot(ctx, id, updates)
}

// UpdateStatus updates only the status of an agent
func (r *BotRepository) UpdateStatus(ctx context.Context, id string, status types.Status) error {
	updates := map[string]interface{}{
		"status": statusToCommon(status),
	}
	return r.storage.UpdateBot(ctx, id, updates)
}

// List retrieves agents with pagination
func (r *BotRepository) List(ctx context.Context, offset, limit int) ([]*types.Agent, int, error) {
	bots, err := r.storage.ListBots(ctx)
	if err != nil {
		return nil, 0, err
	}

	total := len(bots)

	// Apply pagination
	if offset >= total {
		return []*types.Agent{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}

	result := make([]*types.Agent, 0, end-offset)
	for _, bot := range bots[offset:end] {
		result = append(result, botToAgent(bot))
	}
	return result, total, nil
}

// Exists checks if an agent exists by ID
func (r *BotRepository) Exists(ctx context.Context, id string) (bool, error) {
	bot, err := r.storage.GetBot(ctx, id)
	if err != nil {
		if err == common.ErrKeyNotFound {
			return false, nil
		}
		return false, err
	}
	return bot != nil, nil
}

// CountByStatus counts agents by status
func (r *BotRepository) CountByStatus(ctx context.Context, status types.Status) (int, error) {
	bots, err := r.storage.ListBots(ctx)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, bot := range bots {
		if statusFromCommon(bot.Status) == status {
			count++
		}
	}
	return count, nil
}

// CountByCapability counts agents by capability
func (r *BotRepository) CountByCapability(ctx context.Context, capability types.Capability) (int, error) {
	bots, err := r.storage.ListBots(ctx)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, bot := range bots {
		agent := botToAgent(bot)
		if agent.HasCapability(capability) {
			count++
		}
	}
	return count, nil
}

// Helper functions for type conversion

func agentToBot(agent *types.Agent) *common.Bot {
	caps := make([]string, len(agent.Capabilities))
	for i, cap := range agent.Capabilities {
		caps[i] = string(cap)
	}

	return &common.Bot{
		ID:           agent.ID,
		Name:         agent.Name,
		Hostname:     agent.Name, // Using name as hostname
		Status:       statusToCommon(agent.Status),
		LastSeen:     agent.LastHeartbeat,
		RegisteredAt: agent.CreatedAt,
		Capabilities: caps,
		IsOnline:     agent.IsOnline(),
	}
}

func botToAgent(bot *common.Bot) *types.Agent {
	caps := make([]types.Capability, len(bot.Capabilities))
	for i, cap := range bot.Capabilities {
		caps[i] = types.Capability(cap)
	}

	return &types.Agent{
		ID:            bot.ID,
		Name:          bot.Name,
		Status:        statusFromCommon(bot.Status),
		Capabilities:  caps,
		LastHeartbeat: bot.LastSeen,
		CreatedAt:     bot.RegisteredAt,
		UpdatedAt:     bot.LastSeen,
	}
}

func statusToCommon(status types.Status) common.BotStatus {
	switch status {
	case types.StatusIdle:
		return common.BotStatusIdle
	case types.StatusWorking:
		return common.BotStatusBusy
	case types.StatusOffline:
		return common.BotStatusTimedOut
	case types.StatusError:
		return common.BotStatusFailed
	default:
		return common.BotStatusIdle
	}
}

func statusFromCommon(status common.BotStatus) types.Status {
	switch status {
	case common.BotStatusIdle, common.BotStatusRegistering:
		return types.StatusIdle
	case common.BotStatusBusy:
		return types.StatusWorking
	case common.BotStatusTimedOut:
		return types.StatusOffline
	case common.BotStatusFailed:
		return types.StatusError
	default:
		return types.StatusIdle
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0)
}
