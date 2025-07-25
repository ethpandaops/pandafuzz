package types

import (
	"fmt"
)

// Status represents the status of a bot agent
type Status string

const (
	// StatusIdle indicates the bot is idle and ready for work
	StatusIdle Status = "idle"
	// StatusWorking indicates the bot is currently working
	StatusWorking Status = "working"
	// StatusOffline indicates the bot is offline
	StatusOffline Status = "offline"
	// StatusMaintenance indicates the bot is in maintenance mode
	StatusMaintenance Status = "maintenance"
	// StatusError indicates the bot is in an error state
	StatusError Status = "error"
)

// AllStatuses returns all valid bot statuses
func AllStatuses() []Status {
	return []Status{
		StatusIdle,
		StatusWorking,
		StatusOffline,
		StatusMaintenance,
		StatusError,
	}
}

// Validate checks if the status is valid
func (s Status) Validate() error {
	switch s {
	case StatusIdle, StatusWorking, StatusOffline, StatusMaintenance, StatusError:
		return nil
	default:
		return fmt.Errorf("invalid bot status: %s", s)
	}
}

// String returns the string representation of the status
func (s Status) String() string {
	return string(s)
}

// IsOperational returns true if the bot is in an operational state
func (s Status) IsOperational() bool {
	return s == StatusIdle || s == StatusWorking
}

// CanWork returns true if the bot can accept new work
func (s Status) CanWork() bool {
	return s == StatusIdle
}

// Priority returns the priority level of the status (higher is more critical)
func (s Status) Priority() int {
	priorities := map[Status]int{
		StatusIdle:        1,
		StatusWorking:     2,
		StatusMaintenance: 3,
		StatusOffline:     4,
		StatusError:       5,
	}

	priority, exists := priorities[s]
	if !exists {
		return 0
	}
	return priority
}

// ParseStatus converts a string to a Status
func ParseStatus(s string) (Status, error) {
	status := Status(s)
	if err := status.Validate(); err != nil {
		return "", err
	}
	return status, nil
}

// MustParseStatus converts a string to a Status, panicking on error
func MustParseStatus(s string) Status {
	status, err := ParseStatus(s)
	if err != nil {
		panic(err)
	}
	return status
}

// StatusTransition represents a valid status transition
type StatusTransition struct {
	From   Status
	To     Status
	Reason string
}

// IsValidTransition checks if a status transition is valid
func IsValidTransition(from, to Status) bool {
	// Define valid transitions
	validTransitions := map[Status][]Status{
		StatusIdle:        {StatusWorking, StatusOffline, StatusMaintenance, StatusError},
		StatusWorking:     {StatusIdle, StatusOffline, StatusError},
		StatusOffline:     {StatusIdle, StatusMaintenance},
		StatusMaintenance: {StatusIdle, StatusOffline},
		StatusError:       {StatusIdle, StatusOffline, StatusMaintenance},
	}

	allowedTransitions, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, allowed := range allowedTransitions {
		if allowed == to {
			return true
		}
	}
	return false
}
