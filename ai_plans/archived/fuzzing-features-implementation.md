# Fuzzing Platform Features Implementation Plan

## Executive Summary
This plan implements six major features to transform PandaFuzz from a job-based fuzzing system to a campaign-based platform with advanced corpus management. The solution maintains the master as the single source of truth while adding resilient campaign management, intelligent crash deduplication, and corpus evolution tracking.

**Key Changes:**
- **Campaign Layer**: Groups related fuzzing jobs with shared configuration and corpus
- **Stack-based Deduplication**: Reduces duplicate crashes using top-5 stack frames
- **Cross-campaign Corpus Sharing**: Automatically shares coverage-increasing inputs between campaigns
- **Evolution Tracking**: Monitors corpus growth and coverage contribution over time
- **Distributed Sync**: Master-mediated corpus synchronization between bots
- **Modern API**: RESTful v2 API running alongside v1 for backward compatibility
- **Simple Web UI**: Integrated dashboard for campaign and crash management

## Goals & Objectives
### Primary Goals
- **Campaign Management**: Group fuzzing jobs into manageable campaigns with 95% uptime
- **Crash Reduction**: Achieve 80% reduction in duplicate crash reports through stack-based deduplication
- **Coverage Optimization**: Increase fuzzing efficiency by 40% through intelligent corpus sharing
- **Operational Visibility**: Provide real-time campaign progress and corpus evolution metrics

### Secondary Objectives
- Zero-downtime migration from job-based to campaign-based system
- Maintain backward compatibility with existing bot agents
- Enable corpus analysis for security research
- Support distributed fuzzing at scale (100+ bots)

## Solution Overview
### Approach
The implementation adds a campaign management layer above the existing job system, treating campaigns as first-class entities that own jobs. All state remains in the master database with bots reporting to the master. Corpus synchronization happens through the master to maintain consistency.

### Key Components
1. **Campaign Service**: Manages campaign lifecycle, auto-restart, and job orchestration
2. **Deduplication Engine**: Stack-based crash analysis using configurable frame depth
3. **Corpus Manager**: Tracks file evolution, coverage contribution, and cross-campaign sharing
4. **Sync Coordinator**: Manages corpus distribution with conflict resolution
5. **Web Dashboard**: Real-time campaign monitoring and management interface
6. **API v2**: Modern REST endpoints with WebSocket support for real-time updates

### Architecture Diagram
```
[Web UI] → [API Gateway] → [Master Service]
                               ↓
                    [Campaign Manager] ← [State Store]
                         ↓       ↑
                    [Job Service] [Corpus Manager]
                         ↓             ↑
                    [Bot Pool] ←→ [Sync Coordinator]
```

### Data Flow
```
Bot → Crash/Coverage → Master → Dedup → Campaign Stats → UI
                         ↓
                    Corpus Sync → Other Bots
```

