package scheduler

import (
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
)

// LoadBalancer defines the interface for load balancing strategies
type LoadBalancer interface {
	// SelectBot selects a bot for a work unit based on the strategy
	SelectBot(availableBots []*types.Agent, workUnit *WorkUnit) *types.Agent
	// UpdateLoad updates the load information after work assignment
	UpdateLoad(botID string, workUnit WorkUnit)
	// ReleaseLoad releases load information after work completion
	ReleaseLoad(botID string, workUnit WorkUnit)
	// GetBotLoad returns the current load for a bot
	GetBotLoad(botID string) int
}

// LoadBalancerType represents the type of load balancer
type LoadBalancerType string

const (
	LoadBalancerRoundRobin      LoadBalancerType = "round_robin"
	LoadBalancerLeastLoaded     LoadBalancerType = "least_loaded"
	LoadBalancerCapabilityBased LoadBalancerType = "capability_based"
	LoadBalancerWeighted        LoadBalancerType = "weighted"
	LoadBalancerRandom          LoadBalancerType = "random"
)

// NewLoadBalancer creates a new load balancer based on the specified type
func NewLoadBalancer(balancerType LoadBalancerType) (LoadBalancer, error) {
	switch balancerType {
	case LoadBalancerRoundRobin:
		return NewRoundRobinBalancer(), nil
	case LoadBalancerLeastLoaded:
		return NewLeastLoadedBalancer(), nil
	case LoadBalancerCapabilityBased:
		return NewCapabilityBasedBalancer(), nil
	case LoadBalancerWeighted:
		return NewWeightedBalancer(), nil
	case LoadBalancerRandom:
		return NewRandomBalancer(), nil
	default:
		return nil, errors.New("unknown load balancer type")
	}
}

// RoundRobinBalancer implements round-robin load balancing
type RoundRobinBalancer struct {
	counter uint64
	mu      sync.RWMutex
	loads   map[string]int
}

// NewRoundRobinBalancer creates a new round-robin load balancer
func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{
		loads: make(map[string]int),
	}
}

// SelectBot selects the next bot in round-robin fashion
func (rb *RoundRobinBalancer) SelectBot(availableBots []*types.Agent, workUnit *WorkUnit) *types.Agent {
	if len(availableBots) == 0 {
		return nil
	}

	// Use atomic operation for thread-safe counter increment
	index := atomic.AddUint64(&rb.counter, 1) - 1
	return availableBots[index%uint64(len(availableBots))]
}

// UpdateLoad updates the load for a bot
func (rb *RoundRobinBalancer) UpdateLoad(botID string, workUnit WorkUnit) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.loads[botID]++
}

// ReleaseLoad releases the load for a bot
func (rb *RoundRobinBalancer) ReleaseLoad(botID string, workUnit WorkUnit) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.loads[botID] > 0 {
		rb.loads[botID]--
	}
}

// GetBotLoad returns the current load for a bot
func (rb *RoundRobinBalancer) GetBotLoad(botID string) int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.loads[botID]
}

// LeastLoadedBalancer implements least-loaded load balancing
type LeastLoadedBalancer struct {
	mu    sync.RWMutex
	loads map[string]int
}

// NewLeastLoadedBalancer creates a new least-loaded load balancer
func NewLeastLoadedBalancer() *LeastLoadedBalancer {
	return &LeastLoadedBalancer{
		loads: make(map[string]int),
	}
}

// SelectBot selects the bot with the least load
func (lb *LeastLoadedBalancer) SelectBot(availableBots []*types.Agent, workUnit *WorkUnit) *types.Agent {
	if len(availableBots) == 0 {
		return nil
	}

	lb.mu.RLock()
	defer lb.mu.RUnlock()

	var selectedBot *types.Agent
	minLoad := int(^uint(0) >> 1) // Max int

	for _, bot := range availableBots {
		load := lb.loads[bot.ID]
		if load < minLoad {
			minLoad = load
			selectedBot = bot
		}
	}

	return selectedBot
}

// UpdateLoad updates the load for a bot
func (lb *LeastLoadedBalancer) UpdateLoad(botID string, workUnit WorkUnit) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.loads[botID]++
}

