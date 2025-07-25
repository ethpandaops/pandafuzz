package types

import (
	"time"
)

// EventType represents the type of campaign event
type EventType string

const (
	// EventCampaignCreated indicates a campaign was created
	EventCampaignCreated EventType = "campaign.created"
	// EventCampaignStarted indicates a campaign was started
	EventCampaignStarted EventType = "campaign.started"
	// EventCampaignPaused indicates a campaign was paused
	EventCampaignPaused EventType = "campaign.paused"
	// EventCampaignResumed indicates a campaign was resumed
	EventCampaignResumed EventType = "campaign.resumed"
	// EventCampaignCompleted indicates a campaign was completed
	EventCampaignCompleted EventType = "campaign.completed"
	// EventCampaignFailed indicates a campaign failed
	EventCampaignFailed EventType = "campaign.failed"
)

// Event represents a domain event for campaigns
type Event interface {
	GetType() EventType
	GetCampaignID() string
	GetTimestamp() time.Time
	GetData() interface{}
}

// BaseEvent provides common fields for all campaign events
type BaseEvent struct {
	Type       EventType   `json:"type"`
	CampaignID string      `json:"campaign_id"`
	Timestamp  time.Time   `json:"timestamp"`
	Data       interface{} `json:"data,omitempty"`
}

// GetType returns the event type
func (e BaseEvent) GetType() EventType {
	return e.Type
}

// GetCampaignID returns the campaign ID
func (e BaseEvent) GetCampaignID() string {
	return e.CampaignID
}

// GetTimestamp returns the event timestamp
func (e BaseEvent) GetTimestamp() time.Time {
	return e.Timestamp
}

// GetData returns the event data
func (e BaseEvent) GetData() interface{} {
	return e.Data
}

// CampaignCreatedEvent represents a campaign creation event
type CampaignCreatedEvent struct {
	BaseEvent
	Campaign *Campaign `json:"campaign"`
}

// NewCampaignCreatedEvent creates a new campaign created event
func NewCampaignCreatedEvent(campaign *Campaign) *CampaignCreatedEvent {
	return &CampaignCreatedEvent{
		BaseEvent: BaseEvent{
			Type:       EventCampaignCreated,
			CampaignID: campaign.ID,
			Timestamp:  time.Now(),
		},
		Campaign: campaign,
	}
}

// CampaignStartedEvent represents a campaign start event
type CampaignStartedEvent struct {
	BaseEvent
	StartedBy string `json:"started_by"`
}

// NewCampaignStartedEvent creates a new campaign started event
func NewCampaignStartedEvent(campaignID, startedBy string) *CampaignStartedEvent {
	return &CampaignStartedEvent{
		BaseEvent: BaseEvent{
			Type:       EventCampaignStarted,
			CampaignID: campaignID,
			Timestamp:  time.Now(),
		},
		StartedBy: startedBy,
	}
}

// CampaignCompletedEvent represents a campaign completion event
type CampaignCompletedEvent struct {
	BaseEvent
	Duration time.Duration          `json:"duration"`
	Results  map[string]interface{} `json:"results,omitempty"`
}

// NewCampaignCompletedEvent creates a new campaign completed event
func NewCampaignCompletedEvent(campaignID string, duration time.Duration, results map[string]interface{}) *CampaignCompletedEvent {
	return &CampaignCompletedEvent{
		BaseEvent: BaseEvent{
			Type:       EventCampaignCompleted,
			CampaignID: campaignID,
			Timestamp:  time.Now(),
		},
		Duration: duration,
		Results:  results,
	}
}

// CampaignFailedEvent represents a campaign failure event
type CampaignFailedEvent struct {
	BaseEvent
	Error  string `json:"error"`
	Reason string `json:"reason,omitempty"`
}

// NewCampaignFailedEvent creates a new campaign failed event
func NewCampaignFailedEvent(campaignID, err, reason string) *CampaignFailedEvent {
	return &CampaignFailedEvent{
		BaseEvent: BaseEvent{
			Type:       EventCampaignFailed,
			CampaignID: campaignID,
			Timestamp:  time.Now(),
		},
		Error:  err,
		Reason: reason,
	}
}
