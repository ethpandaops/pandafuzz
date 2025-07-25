// Package quarantine provides corpus entry quarantine management functionality.
//
// The quarantine system helps isolate problematic corpus entries that cause issues
// during fuzzing execution, such as crashes, timeouts, or excessive resource usage.
// It follows domain-driven design principles and integrates with the corpus
// repository system.
//
// # Key Features
//
//   - Automatic quarantine based on execution results
//   - Multiple quarantine reasons (crashes, timeouts, memory issues, etc.)
//   - Review and release workflow for quarantined entries
//   - Quarantine history tracking
//   - Event-driven architecture for quarantine actions
//   - Configurable rules and policies
//   - Permanent ban capability for critical issues
//
// # Usage Example
//
//	// Create quarantine service with default rules
//	rules := quarantine.DefaultRules()
//	service, err := quarantine.NewService(corpusRepo, quarantineRepo, eventPublisher, rules)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Process execution result
//	result := quarantine.ExecutionResult{
//	    EntryID:       "entry123",
//	    Crashed:       true,
//	    CrashType:     crashtypes.CrashTypeHeapOverflow,
//	    CrashSignature: "heap-buffer-overflow in parse_input",
//	}
//	err = service.ProcessExecutionResult(ctx, result)
//
//	// Review quarantined entry
//	qEntry, err := service.ReviewEntry(ctx, "entry123", "Reviewed crash - appears to be fixed in latest version")
//
//	// Release entry from quarantine
//	err = service.ReleaseEntry(ctx, "entry123", "Crash fixed in commit abc123")
//
// # Quarantine Reasons
//
// The system supports the following quarantine reasons:
//   - ReasonCrashCausing: Entry causes crashes during execution
//   - ReasonTimeout: Entry causes execution timeouts
//   - ReasonExcessiveMemory: Entry uses excessive memory
//   - ReasonMalformed: Entry contains malformed or invalid data
//   - ReasonSlowExecution: Entry causes slow execution
//   - ReasonManualQuarantine: Entry manually quarantined by user
//   - ReasonRepeatedFailures: Entry causes repeated failures
//
// # Rules Configuration
//
// Quarantine rules can be customized through the RulesConfig:
//
//	config := quarantine.RulesConfig{
//	    MaxExecutionTime:      60 * time.Second,
//	    MaxMemoryUsage:        2 << 30, // 2GB
//	    AutoQuarantineCrashes: true,
//	    MinQuarantinePeriod:   48 * time.Hour,
//	}
//	rules := quarantine.NewRules(config)
//
// # Event System
//
// The quarantine service emits the following events:
//   - EventEntryQuarantined: When an entry is quarantined
//   - EventEntryReleased: When an entry is released from quarantine
//   - EventEntryReviewed: When a quarantined entry is reviewed
//   - EventQuarantineFailed: When quarantine operation fails
//
// Implement the EventPublisher interface to handle these events.
package quarantine
