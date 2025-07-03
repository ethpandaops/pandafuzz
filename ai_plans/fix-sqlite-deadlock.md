# SQLite Deadlock Fix Implementation Plan

## Executive Summary
> PandaFuzz is experiencing API timeouts due to SQLite database operations blocking indefinitely. The root cause is overly aggressive application-level locking that defeats SQLite's built-in concurrency mechanisms. This plan addresses the deadlocking issues through:
> - Removing unnecessary mutex locks that serialize all database access
> - Implementing proper context timeouts throughout the API layer
> - Adding retry logic with exponential backoff for transient lock conflicts
> - Optimizing transaction patterns to minimize lock duration
> - Improving connection pooling configuration for SQLite's WAL mode

## Goals & Objectives
### Primary Goals
- **Eliminate API timeouts**: All API endpoints should respond within 10 seconds
- **Enable concurrent database access**: Multiple readers should operate simultaneously
- **Maintain data consistency**: Ensure ACID properties while improving performance

### Secondary Objectives
- Improve system observability with database operation metrics
- Reduce bot polling contention
- Enable graceful degradation under high load

## Solution Overview
### Approach
The solution removes application-level mutex locks and relies on SQLite's WAL mode for concurrency control. Context timeouts ensure operations don't block indefinitely, while retry logic handles transient conflicts gracefully.

### Key Components
1. **Database Layer**: Remove mutex locks, add retry logic, optimize pragmas
2. **API Layer**: Add context timeouts, improve error handling
3. **State Management**: Simplify locking strategy, use read transactions
4. **Monitoring**: Add metrics for lock wait times and transaction duration

### Architecture Diagram
```
[API Handler] → [Context Timeout] → [State Layer] → [Database Layer]
                     (10s)                            ↓
                                                [Retry Logic]
                                                     ↓
                                              [SQLite (WAL)]
```

### Data Flow
```
Request → Add Timeout → Remove App Lock → SQLite Lock → Retry if Busy → Response
```

### Expected Outcomes
- API endpoints respond within 10 seconds
- Multiple concurrent read operations execute without blocking
- Write operations complete without deadlocking
- System gracefully handles transient lock conflicts

## Implementation Tasks

### CRITICAL IMPLEMENTATION RULES
1. **NO PLACEHOLDER CODE**: Every implementation must be production-ready. NEVER write "TODO", "in a real implementation", or similar placeholders unless explicitly requested by the user.
2. **CROSS-DIRECTORY TASKS**: Group related changes across directories into single tasks to ensure consistency. Never create isolated changes that require follow-up work in sibling directories.
3. **COMPLETE IMPLEMENTATIONS**: Each task must fully implement its feature including all consumers, type updates, and integration points.
4. **DETAILED SPECIFICATIONS**: Each task must include EXACTLY what to implement, including specific functions, types, and integration points to avoid "breaking change" confusion.
5. **CONTEXT AWARENESS**: Each task is part of a larger system - specify how it connects to other parts.
6. **MAKE BREAKING CHANGES**: Unless explicitly requested by the user, you MUST make breaking changes.

### Visual Dependency Tree
```
pkg/
├── common/
│   ├── config.go (Task #0: Add DatabaseTimeout field to TimeoutConfig)
│   └── errors.go (Task #0: Add timeout and retry error types)
│
├── storage/
│   ├── database_config.go (Task #1: Add retry configuration)
│   ├── sqlite_retry.go (Task #2: Implement retry logic utilities)
│   └── sqlite.go (Task #3: Remove mutex locks, add retry wrapper)
│
├── master/
│   ├── api.go (Task #4: Add context timeouts to all handlers)
│   ├── routes.go (Task #5: Add timeout middleware)
│   └── state.go (Task #6: Optimize state layer locking)
│
└── service/
    ├── bot_service.go (Task #7: Optimize bot operations)
    └── job_service.go (Task #7: Optimize job operations)

master.yaml (Task #0: Add database timeout configuration)
```

