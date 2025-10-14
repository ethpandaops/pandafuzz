package sse

import (
	"errors"
	"sync/atomic"
	"time"
)

// Common errors
var (
	ErrManagerAlreadyRunning   = errors.New("SSE manager is already running")
	ErrManagerNotRunning       = errors.New("SSE manager is not running")
	ErrManagerStopped          = errors.New("SSE manager has been stopped")
	ErrMaxClientsReached       = errors.New("maximum number of clients reached")
	ErrRegistrationQueueFull   = errors.New("client registration queue is full")
	ErrUnregistrationQueueFull = errors.New("client unregistration queue is full")
	ErrBroadcastQueueFull      = errors.New("broadcast queue is full")
	ErrClientNotFound          = errors.New("client not found")
	ErrClientClosed            = errors.New("client connection is closed")
	ErrRateLimited             = errors.New("event dropped due to rate limiting")
	ErrBufferFull              = errors.New("client buffer is full")
)

// Metrics holds various metrics for the SSE manager
type Metrics struct {
	activeClients atomic.Int64
	totalClients  atomic.Int64
	eventsSent    atomic.Int64
	eventsDropped atomic.Int64
	startTime     time.Time
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		startTime: time.Now(),
	}
}

// IncrementActiveClients increments the active clients counter
func (m *Metrics) IncrementActiveClients() {
	m.activeClients.Add(1)
	m.totalClients.Add(1)
}

// DecrementActiveClients decrements the active clients counter
func (m *Metrics) DecrementActiveClients() {
	m.activeClients.Add(-1)
}

// IncrementEventsSent increments the events sent counter
func (m *Metrics) IncrementEventsSent() {
	m.eventsSent.Add(1)
}

// IncrementEventsDropped increments the events dropped counter
func (m *Metrics) IncrementEventsDropped() {
	m.eventsDropped.Add(1)
}

// GetActiveClients returns the current number of active clients
func (m *Metrics) GetActiveClients() int64 {
	return m.activeClients.Load()
}

// GetTotalClients returns the total number of clients that have connected
func (m *Metrics) GetTotalClients() int64 {
	return m.totalClients.Load()
}

// GetEventsSent returns the total number of events sent
func (m *Metrics) GetEventsSent() int64 {
	return m.eventsSent.Load()
}

// GetEventsDropped returns the total number of events dropped
func (m *Metrics) GetEventsDropped() int64 {
	return m.eventsDropped.Load()
}

// GetUptime returns the uptime of the metrics tracker
func (m *Metrics) GetUptime() time.Duration {
	return time.Since(m.startTime)
}

// GetEventsPerSecond returns the average events per second
func (m *Metrics) GetEventsPerSecond() float64 {
	uptime := m.GetUptime()
	if uptime.Seconds() == 0 {
		return 0
	}
	return float64(m.GetEventsSent()) / uptime.Seconds()
}

// MetricsSnapshot represents a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	ActiveClients   int64     `json:"active_clients"`
	TotalClients    int64     `json:"total_clients"`
	EventsSent      int64     `json:"events_sent"`
	EventsDropped   int64     `json:"events_dropped"`
	EventsPerSecond float64   `json:"events_per_second"`
	Uptime          string    `json:"uptime"`
	Timestamp       time.Time `json:"timestamp"`
}

// Snapshot creates a metrics snapshot
func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		ActiveClients:   m.GetActiveClients(),
		TotalClients:    m.GetTotalClients(),
		EventsSent:      m.GetEventsSent(),
		EventsDropped:   m.GetEventsDropped(),
		EventsPerSecond: m.GetEventsPerSecond(),
		Uptime:          m.GetUptime().String(),
		Timestamp:       time.Now(),
	}
}

// HealthStatus represents the health of the SSE system
type HealthStatus struct {
	Status    string          `json:"status"` // "healthy", "degraded", "unhealthy"
	Timestamp time.Time       `json:"timestamp"`
	Uptime    string          `json:"uptime"`
	Metrics   MetricsSnapshot `json:"metrics"`
	Issues    []string        `json:"issues,omitempty"`
	Details   map[string]any  `json:"details,omitempty"`
}

// EventHistory stores recent events for replay functionality
type EventHistory struct {
	events  []StoredEvent
	maxSize int
	current int
	full    bool
}

// StoredEvent represents an event stored in history
type StoredEvent struct {
	Event     Event     `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	ID        string    `json:"id"`
}

// NewEventHistory creates a new event history with specified size
func NewEventHistory(maxSize int) *EventHistory {
	return &EventHistory{
		events:  make([]StoredEvent, maxSize),
		maxSize: maxSize,
	}
}

// Add stores an event in the history
func (eh *EventHistory) Add(event Event) {
	stored := StoredEvent{
		Event:     event,
		Timestamp: time.Now(),
		ID:        event.GetID(),
	}

	eh.events[eh.current] = stored
	eh.current = (eh.current + 1) % eh.maxSize
	if eh.current == 0 {
		eh.full = true
	}
}

// GetEventsAfter returns events after the specified event ID
func (eh *EventHistory) GetEventsAfter(eventID string) []StoredEvent {
	var result []StoredEvent

	size := eh.maxSize
	if !eh.full {
		size = eh.current
	}

	found := false
	if eventID == "" {
		found = true // Return all events if no ID specified
	}

	start := eh.current
	if eh.full {
		start = 0
	}

	for i := 0; i < size; i++ {
		idx := (start + i) % eh.maxSize
		event := eh.events[idx]

		if !found {
			if event.ID == eventID {
				found = true
			}
			continue
		}

		if event.Event != nil {
			result = append(result, event)
		}
	}

	return result
}

// ConnectionInfo provides detailed connection information
type ConnectionInfo struct {
	RemoteAddr  string            `json:"remote_addr"`
	UserAgent   string            `json:"user_agent"`
	Headers     map[string]string `json:"headers"`
	ConnectedAt time.Time         `json:"connected_at"`
	LastEventID string            `json:"last_event_id"`
	BytesSent   int64             `json:"bytes_sent"`
	EventsSent  int64             `json:"events_sent"`
}

// TopicStats provides statistics for a topic
type TopicStats struct {
	Name         string    `json:"name"`
	Subscribers  int       `json:"subscribers"`
	EventsSent   int64     `json:"events_sent"`
	LastActivity time.Time `json:"last_activity"`
}

// ManagerStats provides comprehensive statistics about the SSE manager
type ManagerStats struct {
	Status        string          `json:"status"`
	Uptime        string          `json:"uptime"`
	Metrics       MetricsSnapshot `json:"metrics"`
	Topics        []TopicStats    `json:"topics"`
	Clients       []ClientInfo    `json:"clients"`
	Configuration Config          `json:"configuration"`
	Health        HealthStatus    `json:"health"`
}
