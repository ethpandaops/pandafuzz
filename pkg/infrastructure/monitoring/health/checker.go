package health

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Checker implements health checking functionality
type Checker struct {
	logger  logrus.FieldLogger
	options *CheckerOptions

	// Health checks registry
	checks   map[string]Check
	options_ map[string]*CheckOptions
	mu       sync.RWMutex

	// Check state tracking
	states  map[string]*checkState
	stateMu sync.RWMutex

	// Cache for results
	cache        *resultCache
	cacheEnabled bool

	// Control
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// checkState tracks the state of a health check
type checkState struct {
	consecutiveFailures  int
	consecutiveSuccesses int
	lastCheck            time.Time
	lastResult           *CheckResult
}

// resultCache caches health check results
type resultCache struct {
	results map[string]*cachedResult
	mu      sync.RWMutex
}

type cachedResult struct {
	result    *CheckResult
	expiresAt time.Time
}

// NewChecker creates a new health checker
func NewChecker(logger logrus.FieldLogger, options *CheckerOptions) *Checker {
	if logger == nil {
		logger = logrus.New().WithField("component", "health")
	}
	if options == nil {
		options = DefaultCheckerOptions()
	}

	return &Checker{
		logger:       logger,
		options:      options,
		checks:       make(map[string]Check),
		options_:     make(map[string]*CheckOptions),
		states:       make(map[string]*checkState),
		cache:        &resultCache{results: make(map[string]*cachedResult)},
		cacheEnabled: options.CacheDuration > 0,
		stopCh:       make(chan struct{}),
	}
}

// Register registers a health check
func (c *Checker) Register(check Check, options *CheckOptions) error {
	if check == nil {
		return fmt.Errorf("check cannot be nil")
	}

	name := check.Name()
	if name == "" {
		return fmt.Errorf("check name cannot be empty")
	}

	if options == nil {
		options = DefaultCheckOptions()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.checks[name]; exists {
		return fmt.Errorf("check %s already registered", name)
	}

	c.checks[name] = check
	c.options_[name] = options

	// Initialize state
	c.stateMu.Lock()
	c.states[name] = &checkState{}
	c.stateMu.Unlock()

	c.logger.WithField("check", name).Info("health check registered")
	return nil
}

// RegisterFunc registers a function-based health check
func (c *Checker) RegisterFunc(name string, fn CheckFunc, options *CheckOptions) error {
	return c.Register(NewFuncCheck(name, fn), options)
}

// Unregister removes a health check
func (c *Checker) Unregister(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.checks[name]; !exists {
		return fmt.Errorf("check %s not found", name)
	}

	delete(c.checks, name)
	delete(c.options_, name)

	c.stateMu.Lock()
	delete(c.states, name)
	c.stateMu.Unlock()

	// Clear cache
	if c.cacheEnabled {
		c.cache.mu.Lock()
		delete(c.cache.results, name)
		c.cache.mu.Unlock()
	}

	c.logger.WithField("check", name).Info("health check unregistered")
	return nil
}

// CheckHealth performs all health checks and returns the overall status
func (c *Checker) CheckHealth(ctx context.Context) *HealthStatus {
	return c.checkHealthWithType(ctx, nil)
}

// CheckLiveness performs liveness checks
func (c *Checker) CheckLiveness(ctx context.Context) *HealthStatus {
	checkType := Liveness
	return c.checkHealthWithType(ctx, &checkType)
}

// CheckReadiness performs readiness checks
func (c *Checker) CheckReadiness(ctx context.Context) *HealthStatus {
	checkType := Readiness
	return c.checkHealthWithType(ctx, &checkType)
}

// checkHealthWithType performs health checks of a specific type
func (c *Checker) checkHealthWithType(ctx context.Context, checkType *CheckType) *HealthStatus {
	start := time.Now()

	c.mu.RLock()
	checks := make(map[string]Check)
	options := make(map[string]*CheckOptions)
	for name, check := range c.checks {
		opt := c.options_[name]
		if checkType == nil || opt.Type == *checkType {
			checks[name] = check
			options[name] = opt
		}
	}
	c.mu.RUnlock()

	// Run checks concurrently with limit
	results := c.runChecks(ctx, checks, options)

	// Determine overall status
	overallStatus := StatusHealthy
	for _, result := range results {
		if result.Status == StatusUnhealthy {
			// Check if this is a critical check
			if opt, ok := options[result.Name]; ok && opt.Critical {
				overallStatus = StatusUnhealthy
				break
			} else if overallStatus == StatusHealthy {
				overallStatus = StatusDegraded
			}
		}
	}

	return &HealthStatus{
		Status:    overallStatus,
		Checks:    results,
		Timestamp: start,
		Duration:  time.Since(start),
	}
}

// runChecks runs health checks concurrently
func (c *Checker) runChecks(ctx context.Context, checks map[string]Check, options map[string]*CheckOptions) []CheckResult {
	results := make([]CheckResult, 0, len(checks))
	resultsCh := make(chan CheckResult, len(checks))

	// Semaphore for limiting concurrent checks
	sem := make(chan struct{}, c.options.MaxConcurrentChecks)

	var wg sync.WaitGroup
	for name, check := range checks {
		wg.Add(1)
		go func(name string, check Check, opt *CheckOptions) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				resultsCh <- CheckResult{
					Name:      name,
					Status:    StatusUnhealthy,
					Error:     ctx.Err(),
					Timestamp: time.Now(),
				}
				return
			}

			result := c.runCheck(ctx, name, check, opt)
			resultsCh <- result
		}(name, check, options[name])
	}

	// Wait for all checks to complete
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results
	for result := range resultsCh {
		results = append(results, result)
	}

	return results
}