// ReleaseLoad releases the load for a bot
func (lb *LeastLoadedBalancer) ReleaseLoad(botID string, workUnit WorkUnit) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if lb.loads[botID] > 0 {
		lb.loads[botID]--
	}
}

// GetBotLoad returns the current load for a bot
func (lb *LeastLoadedBalancer) GetBotLoad(botID string) int {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.loads[botID]
}

// CapabilityBasedBalancer implements capability-based load balancing
type CapabilityBasedBalancer struct {
	mu              sync.RWMutex
	loads           map[string]int
	capabilityLoads map[types.Capability]map[string]int // capability -> botID -> load
}

// NewCapabilityBasedBalancer creates a new capability-based load balancer
func NewCapabilityBasedBalancer() *CapabilityBasedBalancer {
	return &CapabilityBasedBalancer{
		loads:           make(map[string]int),
		capabilityLoads: make(map[types.Capability]map[string]int),
	}
}

// SelectBot selects a bot based on required capabilities and load
func (cb *CapabilityBasedBalancer) SelectBot(availableBots []*types.Agent, workUnit *WorkUnit) *types.Agent {
	if len(availableBots) == 0 {
		return nil
	}

	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Determine required capability based on work unit
	requiredCapability := cb.getRequiredCapability(workUnit)

	// Filter bots by capability
	capableBots := make([]*types.Agent, 0)
	for _, bot := range availableBots {
		if bot.HasCapability(requiredCapability) {
			capableBots = append(capableBots, bot)
		}
	}

	if len(capableBots) == 0 {
		// Fallback to any available bot
		return cb.selectLeastLoadedBot(availableBots)
	}

	// Select least loaded bot among capable ones
	return cb.selectLeastLoadedBot(capableBots)
}

// getRequiredCapability determines the required capability for a work unit
func (cb *CapabilityBasedBalancer) getRequiredCapability(workUnit *WorkUnit) types.Capability {
	// Map job types to capabilities
	if workUnit.Job.FuzzerType != "" {
		return types.CapabilityFuzzing
	}

	// Default capability
	return types.CapabilityFuzzing
}

// selectLeastLoadedBot selects the bot with the least load from a list
func (cb *CapabilityBasedBalancer) selectLeastLoadedBot(bots []*types.Agent) *types.Agent {
	var selectedBot *types.Agent
	minLoad := int(^uint(0) >> 1) // Max int

	for _, bot := range bots {
		load := cb.loads[bot.ID]
		if load < minLoad {
			minLoad = load
			selectedBot = bot
		}
	}

	return selectedBot
}

// UpdateLoad updates the load for a bot
func (cb *CapabilityBasedBalancer) UpdateLoad(botID string, workUnit WorkUnit) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.loads[botID]++

	// Update capability-specific load
	capability := cb.getRequiredCapability(&workUnit)
	if cb.capabilityLoads[capability] == nil {
		cb.capabilityLoads[capability] = make(map[string]int)
	}
	cb.capabilityLoads[capability][botID]++
}

// ReleaseLoad releases the load for a bot
func (cb *CapabilityBasedBalancer) ReleaseLoad(botID string, workUnit WorkUnit) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.loads[botID] > 0 {
		cb.loads[botID]--
	}

	// Update capability-specific load
	capability := cb.getRequiredCapability(&workUnit)
	if cb.capabilityLoads[capability] != nil && cb.capabilityLoads[capability][botID] > 0 {
		cb.capabilityLoads[capability][botID]--
	}
}

// GetBotLoad returns the current load for a bot
func (cb *CapabilityBasedBalancer) GetBotLoad(botID string) int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.loads[botID]
}

// WeightedBalancer implements weighted load balancing based on bot performance
type WeightedBalancer struct {
	mu      sync.RWMutex
	loads   map[string]int
	weights map[string]float64 // Bot weights based on performance
	metrics map[string]*BotMetrics
}

// BotMetrics tracks performance metrics for a bot
type BotMetrics struct {
	CompletedTasks   int
	FailedTasks      int
	AverageTaskTime  time.Duration
	LastUpdateTime   time.Time
	PerformanceScore float64
}

