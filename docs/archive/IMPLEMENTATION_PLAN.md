# PandaFuzz Implementation Plan

## Project Structure (Revised - Minimalist)
```
pandafuzz/
├── cmd/
│   ├── master/
│   │   └── main.go          # Master server entry point
│   └── bot/
│       └── main.go          # Bot agent entry point
├── pkg/
│   ├── common/
│   │   ├── types.go         # Core data structures
│   │   ├── config.go        # Configuration
│   │   └── database.go      # Database interface
│   ├── master/
│   │   ├── server.go        # HTTP server
│   │   ├── api.go           # API handlers
│   │   ├── state.go         # Persistent state management
│   │   ├── jobs.go          # Job management
│   │   ├── bots.go          # Bot management
│   │   ├── storage.go       # Filesystem operations
│   │   ├── timeouts.go      # Timeout management
│   │   └── recovery.go      # Recovery procedures
│   ├── bot/
│   │   ├── agent.go         # Bot agent logic
│   │   ├── executor.go      # Fuzzer execution
│   │   ├── reporter.go      # Result reporting to master
│   │   └── heartbeat.go     # Heartbeat management
│   ├── fuzzer/
│   │   ├── interface.go     # Fuzzer interface
│   │   ├── aflplusplus.go   # AFL++ integration
│   │   └── libfuzzer.go     # LibFuzzer integration
│   ├── storage/
│   │   ├── sqlite.go        # SQLite implementation
│   │   ├── badger.go        # BadgerDB implementation (optional)
│   │   └── memory.go        # In-memory for testing
│   └── analysis/
│       ├── crash.go         # Crash deduplication
│       └── coverage.go      # Coverage tracking
├── configs/
│   ├── master.yaml          # Master config template
│   └── bot.yaml             # Bot config template
├── scripts/
│   ├── setup.sh             # Setup script
│   └── migrate.sh           # Database migration script
├── tests/
│   ├── integration/         # Integration tests
│   ├── unit/               # Unit tests
│   └── fixtures/           # Test fixtures
├── docs/
│   ├── API.md              # API documentation
│   ├── DEPLOYMENT.md       # Deployment guide
│   └── RECOVERY.md         # Recovery procedures
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── go.sum
└── README.md
```

## Implementation Phases

### Phase 1: Core Infrastructure (Week 1-2)

#### 1.1 Project Setup
```bash
# Initialize project
mkdir -p pandafuzz/{cmd/{master,bot},pkg/{master,bot,common,fuzzer},web/{static,templates},configs,scripts}
cd pandafuzz
go mod init github.com/yourusername/pandafuzz
```

