package strategies

import (
	"container/heap"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
)

// PriorityQueueStrategy implements priority queue-based selection
type PriorityQueueStrategy struct {
	mu                sync.RWMutex
	priorityQueue     PriorityQueue
	priorityFunc      PriorityFunction
	selectionCount    uint64
	totalPriority     float64
	updateInterval    time.Duration
	lastUpdateTime    time.Time
	dynamicAdjustment bool
}

// PriorityFunction defines how to calculate priority for an entry
type PriorityFunction func(entry *types.CorpusEntry, metadata *PriorityMetadata) float64

// PriorityMetadata contains additional context for priority calculation
type PriorityMetadata struct {
	GlobalCoverage   float64
	TotalExecutions  uint64
	AverageAge       time.Duration
	SelectionHistory map[string]uint64
}

// NewPriorityQueueStrategy creates a new priority queue selection strategy
func NewPriorityQueueStrategy(priorityFunc PriorityFunction, dynamicAdjustment bool) *PriorityQueueStrategy {
	if priorityFunc == nil {
		priorityFunc = DefaultPriorityFunction
	}
	return &PriorityQueueStrategy{
		priorityQueue:     make(PriorityQueue, 0),
		priorityFunc:      priorityFunc,
		updateInterval:    5 * time.Minute,
		dynamicAdjustment: dynamicAdjustment,
	}
}

// Name returns the strategy name
func (s *PriorityQueueStrategy) Name() string {
	if s.dynamicAdjustment {
		return "dynamic-priority-queue"
	}
	return "priority-queue"
}