### Execution Plan

#### Group A: Foundation (Execute all in parallel)
- [x] **Task #0**: Update configuration types and add error types
  - Folder: `pkg/common/`
  - Files: `config.go`, `errors.go`
  - Add to TimeoutConfig in config.go:
    ```go
    DatabaseOp      time.Duration `json:"database_op" yaml:"database_op"`
    DatabaseRetries int           `json:"database_retries" yaml:"database_retries"`
    ```
  - Add to errors.go:
    ```go
    type TimeoutError struct {
        Operation string
        Duration  time.Duration
    }
    type RetryExhaustedError struct {
        Operation string
        Attempts  int
        LastError error
    }
    ```
  - Update master.yaml with:
    ```yaml
    timeouts:
      database_op: "10s"
      database_retries: 5
    ```
  - Context: Foundation for timeout and retry handling throughout the system

#### Group B: Database Retry Infrastructure (Execute after Group A)
- [x] **Task #1**: Add retry configuration to database config
  - Folder: `pkg/storage/`
  - File: `database_config.go`
  - Add fields to DatabaseConfig:
    ```go
    MaxRetries     int           `json:"max_retries" yaml:"max_retries"`
    RetryDelay     time.Duration `json:"retry_delay" yaml:"retry_delay"`
    MaxRetryDelay  time.Duration `json:"max_retry_delay" yaml:"max_retry_delay"`
    RetryMultiplier float64      `json:"retry_multiplier" yaml:"retry_multiplier"`
    ```
  - Add validation in Validate() method
  - Set defaults: MaxRetries=5, RetryDelay=10ms, MaxRetryDelay=1s, RetryMultiplier=2.0
  - Context: Configuration for retry behavior

- [x] **Task #2**: Implement database retry utilities
  - Folder: `pkg/storage/`
  - File: `sqlite_retry.go` (new file)
  - Implements:
    ```go
    type RetryableFunc func() error
    type RetryableResultFunc[T any] func() (T, error)
    
    func ExecuteWithRetry(ctx context.Context, config DatabaseConfig, fn RetryableFunc) error
    func ExecuteWithRetryResult[T any](ctx context.Context, config DatabaseConfig, fn RetryableResultFunc[T]) (T, error)
    func isRetryableError(err error) bool
    ```
  - Include exponential backoff logic
  - Check for SQLite busy/locked errors
  - Respect context cancellation
  - Context: Reusable retry logic for all database operations

#### Group C: Database Layer Fixes (Execute after Group B)
- [x] **Task #3**: Fix SQLite implementation - remove locks, add retry
  - Folder: `pkg/storage/`
  - File: `sqlite.go`
  - Changes:
    1. Remove all mutex locks (s.mu) from SQLiteStorage struct and methods
    2. Update Open() to set optimal pragmas:
       ```go
       pragmas := []string{
           "PRAGMA journal_mode = WAL",
           "PRAGMA synchronous = NORMAL",
           "PRAGMA temp_store = MEMORY",
           "PRAGMA cache_size = -64000",
       }
       ```
    3. Wrap all database operations with retry logic:
       - Get, Set, Delete, Update methods
       - Query methods (GetBot, GetJob, etc.)
       - Use ExecuteWithRetry from sqlite_retry.go
    4. Fix Transaction() method:
       ```go
       func (s *SQLiteStorage) Transaction(ctx context.Context, fn func(tx common.Transaction) error) error {
           // NO LOCK HERE - let SQLite handle it
           return ExecuteWithRetry(ctx, s.config, func() error {
               tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
               // Use IMMEDIATE for write transactions
               tx.ExecContext(ctx, "BEGIN IMMEDIATE")
               // ... rest of transaction logic
           })
       }
       ```
    5. Add context timeout checks in long operations
  - Context: Core fix to enable concurrent database access

