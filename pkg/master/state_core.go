package master

import (
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// PersistentState manages all system state with persistence and recovery
type PersistentState struct {
	db           common.Database
	mu           sync.RWMutex
	bots         map[string]*common.Bot
	jobs         map[string]*common.Job
	metadata     map[string]any
	retryManager *common.RetryManager
	logger       *logrus.Logger
	config       *common.MasterConfig
	stats        StateStats

	// Cache management
	maxCacheSize    int
	cacheAccessTime map[string]time.Time // Track last access time for cache eviction

	// Campaign management
	campaignManager *CampaignStateManager

	// Storage backend
	Storage common.Storage // Storage backend for files
}

// StateStats tracks statistics about the state manager
type StateStats struct {
	BotsRegistered   int64     `json:"bots_registered"`
	JobsCreated      int64     `json:"jobs_created"`
	CrashesRecorded  int64     `json:"crashes_recorded"`
	CoverageReports  int64     `json:"coverage_reports"`
	CorpusUpdates    int64     `json:"corpus_updates"`
	TransactionCount int64     `json:"transaction_count"`
	LastRecovery     time.Time `json:"last_recovery"`
	LastBackup       time.Time `json:"last_backup"`
	Uptime           time.Time `json:"uptime"`
}

// NewPersistentState creates a new persistent state manager
func NewPersistentState(db common.Database, config *common.MasterConfig, logger *logrus.Logger) *PersistentState {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	retryPolicy := config.Retry.Database
	if retryPolicy.MaxRetries == 0 {
		retryPolicy = common.DatabaseRetryPolicy
	}

	// Default cache size to 1000 items per type (bots, jobs)
	maxCacheSize := 1000
	if config.Limits.MaxCacheSize > 0 {
		maxCacheSize = config.Limits.MaxCacheSize
	}

	return &PersistentState{
		db:              db,
		bots:            make(map[string]*common.Bot),
		jobs:            make(map[string]*common.Job),
		metadata:        make(map[string]any),
		retryManager:    common.NewRetryManager(retryPolicy),
		logger:          logger,
		config:          config,
		maxCacheSize:    maxCacheSize,
		cacheAccessTime: make(map[string]time.Time),
		stats: StateStats{
			Uptime: time.Now(),
		},
	}
}