// Select selects entries based on priority queue
func (s *PriorityQueueStrategy) Select(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	if count <= 0 {
		return nil, errors.New("count must be positive")
	}

	if len(collection) == 0 {
		return nil, errors.New("collection is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we need to rebuild the priority queue
	if s.shouldRebuildQueue() {
		s.rebuildQueue(collection)
	}

	// Ensure queue has entries
	if s.priorityQueue.Len() == 0 {
		s.rebuildQueue(collection)
	}

	// Select top entries from priority queue
	selected := make([]*types.CorpusEntry, 0, count)
	selectedMap := make(map[string]bool)

	// Create a temporary queue to hold removed items
	tempItems := make([]*PriorityItem, 0)

	for len(selected) < count && s.priorityQueue.Len() > 0 {
		item := heap.Pop(&s.priorityQueue).(*PriorityItem)

		// Check if entry still meets criteria
		if s.meetsSelectionCriteria(item.Entry, options) && !selectedMap[item.Entry.ID] {
			selected = append(selected, item.Entry)
			selectedMap[item.Entry.ID] = true
			s.selectionCount++
			s.totalPriority += item.Priority

			// Adjust priority if dynamic adjustment is enabled
			if s.dynamicAdjustment {
				item.Priority *= 0.9 // Reduce priority after selection
			}
		}

		tempItems = append(tempItems, item)
	}

	// Re-add items to queue
	for _, item := range tempItems {
		heap.Push(&s.priorityQueue, item)
	}

	return selected, nil
}

// Priority computes priority score for an entry
func (s *PriorityQueueStrategy) Priority(entry *types.CorpusEntry) float64 {
	metadata := s.buildPriorityMetadata()
	return s.priorityFunc(entry, metadata)
}

// SupportsCriteria indicates if the strategy supports custom selection criteria
func (s *PriorityQueueStrategy) SupportsCriteria() bool {
	return true
}

// Reset resets the internal state
func (s *PriorityQueueStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.priorityQueue = make(PriorityQueue, 0)
	s.selectionCount = 0
	s.totalPriority = 0
	s.lastUpdateTime = time.Time{}
}

// shouldRebuildQueue checks if the priority queue needs rebuilding
func (s *PriorityQueueStrategy) shouldRebuildQueue() bool {
	if s.priorityQueue.Len() == 0 {
		return true
	}
	if time.Since(s.lastUpdateTime) > s.updateInterval {
		return true
	}
	return false
}

// rebuildQueue rebuilds the priority queue with current entries
func (s *PriorityQueueStrategy) rebuildQueue(entries []*types.CorpusEntry) {
	s.priorityQueue = make(PriorityQueue, 0, len(entries))
	metadata := s.buildPriorityMetadata()

	for _, entry := range entries {
		priority := s.priorityFunc(entry, metadata)
		item := &PriorityItem{
			Entry:    entry,
			Priority: priority,
			Index:    0,
		}
		heap.Push(&s.priorityQueue, item)
	}

	s.lastUpdateTime = time.Now()
}

// buildPriorityMetadata builds metadata for priority calculation
func (s *PriorityQueueStrategy) buildPriorityMetadata() *PriorityMetadata {
	// In a real implementation, this would gather global statistics
	return &PriorityMetadata{
		GlobalCoverage:   0.5,
		TotalExecutions:  1000,
		AverageAge:       24 * time.Hour,
		SelectionHistory: make(map[string]uint64),
	}
}

// meetsSelectionCriteria checks if an entry meets selection criteria
func (s *PriorityQueueStrategy) meetsSelectionCriteria(entry *types.CorpusEntry, options SelectionOptions) bool {
	// Check minimum coverage
	if options.MinCoverage > 0 && entry.Coverage.CoverageScore < options.MinCoverage {
		return false
	}

	// Check maximum age
	if options.MaxAge > 0 {
		age := time.Since(entry.CreatedAt).Seconds()
		if age > float64(options.MaxAge) {
			return false
		}
	}

	// Check tags
	if len(options.Tags) > 0 {
		hasTag := false
		for _, requiredTag := range options.Tags {
			for _, entryTag := range entry.Tags {
				if entryTag == requiredTag {
					hasTag = true
					break
				}
			}
		}
		if !hasTag {
			return false
		}
	}

	return true
}

// GetMetrics returns metrics for this strategy
func (s *PriorityQueueStrategy) GetMetrics() *StrategyMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	avgPriority := 0.0
	if s.selectionCount > 0 {
		avgPriority = s.totalPriority / float64(s.selectionCount)
	}

	return &StrategyMetrics{
		Name:            s.Name(),
		SelectionCount:  s.selectionCount,
		AveragePriority: avgPriority,
	}
}

// UpdatePriorities updates priorities for all entries in the queue
func (s *PriorityQueueStrategy) UpdatePriorities() {
	s.mu.Lock()
	defer s.mu.Unlock()

	metadata := s.buildPriorityMetadata()

	// Update all priorities
	for i := 0; i < s.priorityQueue.Len(); i++ {
		item := s.priorityQueue[i]
		item.Priority = s.priorityFunc(item.Entry, metadata)
	}

	// Re-heapify
	heap.Init(&s.priorityQueue)
	s.lastUpdateTime = time.Now()
}

// DefaultPriorityFunction provides a default priority calculation
func DefaultPriorityFunction(entry *types.CorpusEntry, metadata *PriorityMetadata) float64 {
	priority := 0.0

	// Coverage component (40%)
	priority += entry.Coverage.CoverageScore * 0.4

	// Novelty component (30%)
	if entry.Coverage.NewCoverage {
		priority += 0.3
	} else if entry.Coverage.CoverageGained > 0 {
		priority += float64(entry.Coverage.CoverageGained) / 1000.0 * 0.3
	}

	// Freshness component (20%)
	ageHours := time.Since(entry.CreatedAt).Hours()
	freshness := 1.0 / (1.0 + ageHours/24.0)
	priority += freshness * 0.2

	// Execution efficiency (10%)
	if entry.ExecutionCount > 0 {
		efficiency := 1.0 / (1.0 + float64(entry.ExecutionCount)/100.0)
		priority += efficiency * 0.1
	} else {
		priority += 0.1 // Unexecuted entries get full efficiency score
	}

	return priority
}

// PriorityItem represents an item in the priority queue
type PriorityItem struct {
	Entry    *types.CorpusEntry
	Priority float64
	Index    int // Index in the heap
}

// PriorityQueue implements heap.Interface
type PriorityQueue []*PriorityItem

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// Higher priority comes first
	return pq[i].Priority > pq[j].Priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*PriorityItem)
	item.Index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // Avoid memory leak
	item.Index = -1 // For safety
	*pq = old[0 : n-1]
	return item
}

// MultiQueueStrategy uses multiple priority queues for different entry categories
type MultiQueueStrategy struct {
	mu               sync.RWMutex
	queues           map[string]*PriorityQueueStrategy
	queueWeights     map[string]float64
	selectionCount   uint64
	categoryFunc     CategoryFunction
	defaultQueueName string
}

// CategoryFunction determines which queue an entry belongs to
type CategoryFunction func(entry *types.CorpusEntry) string

// NewMultiQueueStrategy creates a strategy with multiple priority queues
func NewMultiQueueStrategy(categoryFunc CategoryFunction) *MultiQueueStrategy {
	if categoryFunc == nil {
		categoryFunc = DefaultCategoryFunction
	}
	return &MultiQueueStrategy{
		queues:           make(map[string]*PriorityQueueStrategy),
		queueWeights:     make(map[string]float64),
		categoryFunc:     categoryFunc,
		defaultQueueName: "default",
	}
}