// runCheck runs a single health check
func (c *Checker) runCheck(ctx context.Context, name string, check Check, options *CheckOptions) CheckResult {
	// Check cache first
	if c.cacheEnabled {
		if cached := c.getCachedResult(name); cached != nil {
			return *cached
		}
	}

	start := time.Now()

	// Create timeout context
	checkCtx, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()

	// Run the check
	err := check.Check(checkCtx)

	// Determine status
	status := StatusHealthy
	message := "Check passed"
	if err != nil {
		status = StatusUnhealthy
		message = err.Error()
	}

	result := CheckResult{
		Name:      name,
		Status:    status,
		Message:   message,
		Error:     err,
		Duration:  time.Since(start),
		Timestamp: start,
	}

	// Update state
	c.updateCheckState(name, &result, options)

	// Cache result
	if c.cacheEnabled {
		c.cacheResult(name, &result)
	}

	return result
}

// updateCheckState updates the state tracking for a check
func (c *Checker) updateCheckState(name string, result *CheckResult, options *CheckOptions) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()

	state, exists := c.states[name]
	if !exists {
		state = &checkState{}
		c.states[name] = state
	}

	state.lastCheck = result.Timestamp
	state.lastResult = result

	if result.Status == StatusHealthy {
		state.consecutiveSuccesses++
		state.consecutiveFailures = 0
	} else {
		state.consecutiveFailures++
		state.consecutiveSuccesses = 0
	}

	// Apply thresholds
	if state.consecutiveFailures >= options.FailureThreshold {
		result.Status = StatusUnhealthy
		c.logger.WithFields(logrus.Fields{
			"check":    name,
			"failures": state.consecutiveFailures,
		}).Warn("health check failure threshold reached")
	} else if state.consecutiveSuccesses < options.SuccessThreshold && state.consecutiveFailures > 0 {
		result.Status = StatusDegraded
	}
}

// getCachedResult retrieves a cached result if valid
func (c *Checker) getCachedResult(name string) *CheckResult {
	c.cache.mu.RLock()
	defer c.cache.mu.RUnlock()

	cached, exists := c.cache.results[name]
	if !exists || time.Now().After(cached.expiresAt) {
		return nil
	}

	return cached.result
}

// cacheResult caches a health check result
func (c *Checker) cacheResult(name string, result *CheckResult) {
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()

	c.cache.results[name] = &cachedResult{
		result:    result,
		expiresAt: time.Now().Add(c.options.CacheDuration),
	}
}

// StartPeriodicChecks starts periodic health checks for registered checks
func (c *Checker) StartPeriodicChecks(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for name, options := range c.options_ {
		if options.Interval > 0 {
			check := c.checks[name]
			c.wg.Add(1)
			go c.runPeriodicCheck(ctx, name, check, options)
		}
	}

	return nil
}

// runPeriodicCheck runs a health check periodically
func (c *Checker) runPeriodicCheck(ctx context.Context, name string, check Check, options *CheckOptions) {
	defer c.wg.Done()

	ticker := time.NewTicker(options.Interval)
	defer ticker.Stop()

	c.logger.WithFields(logrus.Fields{
		"check":    name,
		"interval": options.Interval,
	}).Info("starting periodic health check")

	for {
		select {
		case <-ticker.C:
			_ = c.runCheck(ctx, name, check, options)
		case <-ctx.Done():
			return
		case <-c.stopCh:
			return
		}
	}
}

// Stop stops the health checker
func (c *Checker) Stop() error {
	close(c.stopCh)
	c.wg.Wait()
	return nil
}

// GetCheckState returns the current state of a health check
func (c *Checker) GetCheckState(name string) (*checkState, error) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	state, exists := c.states[name]
	if !exists {
		return nil, fmt.Errorf("check %s not found", name)
	}

	// Return a copy
	stateCopy := *state
	if state.lastResult != nil {
		resultCopy := *state.lastResult
		stateCopy.lastResult = &resultCopy
	}

	return &stateCopy, nil
}

// GetAllCheckStates returns the state of all health checks
func (c *Checker) GetAllCheckStates() map[string]*checkState {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	states := make(map[string]*checkState)
	for name, state := range c.states {
		// Create a copy
		stateCopy := *state
		if state.lastResult != nil {
			resultCopy := *state.lastResult
			stateCopy.lastResult = &resultCopy
		}
		states[name] = &stateCopy
	}

	return states
}

// Common health check implementations

// PingCheck creates a simple ping health check
func PingCheck() Check {
	return NewFuncCheck("ping", func(ctx context.Context) error {
		return nil
	})
}

// DatabaseCheck creates a database health check
func DatabaseCheck(name string, pingFunc func(ctx context.Context) error) Check {
	return NewFuncCheck(name, pingFunc)
}

// HTTPCheck creates an HTTP endpoint health check
func HTTPCheck(name, url string, expectedStatus int, client interface {
	Do(*http.Request) (*http.Response, error)
}) Check {
	return NewFuncCheck(name, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != expectedStatus {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		return nil
	})
}

// DiskSpaceCheck creates a disk space health check
func DiskSpaceCheck(name string, path string, minFreeBytes int64) Check {
	return NewFuncCheck(name, func(ctx context.Context) error {
		// This would require platform-specific implementation
		// For now, return a placeholder
		return fmt.Errorf("disk space check not implemented")
	})
}

// MemoryCheck creates a memory usage health check
func MemoryCheck(name string, maxUsagePercent float64) Check {
	return NewFuncCheck(name, func(ctx context.Context) error {
		// This would require runtime memory stats
		// For now, return a placeholder
		return fmt.Errorf("memory check not implemented")
	})
}