#### Group D: API Layer Timeouts (Execute after Group C)
- [x] **Task #4**: Add context timeouts to API handlers
  - Folder: `pkg/master/`
  - File: `api.go`
  - For EVERY handler that accesses the database, add timeout:
    ```go
    func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
        ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeouts.DatabaseOp)
        defer cancel()
        // Use ctx for all operations
    }
    ```
  - Update these handlers specifically:
    - handleSystemStats
    - handleBotList
    - handleJobList
    - handleGetCrashes
    - handleJobGet
    - All other database-accessing handlers
  - Add timeout error handling with proper HTTP status codes
  - Context: Ensures API requests don't hang indefinitely

- [x] **Task #5**: Add timeout middleware
  - Folder: `pkg/master/`
  - File: `routes.go`
  - Add middleware function:
    ```go
    func (s *Server) timeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler
    ```
  - Apply to all routes in setupRoutes()
  - Handle timeout errors gracefully with 504 Gateway Timeout
  - Context: Global timeout enforcement for all endpoints

#### Group E: State Layer Optimization (Execute after Group D)
- [x] **Task #6**: Optimize state layer locking and operations
  - Folder: `pkg/master/`
  - File: `state.go`
  - Changes:
    1. Use RLock() for read operations in PersistentState
    2. Minimize lock duration - acquire late, release early
    3. Don't hold locks during database operations
    4. Fix GetCrashes to use proper pagination at database level
    5. Add context support to all state methods
  - Example pattern:
    ```go
    func (ps *PersistentState) GetBot(ctx context.Context, id string) (*common.Bot, error) {
        // Don't lock here - database has its own concurrency control
        return ps.db.GetBot(ctx, id)
    }
    ```
  - Context: Reduces lock contention at state layer

#### Group F: Service Layer Optimization (Execute in parallel after Group E)
- [x] **Task #7**: Optimize bot and job service operations
  - Folders: `pkg/service/`
  - Files: `bot_service.go`, `job_service.go`
  - Bot service optimizations:
    1. Batch bot updates in ProcessHeartbeat
    2. Use UPDATE queries instead of read-modify-write
    3. Add context timeouts to all operations
  - Job service optimizations:
    1. Optimize job assignment queries
    2. Reduce transaction scope in AssignJob
    3. Add proper error handling for lock conflicts
  - Context: Reduces database contention from high-frequency operations

---

## Implementation Workflow

This plan file serves as the authoritative checklist for implementation. When implementing:

### Required Process
1. **Load Plan**: Read this entire plan file before starting
2. **Sync Tasks**: Create TodoWrite tasks matching the checkboxes below
3. **Execute & Update**: For each task:
   - Mark TodoWrite as `in_progress` when starting
   - Update checkbox `[ ]` to `[x]` when completing
   - Mark TodoWrite as `completed` when done
4. **Maintain Sync**: Keep this file and TodoWrite synchronized throughout

### Critical Rules
- This plan file is the source of truth for progress
- Update checkboxes in real-time as work progresses
- Never lose synchronization between plan file and TodoWrite
- Mark tasks complete only when fully implemented (no placeholders)
- Tasks should be run in parallel, unless there are dependencies, using subtasks, to avoid context bloat.

### Progress Tracking
The checkboxes above represent the authoritative status of each task. Keep them updated as you work.

## Testing Strategy

After implementation, verify the fixes with:

1. **Concurrent Read Test**: Multiple goroutines reading simultaneously
2. **Write Lock Test**: Ensure writes don't deadlock under load
3. **Timeout Test**: Verify API endpoints timeout gracefully
4. **Retry Test**: Confirm retry logic handles transient locks
5. **Load Test**: Run with high bot polling frequency

## Monitoring

Add these metrics to track improvement:

- `database_operation_duration_seconds` - Histogram of operation times
- `database_retry_count` - Counter of retry attempts
- `database_timeout_count` - Counter of timeout errors
- `api_request_duration_seconds` - Histogram by endpoint
- `concurrent_readers_gauge` - Current number of active readers