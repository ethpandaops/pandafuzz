package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
)

// Service provides bot registration and management functionality
type Service struct {
	repo          repository.AgentRepository
	eventPub      types.BotEventPublisher
	healthChecker *HealthChecker
	mu            sync.RWMutex

	// Configuration
	staleTimeout     time.Duration
	heartbeatTimeout time.Duration
}

// ServiceOption is a functional option for configuring the Service
type ServiceOption func(*Service)

// WithStaleTimeout sets the stale timeout duration
func WithStaleTimeout(timeout time.Duration) ServiceOption {
	return func(s *Service) {
		s.staleTimeout = timeout
	}
}

// WithHeartbeatTimeout sets the heartbeat timeout duration
func WithHeartbeatTimeout(timeout time.Duration) ServiceOption {
	return func(s *Service) {
		s.heartbeatTimeout = timeout
	}
}

// NewService creates a new bot registry service
func NewService(repo repository.AgentRepository, eventPub types.BotEventPublisher, opts ...ServiceOption) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository cannot be nil")
	}
	if eventPub == nil {
		return nil, errors.New("event publisher cannot be nil")
	}

	s := &Service{
		repo:             repo,
		eventPub:         eventPub,
		staleTimeout:     5 * time.Minute,
		heartbeatTimeout: 30 * time.Second,
	}

	// Apply options
	for _, opt := range opts {
		opt(s)
	}

	// Create health checker
	s.healthChecker = NewHealthChecker(repo, eventPub, s.staleTimeout)

	return s, nil
}

// RegisterBot registers a new bot agent
func (s *Service) RegisterBot(ctx context.Context, id, name string, capabilities []types.Capability) (*types.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if bot already exists
	existing, err := s.repo.FindByID(ctx, id)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("bot with ID %s already exists", id)
	}

	// Create new agent
	agent, err := types.NewAgent(id, name, capabilities)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	// Save to repository
	if err := s.repo.Create(ctx, agent); err != nil {
		return nil, fmt.Errorf("failed to save agent: %w", err)
	}

	// Emit registration event
	event := types.NewBotRegisteredEvent(agent)
	if err := s.eventPub.PublishEvent(event); err != nil {
		// Log error but don't fail registration
		// In production, this would be logged properly
		_ = err
	}

	return agent, nil
}

// DeregisterBot removes a bot from the registry
func (s *Service) DeregisterBot(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the bot
	agent, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("bot not found: %w", err)
	}

	// Check if bot can be deregistered
	if agent.Status == types.StatusWorking {
		return errors.New("cannot deregister bot while it's working")
	}

	// Delete from repository
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete agent: %w", err)
	}

	// Emit deregistration event
	event := &types.BaseBotEvent{
		Type:      types.EventBotDeregistered,
		BotID:     id,
		Timestamp: time.Now(),
	}
	_ = s.eventPub.PublishEvent(event)

	return nil
}

// UpdateBotStatus updates a bot's status with validation
func (s *Service) UpdateBotStatus(ctx context.Context, id string, newStatus types.Status, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the bot
	agent, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("bot not found: %w", err)
	}

	// Check if transition is valid
	if !types.IsValidTransition(agent.Status, newStatus) {
		return fmt.Errorf("invalid status transition from %s to %s", agent.Status, newStatus)
	}

	oldStatus := agent.Status

	// Update status
	if err := agent.UpdateStatus(newStatus); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Save to repository
	if err := s.repo.Update(ctx, agent); err != nil {
		return fmt.Errorf("failed to save agent: %w", err)
	}

	// Emit status change event
	event := types.NewBotStatusChangedEvent(id, oldStatus, newStatus, reason)
	_ = s.eventPub.PublishEvent(event)

	return nil
}

// RecordHeartbeat records a heartbeat from a bot
func (s *Service) RecordHeartbeat(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update heartbeat in repository
	if err := s.repo.UpdateHeartbeat(ctx, id); err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	// Emit heartbeat event
	event := &types.BaseBotEvent{
		Type:      types.EventBotHeartbeat,
		BotID:     id,
		Timestamp: time.Now(),
	}
	_ = s.eventPub.PublishEvent(event)

	return nil
}

// FindAvailableBots finds bots that are available for work
func (s *Service) FindAvailableBots(ctx context.Context) ([]*types.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.repo.FindAvailable(ctx)
}

// FindBotsByCapability finds bots with a specific capability
func (s *Service) FindBotsByCapability(ctx context.Context, capability types.Capability) ([]*types.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find bots with the capability
	agents, err := s.repo.FindByCapability(ctx, capability)
	if err != nil {
		return nil, err
	}

	// Filter for available bots
	var available []*types.Agent
	for _, agent := range agents {
		if agent.IsAvailable() {
			available = append(available, agent)
		}
	}

	return available, nil
}

// GetBot retrieves a bot by ID
func (s *Service) GetBot(ctx context.Context, id string) (*types.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.repo.FindByID(ctx, id)
}

// ListBots lists all registered bots with pagination
func (s *Service) ListBots(ctx context.Context, offset, limit int) ([]*types.Agent, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.repo.List(ctx, offset, limit)
}

