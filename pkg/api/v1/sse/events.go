package sse

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// Event represents a Server-Sent Event
type Event interface {
	GetID() string
	GetType() string
	GetData() string
}

// BaseEvent provides common functionality for all events
type BaseEvent struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// GetID returns the event ID
func (e *BaseEvent) GetID() string {
	return e.ID
}

// GetType returns the event type
func (e *BaseEvent) GetType() string {
	return e.Type
}

// GetData returns the event data as JSON string
func (e *BaseEvent) GetData() string {
	data, err := json.Marshal(e.Data)
	if err != nil {
		return fmt.Sprintf(`{"error": "failed to marshal event data: %s"}`, err.Error())
	}
	return string(data)
}

// NewBaseEvent creates a new base event with generated ID and current timestamp
func NewBaseEvent(eventType string, data map[string]interface{}) *BaseEvent {
	return &BaseEvent{
		ID:        generateEventID(),
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
	}
}

// BotEvent represents bot-related events
type BotEvent struct {
	*BaseEvent
	BotID      openapi_types.UUID  `json:"bot_id"`
	BotName    string              `json:"bot_name,omitempty"`
	CampaignID *openapi_types.UUID `json:"campaign_id,omitempty"`
}

// NewBotEvent creates a new bot event
func NewBotEvent(eventType string, botID openapi_types.UUID, data map[string]interface{}) *BotEvent {
	baseData := map[string]interface{}{
		"bot_id": botID.String(),
	}
	for k, v := range data {
		baseData[k] = v
	}

	return &BotEvent{
		BaseEvent: NewBaseEvent(eventType, baseData),
		BotID:     botID,
	}
}

// JobEvent represents job-related events
type JobEvent struct {
	*BaseEvent
	JobID      openapi_types.UUID  `json:"job_id"`
	BotID      *openapi_types.UUID `json:"bot_id,omitempty"`
	CampaignID openapi_types.UUID  `json:"campaign_id"`
	Status     string              `json:"status,omitempty"`
}

// NewJobEvent creates a new job event
func NewJobEvent(eventType string, jobID, campaignID openapi_types.UUID, data map[string]interface{}) *JobEvent {
	baseData := map[string]interface{}{
		"job_id":      jobID.String(),
		"campaign_id": campaignID.String(),
	}
	for k, v := range data {
		baseData[k] = v
	}

	return &JobEvent{
		BaseEvent:  NewBaseEvent(eventType, baseData),
		JobID:      jobID,
		CampaignID: campaignID,
	}
}

// CampaignEvent represents campaign-related events
type CampaignEvent struct {
	*BaseEvent
	CampaignID openapi_types.UUID `json:"campaign_id"`
	State      string             `json:"state,omitempty"`
}

// NewCampaignEvent creates a new campaign event
func NewCampaignEvent(eventType string, campaignID openapi_types.UUID, data map[string]interface{}) *CampaignEvent {
	baseData := map[string]interface{}{
		"campaign_id": campaignID.String(),
	}
	for k, v := range data {
		baseData[k] = v
	}

	return &CampaignEvent{
		BaseEvent:  NewBaseEvent(eventType, baseData),
		CampaignID: campaignID,
	}
}

// CorpusEvent represents corpus-related events
type CorpusEvent struct {
	*BaseEvent
	CorpusID   *openapi_types.UUID `json:"corpus_id,omitempty"`
	JobID      *openapi_types.UUID `json:"job_id,omitempty"`
	CampaignID *openapi_types.UUID `json:"campaign_id,omitempty"`
	Operation  string              `json:"operation,omitempty"`
}

// NewCorpusEvent creates a new corpus event
func NewCorpusEvent(eventType string, data map[string]interface{}) *CorpusEvent {
	return &CorpusEvent{
		BaseEvent: NewBaseEvent(eventType, data),
	}
}

// CrashEvent represents crash-related events
type CrashEvent struct {
	*BaseEvent
	CrashID    openapi_types.UUID `json:"crash_id"`
	JobID      openapi_types.UUID `json:"job_id"`
	CampaignID openapi_types.UUID `json:"campaign_id"`
	CrashType  string             `json:"crash_type,omitempty"`
	Severity   string             `json:"severity,omitempty"`
}

// NewCrashEvent creates a new crash event
func NewCrashEvent(eventType string, crashID, jobID, campaignID openapi_types.UUID, data map[string]interface{}) *CrashEvent {
	baseData := map[string]interface{}{
		"crash_id":    crashID.String(),
		"job_id":      jobID.String(),
		"campaign_id": campaignID.String(),
	}
	for k, v := range data {
		baseData[k] = v
	}

	return &CrashEvent{
		BaseEvent:  NewBaseEvent(eventType, baseData),
		CrashID:    crashID,
		JobID:      jobID,
		CampaignID: campaignID,
	}
}

// SystemEvent represents system-level events
type SystemEvent struct {
	*BaseEvent
	Component string `json:"component,omitempty"`
	Severity  string `json:"severity,omitempty"`
}

// NewSystemEvent creates a new system event
func NewSystemEvent(eventType string, data map[string]interface{}) *SystemEvent {
	return &SystemEvent{
		BaseEvent: NewBaseEvent(eventType, data),
	}
}

