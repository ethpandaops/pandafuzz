# Complete Fuzzer Integration and Resource Management Implementation Plan

## Executive Summary
> PandaFuzz currently has well-implemented fuzzer components (AFL++ and LibFuzzer) that are disconnected from the bot executor, resulting in duplicate implementations and missing functionality. Additionally, the system lacks critical resource management capabilities, leading to potential disk space exhaustion and resource leaks in production. This plan addresses both issues by:
> - Connecting existing fuzzer implementations to the bot executor through a unified interface
> - Implementing comprehensive result collection and reporting pipeline
> - Adding system resource monitoring for CPU, memory, and disk usage
> - Creating automated cleanup mechanisms for job artifacts and database maintenance
> - Establishing proper process lifecycle management to prevent orphaned processes

## Goals & Objectives
### Primary Goals
- **Complete Fuzzer Integration**: Connect existing fuzzer implementations to eliminate code duplication and enable full feature set (coverage tracking, corpus synchronization, real-time monitoring)
- **Resource Management**: Implement comprehensive cleanup and monitoring to prevent disk exhaustion and resource leaks in production environments

### Secondary Objectives
- Enable distributed corpus sharing between bots for improved fuzzing efficiency
- Add real-time progress streaming to master for better visibility
- Implement crash analysis and triage capabilities
- Establish automated database maintenance routines

## Solution Overview
### Approach
The solution refactors the bot executor to use the existing fuzzer interface implementations, adds a result collection pipeline, implements resource monitoring using system APIs, and creates automated cleanup routines triggered by configurable policies.

### Key Components
1. **Fuzzer Integration Layer**: Unified interface connecting bot executor to fuzzer implementations
2. **Result Collection Pipeline**: Event-driven system for collecting crashes, coverage, and corpus updates
3. **Resource Monitor**: Background service tracking CPU, memory, and disk usage
4. **Cleanup Manager**: Policy-based cleanup of job artifacts, crash files, and database maintenance
5. **Process Manager**: Lifecycle management for fuzzer processes with orphan detection

### Architecture Diagram
```
[Bot Agent]
    ├── [Fuzzer Interface] ←→ [AFL++ Implementation]
    │                      ←→ [LibFuzzer Implementation]
    ├── [Result Collector] → [Master API]
    ├── [Resource Monitor] → [Metrics/Alerts]
    └── [Cleanup Manager]  → [Disk/DB Maintenance]
```

### Data Flow
```
Fuzzer Output → Event Handler → Result Collector → Master API → Database
     ↓
Stats/Coverage → Aggregator → Periodic Reports → Master → Storage
```

