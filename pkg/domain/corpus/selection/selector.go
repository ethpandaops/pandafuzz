package selection

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/selection/strategies"
	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
)

// Selector is the main corpus entry selection service
type Selector interface {
	// Start initializes the selector service
	Start(ctx context.Context) error

	// Stop gracefully shuts down the selector
	Stop() error

	// Select returns corpus entries based on the configured strategy
	Select(ctx context.Context, count int) ([]*types.CorpusEntry, error)

	// SelectWithStrategy selects entries using a specific strategy
	SelectWithStrategy(ctx context.Context, strategyName string, count int) ([]*types.CorpusEntry, error)

	// SelectFromCollection selects entries from a specific collection
	SelectFromCollection(ctx context.Context, collectionName string, count int) ([]*types.CorpusEntry, error)

	// RegisterStrategy registers a new selection strategy
	RegisterStrategy(strategy strategies.SelectionStrategy) error

	// SetDefaultStrategy sets the default selection strategy
	SetDefaultStrategy(name string) error

	// GetMetrics returns selection metrics
	GetMetrics() *strategies.SelectionMetrics

	// GetStrategies returns all registered strategies
	GetStrategies() []string
}

// Config configures the selector service
type Config struct {
	// DefaultStrategy is the default selection strategy
	DefaultStrategy string

	// BatchSize for database queries
	BatchSize int

	// CacheSize for entry caching
	CacheSize int

	// CacheTTL for cached entries
	CacheTTL time.Duration

	// MetricsInterval for updating metrics
	MetricsInterval time.Duration

	// SelectionOptions default options for selection
	SelectionOptions strategies.SelectionOptions
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		DefaultStrategy: "weighted",
		BatchSize:       100,
		CacheSize:       1000,
		CacheTTL:        5 * time.Minute,
		MetricsInterval: 30 * time.Second,
		SelectionOptions: strategies.SelectionOptions{
			MinCoverage:       0.0,
			PreferInteresting: true,
			ExcludeWindow:     60,
			WeightFactors:     strategies.DefaultWeightFactors(),
		},
	}
}

// selector implements the Selector interface
type selector struct {
	mu               sync.RWMutex
	config           Config
	logger           logrus.FieldLogger
	entryRepo        repository.CorpusEntryRepository
	collectionRepo   repository.CorpusCollectionRepository
	strategies       map[string]strategies.SelectionStrategy
	defaultStrategy  string
	cache            *entryCache
	metrics          *strategies.SelectionMetrics
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	metricsCollector *metricsCollector
}

// NewSelector creates a new corpus selector
func NewSelector(
	cfg Config,
	logger logrus.FieldLogger,
	entryRepo repository.CorpusEntryRepository,
	collectionRepo repository.CorpusCollectionRepository,
) (Selector, error) {
	if logger == nil {
		return nil, errors.New("logger is required")
	}
	if entryRepo == nil {
		return nil, errors.New("entry repository is required")
	}
	if collectionRepo == nil {
		return nil, errors.New("collection repository is required")
	}

	logger = logger.WithField("component", "corpus-selector")

	s := &selector{
		config:          cfg,
		logger:          logger,
		entryRepo:       entryRepo,
		collectionRepo:  collectionRepo,
		strategies:      make(map[string]strategies.SelectionStrategy),
		defaultStrategy: cfg.DefaultStrategy,
		cache:           newEntryCache(cfg.CacheSize, cfg.CacheTTL),
		metrics: &strategies.SelectionMetrics{
			SelectionDistribution: make(map[string]uint64),
			StrategyPerformance:   make(map[string]*strategies.StrategyMetrics),
		},
	}

	// Register default strategies
	if err := s.registerDefaultStrategies(); err != nil {
		return nil, fmt.Errorf("failed to register default strategies: %w", err)
	}

	// Create metrics collector
	s.metricsCollector = newMetricsCollector(s.metrics, logger)

	return s, nil
}

