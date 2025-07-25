package types

import (
	"errors"
	"time"
)

// Capability represents a specific capability of a bot agent
type Capability string

const (
	// CapabilityFuzzing indicates the bot can perform fuzzing
	CapabilityFuzzing Capability = "fuzzing"
	// CapabilityAnalysis indicates the bot can perform analysis
	CapabilityAnalysis Capability = "analysis"
	// CapabilityReporting indicates the bot can generate reports
	CapabilityReporting Capability = "reporting"
	// CapabilityCoordination indicates the bot can coordinate other bots
	CapabilityCoordination Capability = "coordination"
)

// Agent represents a bot agent entity
type Agent struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Status        Status                 `json:"status"`
	Capabilities  []Capability           `json:"capabilities"`
	LastHeartbeat time.Time              `json:"last_heartbeat"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// NewAgent creates a new Agent with validation
func NewAgent(id, name string, capabilities []Capability) (*Agent, error) {
	if id == "" {
		return nil, errors.New("agent ID cannot be empty")
	}
	if name == "" {
		return nil, errors.New("agent name cannot be empty")
	}
	if len(capabilities) == 0 {
		return nil, errors.New("agent must have at least one capability")
	}

	now := time.Now()
	return &Agent{
		ID:            id,
		Name:          name,
		Status:        StatusIdle,
		Capabilities:  capabilities,
		LastHeartbeat: now,
		CreatedAt:     now,
		UpdatedAt:     now,
		Metadata:      make(map[string]interface{}),
	}, nil
}

// UpdateStatus updates the agent status and UpdatedAt timestamp
func (a *Agent) UpdateStatus(newStatus Status) error {
	if err := newStatus.Validate(); err != nil {
		return err
	}

	a.Status = newStatus
	a.UpdatedAt = time.Now()
	return nil
}

// UpdateHeartbeat updates the last heartbeat timestamp
func (a *Agent) UpdateHeartbeat() {
	a.LastHeartbeat = time.Now()
	a.UpdatedAt = time.Now()
}

// IsOnline returns true if the agent is considered online
func (a *Agent) IsOnline() bool {
	return a.Status != StatusOffline && time.Since(a.LastHeartbeat) < 5*time.Minute
}

// IsAvailable returns true if the agent can accept new work
func (a *Agent) IsAvailable() bool {
	return a.Status == StatusIdle && a.IsOnline()
}

// HasCapability checks if the agent has a specific capability
func (a *Agent) HasCapability(cap Capability) bool {
	for _, c := range a.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// AddCapability adds a new capability to the agent
func (a *Agent) AddCapability(cap Capability) {
	if !a.HasCapability(cap) {
		a.Capabilities = append(a.Capabilities, cap)
		a.UpdatedAt = time.Now()
	}
}

// RemoveCapability removes a capability from the agent
func (a *Agent) RemoveCapability(cap Capability) {
	var filtered []Capability
	for _, c := range a.Capabilities {
		if c != cap {
			filtered = append(filtered, c)
		}
	}
	a.Capabilities = filtered
	a.UpdatedAt = time.Now()
}

// SetMetadata sets a metadata key-value pair
func (a *Agent) SetMetadata(key string, value interface{}) {
	if a.Metadata == nil {
		a.Metadata = make(map[string]interface{})
	}
	a.Metadata[key] = value
	a.UpdatedAt = time.Now()
}

// GetMetadata retrieves a metadata value by key
func (a *Agent) GetMetadata(key string) (interface{}, bool) {
	if a.Metadata == nil {
		return nil, false
	}
	value, exists := a.Metadata[key]
	return value, exists
}

// Validate ensures the agent has valid data
func (a *Agent) Validate() error {
	if a.ID == "" {
		return errors.New("agent ID cannot be empty")
	}
	if a.Name == "" {
		return errors.New("agent name cannot be empty")
	}
	if len(a.Capabilities) == 0 {
		return errors.New("agent must have at least one capability")
	}
	if err := a.Status.Validate(); err != nil {
		return err
	}
	return nil
}
