# AFL++ Coverage Fix Implementation Plan

## Overview
> This plan addresses the critical issue where AFL++ shows 0 edges when run through PandaFuzz's process management layer, despite working correctly when executed directly. The problem involves process lifecycle management, where AFL++ becomes a zombie process immediately after starting, preventing proper shared memory setup for coverage collection.

## Current State Assessment

### Working Scenario
- AFL++ runs successfully when executed directly in Docker container
- Shows 21 edges when run with test binary compiled with afl-gcc
- Shared memory for coverage map is properly initialized

### Broken Scenario  
- AFL++ shows 0 edges when executed through PandaFuzz
- Process becomes zombie (<defunct>) immediately after starting
- Shared memory for coverage map fails to initialize
- Coverage data cannot be collected or reported

### Previous Fixes Applied
- Removed hardcoded `-n` flag that forced dumb mode from:
  - `/pkg/bot/executor_real.go:313-314`
  - `/pkg/fuzzer/aflplusplus.go:822`
- Removed `AFL_SKIP_BIN_CHECK=1` environment variable from:
  - `/pkg/bot/executor_real.go:333`
  - `/pkg/fuzzer/aflplusplus.go:213`
  - `/Dockerfile:236`
- Changed compilation from afl-clang-fast to afl-gcc

### Technical Constraints
- Must maintain compatibility with LibFuzzer and Honggfuzz
- Process management must handle graceful shutdown
- Coverage collection needs to work across all fuzzer engines
- Docker container constraints for shared memory

## Goals

1. Primary goal: Fix AFL++ process management to prevent zombie process creation
2. Enable proper shared memory initialization for AFL++ coverage map
3. Maintain consistent process lifecycle management across all fuzzer engines
4. Ensure coverage data is properly collected and reported
5. Non-functional requirements:
   - Zero performance degradation
   - Maintain backward compatibility
   - Proper error handling and recovery
   - Clean resource cleanup on shutdown

## Design Approach

### Architecture Overview

The fix requires addressing the process lifecycle management layer where AFL++ is spawned and monitored. The key issue is that the current implementation doesn't properly handle AFL++'s fork-server model, which requires special handling for:
- Process group management
- Signal handling and propagation
- Shared memory segment allocation
- Child process reaping

### Component Breakdown

1. **Process Executor (`pkg/bot/executor_real.go`)**
   - Purpose: Manages fuzzer process lifecycle
   - Responsibilities: 
     - Spawn fuzzer processes with correct process group
     - Handle signal propagation to child processes
     - Prevent zombie process creation
     - Monitor process health
   - Interfaces: Implements `Executor` interface

2. **AFL++ Engine (`pkg/domain/fuzzer/engines/aflplusplus/engine.go`)**
   - Purpose: AFL++-specific fuzzer implementation
   - Responsibilities:
     - Configure AFL++ command and environment
     - Handle AFL++-specific requirements (fork-server, SHM)
     - Parse AFL++ output and coverage data
   - Interfaces: Implements `Fuzzer` interface

3. **Coverage Collector (`pkg/bot/executor_fuzzer.go`)**
   - Purpose: Collect and process coverage data
   - Responsibilities:
     - Read coverage from shared memory or files
     - Parse and normalize coverage metrics
     - Report coverage to master node
   - Interfaces: Works with all fuzzer engines

## Implementation Approach

### 1. Fix Process Group Management

#### Specific Changes
- Modify process creation in `pkg/bot/executor_real.go`
- Set process group for AFL++ processes
- Implement proper signal handling

#### Files Affected
- `pkg/bot/executor_real.go` - Main process execution logic
- `pkg/bot/executor_fuzzer.go` - Fuzzer-specific execution

#### Sample Implementation
```go
// pkg/bot/executor_real.go
func (e *RealExecutor) executeCommand(ctx context.Context, job *types.Job) error {
    cmd := exec.CommandContext(ctx, job.Command, job.Args...)
    
    // Set process group for proper signal handling
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Setpgid: true,
        Pgid:    0, // Create new process group
    }
    
    // Handle AFL++ specific requirements
    if job.Fuzzer == "afl++" {
        // Ensure proper environment for fork-server
        cmd.Env = append(cmd.Env, 
            "AFL_NO_FORKSRV=0", // Enable fork-server
            "AFL_FORKSRV_INIT_TIMEOUT=30000", // 30s timeout
        )
    }
    
    // Start the process
    if err := cmd.Start(); err != nil {
        return fmt.Errorf("failed to start process: %w", err)
    }
    
    // Wait for process in separate goroutine to prevent zombies
    done := make(chan error, 1)
    go func() {
        done <- cmd.Wait()
    }()
    
    select {
    case <-ctx.Done():
        // Kill entire process group on context cancellation
        if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
            e.logger.WithError(err).Error("failed to kill process group")
        }
        return ctx.Err()
    case err := <-done:
        return err
    }
}
```