// Start initializes the selector service
func (s *selector) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ctx != nil {
		return errors.New("selector already started")
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	// Start cache maintenance
	s.wg.Add(1)
	go s.cacheMaintenanceLoop()

	// Start metrics collection
	s.wg.Add(1)
	go s.metricsCollectionLoop()

	s.logger.Info("corpus selector started")
	return nil
}

// Stop gracefully shuts down the selector
func (s *selector) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
		// Wait for goroutines to finish
		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			s.logger.Warn("timeout waiting for selector to stop")
		}
		s.ctx = nil
		s.cancel = nil
	}

	s.logger.Info("corpus selector stopped")
	return nil
}

// Select returns corpus entries based on the configured strategy
func (s *selector) Select(ctx context.Context, count int) ([]*types.CorpusEntry, error) {
	return s.SelectWithStrategy(ctx, s.defaultStrategy, count)
}

// SelectWithStrategy selects entries using a specific strategy
func (s *selector) SelectWithStrategy(ctx context.Context, strategyName string, count int) ([]*types.CorpusEntry, error) {
	if count <= 0 {
		return nil, errors.New("count must be positive")
	}

	// Get strategy without holding lock during selection
	s.mu.RLock()
	strategy, exists := s.strategies[strategyName]
	options := s.config.SelectionOptions // Copy options to avoid race
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("strategy %s not found", strategyName)
	}

	// Try to get entries from cache first
	cachedEntries := s.cache.getAll()
	if len(cachedEntries) >= count {
		// Use cached entries
		selected, err := strategy.Select(ctx, cachedEntries, count, options)
		if err != nil {
			return nil, fmt.Errorf("strategy selection failed: %w", err)
		}
		s.updateMetrics(strategyName, selected)
		return selected, nil
	}

	// Fetch entries from repository
	entries, err := s.fetchEntries(ctx, count*2) // Fetch more for better selection
	if err != nil {
		return nil, fmt.Errorf("failed to fetch entries: %w", err)
	}

	if len(entries) == 0 {
		return nil, errors.New("no entries available")
	}

	// Update cache
	s.cache.addMultiple(entries)

	// Perform selection
	selected, err := strategy.Select(ctx, entries, count, options)
	if err != nil {
		return nil, fmt.Errorf("strategy selection failed: %w", err)
	}

	// Update metrics
	s.updateMetrics(strategyName, selected)

	return selected, nil
}

// SelectFromCollection selects entries from a specific collection
func (s *selector) SelectFromCollection(ctx context.Context, collectionName string, count int) ([]*types.CorpusEntry, error) {
	if count <= 0 {
		return nil, errors.New("count must be positive")
	}

	// Get collection entries
	entries, err := s.collectionRepo.GetCollectionEntries(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to get collection entries: %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("collection %s is empty", collectionName)
	}

	// Use default strategy for selection
	s.mu.RLock()
	strategy := s.strategies[s.defaultStrategy]
	s.mu.RUnlock()

	selected, err := strategy.Select(ctx, entries, count, s.config.SelectionOptions)
	if err != nil {
		return nil, fmt.Errorf("strategy selection failed: %w", err)
	}

	// Update metrics
	s.updateMetrics(s.defaultStrategy, selected)

	return selected, nil
}