// NewWeightedBalancer creates a new weighted load balancer
func NewWeightedBalancer() *WeightedBalancer {
	return &WeightedBalancer{
		loads:   make(map[string]int),
		weights: make(map[string]float64),
		metrics: make(map[string]*BotMetrics),
	}
}

// SelectBot selects a bot based on weights and current load
func (wb *WeightedBalancer) SelectBot(availableBots []*types.Agent, workUnit *WorkUnit) *types.Agent {
	if len(availableBots) == 0 {
		return nil
	}

	wb.mu.RLock()
	defer wb.mu.RUnlock()

	var selectedBot *types.Agent
	bestScore := -1.0

	for _, bot := range availableBots {
		// Calculate score based on weight and current load
		weight := wb.weights[bot.ID]
		if weight == 0 {
			weight = 1.0 // Default weight
		}

		load := float64(wb.loads[bot.ID])
		score := weight / (1.0 + load) // Higher weight and lower load = higher score

		if score > bestScore {
			bestScore = score
			selectedBot = bot
		}
	}

	return selectedBot
}

// UpdateLoad updates the load for a bot
func (wb *WeightedBalancer) UpdateLoad(botID string, workUnit WorkUnit) {
	wb.mu.Lock()
	defer wb.mu.Unlock()
	wb.loads[botID]++
}

// ReleaseLoad releases the load for a bot and updates metrics
func (wb *WeightedBalancer) ReleaseLoad(botID string, workUnit WorkUnit) {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	if wb.loads[botID] > 0 {
		wb.loads[botID]--
	}

	// Update bot metrics
	if wb.metrics[botID] == nil {
		wb.metrics[botID] = &BotMetrics{
			LastUpdateTime: time.Now(),
		}
	}

	metrics := wb.metrics[botID]
	if workUnit.Status == WorkUnitStatusCompleted {
		metrics.CompletedTasks++
	} else if workUnit.Status == WorkUnitStatusFailed {
		metrics.FailedTasks++
	}

	// Update performance score and weight
	wb.updateBotWeight(botID)
}

// updateBotWeight updates the weight for a bot based on its performance
func (wb *WeightedBalancer) updateBotWeight(botID string) {
	metrics := wb.metrics[botID]
	if metrics == nil {
		return
	}

	// Calculate performance score (0.0 to 1.0)
	totalTasks := metrics.CompletedTasks + metrics.FailedTasks
	if totalTasks == 0 {
		wb.weights[botID] = 1.0
		return
	}

	successRate := float64(metrics.CompletedTasks) / float64(totalTasks)

	// Adjust weight based on success rate
	// Weight ranges from 0.5 (50% success) to 1.5 (100% success)
	wb.weights[botID] = 0.5 + successRate
}

// GetBotLoad returns the current load for a bot
func (wb *WeightedBalancer) GetBotLoad(botID string) int {
	wb.mu.RLock()
	defer wb.mu.RUnlock()
	return wb.loads[botID]
}

// RandomBalancer implements random load balancing
type RandomBalancer struct {
	mu    sync.RWMutex
	loads map[string]int
	rand  *rand.Rand
}

// NewRandomBalancer creates a new random load balancer
func NewRandomBalancer() *RandomBalancer {
	return &RandomBalancer{
		loads: make(map[string]int),
		rand:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SelectBot randomly selects a bot
func (rb *RandomBalancer) SelectBot(availableBots []*types.Agent, workUnit *WorkUnit) *types.Agent {
	if len(availableBots) == 0 {
		return nil
	}

	rb.mu.Lock()
	index := rb.rand.Intn(len(availableBots))
	rb.mu.Unlock()

	return availableBots[index]
}

// UpdateLoad updates the load for a bot
func (rb *RandomBalancer) UpdateLoad(botID string, workUnit WorkUnit) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.loads[botID]++
}

// ReleaseLoad releases the load for a bot
func (rb *RandomBalancer) ReleaseLoad(botID string, workUnit WorkUnit) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.loads[botID] > 0 {
		rb.loads[botID]--
	}
}

// GetBotLoad returns the current load for a bot
func (rb *RandomBalancer) GetBotLoad(botID string) int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.loads[botID]
}
