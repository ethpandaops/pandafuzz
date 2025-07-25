package types

import (
	"fmt"
)

// State represents the state of a campaign
type State string

const (
	// StateDraft indicates the campaign is being configured
	StateDraft State = "draft"
	// StateActive indicates the campaign is running
	StateActive State = "active"
	// StatePaused indicates the campaign is temporarily stopped
	StatePaused State = "paused"
	// StateCompleted indicates the campaign has finished
	StateCompleted State = "completed"
)

// AllStates returns all valid campaign states
func AllStates() []State {
	return []State{
		StateDraft,
		StateActive,
		StatePaused,
		StateCompleted,
	}
}

// Validate checks if the state is valid
func (s State) Validate() error {
	switch s {
	case StateDraft, StateActive, StatePaused, StateCompleted:
		return nil
	default:
		return fmt.Errorf("invalid campaign state: %s", s)
	}
}

// String returns the string representation of the state
func (s State) String() string {
	return string(s)
}

// CanTransitionTo checks if the state can transition to another state
func (s State) CanTransitionTo(target State) bool {
	transitions := map[State][]State{
		StateDraft:     {StateActive},
		StateActive:    {StatePaused, StateCompleted},
		StatePaused:    {StateActive, StateCompleted},
		StateCompleted: {}, // Terminal state
	}

	allowedTransitions, exists := transitions[s]
	if !exists {
		return false
	}

	for _, allowed := range allowedTransitions {
		if allowed == target {
			return true
		}
	}
	return false
}

// IsTerminal returns true if the state is terminal (no further transitions allowed)
func (s State) IsTerminal() bool {
	return s == StateCompleted
}

// ParseState converts a string to a State
func ParseState(s string) (State, error) {
	state := State(s)
	if err := state.Validate(); err != nil {
		return "", err
	}
	return state, nil
}

// MustParseState converts a string to a State, panicking on error
func MustParseState(s string) State {
	state, err := ParseState(s)
	if err != nil {
		panic(err)
	}
	return state
}