#### 1.2 Core Types (pkg/common/types.go)
```go
package common

import "time"

// Bot management
type Bot struct {
    ID           string    `json:"id" db:"id"`
    Hostname     string    `json:"hostname" db:"hostname"`
    Status       BotStatus `json:"status" db:"status"`
    LastSeen     time.Time `json:"last_seen" db:"last_seen"`
    RegisteredAt time.Time `json:"registered_at" db:"registered_at"`
    CurrentJob   *string   `json:"current_job" db:"current_job"`
    Capabilities []string  `json:"capabilities" db:"capabilities"`
    TimeoutAt    time.Time `json:"timeout_at" db:"timeout_at"`
    FailureCount int       `json:"failure_count" db:"failure_count"`
}

type BotStatus string
const (
    BotStatusIdle      BotStatus = "idle"
    BotStatusBusy      BotStatus = "busy"
    BotStatusTimedOut  BotStatus = "timed_out"
    BotStatusFailed    BotStatus = "failed"
)

// Job management
type Job struct {
    ID          string    `json:"id" db:"id"`
    Name        string    `json:"name" db:"name"`
    Target      string    `json:"target" db:"target"`
    Fuzzer      string    `json:"fuzzer" db:"fuzzer"`
    Status      JobStatus `json:"status" db:"status"`
    AssignedBot *string   `json:"assigned_bot" db:"assigned_bot"`
    CreatedAt   time.Time `json:"created_at" db:"created_at"`
    StartedAt   *time.Time `json:"started_at" db:"started_at"`
    CompletedAt *time.Time `json:"completed_at" db:"completed_at"`
    TimeoutAt   time.Time `json:"timeout_at" db:"timeout_at"`
    WorkDir     string    `json:"work_dir" db:"work_dir"`
}

type JobStatus string
const (
    JobStatusPending   JobStatus = "pending"
    JobStatusAssigned  JobStatus = "assigned"
    JobStatusRunning   JobStatus = "running"
    JobStatusCompleted JobStatus = "completed"
    JobStatusFailed    JobStatus = "failed"
    JobStatusTimedOut  JobStatus = "timed_out"
    JobStatusCancelled JobStatus = "cancelled"
)

// Results and findings
type CrashResult struct {
    ID        string    `json:"id" db:"id"`
    JobID     string    `json:"job_id" db:"job_id"`
    BotID     string    `json:"bot_id" db:"bot_id"`
    Hash      string    `json:"hash" db:"hash"`
    FilePath  string    `json:"file_path" db:"file_path"`
    Type      string    `json:"type" db:"type"`
    Signal    int       `json:"signal" db:"signal"`
    ExitCode  int       `json:"exit_code" db:"exit_code"`
    Timestamp time.Time `json:"timestamp" db:"timestamp"`
    Size      int64     `json:"size" db:"size"`
    IsUnique  bool      `json:"is_unique" db:"is_unique"`
}

type CoverageResult struct {
    ID        string    `json:"id" db:"id"`
    JobID     string    `json:"job_id" db:"job_id"`
    BotID     string    `json:"bot_id" db:"bot_id"`
    Edges     int       `json:"edges" db:"edges"`
    NewEdges  int       `json:"new_edges" db:"new_edges"`
    Timestamp time.Time `json:"timestamp" db:"timestamp"`
    ExecCount int64     `json:"exec_count" db:"exec_count"`
}

type CorpusUpdate struct {
    ID        string    `json:"id" db:"id"`
    JobID     string    `json:"job_id" db:"job_id"`
    BotID     string    `json:"bot_id" db:"bot_id"`
    Files     []string  `json:"files" db:"files"`
    Timestamp time.Time `json:"timestamp" db:"timestamp"`
    TotalSize int64     `json:"total_size" db:"total_size"`
}
```

#### 1.3 Database Interface (pkg/common/database.go)
```go
package common

type Database interface {
    Store(key string, value interface{}) error
    Get(key string, dest interface{}) error
    Delete(key string) error
    Transaction(fn func(tx Transaction) error) error
    Close() error
}

type Transaction interface {
    Store(key string, value interface{}) error
    Get(key string, dest interface{}) error
    Delete(key string) error
    Commit() error
    Rollback() error
}
```

#### 1.4 Retry Logic Implementation (pkg/common/retry.go)
```go
package common

import (
    "fmt"
    "math/rand"
    "strings"
    "time"
)

type RetryPolicy struct {
    MaxRetries      int           `yaml:"max_retries"`
    InitialDelay    time.Duration `yaml:"initial_delay"`
    MaxDelay        time.Duration `yaml:"max_delay"`
    Multiplier      float64       `yaml:"multiplier"`
    Jitter          bool          `yaml:"jitter"`
    RetryableErrors []string      `yaml:"retryable_errors"`
}

type RetryManager struct {
    policy RetryPolicy
    random *rand.Rand
}

func NewRetryManager(policy RetryPolicy) *RetryManager {
    return &RetryManager{
        policy: policy,
        random: rand.New(rand.NewSource(time.Now().UnixNano())),
    }
}

func (rm *RetryManager) Execute(operation func() error) error {
    var lastErr error
    delay := rm.policy.InitialDelay
    
    for attempt := 0; attempt <= rm.policy.MaxRetries; attempt++ {
        err := operation()
        if err == nil {
            return nil
        }
        
        lastErr = err
        
        if !rm.isRetryableError(err) {
            return err
        }
        
        if attempt == rm.policy.MaxRetries {
            break
        }
        
        actualDelay := rm.calculateDelay(delay)
        time.Sleep(actualDelay)
        
        delay = time.Duration(float64(delay) * rm.policy.Multiplier)
        if delay > rm.policy.MaxDelay {
            delay = rm.policy.MaxDelay
        }
    }
    
    return fmt.Errorf("operation failed after %d attempts: %w", rm.policy.MaxRetries+1, lastErr)
}

func (rm *RetryManager) calculateDelay(baseDelay time.Duration) time.Duration {
    if !rm.policy.Jitter {
        return baseDelay
    }
    
    jitter := time.Duration(rm.random.Float64() * float64(baseDelay) * 0.1)
    return baseDelay + jitter
}

func (rm *RetryManager) isRetryableError(err error) bool {
    if len(rm.policy.RetryableErrors) == 0 {
        return isNetworkError(err) || isTimeoutError(err) || isTemporaryError(err)
    }
    
    errStr := err.Error()
    for _, retryable := range rm.policy.RetryableErrors {
        if strings.Contains(errStr, retryable) {
            return true
        }
    }
    return false
}
```