// RegisterStrategy registers a new selection strategy
func (s *selector) RegisterStrategy(strategy strategies.SelectionStrategy) error {
	if strategy == nil {
		return errors.New("strategy cannot be nil")
	}

	name := strategy.Name()
	if name == "" {
		return errors.New("strategy name cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.strategies[name]; exists {
		return fmt.Errorf("strategy %s already registered", name)
	}

	s.strategies[name] = strategy
	s.logger.WithField("strategy", name).Info("registered selection strategy")

	return nil
}

// SetDefaultStrategy sets the default selection strategy
func (s *selector) SetDefaultStrategy(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.strategies[name]; !exists {
		return fmt.Errorf("strategy %s not found", name)
	}

	s.defaultStrategy = name
	s.logger.WithField("strategy", name).Info("set default selection strategy")

	return nil
}

// GetMetrics returns selection metrics
func (s *selector) GetMetrics() *strategies.SelectionMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create a copy of metrics
	metrics := &strategies.SelectionMetrics{
		TotalSelections:       s.metrics.TotalSelections,
		UniqueSelections:      s.metrics.UniqueSelections,
		CoverageImprovement:   s.metrics.CoverageImprovement,
		SelectionDistribution: make(map[string]uint64),
		StrategyPerformance:   make(map[string]*strategies.StrategyMetrics),
	}

	// Copy distribution
	for k, v := range s.metrics.SelectionDistribution {
		metrics.SelectionDistribution[k] = v
	}

	// Copy strategy performance
	for k, v := range s.metrics.StrategyPerformance {
		metrics.StrategyPerformance[k] = &strategies.StrategyMetrics{
			Name:            v.Name,
			SelectionCount:  v.SelectionCount,
			AveragePriority: v.AveragePriority,
			CoverageGained:  v.CoverageGained,
			ExecutionTime:   v.ExecutionTime,
		}
	}

	return metrics
}

// GetStrategies returns all registered strategies
func (s *selector) GetStrategies() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.strategies))
	for name := range s.strategies {
		names = append(names, name)
	}
	return names
}

// registerDefaultStrategies registers the built-in strategies
func (s *selector) registerDefaultStrategies() error {
	// Random strategies
	if err := s.RegisterStrategy(strategies.NewUniformRandomStrategy()); err != nil {
		return err
	}

	// Weighted strategies
	weightFactors := s.config.SelectionOptions.WeightFactors
	if err := s.RegisterStrategy(strategies.NewWeightedRandomStrategy(0, weightFactors)); err != nil {
		return err
	}
	if err := s.RegisterStrategy(strategies.NewWeightedSelectionStrategy(weightFactors, false)); err != nil {
		return err
	}
	if err := s.RegisterStrategy(strategies.NewWeightedSelectionStrategy(weightFactors, true)); err != nil {
		return err
	}

	// Coverage strategies
	if err := s.RegisterStrategy(strategies.NewCoverageBasedStrategy(0.5)); err != nil {
		return err
	}
	if err := s.RegisterStrategy(strategies.NewIncrementalCoverageStrategy(0.01)); err != nil {
		return err
	}
	if err := s.RegisterStrategy(strategies.NewRareCoverageStrategy(5)); err != nil {
		return err
	}

	// Reservoir sampling
	if err := s.RegisterStrategy(strategies.NewReservoirSamplingStrategy(100, 0)); err != nil {
		return err
	}

	return nil
}

// fetchEntries fetches entries from the repository
func (s *selector) fetchEntries(ctx context.Context, limit int) ([]*types.CorpusEntry, error) {
	// Try interesting entries first
	interesting, err := s.entryRepo.FindInteresting(ctx)
	if err != nil {
		s.logger.WithError(err).Warn("failed to fetch interesting entries")
	}

	// If we need more, fetch by coverage
	remaining := limit - len(interesting)
	if remaining > 0 {
		coverageEntries, err := s.entryRepo.FindByCoverage(ctx, s.config.SelectionOptions.MinCoverage)
		if err != nil {
			s.logger.WithError(err).Warn("failed to fetch coverage entries")
		} else {
			interesting = append(interesting, coverageEntries...)
		}
	}

	// If still need more, fetch recent entries
	remaining = limit - len(interesting)
	if remaining > 0 {
		recent, _, err := s.entryRepo.List(ctx, 0, remaining)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch recent entries: %w", err)
		}
		interesting = append(interesting, recent...)
	}

	return interesting, nil
}

