package sync

import (
	"time"
)

// EventType represents the type of sync event
type EventType string

const (
	// EventSyncStarted indicates a sync operation started
	EventSyncStarted EventType = "sync.started"
	// EventSyncProgress indicates sync progress update
	EventSyncProgress EventType = "sync.progress"
	// EventSyncCompleted indicates a sync operation completed
	EventSyncCompleted EventType = "sync.completed"
	// EventSyncFailed indicates a sync operation failed
	EventSyncFailed EventType = "sync.failed"
	// EventSyncConflict indicates a sync conflict occurred
	EventSyncConflict EventType = "sync.conflict"
	// EventSyncCancelled indicates a sync operation was cancelled
	EventSyncCancelled EventType = "sync.cancelled"
)

// Event represents a domain event for sync operations
type Event interface {
	GetType() EventType
	GetSyncID() string
	GetTimestamp() time.Time
	GetData() interface{}
}

// BaseEvent provides common fields for all sync events
type BaseEvent struct {
	Type      EventType   `json:"type"`
	SyncID    string      `json:"sync_id"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

// GetType returns the event type
func (e BaseEvent) GetType() EventType {
	return e.Type
}

// GetSyncID returns the sync operation ID
func (e BaseEvent) GetSyncID() string {
	return e.SyncID
}

// GetTimestamp returns the event timestamp
func (e BaseEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

// GetData returns the event data
func (e BaseEvent) GetData() interface{} {
	return e.Data
}

// SyncStartedEvent represents a sync start event
type SyncStartedEvent struct {
	BaseEvent
	CollectionCount int `json:"collection_count"`
}

// NewSyncStartedEvent creates a new sync started event
func NewSyncStartedEvent(syncID string, collectionCount int) *SyncStartedEvent {
	return &SyncStartedEvent{
		BaseEvent: BaseEvent{
			Type:      EventSyncStarted,
			SyncID:    syncID,
			Timestamp: time.Now(),
		},
		CollectionCount: collectionCount,
	}
}

// SyncProgressEvent represents a sync progress event
type SyncProgressEvent struct {
	BaseEvent
	CollectionName string      `json:"collection_name"`
	Progress       *SyncResult `json:"progress"`
}

// NewSyncProgressEvent creates a new sync progress event
func NewSyncProgressEvent(syncID string, collectionName string, progress *SyncResult) *SyncProgressEvent {
	return &SyncProgressEvent{
		BaseEvent: BaseEvent{
			Type:      EventSyncProgress,
			SyncID:    syncID,
			Timestamp: time.Now(),
		},
		CollectionName: collectionName,
		Progress:       progress,
	}
}

// SyncCompletedEvent represents a sync completion event
type SyncCompletedEvent struct {
	BaseEvent
	Result *SyncResult `json:"result"`
}

// NewSyncCompletedEvent creates a new sync completed event
func NewSyncCompletedEvent(syncID string, result *SyncResult) *SyncCompletedEvent {
	return &SyncCompletedEvent{
		BaseEvent: BaseEvent{
			Type:      EventSyncCompleted,
			SyncID:    syncID,
			Timestamp: time.Now(),
		},
		Result: result,
	}
}

// SyncFailedEvent represents a sync failure event
type SyncFailedEvent struct {
	BaseEvent
	Errors []error `json:"errors"`
}

// NewSyncFailedEvent creates a new sync failed event
func NewSyncFailedEvent(syncID string, errors []error) *SyncFailedEvent {
	return &SyncFailedEvent{
		BaseEvent: BaseEvent{
			Type:      EventSyncFailed,
			SyncID:    syncID,
			Timestamp: time.Now(),
		},
		Errors: errors,
	}
}

// SyncConflictEvent represents a sync conflict event
type SyncConflictEvent struct {
	BaseEvent
	Conflict SyncConflict `json:"conflict"`
}

// NewSyncConflictEvent creates a new sync conflict event
func NewSyncConflictEvent(syncID string, conflict SyncConflict) *SyncConflictEvent {
	return &SyncConflictEvent{
		BaseEvent: BaseEvent{
			Type:      EventSyncConflict,
			SyncID:    syncID,
			Timestamp: time.Now(),
		},
		Conflict: conflict,
	}
}

// SyncCancelledEvent represents a sync cancellation event
type SyncCancelledEvent struct {
	BaseEvent
	Reason string `json:"reason"`
}

// NewSyncCancelledEvent creates a new sync cancelled event
func NewSyncCancelledEvent(syncID string, reason string) *SyncCancelledEvent {
	return &SyncCancelledEvent{
		BaseEvent: BaseEvent{
			Type:      EventSyncCancelled,
			SyncID:    syncID,
			Timestamp: time.Now(),
		},
		Reason: reason,
	}
}