// GetBotStatistics returns statistics about registered bots
func (s *Service) GetBotStatistics(ctx context.Context) (*BotStatistics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &BotStatistics{
		StatusCounts:     make(map[types.Status]int),
		CapabilityCounts: make(map[types.Capability]int),
	}

	// Count by status
	for _, status := range types.AllStatuses() {
		count, err := s.repo.CountByStatus(ctx, status)
		if err != nil {
			return nil, fmt.Errorf("failed to count by status %s: %w", status, err)
		}
		stats.StatusCounts[status] = count
		stats.TotalBots += count
	}

	// Count by capability
	capabilities := []types.Capability{
		types.CapabilityFuzzing,
		types.CapabilityAnalysis,
		types.CapabilityReporting,
		types.CapabilityCoordination,
	}

	for _, cap := range capabilities {
		count, err := s.repo.CountByCapability(ctx, cap)
		if err != nil {
			return nil, fmt.Errorf("failed to count by capability %s: %w", cap, err)
		}
		stats.CapabilityCounts[cap] = count
	}

	// Count online bots
	online, err := s.repo.FindOnline(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to find online bots: %w", err)
	}
	stats.OnlineBots = len(online)

	// Count available bots
	available, err := s.repo.FindAvailable(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to find available bots: %w", err)
	}
	stats.AvailableBots = len(available)

	return stats, nil
}

// AssignWork assigns work to a bot and updates its status
func (s *Service) AssignWork(ctx context.Context, botID, jobID, jobType string, metadata map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the bot
	agent, err := s.repo.FindByID(ctx, botID)
	if err != nil {
		return fmt.Errorf("bot not found: %w", err)
	}

	// Check if bot can accept work
	if !agent.IsAvailable() {
		return fmt.Errorf("bot %s is not available for work", botID)
	}

	// Update status to working
	if err := agent.UpdateStatus(types.StatusWorking); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Store job information in metadata
	agent.SetMetadata("current_job_id", jobID)
	agent.SetMetadata("current_job_type", jobType)
	agent.SetMetadata("job_start_time", time.Now().Format(time.RFC3339))

	// Save to repository
	if err := s.repo.Update(ctx, agent); err != nil {
		return fmt.Errorf("failed to save agent: %w", err)
	}

	// Emit work assigned event
	event := types.NewBotWorkAssignedEvent(botID, jobID, jobType, metadata)
	_ = s.eventPub.PublishEvent(event)

	return nil
}

// CompleteWork marks work as completed for a bot
func (s *Service) CompleteWork(ctx context.Context, botID, jobID string, results map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the bot
	agent, err := s.repo.FindByID(ctx, botID)
	if err != nil {
		return fmt.Errorf("bot not found: %w", err)
	}

	// Verify job ID matches
	currentJobID, exists := agent.GetMetadata("current_job_id")
	if !exists || currentJobID != jobID {
		return fmt.Errorf("job ID mismatch for bot %s", botID)
	}

	// Calculate duration
	startTimeStr, _ := agent.GetMetadata("job_start_time")
	var duration time.Duration
	if startTime, err := time.Parse(time.RFC3339, startTimeStr.(string)); err == nil {
		duration = time.Since(startTime)
	}

	// Update status to idle
	if err := agent.UpdateStatus(types.StatusIdle); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Clear job metadata
	agent.SetMetadata("current_job_id", nil)
	agent.SetMetadata("current_job_type", nil)
	agent.SetMetadata("job_start_time", nil)
	agent.SetMetadata("last_job_id", jobID)
	agent.SetMetadata("last_job_completed", time.Now().Format(time.RFC3339))

	// Save to repository
	if err := s.repo.Update(ctx, agent); err != nil {
		return fmt.Errorf("failed to save agent: %w", err)
	}

	// Emit work completed event
	event := types.NewBotWorkCompletedEvent(botID, jobID, duration, results)
	_ = s.eventPub.PublishEvent(event)

	return nil
}

// FailWork marks work as failed for a bot
func (s *Service) FailWork(ctx context.Context, botID, jobID, errorMsg, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the bot
	agent, err := s.repo.FindByID(ctx, botID)
	if err != nil {
		return fmt.Errorf("bot not found: %w", err)
	}

	// Update status to error
	if err := agent.UpdateStatus(types.StatusError); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Store error information
	agent.SetMetadata("last_error", errorMsg)
	agent.SetMetadata("last_error_time", time.Now().Format(time.RFC3339))

	// Save to repository
	if err := s.repo.Update(ctx, agent); err != nil {
		return fmt.Errorf("failed to save agent: %w", err)
	}

	// Emit work failed event
	event := types.NewBotWorkFailedEvent(botID, jobID, errorMsg, reason)
	_ = s.eventPub.PublishEvent(event)

	return nil
}

// StartHealthChecking starts the health checking process
func (s *Service) StartHealthChecking(ctx context.Context, interval time.Duration) {
	s.healthChecker.Start(ctx, interval)
}

// StopHealthChecking stops the health checking process
func (s *Service) StopHealthChecking() {
	s.healthChecker.Stop()
}

// BotStatistics holds statistical information about bots
type BotStatistics struct {
	TotalBots        int                      `json:"total_bots"`
	OnlineBots       int                      `json:"online_bots"`
	AvailableBots    int                      `json:"available_bots"`
	StatusCounts     map[types.Status]int     `json:"status_counts"`
	CapabilityCounts map[types.Capability]int `json:"capability_counts"`
}