// updateMetrics updates selection metrics
func (s *selector) updateMetrics(strategyName string, selected []*types.CorpusEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics.TotalSelections++

	// Update distribution
	uniqueMap := make(map[string]bool)
	for _, entry := range selected {
		s.metrics.SelectionDistribution[entry.ID]++
		uniqueMap[entry.ID] = true
	}
	s.metrics.UniqueSelections = uint64(len(s.metrics.SelectionDistribution))

	// Update strategy performance
	if _, exists := s.metrics.StrategyPerformance[strategyName]; !exists {
		s.metrics.StrategyPerformance[strategyName] = &strategies.StrategyMetrics{
			Name: strategyName,
		}
	}
	s.metrics.StrategyPerformance[strategyName].SelectionCount++
}

// cacheMaintenanceLoop performs periodic cache maintenance
func (s *selector) cacheMaintenanceLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cache.cleanup()
		}
	}
}

// metricsCollectionLoop periodically collects and updates metrics
func (s *selector) metricsCollectionLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.config.MetricsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.metricsCollector.collect(s.ctx, s.entryRepo)
		}
	}
}

// entryCache provides a simple cache for corpus entries
type entryCache struct {
	mu        sync.RWMutex
	entries   map[string]*cacheEntry
	maxSize   int
	ttl       time.Duration
	evictList []string
}

type cacheEntry struct {
	entry     *types.CorpusEntry
	timestamp time.Time
}

func newEntryCache(maxSize int, ttl time.Duration) *entryCache {
	return &entryCache{
		entries:   make(map[string]*cacheEntry),
		maxSize:   maxSize,
		ttl:       ttl,
		evictList: make([]string, 0, maxSize),
	}
}

func (c *entryCache) add(entry *types.CorpusEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict
	if len(c.entries) >= c.maxSize {
		// Evict oldest entry
		if len(c.evictList) > 0 {
			oldestID := c.evictList[0]
			delete(c.entries, oldestID)
			c.evictList = c.evictList[1:]
		}
	}

	c.entries[entry.ID] = &cacheEntry{
		entry:     entry,
		timestamp: time.Now(),
	}
	c.evictList = append(c.evictList, entry.ID)
}

func (c *entryCache) addMultiple(entries []*types.CorpusEntry) {
	for _, entry := range entries {
		c.add(entry)
	}
}

func (c *entryCache) get(id string) (*types.CorpusEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if cached, exists := c.entries[id]; exists {
		if time.Since(cached.timestamp) < c.ttl {
			return cached.entry, true
		}
	}
	return nil, false
}

func (c *entryCache) getAll() []*types.CorpusEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries := make([]*types.CorpusEntry, 0, len(c.entries))
	now := time.Now()

	for _, cached := range c.entries {
		if now.Sub(cached.timestamp) < c.ttl {
			entries = append(entries, cached.entry)
		}
	}

	return entries
}

func (c *entryCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	newEvictList := make([]string, 0, len(c.evictList))

	for _, id := range c.evictList {
		if cached, exists := c.entries[id]; exists {
			if now.Sub(cached.timestamp) >= c.ttl {
				delete(c.entries, id)
			} else {
				newEvictList = append(newEvictList, id)
			}
		}
	}

	c.evictList = newEvictList
}

// metricsCollector collects metrics from various sources
type metricsCollector struct {
	metrics *strategies.SelectionMetrics
	logger  logrus.FieldLogger
}

func newMetricsCollector(metrics *strategies.SelectionMetrics, logger logrus.FieldLogger) *metricsCollector {
	return &metricsCollector{
		metrics: metrics,
		logger:  logger,
	}
}

func (m *metricsCollector) collect(ctx context.Context, repo repository.CorpusEntryRepository) {
	// Collect coverage statistics
	stats, err := repo.GetStats(ctx)
	if err != nil {
		m.logger.WithError(err).Warn("failed to collect coverage stats")
		return
	}

	if stats != nil {
		// Update coverage improvement metric
		// This is simplified - in reality would track improvement over time
		m.metrics.CoverageImprovement = stats.AverageCoverage
	}
}
