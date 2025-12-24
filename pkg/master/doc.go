// Package master provides the PandaFuzz master server implementation.
//
// The master server is the central coordination point for the distributed
// fuzzing system. It manages bot registration, job scheduling, crash storage,
// and provides REST APIs for clients.
//
// # Architecture
//
// The master server consists of several key components:
//
//   - Server: HTTP server with routing, middleware, and API handlers
//   - PersistentState: State management with SQLite persistence
//   - StateStoreAdapter: Adapts PersistentState to service.StateStore interface
//   - CampaignStateManager: Campaign-specific state and lifecycle management
//
// # State Management
//
// State is split across multiple focused files for maintainability:
//
//   - state_core.go: PersistentState struct and constructor
//   - state_bot.go: Bot CRUD, timeout detection, cache operations
//   - state_job.go: Job CRUD, assignment, completion, leasing
//   - state_crash.go: Crash/coverage/corpus processing
//   - state.go: Recovery, stats, lifecycle, maintenance
//   - state_adapter.go: StateStore interface adapter
//
// # Thread Safety
//
// All state operations are thread-safe. The PersistentState uses sync.RWMutex
// for protecting in-memory caches, with database operations handling their own
// transaction isolation.
//
// Lock ordering to prevent deadlocks:
//
//	// CORRECT: Short lock, release before DB operation
//	ps.mu.Lock()
//	ps.bots[bot.ID] = bot  // Quick in-memory update
//	ps.mu.Unlock()
//	err := ps.db.Transaction(ctx, func(tx common.Transaction) error { ... })
//
// # API Versions
//
// The master server exposes multiple API versions:
//
//   - Legacy routes (routes.go): Main production API, uses Gorilla mux
//   - API v3 (api_v3/): Extended functionality for advanced operations
//   - API v1 (pkg/api/v1/): New architecture with OpenAPI code generation
//
// # Job Assignment
//
// Jobs are assigned atomically with a lease mechanism to prevent duplicate
// execution:
//
//  1. Bot requests next job via heartbeat
//  2. Master finds pending job matching bot capabilities
//  3. Master assigns job within database transaction
//  4. Bot receives job with lease token and expiration time
//  5. If bot fails to ACK, lease expires and job becomes available
//
// # Example Usage
//
// Creating and starting the master server:
//
//	cfg := &common.MasterConfig{...}
//	server, err := master.NewServer(cfg, logger)
//	if err != nil {
//	    return err
//	}
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	if err := server.Start(ctx); err != nil {
//	    return err
//	}
package master