### Expected Outcomes
- Fuzzing jobs execute using the proper fuzzer interface with all features enabled
- Coverage and corpus data is collected and synchronized between bots
- Resource usage stays within configured limits with automatic cleanup
- No orphaned processes or accumulated artifacts after job completion
- Database size remains manageable with automated maintenance

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
│   ├── types.go (Task #0: Add FuzzerEvent, ResourceMetrics types)
│   └── interfaces.go (Task #0: Define ResourceMonitor, CleanupPolicy interfaces)
│
├── bot/
│   ├── executor_fuzzer.go (Task #3: New fuzzer-based executor)
│   ├── result_collector.go (Task #4: Result collection pipeline)
│   ├── resource_monitor.go (Task #1: System resource monitoring)
│   ├── cleanup_manager.go (Task #2: Cleanup routines)
│   └── agent.go (Task #8: Wire everything together)
│
├── fuzzer/
│   ├── aflplusplus.go (Task #5: Add event handlers and result streaming)
│   ├── libfuzzer.go (Task #5: Add event handlers and result streaming)
│   └── base.go (Task #0: Add base fuzzer event types)
│
├── monitoring/
│   └── metrics.go (Task #6: Add resource usage metrics)
│
├── master/
│   ├── api.go (Task #7: Add streaming endpoints)
│   └── maintenance.go (Task #2: Database maintenance routines)
│
└── storage/
    └── sqlite.go (Task #2: Add cleanup queries)
```

### Execution Plan

#### Group A: Foundation Types and Interfaces (Execute all in parallel)
- [x] **Task #0**: Create fuzzer event types and resource monitoring interfaces
  - Folder: `pkg/common/`
  - Files: `types.go`, `interfaces.go`
  - New types in `types.go`:
    ```go
    type FuzzerEvent struct {
        Type      string    // "crash", "coverage", "corpus", "stats"
        Timestamp time.Time
        JobID     string
        Data      interface{}
    }
    
    type ResourceMetrics struct {
        CPUPercent    float64
        MemoryMB      int64
        DiskUsedMB    int64
        DiskFreeMB    int64
        ProcessCount  int
        Timestamp     time.Time
    }
    
    type CleanupPolicy struct {
        MaxJobAge       time.Duration
        MaxCrashAge     time.Duration
        MaxCorpusSize   int64
        MaxDiskUsage    float64 // percentage
        CleanupInterval time.Duration
    }
    ```
  - New interfaces in `interfaces.go`:
    ```go
    type ResourceMonitor interface {
        GetMetrics() (*ResourceMetrics, error)
        StartMonitoring(ctx context.Context, interval time.Duration)
        SetAlertThresholds(cpu, memory, disk float64)
    }
    
    type CleanupManager interface {
        SetPolicy(policy CleanupPolicy)
        RunCleanup(ctx context.Context) error
        ScheduleCleanup(ctx context.Context)
    }
    
    type FuzzerEventHandler interface {
        HandleEvent(event FuzzerEvent) error
    }
    ```
  - Exports: All types and interfaces
  - Context: Foundation for resource management and fuzzer integration

- [x] **Task #0**: Add fuzzer base event handling
  - Folder: `pkg/fuzzer/`
  - File: `base.go`
  - Implements:
    ```go
    type BaseFuzzer struct {
        eventHandlers []FuzzerEventHandler
    }
    
    func (b *BaseFuzzer) RegisterEventHandler(handler FuzzerEventHandler)
    func (b *BaseFuzzer) EmitEvent(event FuzzerEvent)
    ```
  - Exports: BaseFuzzer struct and methods
  - Context: Enables event-driven result collection from fuzzers

#### Group B: Core Services (Execute all in parallel after Group A)
- [x] **Task #1**: Implement system resource monitoring
  - Folder: `pkg/bot/`
  - File: `resource_monitor.go`
  - Imports:
    - `github.com/shirou/gopsutil/v3/cpu`
    - `github.com/shirou/gopsutil/v3/mem`
    - `github.com/shirou/gopsutil/v3/disk`
    - `github.com/shirou/gopsutil/v3/process`
  - Implements:
    ```go
    type SystemResourceMonitor struct {
        config        *common.BotConfig
        logger        *logrus.Logger
        cpuThreshold  float64
        memThreshold  float64
        diskThreshold float64
        alertChan     chan *common.ResourceMetrics
    }
    
    func NewResourceMonitor(config *common.BotConfig, logger *logrus.Logger) *SystemResourceMonitor
    func (m *SystemResourceMonitor) GetMetrics() (*common.ResourceMetrics, error)
    func (m *SystemResourceMonitor) StartMonitoring(ctx context.Context, interval time.Duration)
    func (m *SystemResourceMonitor) SetAlertThresholds(cpu, memory, disk float64)
    func (m *SystemResourceMonitor) checkThresholds(metrics *common.ResourceMetrics)
    ```
  - Integration: Used by bot agent to monitor resources and trigger cleanup
  - Note: Must handle cross-platform differences (Linux/Windows/Mac)

- [x] **Task #2**: Implement cleanup manager and database maintenance
  - Folder: `pkg/bot/`, `pkg/master/`, `pkg/storage/`
  - Files: 
    - `pkg/bot/cleanup_manager.go`
    - `pkg/master/maintenance.go`
    - `pkg/storage/sqlite.go` (additions)
  - Bot cleanup manager implements:
    ```go
    type JobCleanupManager struct {
        config   *common.BotConfig
        logger   *logrus.Logger
        policy   common.CleanupPolicy
        workDir  string
    }
    
    func NewCleanupManager(config *common.BotConfig, logger *logrus.Logger) *JobCleanupManager
    func (m *JobCleanupManager) SetPolicy(policy common.CleanupPolicy)
    func (m *JobCleanupManager) RunCleanup(ctx context.Context) error
    func (m *JobCleanupManager) ScheduleCleanup(ctx context.Context)
    func (m *JobCleanupManager) cleanJobDirectories(maxAge time.Duration) error
    func (m *JobCleanupManager) cleanCrashFiles(maxAge time.Duration) error
    func (m *JobCleanupManager) cleanCorpusFiles(maxSize int64) error
    func (m *JobCleanupManager) cleanLogs(maxAge time.Duration) error
    ```
  - Master maintenance implements:
    ```go
    type MaintenanceService struct {
        db     common.Database
        logger *logrus.Logger
        config *common.MasterConfig
    }
    
    func NewMaintenanceService(db common.Database, config *common.MasterConfig, logger *logrus.Logger) *MaintenanceService
    func (s *MaintenanceService) RunMaintenance(ctx context.Context) error
    func (s *MaintenanceService) ScheduleMaintenance(ctx context.Context, interval time.Duration)
    func (s *MaintenanceService) vacuumDatabase() error
    func (s *MaintenanceService) purgeOldCrashes(maxAge time.Duration) error
    func (s *MaintenanceService) purgeOldJobs(maxAge time.Duration) error
    ```
  - SQLite additions:
    ```go
    func (s *SQLiteStorage) DeleteOldCrashes(ctx context.Context, before time.Time) error
    func (s *SQLiteStorage) DeleteOldJobs(ctx context.Context, before time.Time) error
    func (s *SQLiteStorage) GetDatabaseSize(ctx context.Context) (int64, error)
    ```
  - Integration: Scheduled by bot/master to run periodically based on policy

#### Group C: Fuzzer Integration (Sequential after Group B)
- [x] **Task #3**: Create new fuzzer-based executor
  - Folder: `pkg/bot/`
  - File: `executor_fuzzer.go`
  - Imports:
    - `github.com/ethpandaops/pandafuzz/pkg/fuzzer`
    - `github.com/ethpandaops/pandafuzz/pkg/common`
  - Implements:
    ```go
    type FuzzerJobExecutor struct {
        config         *common.BotConfig
        logger         *logrus.Logger
        resultChan     chan common.FuzzerEvent
        activeFuzzers  map[string]fuzzer.Fuzzer
        mu             sync.RWMutex
    }
    
    func NewFuzzerJobExecutor(config *common.BotConfig, logger *logrus.Logger) *FuzzerJobExecutor
    func (e *FuzzerJobExecutor) ExecuteJob(job *common.Job) (bool, string, error)
    func (e *FuzzerJobExecutor) StopJob(jobID string) error
    func (e *FuzzerJobExecutor) GetJobStatus(jobID string) (string, bool)
    func (e *FuzzerJobExecutor) createFuzzer(job *common.Job) (fuzzer.Fuzzer, error)
    func (e *FuzzerJobExecutor) handleFuzzerEvents(jobID string)
    ```
  - Integration: Replaces the current RealJobExecutor in agent.go
  - Note: Must maintain backward compatibility with existing job structure

- [x] **Task #4**: Implement result collection pipeline
  - Folder: `pkg/bot/`
  - File: `result_collector.go`
  - Implements:
    ```go
    type ResultCollector struct {
        config      *common.BotConfig
        logger      *logrus.Logger
        masterURL   string
        httpClient  *http.Client
        eventChan   chan common.FuzzerEvent
        batchSize   int
        flushTicker *time.Ticker
    }
    
    func NewResultCollector(config *common.BotConfig, masterURL string, logger *logrus.Logger) *ResultCollector
    func (c *ResultCollector) Start(ctx context.Context)
    func (c *ResultCollector) HandleEvent(event common.FuzzerEvent) error
    func (c *ResultCollector) processCrashEvent(event common.FuzzerEvent) error
    func (c *ResultCollector) processCoverageEvent(event common.FuzzerEvent) error
    func (c *ResultCollector) processCorpusEvent(event common.FuzzerEvent) error
    func (c *ResultCollector) processStatsEvent(event common.FuzzerEvent) error
    func (c *ResultCollector) batchAndSend(ctx context.Context)
    ```
  - Integration: Receives events from FuzzerJobExecutor and sends to master
  - Note: Must handle network failures with retry logic

#### Group D: Fuzzer Enhancements (Execute in parallel after Group C)
- [x] **Task #5**: Add event handlers to fuzzer implementations
  - Folder: `pkg/fuzzer/`
  - Files: `aflplusplus.go`, `libfuzzer.go`
  - AFL++ additions:
    ```go
    func (a *AFLPlusPlus) parseAndEmitStats(stats map[string]string)
    func (a *AFLPlusPlus) detectAndEmitCrash(output string)
    func (a *AFLPlusPlus) monitorCorpusChanges()
    ```
  - LibFuzzer additions:
    ```go
    func (l *LibFuzzer) parseAndEmitCoverage(output string)
    func (l *LibFuzzer) detectAndEmitCrash(output string)
    func (l *LibFuzzer) emitPeriodicStats()
    ```
  - Integration: Emit events through BaseFuzzer to registered handlers
  - Note: Parse existing output monitoring to generate appropriate events

- [x] **Task #6**: Add resource usage metrics to monitoring
  - Folder: `pkg/monitoring/`
  - File: `metrics.go`
  - Additions:
    ```go
    var (
        cpuUsageGauge = prometheus.NewGaugeVec(...)
        memoryUsageGauge = prometheus.NewGaugeVec(...)
        diskUsageGauge = prometheus.NewGaugeVec(...)
        activeProcessesGauge = prometheus.NewGauge(...)
    )
    
    func UpdateResourceMetrics(metrics *common.ResourceMetrics)
    func RegisterResourceMetrics()
    ```
  - Integration: Called by resource monitor to expose metrics
  - Note: Follow existing Prometheus patterns in the file

#### Group E: API and Integration (Sequential after Group D)
- [x] **Task #7**: Add streaming and maintenance endpoints
  - Folder: `pkg/master/`
  - File: `api.go`
  - New endpoints:
    ```go
    func (s *Server) handleJobProgress(w http.ResponseWriter, r *http.Request) // GET /api/v1/jobs/{id}/progress (SSE)
    func (s *Server) handleBatchResults(w http.ResponseWriter, r *http.Request) // POST /api/v1/results/batch
    func (s *Server) handleMaintenanceTrigger(w http.ResponseWriter, r *http.Request) // POST /api/v1/system/maintenance
    func (s *Server) handleResourceMetrics(w http.ResponseWriter, r *http.Request) // GET /api/v1/bots/{id}/resources
    ```
  - Integration: Add routes in router setup, use new services
  - Note: SSE endpoint for real-time progress streaming

- [x] **Task #8**: Wire everything together in bot agent
  - Folder: `pkg/bot/`
  - File: `agent.go`
  - Changes:
    - Replace `executor: NewRealJobExecutor(...)` with `executor: NewFuzzerJobExecutor(...)`
    - Add fields:
      ```go
      resourceMonitor *SystemResourceMonitor
      cleanupManager  *JobCleanupManager
      resultCollector *ResultCollector
      ```
    - In `NewAgent()`:
      ```go
      resourceMonitor := NewResourceMonitor(config, logger)
      cleanupManager := NewCleanupManager(config, logger)
      resultCollector := NewResultCollector(config, masterURL, logger)
      ```
    - In `Start()`:
      ```go
      go a.resourceMonitor.StartMonitoring(ctx, 30*time.Second)
      go a.cleanupManager.ScheduleCleanup(ctx)
      go a.resultCollector.Start(ctx)
      ```
    - Connect result collector to executor event channel
  - Integration: Final wiring to make all components work together

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