# 01: Replace Panics with Error Returns

## Priority: HIGH
## Risk Level: LOW
## Estimated Effort: 2-4 hours

## Problem Statement

The codebase uses `panic()` in 19 locations for recoverable error conditions. This causes application crashes instead of graceful error handling, making the system fragile and harder to debug.

## Invariants (MUST NOT CHANGE)

1. All existing functionality must continue to work
2. Error messages must be at least as descriptive as panic messages
3. Callers must handle the new errors (compile-time verification)
4. No silent failures - all errors must be logged or returned
5. Existing API responses must remain unchanged

## Files to Modify

### 1. pkg/service/manager.go (Lines 38-50)

**Current Code:**
```go
func NewManager(...) *Manager {
    if state == nil {
        panic("service manager requires state store to be initialized")
    }
    if timeoutManager == nil {
        panic("service manager requires timeout manager to be initialized")
    }
    if recoveryManager == nil {
        panic("service manager requires recovery manager to be initialized")
    }
    if config == nil {
        panic("service manager requires configuration to be initialized")
    }
    if logger == nil {
        panic("service manager requires logger to be initialized")
    }
    // ...
}
```

**Refactored Code:**
```go
func NewManager(...) (*Manager, error) {
    if state == nil {
        return nil, fmt.Errorf("service manager: state store is required")
    }
    if timeoutManager == nil {
        return nil, fmt.Errorf("service manager: timeout manager is required")
    }
    if recoveryManager == nil {
        return nil, fmt.Errorf("service manager: recovery manager is required")
    }
    if config == nil {
        return nil, fmt.Errorf("service manager: configuration is required")
    }
    if logger == nil {
        return nil, fmt.Errorf("service manager: logger is required")
    }
    // ...
    return manager, nil
}
```

**Callers to Update:**
- Search for `service.NewManager(` and update all call sites
- Expected locations: `cmd/master/main.go`

---

### 2. pkg/service/corpus_service.go (Lines 31, 34)

**Current Code:**
```go
func NewCorpusService(...) *CorpusService {
    if storage == nil {
        panic("corpus service requires storage to be initialized")
    }
    if logger == nil {
        panic("corpus service requires logger to be initialized")
    }
    // ...
}
```

**Refactored Code:**
```go
func NewCorpusService(...) (*CorpusService, error) {
    if storage == nil {
        return nil, fmt.Errorf("corpus service: storage is required")
    }
    if logger == nil {
        return nil, fmt.Errorf("corpus service: logger is required")
    }
    // ...
    return service, nil
}
```

**Callers to Update:**
- `pkg/service/manager.go` (line ~88)

---

### 3. pkg/domain/campaign/types/state.go (Line 86)

**Current Code:**
```go
func (s *CampaignState) UnmarshalJSON(data []byte) error {
    // ...
    if err := json.Unmarshal(data, &aux); err != nil {
        panic(err)  // Line 86
    }
    // ...
}
```

**Refactored Code:**
```go
func (s *CampaignState) UnmarshalJSON(data []byte) error {
    // ...
    if err := json.Unmarshal(data, &aux); err != nil {
        return fmt.Errorf("failed to unmarshal campaign state: %w", err)
    }
    // ...
}
```

**Note:** This function already returns error, so just change panic to return.

---

### 4. pkg/domain/bot/types/status.go (Line 89)

**Current Code:**
```go
func (s *BotStatus) UnmarshalJSON(data []byte) error {
    // ...
    panic(err)  // Line 89
    // ...
}
```

**Refactored Code:**
```go
func (s *BotStatus) UnmarshalJSON(data []byte) error {
    // ...
    return fmt.Errorf("failed to unmarshal bot status: %w", err)
    // ...
}
```

---

### 5. pkg/infrastructure/persistence/sqlite/helpers.go (Lines 131, 150, 174)

**Current Code:**
```go
func BuildInsertQuery(table string, columns []string) string {
    if len(columns) == 0 {
        panic("BuildInsertQuery: no columns provided")
    }
    // ...
}

func BuildUpdateQuery(table string, columns []string, whereColumn string) string {
    if len(columns) == 0 {
        panic("BuildUpdateQuery: no columns provided")
    }
    // ...
}

func BuildBulkInsertQuery(table string, columns []string, rowCount int) string {
    if len(columns) == 0 || rowCount <= 0 {
        panic("BuildBulkInsertQuery: invalid parameters")
    }
    // ...
}
```