### 2. Implement Proper Child Process Reaping

#### Specific Changes
- Add SIGCHLD handler for zombie prevention
- Implement waitpid loop for child processes
- Ensure all child processes are properly reaped

#### Files Affected
- `pkg/bot/executor_real.go` - Add signal handling
- `pkg/bot/worker.go` - Worker lifecycle management

#### Sample Implementation
```go
// pkg/bot/executor_real.go
func (e *RealExecutor) setupSignalHandling() {
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGCHLD)
    
    go func() {
        for range sigChan {
            // Reap any zombie children
            for {
                var status syscall.WaitStatus
                pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
                if err != nil || pid <= 0 {
                    break
                }
                e.logger.WithField("pid", pid).Debug("reaped child process")
            }
        }
    }()
}
```

### 3. Fix Shared Memory Initialization

#### Specific Changes
- Ensure proper SHM permissions and size
- Wait for AFL++ initialization before proceeding
- Validate SHM creation and accessibility

#### Files Affected
- `pkg/domain/fuzzer/engines/aflplusplus/engine.go` - AFL++ configuration
- `pkg/fuzzer/aflplusplus.go` - AFL++ wrapper logic

#### Sample Implementation
```go
// pkg/domain/fuzzer/engines/aflplusplus/engine.go
func (e *AFLPlusPlusEngine) Start(ctx context.Context) error {
    // Set up shared memory environment
    shmID := fmt.Sprintf("afl_%d", os.Getpid())
    e.env = append(e.env,
        fmt.Sprintf("__AFL_SHM_ID=%s", shmID),
        "AFL_MAP_SIZE=65536", // Standard AFL map size
    )
    
    // Start AFL++ process
    if err := e.startProcess(ctx); err != nil {
        return fmt.Errorf("failed to start AFL++: %w", err)
    }
    
    // Wait for AFL++ initialization (fork-server setup)
    if err := e.waitForInitialization(ctx, 30*time.Second); err != nil {
        return fmt.Errorf("AFL++ initialization failed: %w", err)
    }
    
    return nil
}

func (e *AFLPlusPlusEngine) waitForInitialization(ctx context.Context, timeout time.Duration) error {
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            // Check if AFL++ has initialized (look for coverage map)
            if e.isCoverageMapReady() {
                return nil
            }
        }
    }
}
```

### 4. Improve Process Monitoring and Health Checks

#### Specific Changes
- Add periodic health checks for fuzzer processes
- Detect and recover from zombie states
- Implement proper error reporting

#### Files Affected
- `pkg/bot/executor_fuzzer.go` - Fuzzer execution monitoring
- `pkg/bot/worker.go` - Worker health monitoring

#### Sample Implementation
```go
// pkg/bot/executor_fuzzer.go
func (e *FuzzerExecutor) monitorProcess(ctx context.Context, cmd *exec.Cmd) error {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            // Check process state
            if err := e.checkProcessHealth(cmd); err != nil {
                e.logger.WithError(err).Error("process health check failed")
                
                // Attempt recovery
                if err := e.recoverProcess(ctx, cmd); err != nil {
                    return fmt.Errorf("failed to recover process: %w", err)
                }
            }
        }
    }
}

func (e *FuzzerExecutor) checkProcessHealth(cmd *exec.Cmd) error {
    // Check if process is zombie
    statPath := fmt.Sprintf("/proc/%d/stat", cmd.Process.Pid)
    data, err := os.ReadFile(statPath)
    if err != nil {
        return fmt.Errorf("failed to read process stat: %w", err)
    }
    
    // Parse stat to check for zombie state (Z)
    if strings.Contains(string(data), ") Z ") {
        return fmt.Errorf("process is zombie")
    }
    
    return nil
}
```

### 5. Add Comprehensive Logging and Debugging

#### Specific Changes
- Add detailed logging for process lifecycle events
- Log environment variables and command arguments
- Add coverage collection debugging output

#### Files Affected
- All executor and fuzzer files
- Add structured logging fields

#### Sample Implementation
```go
// pkg/bot/executor_real.go
func (e *RealExecutor) logProcessStart(job *types.Job, cmd *exec.Cmd) {
    e.logger.WithFields(logrus.Fields{
        "job_id":     job.ID,
        "fuzzer":     job.Fuzzer,
        "command":    cmd.Path,
        "args":       cmd.Args,
        "env_count":  len(cmd.Env),
        "pid":        cmd.Process.Pid,
        "pgid":       cmd.SysProcAttr.Pgid,
    }).Info("starting fuzzer process")
    
    // Log AFL++ specific environment in debug mode
    if job.Fuzzer == "afl++" {
        for _, env := range cmd.Env {
            if strings.HasPrefix(env, "AFL_") || strings.HasPrefix(env, "__AFL_") {
                e.logger.WithField("env", env).Debug("AFL++ environment variable")
            }
        }
    }
}
```