// Name returns the strategy name
func (s *MultiQueueStrategy) Name() string {
	return "multi-queue"
}

// AddQueue adds a new priority queue with a specific weight
func (s *MultiQueueStrategy) AddQueue(name string, weight float64, priorityFunc PriorityFunction) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.queues[name] = NewPriorityQueueStrategy(priorityFunc, false)
	s.queueWeights[name] = weight
}

// Select selects entries from multiple queues based on weights
func (s *MultiQueueStrategy) Select(ctx context.Context, collection []*types.CorpusEntry, count int, options SelectionOptions) ([]*types.CorpusEntry, error) {
	if count <= 0 {
		return nil, errors.New("count must be positive")
	}

	if len(collection) == 0 {
		return nil, errors.New("collection is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Categorize entries into queues
	s.categorizeEntries(collection)

	// Calculate selections per queue based on weights
	queueSelections := s.calculateQueueSelections(count)

	// Select from each queue
	selected := make([]*types.CorpusEntry, 0, count)
	for queueName, queueCount := range queueSelections {
		if queue, exists := s.queues[queueName]; exists && queueCount > 0 {
			// Get entries for this queue
			queueEntries := s.getQueueEntries(collection, queueName)
			if len(queueEntries) > 0 {
				queueSelected, err := queue.Select(ctx, queueEntries, queueCount, options)
				if err == nil {
					selected = append(selected, queueSelected...)
				}
			}
		}
	}

	s.selectionCount++

	return selected, nil
}

// Priority computes priority score for an entry
func (s *MultiQueueStrategy) Priority(entry *types.CorpusEntry) float64 {
	category := s.categoryFunc(entry)
	if queue, exists := s.queues[category]; exists {
		return queue.Priority(entry)
	}
	if defaultQueue, exists := s.queues[s.defaultQueueName]; exists {
		return defaultQueue.Priority(entry)
	}
	return 0.0
}

// SupportsCriteria indicates if the strategy supports custom selection criteria
func (s *MultiQueueStrategy) SupportsCriteria() bool {
	return true
}

// Reset resets the internal state
func (s *MultiQueueStrategy) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, queue := range s.queues {
		queue.Reset()
	}
	s.selectionCount = 0
}

// categorizeEntries assigns entries to their appropriate queues
func (s *MultiQueueStrategy) categorizeEntries(entries []*types.CorpusEntry) {
	for _, entry := range entries {
		category := s.categoryFunc(entry)
		if _, exists := s.queues[category]; !exists {
			// Create default queue if category doesn't exist
			s.queues[category] = NewPriorityQueueStrategy(DefaultPriorityFunction, false)
			s.queueWeights[category] = 1.0
		}
	}
}

// calculateQueueSelections determines how many to select from each queue
func (s *MultiQueueStrategy) calculateQueueSelections(totalCount int) map[string]int {
	selections := make(map[string]int)
	totalWeight := 0.0

	for _, weight := range s.queueWeights {
		totalWeight += weight
	}

	if totalWeight == 0 {
		return selections
	}

	allocated := 0
	for name, weight := range s.queueWeights {
		count := int(float64(totalCount) * (weight / totalWeight))
		selections[name] = count
		allocated += count
	}

	// Allocate remaining to highest weight queue
	if allocated < totalCount {
		maxQueue := ""
		maxWeight := 0.0
		for name, weight := range s.queueWeights {
			if weight > maxWeight {
				maxWeight = weight
				maxQueue = name
			}
		}
		if maxQueue != "" {
			selections[maxQueue] += totalCount - allocated
		}
	}

	return selections
}

// getQueueEntries gets entries belonging to a specific queue
func (s *MultiQueueStrategy) getQueueEntries(entries []*types.CorpusEntry, queueName string) []*types.CorpusEntry {
	queueEntries := make([]*types.CorpusEntry, 0)
	for _, entry := range entries {
		if s.categoryFunc(entry) == queueName {
			queueEntries = append(queueEntries, entry)
		}
	}
	return queueEntries
}

// DefaultCategoryFunction provides default categorization
func DefaultCategoryFunction(entry *types.CorpusEntry) string {
	if entry.Coverage.NewCoverage {
		return "new-coverage"
	}
	if entry.Coverage.CoverageScore > 0.8 {
		return "high-coverage"
	}
	if entry.ExecutionCount == 0 {
		return "unexecuted"
	}
	if entry.IsInteresting() {
		return "interesting"
	}
	return "default"
}