// Event type constants for consistency
const (
	// Bot events
	EventBotRegistered    = "bot.registered"
	EventBotUnregistered  = "bot.unregistered"
	EventBotHeartbeat     = "bot.heartbeat"
	EventBotStatusChanged = "bot.status_changed"
	EventBotJobAssigned   = "bot.job_assigned"
	EventBotJobCompleted  = "bot.job_completed"
	EventBotError         = "bot.error"

	// Job events
	EventJobCreated   = "job.created"
	EventJobStarted   = "job.started"
	EventJobProgress  = "job.progress"
	EventJobCompleted = "job.completed"
	EventJobFailed    = "job.failed"
	EventJobCancelled = "job.cancelled"
	EventJobTimeout   = "job.timeout"
	EventJobRestarted = "job.restarted"

	// Campaign events
	EventCampaignCreated   = "campaign.created"
	EventCampaignStarted   = "campaign.started"
	EventCampaignPaused    = "campaign.paused"
	EventCampaignResumed   = "campaign.resumed"
	EventCampaignCompleted = "campaign.completed"
	EventCampaignFailed    = "campaign.failed"
	EventCampaignDeleted   = "campaign.deleted"
	EventCampaignUpdated   = "campaign.updated"

	// Corpus events
	EventCorpusSync        = "corpus.sync"
	EventCorpusUpdated     = "corpus.updated"
	EventCorpusQuarantined = "corpus.quarantined"
	EventCorpusRestored    = "corpus.restored"
	EventCorpusDeleted     = "corpus.deleted"
	EventCorpusCreated     = "corpus.created"

	// Crash events
	EventCrashDetected   = "crash.detected"
	EventCrashAnalyzed   = "crash.analyzed"
	EventCrashDuplicate  = "crash.duplicate"
	EventCrashMinimized  = "crash.minimized"
	EventCrashReproduced = "crash.reproduced"
	EventCrashClassified = "crash.classified"

	// System events
	EventSystemHeartbeat    = "system.heartbeat"
	EventSystemAlert        = "system.alert"
	EventSystemMaintenance  = "system.maintenance"
	EventClientConnected    = "client.connected"
	EventClientDisconnected = "client.disconnected"
	EventClientReconnected  = "client.reconnected"
)

// Helper functions for creating specific event types

// CreateBotRegisteredEvent creates a bot registration event
func CreateBotRegisteredEvent(botID openapi_types.UUID, botName string, capabilities []string) *BotEvent {
	return NewBotEvent(EventBotRegistered, botID, map[string]interface{}{
		"bot_name":     botName,
		"capabilities": capabilities,
	})
}

// CreateBotStatusChangedEvent creates a bot status change event
func CreateBotStatusChangedEvent(botID openapi_types.UUID, oldStatus, newStatus string) *BotEvent {
	return NewBotEvent(EventBotStatusChanged, botID, map[string]interface{}{
		"old_status": oldStatus,
		"new_status": newStatus,
	})
}

// CreateJobProgressEvent creates a job progress event
func CreateJobProgressEvent(jobID, campaignID openapi_types.UUID, progress float64, details map[string]interface{}) *JobEvent {
	data := map[string]interface{}{
		"progress": progress,
	}
	for k, v := range details {
		data[k] = v
	}

	return NewJobEvent(EventJobProgress, jobID, campaignID, data)
}

// CreateJobCompletedEvent creates a job completion event
func CreateJobCompletedEvent(jobID, campaignID openapi_types.UUID, duration time.Duration, results map[string]interface{}) *JobEvent {
	data := map[string]interface{}{
		"duration_seconds": duration.Seconds(),
	}
	for k, v := range results {
		data[k] = v
	}

	return NewJobEvent(EventJobCompleted, jobID, campaignID, data)
}

// CreateCrashDetectedEvent creates a crash detection event
func CreateCrashDetectedEvent(crashID, jobID, campaignID openapi_types.UUID, crashType, stackTrace string) *CrashEvent {
	return NewCrashEvent(EventCrashDetected, crashID, jobID, campaignID, map[string]interface{}{
		"crash_type":  crashType,
		"stack_trace": stackTrace,
	})
}

// CreateCampaignStateChangeEvent creates a campaign state change event
func CreateCampaignStateChangeEvent(campaignID openapi_types.UUID, oldState, newState string) *CampaignEvent {
	return NewCampaignEvent(EventCampaignUpdated, campaignID, map[string]interface{}{
		"old_state": oldState,
		"new_state": newState,
	})
}

// CreateCorpusSyncEvent creates a corpus synchronization event
func CreateCorpusSyncEvent(jobID, campaignID openapi_types.UUID, operation string, count int) *CorpusEvent {
	event := NewCorpusEvent(EventCorpusSync, map[string]interface{}{
		"job_id":      jobID.String(),
		"campaign_id": campaignID.String(),
		"operation":   operation,
		"count":       count,
	})
	event.JobID = &jobID
	event.CampaignID = &campaignID
	event.Operation = operation
	return event
}

// CreateSystemAlertEvent creates a system alert event
func CreateSystemAlertEvent(component, severity, message string, details map[string]interface{}) *SystemEvent {
	data := map[string]interface{}{
		"component": component,
		"severity":  severity,
		"message":   message,
	}
	for k, v := range details {
		data[k] = v
	}

	event := NewSystemEvent(EventSystemAlert, data)
	event.Component = component
	event.Severity = severity
	return event
}

// generateEventID creates a unique event ID
func generateEventID() string {
	return fmt.Sprintf("evt_%d_%s", time.Now().UnixNano(), uuid.New().String()[:8])
}
