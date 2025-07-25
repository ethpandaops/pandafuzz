// Package messaging provides event-driven communication infrastructure for
// the pandafuzz system. It implements a flexible publish-subscribe pattern
// that enables decoupled communication between domain components.
//
// The messaging system provides:
//   - Asynchronous event publishing and handling
//   - Type-safe event definitions
//   - Multiple handlers per event type
//   - Event buffering and retry logic
//   - Handler registration and unregistration
//   - Event filtering and routing
//
// Example usage:
//
//	// Create event bus
//	bus := events.NewBus(events.BusConfig{
//		BufferSize: 1000,
//		Workers:    10,
//	})
//
//	// Register handler
//	handler := func(ctx context.Context, event events.Event) error {
//		jobEvent := event.(*JobCompletedEvent)
//		// Handle event
//		return nil
//	}
//	bus.Subscribe("job.completed", handler)
//
//	// Publish event
//	event := &JobCompletedEvent{JobID: "123"}
//	bus.Publish(ctx, event)
//
// The messaging package is designed to be used across the domain layer
// for implementing event-driven architectures and reactive systems.
package messaging