**Refactored Code:**
```go
func BuildInsertQuery(table string, columns []string) (string, error) {
    if len(columns) == 0 {
        return "", fmt.Errorf("BuildInsertQuery: no columns provided for table %s", table)
    }
    // ...
    return query, nil
}

func BuildUpdateQuery(table string, columns []string, whereColumn string) (string, error) {
    if len(columns) == 0 {
        return "", fmt.Errorf("BuildUpdateQuery: no columns provided for table %s", table)
    }
    // ...
    return query, nil
}

func BuildBulkInsertQuery(table string, columns []string, rowCount int) (string, error) {
    if len(columns) == 0 {
        return "", fmt.Errorf("BuildBulkInsertQuery: no columns provided for table %s", table)
    }
    if rowCount <= 0 {
        return "", fmt.Errorf("BuildBulkInsertQuery: invalid row count %d for table %s", rowCount, table)
    }
    // ...
    return query, nil
}
```

**Callers to Update:**
- Search for `BuildInsertQuery(`, `BuildUpdateQuery(`, `BuildBulkInsertQuery(`
- Expected in: `pkg/infrastructure/persistence/sqlite/` repository files

---

### 6. pkg/infrastructure/persistence/sqlite/connection.go (Line 228)

**Current Code:**
```go
func (c *Connection) Transaction(ctx context.Context, fn func(tx *sql.Tx) error) error {
    // ...
    defer func() {
        if p := recover(); p != nil {
            tx.Rollback()
            panic(p) // Re-panic after rollback
        }
    }()
    // ...
}
```

**Decision:** KEEP THIS PANIC - It's a re-panic pattern for proper transaction cleanup. This is acceptable as it preserves the original panic after ensuring rollback.

---

### 7. pkg/storage/sqlite.go (Line 410) and pkg/storage/sqlite_retry.go (Line 265)

**Current Code:**
```go
defer func() {
    if p := recover(); p != nil {
        tx.Rollback()
        panic(p)
    }
}()
```

**Decision:** KEEP THESE PANICS - Same re-panic pattern for transaction safety.

---

### 8. pkg/infrastructure/messaging/example_usage.go (Line 167)

**Current Code:**
```go
panic(fmt.Sprintf("Failed to subscribe handler: %v", err))
```

**Decision:** This is in an example file. Either:
- Option A: Convert to `log.Fatalf` (preferred for examples)
- Option B: Add error return

**Refactored Code (Option A):**
```go
log.Fatalf("Failed to subscribe handler: %v", err)
```

---

### 9. pkg/web/api_docs/server.go (Line 227)

**Current Code:**
```go
panic(err)
```

**Context Needed:** Read the surrounding code to determine proper handling.

**Likely Refactor:**
```go
return fmt.Errorf("api docs server initialization failed: %w", err)
```

---

## Verification Steps

### 1. Compile-Time Verification
```bash
make build
```
All callers must be updated or compilation will fail.

### 2. Test Verification
```bash
make test
```

### 3. Grep for Remaining Panics
```bash
grep -rn "panic(" pkg/ --include="*.go" | grep -v "_test.go" | grep -v "recover()"
```
Should only show the acceptable re-panic patterns in transaction handling.

### 4. Error Path Testing
For each modified function, verify error paths work:
- Pass nil to NewManager() - should return error, not panic
- Pass empty columns to BuildInsertQuery() - should return error

## Rollback Plan

If issues arise:
1. `git checkout -- pkg/service/manager.go pkg/service/corpus_service.go`
2. Revert specific files as needed
3. All changes are backward compatible at runtime

## Notes for Future Runs

- The re-panic pattern in transaction handlers is INTENTIONAL and should NOT be changed
- Error messages should include context (function name, parameter values)
- Use `fmt.Errorf` with `%w` for error wrapping to preserve stack traces
- Sentinel errors are not needed here - descriptive messages are sufficient

## Completion Checklist

- [ ] pkg/service/manager.go - NewManager returns error
- [ ] pkg/service/corpus_service.go - NewCorpusService returns error
- [ ] pkg/domain/campaign/types/state.go - Return error instead of panic
- [ ] pkg/domain/bot/types/status.go - Return error instead of panic
- [ ] pkg/infrastructure/persistence/sqlite/helpers.go - All Build* functions return error
- [ ] pkg/infrastructure/messaging/example_usage.go - Use log.Fatalf
- [ ] pkg/web/api_docs/server.go - Return error
- [ ] All callers updated
- [ ] make build passes
- [ ] make test passes
- [ ] Manual verification of error paths