## Testing Strategy

### Unit Testing
- [ ] Test process group creation and management
- [ ] Test signal propagation to child processes
- [ ] Test zombie process prevention
- [ ] Test shared memory initialization
- [ ] Test coverage data collection
- [ ] Mock process execution for error scenarios

### Integration Testing
- [ ] Test AFL++ execution through PandaFuzz
- [ ] Verify coverage edges are detected (>0)
- [ ] Test graceful shutdown and cleanup
- [ ] Test with different AFL++ configurations
- [ ] Verify compatibility with LibFuzzer and Honggfuzz
- [ ] Test Docker container shared memory limits

### Manual Validation
- [ ] Run AFL++ directly vs through PandaFuzz
- [ ] Compare coverage metrics between direct and managed execution
- [ ] Monitor process states during execution
- [ ] Check for zombie processes after shutdown
- [ ] Verify shared memory segments are cleaned up

### Validation Criteria
- [ ] AFL++ shows >0 edges when run through PandaFuzz
- [ ] No zombie processes created during execution
- [ ] Coverage data matches direct execution results
- [ ] Clean shutdown without orphaned processes
- [ ] All fuzzer engines continue to work

## Implementation Timeline

### Phase 1: Process Management Fix (Priority: Critical)
- [ ] Implement process group management
- [ ] Add SIGCHLD handler for zombie prevention
- [ ] Update process execution in executor_real.go
- [ ] Test process lifecycle management
- Dependencies: None

### Phase 2: Shared Memory Fix (Priority: Critical)
- [ ] Fix shared memory initialization
- [ ] Add AFL++ initialization wait logic
- [ ] Implement coverage map validation
- [ ] Test shared memory access
- Dependencies: Phase 1 completion

### Phase 3: Monitoring and Recovery (Priority: High)
- [ ] Add process health monitoring
- [ ] Implement zombie detection
- [ ] Add recovery mechanisms
- [ ] Test error scenarios
- Dependencies: Phase 1, Phase 2

### Phase 4: Testing and Validation (Priority: Critical)
- [ ] Run comprehensive unit tests
- [ ] Execute integration test suite
- [ ] Perform manual validation
- [ ] Document findings
- Dependencies: Phase 1, Phase 2, Phase 3

### Phase 5: Cleanup and Documentation (Priority: Medium)
- [ ] Add comprehensive logging
- [ ] Update documentation
- [ ] Clean up debug code
- [ ] Performance optimization
- Dependencies: Phase 4

## Risks and Considerations

### Implementation Risks
- **Process Group Changes**: May affect other fuzzer engines
  - Mitigation: Test all engines thoroughly, make changes conditional
- **Signal Handling**: Could interfere with existing signal handlers
  - Mitigation: Use targeted signal handling, coordinate with existing handlers
- **Shared Memory Limits**: Docker may have SHM size restrictions
  - Mitigation: Configure Docker with appropriate --shm-size, add validation

### Performance Considerations
- **Health Monitoring Overhead**: Frequent checks could impact performance
  - Mitigation: Use appropriate check intervals, optimize check logic
- **Process Reaping**: Wait4 calls could block
  - Mitigation: Use WNOHANG flag, implement in separate goroutine

### Security Considerations
- **Shared Memory Access**: Ensure proper permissions
  - Mitigation: Set restrictive permissions, validate access
- **Process Isolation**: Maintain process boundaries
  - Mitigation: Use process groups, proper signal masking

### Compatibility Concerns
- **Other Fuzzer Engines**: Changes must not break LibFuzzer/Honggfuzz
  - Mitigation: Make AFL++-specific changes conditional
- **Docker Versions**: Different Docker versions handle SHM differently
  - Mitigation: Test on multiple Docker versions, add version checks

## Expected Outcomes

### Success Metrics
- AFL++ coverage edges: >0 when run through PandaFuzz
- Process zombie count: 0 during and after execution
- Coverage accuracy: 95%+ match with direct execution
- Process cleanup time: <1 second after termination
- Memory leak detection: 0 leaks in 24-hour run

### Functional Outcomes
- AFL++ runs successfully through PandaFuzz process management
- Coverage data is accurately collected and reported
- No zombie processes are created during execution
- Clean shutdown with proper resource cleanup
- All three fuzzer engines work consistently

### Performance Targets
- Process startup time: <500ms
- Coverage collection overhead: <5%
- Memory usage: No increase from current baseline
- CPU overhead for monitoring: <1%

### User Experience Improvements
- Clear error messages when issues occur
- Detailed logging for debugging
- Consistent behavior across all fuzzer engines
- Reliable coverage reporting