#### 1.5 Persistent State with Retry Logic (pkg/master/state.go)
```go
package master

type PersistentState struct {
    db           Database
    mu           sync.RWMutex
    bots         map[string]*Bot
    jobs         map[string]*Job
    metadata     map[string]interface{}
    retryManager *RetryManager
}

func NewPersistentState(db Database, retryPolicy RetryPolicy) *PersistentState {
    return &PersistentState{
        db:           db,
        bots:         make(map[string]*Bot),
        jobs:         make(map[string]*Job),
        metadata:     make(map[string]interface{}),
        retryManager: NewRetryManager(retryPolicy),
    }
}

func (ps *PersistentState) SaveBotWithRetry(bot *Bot) error {
    return ps.retryManager.Execute(func() error {
        return ps.db.Transaction(func(tx Transaction) error {
            ps.mu.Lock()
            defer ps.mu.Unlock()
            ps.bots[bot.ID] = bot
            return tx.Store("bot:"+bot.ID, bot)
        })
    })
}

func (ps *PersistentState) AtomicJobAssignmentWithRetry(botID string) (*Job, error) {
    var assignedJob *Job
    err := ps.retryManager.Execute(func() error {
        return ps.db.Transaction(func(tx Transaction) error {
            // Read operations first (outside mutex for deadlock prevention)
            job, err := ps.findAvailableJobTx(tx)
            if err != nil {
                return err
            }
            if job == nil {
                return ErrNoJobsAvailable
            }
            
            bot, err := ps.getBotTx(tx, botID)
            if err != nil {
                return err
            }
            
            // Quick in-memory updates
            ps.mu.Lock()
            job.Status = JobStatusAssigned
            job.AssignedBot = &botID
            job.StartedAt = &now
            bot.Status = BotStatusBusy
            bot.CurrentJob = &job.ID
            ps.bots[botID] = bot
            ps.jobs[job.ID] = job
            ps.mu.Unlock()
            
            // Batch persist
            if err := tx.Store("job:"+job.ID, job); err != nil {
                return err
            }
            if err := tx.Store("bot:"+botID, bot); err != nil {
                return err
            }
            
            assignedJob = job
            return nil
        })
    })
    
    return assignedJob, err
}
```

### Phase 2: Basic Fuzzing Integration (Week 3-4)

#### 2.1 Enhanced Fuzzer Interface
```go
package fuzzer

type Fuzzer interface {
    Name() string
    Type() string // "coverage", "blackbox", "grammar"
    Run(config FuzzConfig) (*Result, error)
    Stop() error
    GetCapabilities() []string
}

type FuzzConfig struct {
    Target     string
    Duration   time.Duration
    WorkDir    string
    Strategy   FuzzStrategy
    Mutators   []Mutator
    Grammar    *Grammar
    Dictionary *string
}
```

#### 2.2 AFL++ Integration
- Enhanced subprocess management with resource limits
- Real-time stats parsing and reporting
- Corpus synchronization with energy tracking
- Crash deduplication using hashing

#### 2.3 LibFuzzer Integration
- Dynamic command-line generation based on strategy
- Coverage extraction and edge counting
- Sanitizer integration (ASAN, MSAN, UBSAN)
- Corpus management and merging

