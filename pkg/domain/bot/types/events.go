package types

import (
	"time"
)

// BotEventType represents the type of bot event
type BotEventType string

const (
	// EventBotRegistered indicates a bot was registered
	EventBotRegistered BotEventType = "bot.registered"
	// EventBotDeregistered indicates a bot was deregistered
	EventBotDeregistered BotEventType = "bot.deregistered"
	// EventBotStatusChanged indicates a bot status changed
	EventBotStatusChanged BotEventType = "bot.status_changed"
	// EventBotHeartbeat indicates a bot heartbeat was received
	EventBotHeartbeat BotEventType = "bot.heartbeat"
	// EventBotWorkAssigned indicates work was assigned to a bot
	EventBotWorkAssigned BotEventType = "bot.work_assigned"
	// EventBotWorkCompleted indicates a bot completed work
	EventBotWorkCompleted BotEventType = "bot.work_completed"
	// EventBotWorkFailed indicates a bot's work failed
	EventBotWorkFailed BotEventType = "bot.work_failed"
	// EventBotHealthCheckFailed indicates a bot health check failed
	EventBotHealthCheckFailed BotEventType = "bot.health_check_failed"
)

// BotEvent represents a domain event for bots
type BotEvent interface {
	GetType() BotEventType
	GetBotID() string
	GetTimestamp() time.Time
	GetData() interface{}
}

// BaseBotEvent provides common fields for all bot events
type BaseBotEvent struct {
	Type      BotEventType `json:"type"`
	BotID     string       `json:"bot_id"`
	Timestamp time.Time    `json:"timestamp"`
	Data      interface{}  `json:"data,omitempty"`
}

// GetType returns the event type
func (e BaseBotEvent) GetType() BotEventType {
	return e.Type
}

// GetBotID returns the bot ID
func (e BaseBotEvent) GetBotID() string {
	return e.BotID
}

// GetTimestamp returns the event timestamp
func (e BaseBotEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

// GetData returns the event data
func (e BaseBotEvent) GetData() interface{} {
	return e.Data
}

// BotRegisteredEvent represents a bot registration event
type BotRegisteredEvent struct {
	BaseBotEvent
	Agent *Agent `json:"agent"`
}

// NewBotRegisteredEvent creates a new bot registered event
func NewBotRegisteredEvent(agent *Agent) *BotRegisteredEvent {
	return &BotRegisteredEvent{
		BaseBotEvent: BaseBotEvent{
			Type:      EventBotRegistered,
			BotID:     agent.ID,
			Timestamp: time.Now(),
		},
		Agent: agent,
	}
}

// BotStatusChangedEvent represents a bot status change event
type BotStatusChangedEvent struct {
	BaseBotEvent
	OldStatus Status `json:"old_status"`
	NewStatus Status `json:"new_status"`
	Reason    string `json:"reason,omitempty"`
}

// NewBotStatusChangedEvent creates a new bot status changed event
func NewBotStatusChangedEvent(botID string, oldStatus, newStatus Status, reason string) *BotStatusChangedEvent {
	return &BotStatusChangedEvent{
		BaseBotEvent: BaseBotEvent{
			Type:      EventBotStatusChanged,
			BotID:     botID,
			Timestamp: time.Now(),
		},
		OldStatus: oldStatus,
		NewStatus: newStatus,
		Reason:    reason,
	}
}

// BotWorkAssignedEvent represents a work assignment event
type BotWorkAssignedEvent struct {
	BaseBotEvent
	JobID       string                 `json:"job_id"`
	JobType     string                 `json:"job_type"`
	JobMetadata map[string]interface{} `json:"job_metadata,omitempty"`
}

// NewBotWorkAssignedEvent creates a new work assigned event
func NewBotWorkAssignedEvent(botID, jobID, jobType string, metadata map[string]interface{}) *BotWorkAssignedEvent {
	return &BotWorkAssignedEvent{
		BaseBotEvent: BaseBotEvent{
			Type:      EventBotWorkAssigned,
			BotID:     botID,
			Timestamp: time.Now(),
		},
		JobID:       jobID,
		JobType:     jobType,
		JobMetadata: metadata,
	}
}

// BotWorkCompletedEvent represents a work completion event
type BotWorkCompletedEvent struct {
	BaseBotEvent
	JobID    string                 `json:"job_id"`
	Duration time.Duration          `json:"duration"`
	Results  map[string]interface{} `json:"results,omitempty"`
}

// NewBotWorkCompletedEvent creates a new work completed event
func NewBotWorkCompletedEvent(botID, jobID string, duration time.Duration, results map[string]interface{}) *BotWorkCompletedEvent {
	return &BotWorkCompletedEvent{
		BaseBotEvent: BaseBotEvent{
			Type:      EventBotWorkCompleted,
			BotID:     botID,
			Timestamp: time.Now(),
		},
		JobID:    jobID,
		Duration: duration,
		Results:  results,
	}
}

// BotWorkFailedEvent represents a work failure event
type BotWorkFailedEvent struct {
	BaseBotEvent
	JobID  string `json:"job_id"`
	Error  string `json:"error"`
	Reason string `json:"reason,omitempty"`
}

// NewBotWorkFailedEvent creates a new work failed event
func NewBotWorkFailedEvent(botID, jobID, err, reason string) *BotWorkFailedEvent {
	return &BotWorkFailedEvent{
		BaseBotEvent: BaseBotEvent{
			Type:      EventBotWorkFailed,
			BotID:     botID,
			Timestamp: time.Now(),
		},
		JobID:  jobID,
		Error:  err,
		Reason: reason,
	}
}

// BotHealthCheckFailedEvent represents a health check failure event
type BotHealthCheckFailedEvent struct {
	BaseBotEvent
	Error     string        `json:"error"`
	LastSeen  time.Time     `json:"last_seen"`
	Downtime  time.Duration `json:"downtime"`
	Attempts  int           `json:"attempts"`
	WillRetry bool          `json:"will_retry"`
	NextRetry *time.Time    `json:"next_retry,omitempty"`
}

// NewBotHealthCheckFailedEvent creates a new health check failed event
func NewBotHealthCheckFailedEvent(botID string, err string, lastSeen time.Time, attempts int, willRetry bool, nextRetry *time.Time) *BotHealthCheckFailedEvent {
	return &BotHealthCheckFailedEvent{
		BaseBotEvent: BaseBotEvent{
			Type:      EventBotHealthCheckFailed,
			BotID:     botID,
			Timestamp: time.Now(),
		},
		Error:     err,
		LastSeen:  lastSeen,
		Downtime:  time.Since(lastSeen),
		Attempts:  attempts,
		WillRetry: willRetry,
		NextRetry: nextRetry,
	}
}

// BotEventHandler defines the interface for handling bot events
type BotEventHandler interface {
	// HandleEvent handles a bot event
	HandleEvent(event BotEvent) error
}

// BotEventPublisher defines the interface for publishing bot events
type BotEventPublisher interface {
	// PublishEvent publishes a bot event
	PublishEvent(event BotEvent) error
}

// BotEventSubscriber defines the interface for subscribing to bot events
type BotEventSubscriber interface {
	// Subscribe subscribes to bot events of specific types
	Subscribe(eventTypes []BotEventType, handler BotEventHandler) error
	// Unsubscribe removes a subscription
	Unsubscribe(handler BotEventHandler) error
}
