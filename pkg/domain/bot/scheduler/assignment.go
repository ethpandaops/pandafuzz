package scheduler

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
	jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

// WorkUnit represents a unit of work to be assigned to a bot
type WorkUnit struct {
	ID            string
	Job           *jobtypes.Job
	AssignedBotID string
	AssignedAt    time.Time
	Priority      jobtypes.JobPriority
	Retries       int
	MaxRetries    int
	Status        WorkUnitStatus
	Error         error
}

// WorkUnitStatus represents the status of a work unit
type WorkUnitStatus string

const (
	WorkUnitStatusPending   WorkUnitStatus = "pending"
	WorkUnitStatusAssigned  WorkUnitStatus = "assigned"
	WorkUnitStatusRunning   WorkUnitStatus = "running"
	WorkUnitStatusCompleted WorkUnitStatus = "completed"
	WorkUnitStatusFailed    WorkUnitStatus = "failed"
)

// WorkAssignment handles assignment of work units to bots
type WorkAssignment struct {
	mu             sync.RWMutex
	pendingWork    []WorkUnit
	assignedWork   map[string]*WorkUnit // BotID -> WorkUnit
	completedWork  []WorkUnit
	loadBalancer   LoadBalancer
	eventPublisher types.BotEventPublisher
	botRegistry    BotRegistry
}

// BotRegistry interface for accessing bot information
type BotRegistry interface {
	GetBot(botID string) (*types.Agent, error)
	GetAvailableBots() ([]*types.Agent, error)
	UpdateBotStatus(botID string, status types.Status) error
}

// NewWorkAssignment creates a new work assignment manager
func NewWorkAssignment(loadBalancer LoadBalancer, eventPublisher types.BotEventPublisher, botRegistry BotRegistry) (*WorkAssignment, error) {
	if loadBalancer == nil {
		return nil, errors.New("load balancer cannot be nil")
	}
	if eventPublisher == nil {
		return nil, errors.New("event publisher cannot be nil")
	}
	if botRegistry == nil {
		return nil, errors.New("bot registry cannot be nil")
	}

	return &WorkAssignment{
		pendingWork:    make([]WorkUnit, 0),
		assignedWork:   make(map[string]*WorkUnit),
		completedWork:  make([]WorkUnit, 0),
		loadBalancer:   loadBalancer,
		eventPublisher: eventPublisher,
		botRegistry:    botRegistry,
	}, nil
}