### Expected Outcomes
- Campaigns automatically restart on completion with updated corpus
- Duplicate crashes reduced from 100s to unique stacks only
- Coverage increases 40% faster through corpus sharing
- Real-time visibility into fuzzing progress via web dashboard
- Bots remain synchronized within 30 seconds of new coverage

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
│   ├── types.go (Task #0: Campaign, StackTrace, CorpusFile types)
│   ├── interfaces.go (Task #0: Service interfaces for DI)
│   └── errors.go (Task #0: Custom error types)
│
├── storage/
│   ├── migrations.go (Task #1: Campaign schema migrations)
│   ├── campaign_queries.go (Task #2: Campaign CRUD operations)
│   ├── dedup_queries.go (Task #2: Stack-based dedup queries)
│   └── corpus_queries.go (Task #2: Corpus evolution queries)
│
├── service/
│   ├── campaign_service.go (Task #3: Campaign business logic)
│   ├── dedup_service.go (Task #3: Crash deduplication logic)
│   └── corpus_service.go (Task #3: Corpus management logic)
│
├── master/
│   ├── handlers_campaign.go (Task #4: Campaign API handlers)
│   ├── handlers_corpus.go (Task #4: Corpus sync handlers)
│   ├── api_v2.go (Task #5: API v2 implementation)
│   ├── websocket.go (Task #5: Real-time updates)
│   └── campaign_state.go (Task #6: Campaign state management)
│
├── bot/
│   ├── corpus_sync.go (Task #7: Bot corpus sync client)
│   └── stack_reporter.go (Task #7: Enhanced crash reporting)
│
web/
├── static/
│   ├── index.html (Task #8: Main dashboard)
│   ├── campaigns.html (Task #8: Campaign management)
│   └── crashes.html (Task #8: Crash analysis view)
│
├── js/
│   ├── api.js (Task #9: API v2 client)
│   ├── campaigns.js (Task #9: Campaign UI logic)
│   └── websocket.js (Task #9: Real-time updates)
│
└── css/
    └── dashboard.css (Task #10: Simple, clean styling)
```

### Execution Plan

#### Group A: Foundation Types & Storage (Execute all in parallel)
- [x] **Task #0**: Create campaign and corpus types
  - Folder: `pkg/common/`
  - Files: `types.go`, `interfaces.go`, `errors.go`
  - Implements in types.go:
    ```go
    type Campaign struct {
        ID            string
        Name          string
        Description   string
        Status        CampaignStatus
        TargetBinary  string
        BinaryHash    string
        CreatedAt     time.Time
        UpdatedAt     time.Time
        CompletedAt   *time.Time
        AutoRestart   bool
        MaxDuration   time.Duration
        MaxJobs       int
        JobTemplate   JobConfig
        SharedCorpus  bool
        Tags          []string
    }
    
    type CampaignStatus string
    const (
        CampaignStatusPending   = "pending"
        CampaignStatusRunning   = "running"
        CampaignStatusCompleted = "completed"
        CampaignStatusFailed    = "failed"
        CampaignStatusPaused    = "paused"
    )
    
    type StackFrame struct {
        Function string
        File     string
        Line     int
        Offset   uint64
    }
    
    type StackTrace struct {
        Frames     []StackFrame
        TopNHash   string  // Hash of top N frames
        FullHash   string  // Hash of complete trace
        RawTrace   string
    }
    
    type CorpusFile struct {
        ID           string
        CampaignID   string
        JobID        string
        BotID        string
        Filename     string
        Hash         string
        Size         int64
        Coverage     int64     // Edges covered
        NewCoverage  int64     // New edges this file found
        ParentHash   string    // File this was mutated from
        Generation   int       // Mutation generation
        CreatedAt    time.Time
        SyncedAt     *time.Time
        IsSeed       bool
    }
    
    type CorpusEvolution struct {
        CampaignID    string
        Timestamp     time.Time
        TotalFiles    int
        TotalSize     int64
        TotalCoverage int64
        NewFiles      int
        NewCoverage   int64
    }
    
    type CrashGroup struct {
        ID            string
        CampaignID    string
        StackHash     string
        FirstSeen     time.Time
        LastSeen      time.Time
        Count         int
        Severity      string
        StackFrames   []StackFrame
        ExampleCrash  string  // ID of representative crash
    }
    ```
  - Implements in interfaces.go:
    ```go
    type CampaignService interface {
        Create(ctx context.Context, campaign *Campaign) error
        Get(ctx context.Context, id string) (*Campaign, error)
        List(ctx context.Context, filters CampaignFilters) ([]*Campaign, error)
        Update(ctx context.Context, id string, updates CampaignUpdates) error
        Delete(ctx context.Context, id string) error
        GetStatistics(ctx context.Context, id string) (*CampaignStats, error)
        RestartCampaign(ctx context.Context, id string) error
    }
    
    type DeduplicationService interface {
        ProcessCrash(ctx context.Context, crash *CrashResult) (*CrashGroup, bool, error)
        GetCrashGroups(ctx context.Context, campaignID string) ([]*CrashGroup, error)
        GetStackTrace(ctx context.Context, crashID string) (*StackTrace, error)
    }
    
    type CorpusService interface {
        AddFile(ctx context.Context, file *CorpusFile) error
        GetEvolution(ctx context.Context, campaignID string) ([]*CorpusEvolution, error)
        SyncCorpus(ctx context.Context, campaignID string, botID string) ([]*CorpusFile, error)
        ShareCorpus(ctx context.Context, fromCampaign, toCampaign string) error
    }
    ```
  - Implements in errors.go:
    ```go
    var (
        ErrCampaignNotFound = errors.New("campaign not found")
        ErrCampaignRunning = errors.New("campaign is already running")
        ErrInvalidStackTrace = errors.New("invalid stack trace format")
        ErrCorpusFileTooLarge = errors.New("corpus file exceeds size limit")
        ErrDuplicateCorpusFile = errors.New("corpus file already exists")
    )
    ```
  - Exports: All types, interfaces, and errors
  - Context: Foundation for campaign system

- [x] **Task #1**: Create database migrations for campaigns
  - Folder: `pkg/storage/`
  - File: `migrations.go`
  - Implements: AddCampaignTables() migration function
  - SQL schemas:
    ```sql
    -- Campaigns table
    CREATE TABLE campaigns (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        description TEXT,
        status TEXT NOT NULL,
        target_binary TEXT NOT NULL,
        binary_hash TEXT NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        completed_at TIMESTAMP,
        auto_restart BOOLEAN DEFAULT FALSE,
        max_duration INTEGER,
        max_jobs INTEGER DEFAULT 10,
        job_template TEXT NOT NULL, -- JSON
        shared_corpus BOOLEAN DEFAULT TRUE,
        tags TEXT -- JSON array
    );
    
    -- Campaign jobs relationship
    CREATE TABLE campaign_jobs (
        campaign_id TEXT NOT NULL,
        job_id TEXT NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        PRIMARY KEY (campaign_id, job_id),
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id),
        FOREIGN KEY (job_id) REFERENCES jobs(id)
    );
    
    -- Corpus files tracking
    CREATE TABLE corpus_files (
        id TEXT PRIMARY KEY,
        campaign_id TEXT NOT NULL,
        job_id TEXT,
        bot_id TEXT,
        filename TEXT NOT NULL,
        hash TEXT NOT NULL UNIQUE,
        size INTEGER NOT NULL,
        coverage INTEGER DEFAULT 0,
        new_coverage INTEGER DEFAULT 0,
        parent_hash TEXT,
        generation INTEGER DEFAULT 0,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        synced_at TIMESTAMP,
        is_seed BOOLEAN DEFAULT FALSE,
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id)
    );
    
    -- Corpus evolution tracking
    CREATE TABLE corpus_evolution (
        campaign_id TEXT NOT NULL,
        timestamp TIMESTAMP NOT NULL,
        total_files INTEGER NOT NULL,
        total_size INTEGER NOT NULL,
        total_coverage INTEGER NOT NULL,
        new_files INTEGER DEFAULT 0,
        new_coverage INTEGER DEFAULT 0,
        PRIMARY KEY (campaign_id, timestamp),
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id)
    );
    
    -- Crash groups for deduplication
    CREATE TABLE crash_groups (
        id TEXT PRIMARY KEY,
        campaign_id TEXT NOT NULL,
        stack_hash TEXT NOT NULL,
        first_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        last_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        count INTEGER DEFAULT 1,
        severity TEXT,
        stack_frames TEXT NOT NULL, -- JSON
        example_crash TEXT,
        UNIQUE(campaign_id, stack_hash),
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id)
    );
    
    -- Stack traces
    CREATE TABLE stack_traces (
        crash_id TEXT PRIMARY KEY,
        top_n_hash TEXT NOT NULL,
        full_hash TEXT NOT NULL,
        frames TEXT NOT NULL, -- JSON
        raw_trace TEXT,
        FOREIGN KEY (crash_id) REFERENCES crashes(id)
    );
    
    -- Add campaign_id to crashes table
    ALTER TABLE crashes ADD COLUMN campaign_id TEXT;
    ALTER TABLE crashes ADD COLUMN crash_group_id TEXT;
    
    -- Indexes for performance
    CREATE INDEX idx_campaign_status ON campaigns(status);
    CREATE INDEX idx_corpus_campaign ON corpus_files(campaign_id);
    CREATE INDEX idx_corpus_hash ON corpus_files(hash);
    CREATE INDEX idx_crash_group_campaign ON crash_groups(campaign_id);
    CREATE INDEX idx_stack_trace_hash ON stack_traces(top_n_hash);
    ```
  - Integration: Run after existing migrations
  - Context: Extends existing schema for campaigns

#### Group B: Data Access Layer (Depends on Group A)
- [x] **Task #2**: Implement campaign data access
  - Folder: `pkg/storage/`
  - Files: `campaign_queries.go`, `dedup_queries.go`, `corpus_queries.go`
  - Implements in campaign_queries.go:
    ```go
    // All methods assume s *SQLiteStorage receiver
    func (s *SQLiteStorage) CreateCampaign(ctx context.Context, c *Campaign) error
    func (s *SQLiteStorage) GetCampaign(ctx context.Context, id string) (*Campaign, error)
    func (s *SQLiteStorage) ListCampaigns(ctx context.Context, limit, offset int, status string) ([]*Campaign, error)
    func (s *SQLiteStorage) UpdateCampaign(ctx context.Context, id string, updates map[string]interface{}) error
    func (s *SQLiteStorage) DeleteCampaign(ctx context.Context, id string) error
    func (s *SQLiteStorage) GetCampaignJobs(ctx context.Context, campaignID string) ([]*Job, error)
    func (s *SQLiteStorage) LinkJobToCampaign(ctx context.Context, campaignID, jobID string) error
    func (s *SQLiteStorage) GetCampaignStatistics(ctx context.Context, campaignID string) (*CampaignStats, error)
    ```
  - Implements in dedup_queries.go:
    ```go
    func (s *SQLiteStorage) CreateCrashGroup(ctx context.Context, cg *CrashGroup) error
    func (s *SQLiteStorage) GetCrashGroup(ctx context.Context, campaignID, stackHash string) (*CrashGroup, error)
    func (s *SQLiteStorage) UpdateCrashGroupCount(ctx context.Context, id string) error
    func (s *SQLiteStorage) ListCrashGroups(ctx context.Context, campaignID string) ([]*CrashGroup, error)
    func (s *SQLiteStorage) CreateStackTrace(ctx context.Context, crashID string, st *StackTrace) error
    func (s *SQLiteStorage) GetStackTrace(ctx context.Context, crashID string) (*StackTrace, error)
    func (s *SQLiteStorage) LinkCrashToGroup(ctx context.Context, crashID, groupID string) error
    ```
  - Implements in corpus_queries.go:
    ```go
    func (s *SQLiteStorage) AddCorpusFile(ctx context.Context, cf *CorpusFile) error
    func (s *SQLiteStorage) GetCorpusFiles(ctx context.Context, campaignID string) ([]*CorpusFile, error)
    func (s *SQLiteStorage) GetCorpusFileByHash(ctx context.Context, hash string) (*CorpusFile, error)
    func (s *SQLiteStorage) UpdateCorpusCoverage(ctx context.Context, id string, coverage, newCoverage int64) error
    func (s *SQLiteStorage) RecordCorpusEvolution(ctx context.Context, ce *CorpusEvolution) error
    func (s *SQLiteStorage) GetCorpusEvolution(ctx context.Context, campaignID string, limit int) ([]*CorpusEvolution, error)
    func (s *SQLiteStorage) GetUnsyncedCorpusFiles(ctx context.Context, campaignID, botID string) ([]*CorpusFile, error)
    func (s *SQLiteStorage) MarkCorpusFilesSynced(ctx context.Context, fileIDs []string, botID string) error
    ```
  - SQL queries: Use prepared statements with s.db.PrepareContext()
  - Error handling: Wrap all errors with context
  - Context: Data layer for campaign features

#### Group C: Business Logic Layer (Depends on Group B)
- [x] **Task #3**: Implement campaign services
  - Folder: `pkg/service/`
  - Files: `campaign_service.go`, `dedup_service.go`, `corpus_service.go`
  - Implements in campaign_service.go:
    ```go
    type campaignService struct {
        storage     Storage
        jobService  JobService
        logger      logrus.FieldLogger
    }
    
    func NewCampaignService(storage Storage, jobService JobService, logger logrus.FieldLogger) CampaignService
    func (cs *campaignService) Create(ctx context.Context, campaign *Campaign) error
    func (cs *campaignService) Get(ctx context.Context, id string) (*Campaign, error)
    func (cs *campaignService) List(ctx context.Context, filters CampaignFilters) ([]*Campaign, error)
    func (cs *campaignService) Update(ctx context.Context, id string, updates CampaignUpdates) error
    func (cs *campaignService) Delete(ctx context.Context, id string) error
    func (cs *campaignService) GetStatistics(ctx context.Context, id string) (*CampaignStats, error)
    func (cs *campaignService) RestartCampaign(ctx context.Context, id string) error
    func (cs *campaignService) createJobsForCampaign(ctx context.Context, campaign *Campaign) error
    func (cs *campaignService) checkCampaignCompletion(ctx context.Context, campaignID string) error
    ```
  - Implements in dedup_service.go:
    ```go
    type dedupService struct {
        storage Storage
        logger  logrus.FieldLogger
        topNFrames int  // Configurable, default 5
    }
    
    func NewDeduplicationService(storage Storage, logger logrus.FieldLogger) DeduplicationService
    func (ds *dedupService) ProcessCrash(ctx context.Context, crash *CrashResult) (*CrashGroup, bool, error)
    func (ds *dedupService) parseStackTrace(rawTrace string) (*StackTrace, error)
    func (ds *dedupService) computeStackHash(frames []StackFrame, n int) string
    func (ds *dedupService) GetCrashGroups(ctx context.Context, campaignID string) ([]*CrashGroup, error)
    func (ds *dedupService) GetStackTrace(ctx context.Context, crashID string) (*StackTrace, error)
    ```
  - Implements in corpus_service.go:
    ```go
    type corpusService struct {
        storage     Storage
        fileStorage FileStorage
        logger      logrus.FieldLogger
    }
    
    func NewCorpusService(storage Storage, fileStorage FileStorage, logger logrus.FieldLogger) CorpusService
    func (cs *corpusService) AddFile(ctx context.Context, file *CorpusFile) error
    func (cs *corpusService) GetEvolution(ctx context.Context, campaignID string) ([]*CorpusEvolution, error)
    func (cs *corpusService) SyncCorpus(ctx context.Context, campaignID string, botID string) ([]*CorpusFile, error)
    func (cs *corpusService) ShareCorpus(ctx context.Context, fromCampaign, toCampaign string) error
    func (cs *corpusService) trackEvolution(ctx context.Context, campaignID string) error
    func (cs *corpusService) findCoverageIncreasingFiles(ctx context.Context, campaignID string) ([]*CorpusFile, error)
    ```
  - Business rules: Auto-restart logic, dedup threshold, sharing criteria
  - Context: Core business logic for campaigns

#### Group D: API Layer (Depends on Group C)
- [x] **Task #4**: Implement campaign API handlers
  - Folder: `pkg/master/`
  - Files: `handlers_campaign.go`, `handlers_corpus.go`
  - Implements in handlers_campaign.go:
    ```go
    func (s *Server) handleCreateCampaign(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleGetCampaign(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleListCampaigns(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleUpdateCampaign(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleDeleteCampaign(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleRestartCampaign(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleGetCampaignStats(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleUploadCampaignBinary(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleUploadCampaignCorpus(w http.ResponseWriter, r *http.Request)
    ```
  - Implements in handlers_corpus.go:
    ```go
    func (s *Server) handleGetCorpusEvolution(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleSyncCorpus(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleShareCorpus(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleListCorpusFiles(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleDownloadCorpusFile(w http.ResponseWriter, r *http.Request)
    ```
  - Routes in routes.go:
    ```go
    // Campaign routes
    r.HandleFunc("/api/v1/campaigns", s.handleCreateCampaign).Methods("POST")
    r.HandleFunc("/api/v1/campaigns", s.handleListCampaigns).Methods("GET")
    r.HandleFunc("/api/v1/campaigns/{id}", s.handleGetCampaign).Methods("GET")
    r.HandleFunc("/api/v1/campaigns/{id}", s.handleUpdateCampaign).Methods("PUT")
    r.HandleFunc("/api/v1/campaigns/{id}", s.handleDeleteCampaign).Methods("DELETE")
    r.HandleFunc("/api/v1/campaigns/{id}/restart", s.handleRestartCampaign).Methods("POST")
    r.HandleFunc("/api/v1/campaigns/{id}/stats", s.handleGetCampaignStats).Methods("GET")
    r.HandleFunc("/api/v1/campaigns/{id}/binary", s.handleUploadCampaignBinary).Methods("POST")
    r.HandleFunc("/api/v1/campaigns/{id}/corpus", s.handleUploadCampaignCorpus).Methods("POST")
    
    // Corpus routes
    r.HandleFunc("/api/v1/campaigns/{id}/corpus/evolution", s.handleGetCorpusEvolution).Methods("GET")
    r.HandleFunc("/api/v1/campaigns/{id}/corpus/sync", s.handleSyncCorpus).Methods("POST")
    r.HandleFunc("/api/v1/campaigns/{id}/corpus/share", s.handleShareCorpus).Methods("POST")
    r.HandleFunc("/api/v1/campaigns/{id}/corpus/files", s.handleListCorpusFiles).Methods("GET")
    r.HandleFunc("/api/v1/campaigns/{id}/corpus/files/{hash}", s.handleDownloadCorpusFile).Methods("GET")
    ```
  - Integration: Add to existing router
  - Context: HTTP interface for campaigns

- [x] **Task #5**: Implement API v2 with WebSocket support
  - Folder: `pkg/master/`
  - Files: `api_v2.go`, `websocket.go`
  - Implements in api_v2.go:
    ```go
    func (s *Server) setupAPIv2Routes(r *mux.Router)
    // All v1 endpoints with improved structure
    // Plus new endpoints:
    func (s *Server) handleV2GetCampaignTimeline(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleV2GetCrashGroups(w http.ResponseWriter, r *http.Request)
    func (s *Server) handleV2StreamBotMetrics(w http.ResponseWriter, r *http.Request)
    ```
  - Implements in websocket.go:
    ```go
    type WSHub struct {
        clients    map[*WSClient]bool
        broadcast  chan WSMessage
        register   chan *WSClient
        unregister chan *WSClient
    }
    
    func NewWSHub() *WSHub
    func (h *WSHub) Run()
    func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request)
    func (s *Server) broadcastCampaignUpdate(campaignID string, update interface{})
    func (s *Server) broadcastCrashFound(crash *CrashResult)
    func (s *Server) broadcastCorpusUpdate(campaignID string, update *CorpusEvolution)
    ```
  - WebSocket messages: JSON with type field
  - API versioning: /api/v2/ prefix
  - Context: Modern API with real-time updates

#### Group E: State Management (Depends on Group D)
- [x] **Task #6**: Implement campaign state management
  - Folder: `pkg/master/`
  - File: `campaign_state.go`
  - Implements:
    ```go
    type CampaignStateManager struct {
        campaigns map[string]*CampaignState
        mu        sync.RWMutex
        storage   Storage
        jobSvc    JobService
        corpusSvc CorpusService
        wsHub     *WSHub
        logger    logrus.FieldLogger
    }
    
    type CampaignState struct {
        Campaign      *Campaign
        ActiveJobs    map[string]*Job
        CompletedJobs map[string]*Job
        LastUpdate    time.Time
        Metrics       *CampaignMetrics
    }
    
    func NewCampaignStateManager(...) *CampaignStateManager
    func (csm *CampaignStateManager) Start(ctx context.Context) error
    func (csm *CampaignStateManager) Stop() error
    func (csm *CampaignStateManager) monitorCampaigns(ctx context.Context)
    func (csm *CampaignStateManager) checkAutoRestart(ctx context.Context, campaignID string)
    func (csm *CampaignStateManager) updateMetrics(campaignID string)
    func (csm *CampaignStateManager) handleJobCompletion(jobID string)
    ```
  - Heartbeat monitoring: Check campaigns every 30 seconds
  - Auto-restart: Trigger when all jobs complete
  - Broadcast updates: Send via WebSocket hub
  - Context: Maintains campaign state in memory

#### Group F: Bot Integration (Depends on Group E)
- [x] **Task #7**: Update bot for corpus sync and stack reporting
  - Folder: `pkg/bot/`
  - Files: `corpus_sync.go`, `stack_reporter.go`
  - Implements in corpus_sync.go:
    ```go
    type CorpusSyncClient struct {
        client     *Client
        campaignID string
        botID      string
        syncDir    string
        logger     logrus.FieldLogger
    }
    
    func NewCorpusSyncClient(client *Client, campaignID, botID, syncDir string, logger logrus.FieldLogger) *CorpusSyncClient
    func (csc *CorpusSyncClient) Start(ctx context.Context) error
    func (csc *CorpusSyncClient) Stop() error
    func (csc *CorpusSyncClient) syncLoop(ctx context.Context)
    func (csc *CorpusSyncClient) downloadNewFiles(ctx context.Context) error
    func (csc *CorpusSyncClient) reportNewCoverage(ctx context.Context, file string, coverage int64) error
    ```
  - Implements in stack_reporter.go:
    ```go
    type StackReporter struct {
        logger logrus.FieldLogger
    }
    
    func NewStackReporter(logger logrus.FieldLogger) *StackReporter
    func (sr *StackReporter) ParseStackTrace(output string) (*StackTrace, error)
    func (sr *StackReporter) ExtractFrames(trace string) []StackFrame
    func (sr *StackReporter) EnhanceCrashResult(crash *CrashResult, trace *StackTrace)
    ```
  - Integration: Add to Agent's result reporting
  - Sync interval: Every 30 seconds
  - Context: Bot-side corpus management

#### Group G: Web UI Foundation (Can start with Group A)
- [x] **Task #8**: Create web UI structure
  - Folder: `web/static/`
  - Files: `index.html`, `campaigns.html`, `crashes.html`
  - Implements in index.html:
    ```html
    <!DOCTYPE html>
    <html>
    <head>
        <title>PandaFuzz Dashboard</title>
        <link rel="stylesheet" href="/css/dashboard.css">
    </head>
    <body>
        <nav id="main-nav">
            <a href="/">Dashboard</a>
            <a href="/campaigns.html">Campaigns</a>
            <a href="/crashes.html">Crashes</a>
        </nav>
        <main>
            <section id="summary-cards">
                <div class="card">
                    <h3>Active Campaigns</h3>
                    <p class="metric" id="active-campaigns">0</p>
                </div>
                <div class="card">
                    <h3>Total Coverage</h3>
                    <p class="metric" id="total-coverage">0</p>
                </div>
                <div class="card">
                    <h3>Unique Crashes</h3>
                    <p class="metric" id="unique-crashes">0</p>
                </div>
                <div class="card">
                    <h3>Active Bots</h3>
                    <p class="metric" id="active-bots">0</p>
                </div>
            </section>
            <section id="recent-activity">
                <h2>Recent Activity</h2>
                <div id="activity-feed"></div>
            </section>
        </main>
        <script src="/js/api.js"></script>
        <script src="/js/websocket.js"></script>
        <script src="/js/dashboard.js"></script>
    </body>
    </html>
    ```
  - Implements in campaigns.html: Campaign list, create form, detail view
  - Implements in crashes.html: Crash groups, stack traces, download inputs
  - Features: Real-time updates, simple forms, responsive tables
  - Context: User interface foundation

- [x] **Task #9**: Implement UI JavaScript
  - Folder: `web/js/`
  - Files: `api.js`, `campaigns.js`, `websocket.js`
  - Implements in api.js:
    ```javascript
    class PandaFuzzAPI {
        constructor(baseURL = '/api/v2') {
            this.baseURL = baseURL;
        }
        
        async createCampaign(campaign) { }
        async listCampaigns(filters = {}) { }
        async getCampaign(id) { }
        async getCampaignStats(id) { }
        async restartCampaign(id) { }
        async getCrashGroups(campaignId) { }
        async getCorpusEvolution(campaignId) { }
        
        async uploadBinary(campaignId, file) { }
        async uploadCorpus(campaignId, files) { }
    }
    ```
  - Implements in websocket.js:
    ```javascript
    class PandaFuzzWebSocket {
        constructor(url = 'ws://localhost:8080/ws') {
            this.url = url;
            this.handlers = {};
            this.reconnectInterval = 5000;
        }
        
        connect() { }
        on(event, handler) { }
        reconnect() { }
        handleMessage(event) { }
    }
    ```
  - Implements in campaigns.js: Form handling, table updates, charts
  - Features: Auto-reconnect, error handling, loading states
  - Context: Interactive UI logic

- [x] **Task #10**: Style the dashboard
  - Folder: `web/css/`
  - File: `dashboard.css`
  - Implements:
    ```css
    /* Simple, clean design with good contrast */
    :root {
        --primary: #2563eb;
        --success: #10b981;
        --danger: #ef4444;
        --background: #f9fafb;
        --card-bg: #ffffff;
        --text: #111827;
        --border: #e5e7eb;
    }
    
    /* Card-based layout */
    .card { }
    .metric { }
    
    /* Responsive tables */
    .data-table { }
    
    /* Form styling */
    .form-group { }
    .btn { }
    
    /* Status indicators */
    .status-badge { }
    
    /* Activity feed */
    .activity-item { }
    ```
  - Design: Clean, minimal, high contrast
  - Responsive: Works on mobile and desktop
  - Context: Visual design for dashboard

#### Group H: Master Service Integration (Depends on all groups)
- [x] **Task #11**: Wire everything into master service
  - Folder: `pkg/master/`
  - Files: Update `state.go`, `recovery.go`, `routes.go`
  - Updates in state.go:
    - Add campaignManager *CampaignStateManager
    - Initialize in NewStateManager
    - Start/stop campaign manager
  - Updates in recovery.go:
    - Add campaign recovery logic
    - Restore campaign states on startup
    - Handle campaign job reassignment
  - Updates in routes.go:
    - Add all campaign routes
    - Add v2 API routes
    - Add WebSocket endpoint
    - Serve static files for web UI
  - Integration points:
    - Inject services into handlers
    - Connect WebSocket hub to state updates
    - Add campaign checks to job completion
  - Context: Final integration layer

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