#### 2.4 Basic Analysis Components
```go
// pkg/analysis/crash.go
type CrashAnalyzer struct {
    seen map[string]bool
}

func (c *CrashAnalyzer) Deduplicate(crash CrashInfo) bool
func (c *CrashAnalyzer) Categorize(crash CrashInfo) string
func (c *CrashAnalyzer) AssignSeverity(crash CrashInfo) string
```

### Phase 3: Advanced Fuzzing Features (Week 5-6)

#### 3.1 Blackbox Fuzzing
```go
// pkg/fuzzer/blackbox.go
type BlackboxFuzzer struct {
    binary string
    timeout time.Duration
}

func (b *BlackboxFuzzer) Run(config FuzzConfig) (*Result, error)
func (b *BlackboxFuzzer) MonitorCrashes() error
func (b *BlackboxFuzzer) GenerateInputs() error
```

#### 3.2 Grammar-Based Fuzzing
```go
// pkg/fuzzer/grammar.go
type GrammarFuzzer struct {
    grammar Grammar
    generator *Generator
}

func (g *GrammarFuzzer) LoadGrammar(format, definition string) error
func (g *GrammarFuzzer) Generate() ([]byte, error)
func (g *GrammarFuzzer) Run(config FuzzConfig) (*Result, error)
```

#### 3.3 Custom Mutator Framework
```go
// pkg/mutator/interface.go
type Mutator interface {
    Name() string
    Mutate(input []byte) ([]byte, error)
    Initialize(config map[string]interface{}) error
}

// pkg/mutator/custom.go
type CustomMutator struct {
    script string
    lang   string
}

func LoadMutator(path string) (Mutator, error)
func ExecuteScript(script, input string) (string, error)
```

#### 3.4 Memory Leak Detection
```go
// pkg/analysis/leak.go
type LeakDetector struct {
    tools []string // valgrind, asan, lsan
}

func (l *LeakDetector) RunWithTool(tool, binary string) ([]LeakInfo, error)
func (l *LeakDetector) ParseValgrindOutput(output string) ([]LeakInfo, error)
func (l *LeakDetector) ParseSanitizerOutput(output string) ([]LeakInfo, error)
```

### Phase 4: Strategy & Corpus Management (Week 7-8)

#### 4.1 Fuzzing Strategies
```go
// pkg/strategy/evolutionary.go
type EvolutionaryStrategy struct {
    population []Seed
    generation int
    mutationRate float64
}

func (e *EvolutionaryStrategy) Evolve() []Seed
func (e *EvolutionaryStrategy) Select() []Seed
func (e *EvolutionaryStrategy) Crossover(parent1, parent2 Seed) Seed

// pkg/strategy/dictionary.go
type DictionaryStrategy struct {
    dictionary []string
    weights    map[string]float64
}

func (d *DictionaryStrategy) LoadDictionary(path string) error
func (d *DictionaryStrategy) GetMutations(input []byte) [][]byte
```

#### 4.2 Advanced Corpus Management
```go
// pkg/corpus/minimizer.go
type CorpusMinimizer struct {
    coverage map[string]bool
    seeds    []Seed
}

func (c *CorpusMinimizer) Minimize() []Seed
func (c *CorpusMinimizer) RemoveRedundant() int
func (c *CorpusMinimizer) MergeCoverage() error

// pkg/corpus/energy.go
type EnergyScheduler struct {
    seeds []Seed
    power map[string]float64
}

func (e *EnergyScheduler) AssignEnergy(seed Seed) float64
func (e *EnergyScheduler) SelectNext() *Seed
func (e *EnergyScheduler) UpdateRareness() error
```

#### 4.3 Enhanced Storage System
```go
type Storage struct {
    basePath string
    corpus   *CorpusManager
    crashes  *CrashManager
}

func (s *Storage) SaveCrashWithMetadata(crash CrashInfo) error
func (s *Storage) GetCorpusSeeds(fuzzer string) ([]Seed, error)
func (s *Storage) MinimizeCorpus(fuzzer string) error
func (s *Storage) CrossPollinate(source, target string) error
```

### Phase 5: Monitoring & Analysis (Week 9)

#### 5.1 Enhanced Web Dashboard
- Real-time coverage visualization
- Crash clustering and analysis
- Performance metrics tracking
- Leak detection reporting
- Strategy comparison charts