// AddWorkUnit adds a new work unit to the pending queue
func (wa *WorkAssignment) AddWorkUnit(job *jobtypes.Job) (*WorkUnit, error) {
	wa.mu.Lock()
	defer wa.mu.Unlock()

	if job == nil {
		return nil, errors.New("job cannot be nil")
	}

	workUnit := WorkUnit{
		ID:         fmt.Sprintf("work_%s_%d", job.ID, time.Now().UnixNano()),
		Job:        job,
		Priority:   job.Priority,
		Status:     WorkUnitStatusPending,
		MaxRetries: 3,
	}

	// Insert work unit based on priority
	inserted := false
	for i, wu := range wa.pendingWork {
		if workUnit.Priority > wu.Priority {
			wa.pendingWork = append(wa.pendingWork[:i], append([]WorkUnit{workUnit}, wa.pendingWork[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		wa.pendingWork = append(wa.pendingWork, workUnit)
	}

	return &workUnit, nil
}

// AssignWork assigns pending work to available bots
func (wa *WorkAssignment) AssignWork() error {
	wa.mu.Lock()
	defer wa.mu.Unlock()

	if len(wa.pendingWork) == 0 {
		return nil
	}

	availableBots, err := wa.botRegistry.GetAvailableBots()
	if err != nil {
		return fmt.Errorf("failed to get available bots: %w", err)
	}

	if len(availableBots) == 0 {
		return nil
	}

	assignments := make([]Assignment, 0)
	assignedIndices := make([]int, 0)

	// Try to assign each pending work unit
	for i, workUnit := range wa.pendingWork {
		bot := wa.loadBalancer.SelectBot(availableBots, &workUnit)
		if bot == nil {
			continue
		}

		// Check if bot has required capabilities
		if !wa.canBotHandleWork(bot, &workUnit) {
			continue
		}

		assignment := Assignment{
			WorkUnit: workUnit,
			BotID:    bot.ID,
			Bot:      bot,
		}
		assignments = append(assignments, assignment)
		assignedIndices = append(assignedIndices, i)
	}

	// Process assignments
	for i, assignment := range assignments {
		if err := wa.processAssignment(assignment); err != nil {
			// Log error but continue with other assignments
			fmt.Printf("Failed to process assignment: %v\n", err)
			continue
		}

		// Update load balancer with assignment
		wa.loadBalancer.UpdateLoad(assignment.BotID, assignment.WorkUnit)
	}

	// Remove assigned work from pending queue (process in reverse order)
	for i := len(assignedIndices) - 1; i >= 0; i-- {
		idx := assignedIndices[i]
		wa.pendingWork = append(wa.pendingWork[:idx], wa.pendingWork[idx+1:]...)
	}

	return nil
}

// processAssignment handles the assignment of work to a bot
func (wa *WorkAssignment) processAssignment(assignment Assignment) error {
	workUnit := assignment.WorkUnit
	workUnit.AssignedBotID = assignment.BotID
	workUnit.AssignedAt = time.Now()
	workUnit.Status = WorkUnitStatusAssigned

	// Store assigned work
	wa.assignedWork[assignment.BotID] = &workUnit

	// Update bot status
	if err := wa.botRegistry.UpdateBotStatus(assignment.BotID, types.StatusWorking); err != nil {
		return fmt.Errorf("failed to update bot status: %w", err)
	}

	// Emit work assigned event
	event := types.NewBotWorkAssignedEvent(
		assignment.BotID,
		workUnit.Job.ID,
		workUnit.Job.FuzzerType,
		map[string]interface{}{
			"work_unit_id": workUnit.ID,
			"priority":     workUnit.Priority.String(),
			"job_name":     workUnit.Job.Name,
		},
	)

	if err := wa.eventPublisher.PublishEvent(event); err != nil {
		return fmt.Errorf("failed to publish work assigned event: %w", err)
	}

	return nil
}

// CompleteWork marks work as completed for a bot
func (wa *WorkAssignment) CompleteWork(botID string, results map[string]interface{}) error {
	wa.mu.Lock()
	defer wa.mu.Unlock()

	workUnit, exists := wa.assignedWork[botID]
	if !exists {
		return fmt.Errorf("no work assigned to bot %s", botID)
	}

	workUnit.Status = WorkUnitStatusCompleted
	duration := time.Since(workUnit.AssignedAt)

	// Move to completed
	wa.completedWork = append(wa.completedWork, *workUnit)
	delete(wa.assignedWork, botID)

	// Update bot status
	if err := wa.botRegistry.UpdateBotStatus(botID, types.StatusIdle); err != nil {
		return fmt.Errorf("failed to update bot status: %w", err)
	}

	// Update load balancer
	wa.loadBalancer.ReleaseLoad(botID, *workUnit)

	// Emit work completed event
	event := types.NewBotWorkCompletedEvent(botID, workUnit.Job.ID, duration, results)
	if err := wa.eventPublisher.PublishEvent(event); err != nil {
		return fmt.Errorf("failed to publish work completed event: %w", err)
	}

	return nil
}

// FailWork marks work as failed for a bot
func (wa *WorkAssignment) FailWork(botID string, err error) error {
	wa.mu.Lock()
	defer wa.mu.Unlock()

	workUnit, exists := wa.assignedWork[botID]
	if !exists {
		return fmt.Errorf("no work assigned to bot %s", botID)
	}

	workUnit.Status = WorkUnitStatusFailed
	workUnit.Error = err
	workUnit.Retries++

	// Remove from assigned
	delete(wa.assignedWork, botID)

	// Update bot status
	if err := wa.botRegistry.UpdateBotStatus(botID, types.StatusIdle); err != nil {
		return fmt.Errorf("failed to update bot status: %w", err)
	}

	// Update load balancer
	wa.loadBalancer.ReleaseLoad(botID, *workUnit)

	// Check if should retry
	if workUnit.Retries < workUnit.MaxRetries {
		workUnit.Status = WorkUnitStatusPending
		workUnit.AssignedBotID = ""
		wa.pendingWork = append([]WorkUnit{*workUnit}, wa.pendingWork...)
	} else {
		// Move to completed with failed status
		wa.completedWork = append(wa.completedWork, *workUnit)
	}

	// Emit work failed event
	event := types.NewBotWorkFailedEvent(botID, workUnit.Job.ID, err.Error(), fmt.Sprintf("Retries: %d/%d", workUnit.Retries, workUnit.MaxRetries))
	if err := wa.eventPublisher.PublishEvent(event); err != nil {
		return fmt.Errorf("failed to publish work failed event: %w", err)
	}

	return nil
}

// ReassignWork reassigns work from an offline or failed bot
func (wa *WorkAssignment) ReassignWork(botID string) error {
	wa.mu.Lock()
	defer wa.mu.Unlock()

	workUnit, exists := wa.assignedWork[botID]
	if !exists {
		return nil // No work to reassign
	}

	// Remove from assigned
	delete(wa.assignedWork, botID)

	// Update load balancer
	wa.loadBalancer.ReleaseLoad(botID, *workUnit)

	// Add back to pending queue with higher priority
	workUnit.Status = WorkUnitStatusPending
	workUnit.AssignedBotID = ""
	workUnit.Retries++

	// Insert at front of pending queue for quick reassignment
	wa.pendingWork = append([]WorkUnit{*workUnit}, wa.pendingWork...)

	return nil
}

// GetPendingWorkCount returns the number of pending work units
func (wa *WorkAssignment) GetPendingWorkCount() int {
	wa.mu.RLock()
	defer wa.mu.RUnlock()
	return len(wa.pendingWork)
}

// GetAssignedWorkCount returns the number of assigned work units
func (wa *WorkAssignment) GetAssignedWorkCount() int {
	wa.mu.RLock()
	defer wa.mu.RUnlock()
	return len(wa.assignedWork)
}

// GetBotWorkload returns the current work assigned to a bot
func (wa *WorkAssignment) GetBotWorkload(botID string) (*WorkUnit, bool) {
	wa.mu.RLock()
	defer wa.mu.RUnlock()
	workUnit, exists := wa.assignedWork[botID]
	return workUnit, exists
}

// GetWorkQueueStats returns statistics about the work queue
func (wa *WorkAssignment) GetWorkQueueStats() WorkQueueStats {
	wa.mu.RLock()
	defer wa.mu.RUnlock()

	stats := WorkQueueStats{
		PendingCount:   len(wa.pendingWork),
		AssignedCount:  len(wa.assignedWork),
		CompletedCount: len(wa.completedWork),
		ByPriority:     make(map[string]int),
	}

	// Count by priority
	for _, wu := range wa.pendingWork {
		priority := wu.Priority.String()
		stats.ByPriority[priority]++
	}

	return stats
}

// canBotHandleWork checks if a bot has the required capabilities for a work unit
func (wa *WorkAssignment) canBotHandleWork(bot *types.Agent, workUnit *WorkUnit) bool {
	// For fuzzing jobs, bot needs fuzzing capability
	if workUnit.Job.FuzzerType != "" {
		return bot.HasCapability(types.CapabilityFuzzing)
	}

	// Default: bot can handle work
	return true
}

// Assignment represents a work assignment decision
type Assignment struct {
	WorkUnit WorkUnit
	BotID    string
	Bot      *types.Agent
}

// WorkQueueStats provides statistics about the work queue
type WorkQueueStats struct {
	PendingCount   int
	AssignedCount  int
	CompletedCount int
	ByPriority     map[string]int
}