#### 5.2 Advanced Analytics
```go
// pkg/analysis/coverage.go
type CoverageAnalyzer struct {
    edges map[string]int
    functions map[string]bool
}

func (c *CoverageAnalyzer) AnalyzeCoverage(gcov string) CoverageData
func (c *CoverageAnalyzer) FindNewEdges(current, previous []byte) []int
func (c *CoverageAnalyzer) GenerateReport() Report
```

### Phase 6: Docker & Deployment (Week 10)

#### 4.1 Dockerfile
```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder
RUN apk add --no-cache git gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o pandafuzz-master ./cmd/master
RUN go build -o pandafuzz-bot ./cmd/bot

# Runtime stage
FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/pandafuzz-master .
COPY --from=builder /app/pandafuzz-bot .
COPY --from=builder /app/web ./web
COPY --from=builder /app/configs ./configs

# Install fuzzers
RUN apk add --no-cache afl++ libfuzzer

EXPOSE 8080
CMD ["./pandafuzz-master"]
```

#### 4.2 Docker Compose
```yaml
version: '3.8'
services:
  master:
    build: .
    command: ./pandafuzz-master
    ports:
      - "8080:8080"
    volumes:
      - ./storage:/storage
      - ./configs/master.yaml:/app/config.yaml
    
  bot:
    build: .
    command: ./pandafuzz-bot
    environment:
      - MASTER_URL=http://master:8080
    volumes:
      - ./configs/bot.yaml:/app/config.yaml
    depends_on:
      - master
    deploy:
      replicas: 3
```

## Development Timeline (Revised - 12 Weeks)

### Week 1-2: Core Infrastructure
- [ ] Setup minimalist project structure
- [ ] Implement core data models (Bot, Job, Results)
- [ ] Create master HTTP server with essential API
- [ ] Implement SQLite database integration
- [ ] Build persistent state management with recovery
- [ ] Add atomic job assignment mechanism
- [ ] Implement timeout management for all operations

### Week 3-4: Bot Communication
- [ ] Create bot agent with master communication
- [ ] Implement heartbeat mechanism with failure detection
- [ ] Build result reporting API (crashes, coverage, corpus)
- [ ] Add bot failure handling and recovery
- [ ] Ensure master-only filesystem writes
- [ ] Create job-specific directory isolation

### Week 5-6: Basic Fuzzing
- [ ] Integrate AFL++ with basic options
- [ ] Integrate LibFuzzer with basic options
- [ ] Implement crash collection with SHA256 deduplication
- [ ] Build simple corpus management (file-based)
- [ ] Add basic coverage tracking
- [ ] Implement job timeout and cleanup mechanisms

### Week 7-8: Persistence & Recovery
- [ ] Complete state persistence to disk
- [ ] Implement corpus metadata persistence
- [ ] Build comprehensive recovery procedures
- [ ] Add orphaned job cleanup
- [ ] Implement system health monitoring
- [ ] Add error handling and logging

### Week 9-10: Production Readiness
- [ ] Implement resource management and limits
- [ ] Add comprehensive error handling
- [ ] Build basic monitoring and metrics
- [ ] Optimize Docker packaging
- [ ] Create integration test suite
- [ ] Add input validation and security

### Week 11-12: Testing & Documentation
- [ ] Build comprehensive test suite
- [ ] Add chaos engineering tests
- [ ] Create operational documentation
- [ ] Implement deployment automation
- [ ] Conduct performance testing
- [ ] Create recovery runbooks

## Key Go Libraries
- `gorilla/mux` - HTTP routing
- `spf13/viper` - Configuration
- `sirupsen/logrus` - Logging
- `gorilla/websocket` - Real-time updates
- `golang.org/x/crypto` - Hashing and crypto
- `github.com/shirou/gopsutil` - System monitoring
- `github.com/antlr/antlr4/runtime/Go/antlr` - Grammar parsing
- `go-yaml/yaml` - YAML processing
- Standard library for core functionality

## Testing Strategy
1. Unit tests for core components
2. Integration tests using Docker Compose
3. Fuzzer simulation for testing
4. Load testing with multiple bots

## Documentation
- API documentation using Swagger
- Setup guide
- Bot deployment instructions
- Fuzzer configuration examples