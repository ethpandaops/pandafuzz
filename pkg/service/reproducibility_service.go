package service

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/errors"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// reproducibilityService implements ReproducibilityService interface
type reproducibilityService struct {
	storage common.Storage
	config  *common.MasterConfig
	logger  logrus.FieldLogger
	scorer  *reproducibilityScorer // Advanced scoring logic

	// Queue management
	requestQueue  chan *reproductionRequest
	priorityQueue *priorityQueue
	queueMu       sync.Mutex
	workers       int
	workerWg      sync.WaitGroup

	// Status tracking
	statusMu      sync.RWMutex
	requestStatus map[string]*common.ReproductionRequest

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// reproductionRequest is an internal struct for priority queue
type reproductionRequest struct {
	*common.ReproductionRequest
	retryChannel chan error // For retry handling
	index        int        // Index in the heap
}

// priorityQueue implements heap.Interface for priority-based ordering
type priorityQueue []*reproductionRequest

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// Higher priority = earlier in queue
	// If priorities are equal, earlier requests come first
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority > pq[j].Priority
	}
	return pq[i].RequestedAt.Before(pq[j].RequestedAt)
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x any) {
	n := len(*pq)
	item := x.(*reproductionRequest)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// Compile-time interface compliance check
var _ common.ReproducibilityService = (*reproducibilityService)(nil)

// NewReproducibilityService creates a new reproducibility service
func NewReproducibilityService(
	storage common.Storage,
	config *common.MasterConfig,
	logger *logrus.Logger,
) common.ReproducibilityService {
	numWorkers := 5 // Default number of workers
	if config != nil && config.Timeouts.JobExecution > 0 {
		// Adjust workers based on config if needed
		numWorkers = 10
	}

	pq := make(priorityQueue, 0)

	service := &reproducibilityService{
		storage:       storage,
		config:        config,
		logger:        logger.WithField("service", "reproducibility"),
		requestQueue:  make(chan *reproductionRequest, numWorkers), // Channel for active work
		priorityQueue: &pq,
		workers:       numWorkers,
		requestStatus: make(map[string]*common.ReproductionRequest),
		done:          make(chan struct{}),
	}

	// Initialize the scorer
	service.scorer = NewReproducibilityScorer(storage, service.logger)

	return service
}

// Start initializes the reproducibility service
func (s *reproducibilityService) Start(ctx context.Context) error {
	s.logger.Info("Starting reproducibility service")

	s.ctx, s.cancel = context.WithCancel(ctx)

	// Initialize the heap
	heap.Init(s.priorityQueue)

	// Start worker pool
	for i := 0; i < s.workers; i++ {
		s.workerWg.Add(1)
		go s.worker(i)
	}

	// Start queue manager (moves items from priority queue to work channel)
	go s.queueManager()

	// Start queue monitor
	go s.monitorQueue()

	s.logger.WithField("workers", s.workers).Info("Reproducibility service started")
	return nil
}

// Stop gracefully shuts down the reproducibility service
func (s *reproducibilityService) Stop() error {
	s.logger.Info("Stopping reproducibility service")

	// Signal shutdown
	close(s.done)

	// Cancel context
	if s.cancel != nil {
		s.cancel()
	}

	// Close queue channel
	close(s.requestQueue)

	// Wait for workers to finish
	s.workerWg.Wait()

	s.logger.Info("Reproducibility service stopped")
	return nil
}

// QueueReproduction adds a crash to the reproduction queue
func (s *reproducibilityService) QueueReproduction(ctx context.Context, crashID string, priority int) error {
	s.logger.WithFields(logrus.Fields{
		"crash_id": crashID,
		"priority": priority,
	}).Debug("Queueing reproduction request")

	// Validate input
	if crashID == "" {
		return errors.NewValidationError("queue_reproduction", "Crash ID is required")
	}

	// Check if crash exists
	// TODO: GetCrashResult method needs to be added to Storage interface
	// For now, we'll create a minimal request
	crash, err := s.storage.GetCrash(ctx, crashID)
	if err != nil {
		return fmt.Errorf("failed to get crash: %w", err)
	}
	if crash == nil {
		return errors.NewNotFoundError("crash", crashID)
	}

	// Create reproduction request
	request := &common.ReproductionRequest{
		ID:           uuid.New().String(),
		CrashID:      crashID,
		CampaignID:   crash.CampaignID,
		JobID:        crash.JobID,
		Status:       common.ReproducibilityStatusTesting,
		Priority:     priority,
		AttemptCount: 0,
		MaxAttempts:  3, // Default max attempts
		RequestedAt:  time.Now(),
		TimeoutAt:    time.Now().Add(30 * time.Minute), // 30 minute timeout
	}

	// TODO: CreateReproductionRequest method needs to be added to Storage interface
	// For now, we'll track in memory only
	// if err := s.storage.CreateReproductionRequest(ctx, request); err != nil {
	//     return fmt.Errorf("failed to store reproduction request: %w", err)
	// }

	// Add to in-memory tracking
	s.statusMu.Lock()
	s.requestStatus[crashID] = request
	s.statusMu.Unlock()

	// Add to priority queue
	s.queueMu.Lock()
	heap.Push(s.priorityQueue, &reproductionRequest{
		ReproductionRequest: request,
		retryChannel:        make(chan error, 1),
	})
	s.queueMu.Unlock()

	s.logger.WithField("crash_id", crashID).Info("Reproduction request queued successfully")

	return nil
}

// GetReproductionStatus gets the current status of a reproduction task
func (s *reproducibilityService) GetReproductionStatus(ctx context.Context, crashID string) (*common.ReproductionRequest, error) {
	// Check in-memory cache first
	s.statusMu.RLock()
	if request, ok := s.requestStatus[crashID]; ok {
		s.statusMu.RUnlock()
		return request, nil
	}
	s.statusMu.RUnlock()

	// TODO: GetReproductionRequest method needs to be added to Storage interface
	// For now, return nil if not found in memory
	return nil, errors.NewNotFoundError("reproduction_request", crashID)
}

// RecordReproductionResult records the result of a reproduction attempt
func (s *reproducibilityService) RecordReproductionResult(ctx context.Context, result *common.ReproductionResult) error {
	if result == nil {
		return errors.NewValidationError("record_result", "Result is required")
	}

	s.logger.WithFields(logrus.Fields{
		"crash_id":         result.CrashID,
		"reproduced":       result.Reproduced,
		"attempt":          result.AttemptNumber,
		"matches_original": result.MatchesOriginal,
	}).Debug("Recording reproduction result")

	// Store result
	if err := s.storage.CreateReproductionResult(ctx, result); err != nil {
		return fmt.Errorf("failed to store reproduction result: %w", err)
	}

	// Update request status
	s.statusMu.Lock()
	if request, ok := s.requestStatus[result.CrashID]; ok {
		request.AttemptCount++

		// Use advanced scoring
		score, err := s.scorer.CalculateScore(ctx, result.CrashID)
		if err == nil {
			request.Status = score.Status

			// Log score details
			s.logger.WithFields(logrus.Fields{
				"crash_id":   result.CrashID,
				"base_score": score.BaseScore,
				"confidence": score.Confidence,
				"status":     score.Status,
				"platforms":  len(score.PlatformScores),
			}).Debug("Updated reproducibility score")
		}

		// Mark completed if final attempt or confirmed
		if request.AttemptCount >= request.MaxAttempts || request.Status == common.ReproducibilityStatusConfirmed {
			now := time.Now()
			request.CompletedAt = &now
		}

		// TODO: UpdateReproductionRequest method needs to be added to Storage interface
		// For now, the update is only in memory
		// if err := s.storage.UpdateReproductionRequest(ctx, request.ID, request); err != nil {
		//     s.logger.WithError(err).Error("Failed to update reproduction request")
		// }
	}
	s.statusMu.Unlock()

	return nil
}

// GetReproductionResults gets all reproduction results for a crash
func (s *reproducibilityService) GetReproductionResults(ctx context.Context, crashID string) ([]*common.ReproductionResult, error) {
	return s.storage.GetReproductionResults(ctx, crashID)
}

// CalculateReproducibilityScore calculates the reproducibility score for a crash
func (s *reproducibilityService) CalculateReproducibilityScore(ctx context.Context, crashID string) (float64, error) {
	score, err := s.scorer.CalculateScore(ctx, crashID)
	if err != nil {
		return 0, err
	}
	return score.BaseScore, nil
}

// GetDetailedScore returns the full reproducibility score with all components
func (s *reproducibilityService) GetDetailedScore(ctx context.Context, crashID string) (interface{}, error) {
	return s.scorer.CalculateScore(ctx, crashID)
}

// GetPlatformAnalysis returns platform-specific reproduction analysis
func (s *reproducibilityService) GetPlatformAnalysis(ctx context.Context, crashID string) (map[string]interface{}, error) {
	score, err := s.scorer.CalculateScore(ctx, crashID)
	if err != nil {
		return nil, err
	}

	// Convert PlatformScore map to interface{} map
	result := make(map[string]interface{})
	for platform, ps := range score.PlatformScores {
		result[platform] = ps
	}
	return result, nil
}

// GetTrendAnalysis returns reproduction trend analysis over time
func (s *reproducibilityService) GetTrendAnalysis(ctx context.Context, crashID string) (map[string]interface{}, error) {
	return s.scorer.GetTrendAnalysis(ctx, crashID)
}

// VerifyFix triggers verification of a fix for a crash
func (s *reproducibilityService) VerifyFix(ctx context.Context, crashID, fixCommit string) error {
	// Create high-priority reproduction request for fix verification
	verification, err := s.scorer.VerifyFix(ctx, crashID, fixCommit)
	if err != nil {
		return err
	}

	// Queue special reproduction job with high priority
	err = s.QueueReproduction(ctx, crashID, 100) // Max priority
	if err != nil {
		return fmt.Errorf("failed to queue fix verification: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"crash_id":        crashID,
		"fix_commit":      fixCommit,
		"verification_id": verification.ID,
	}).Info("Fix verification queued")

	return nil
}

// worker processes reproduction requests
func (s *reproducibilityService) worker(id int) {
	defer s.workerWg.Done()

	logger := s.logger.WithField("worker_id", id)
	logger.Info("Reproduction worker started")

	for {
		select {
		case req, ok := <-s.requestQueue:
			if !ok {
				logger.Info("Worker stopping - queue closed")
				return
			}

			// Process the request
			s.processReproductionRequest(req)

		case <-s.done:
			logger.Info("Worker stopping - service shutdown")
			return
		case <-s.ctx.Done():
			logger.Info("Worker stopping - context cancelled")
			return
		}
	}
}

// processReproductionRequest handles a single reproduction request
func (s *reproducibilityService) processReproductionRequest(req *reproductionRequest) {
	logger := s.logger.WithFields(logrus.Fields{
		"crash_id":   req.CrashID,
		"request_id": req.ID,
		"attempt":    req.AttemptCount + 1,
	})

	logger.Info("Processing reproduction request")

	// Update status to indicate processing
	now := time.Now()
	req.StartedAt = &now

	s.statusMu.Lock()
	s.requestStatus[req.CrashID] = req.ReproductionRequest
	s.statusMu.Unlock()

	// TODO: Actual reproduction logic would go here
	// This would involve:
	// 1. Getting the crash input/test case
	// 2. Finding an available bot with required capabilities
	// 3. Sending reproduction job to the bot
	// 4. Waiting for and processing the result

	// For now, we'll simulate the result with more realistic data
	os := "linux"
	arch := "amd64"
	fuzzer := "libfuzzer"

	// Simulate platform variation
	if req.AttemptCount%3 == 0 {
		os = "darwin"
		arch = "arm64"
	} else if req.AttemptCount%3 == 1 {
		os = "windows"
		arch = "amd64"
	}

	// Simulate realistic reproduction behavior
	reproduced := req.AttemptCount%2 == 0 || req.AttemptCount%3 == 0
	stackHash := "abc123"
	if reproduced && req.AttemptCount%4 == 0 {
		stackHash = "def456" // Simulate occasional different stack
	}

	result := &common.ReproductionResult{
		ID:              uuid.New().String(),
		RequestID:       req.ID,
		CrashID:         req.CrashID,
		BotID:           fmt.Sprintf("bot-%s-%s", os, arch),
		AttemptNumber:   req.AttemptCount + 1,
		Status:          common.ReproducibilityStatusTesting,
		Reproduced:      reproduced,
		ExecutionTime:   time.Duration(3+req.AttemptCount%5) * time.Second,
		Signal:          11, // SIGSEGV
		ExitCode:        -1,
		Output:          fmt.Sprintf("Reproduction output on %s/%s", os, arch),
		StackTrace:      fmt.Sprintf("Stack trace from %s/%s\n#0 0x1234 in vulnerable_function\n#1 0x5678 in main", os, arch),
		StackHash:       stackHash,
		MatchesOriginal: reproduced && stackHash == "abc123",
		EnvironmentInfo: map[string]string{
			"os":       os,
			"arch":     arch,
			"fuzzer":   fuzzer,
			"version":  "1.0.0",
			"compiler": "clang-12",
		},
		Timestamp: time.Now(),
	}

	// Record the result
	if err := s.RecordReproductionResult(context.Background(), result); err != nil {
		logger.WithError(err).Error("Failed to record reproduction result")
	}

	logger.WithField("reproduced", result.Reproduced).Info("Reproduction request completed")
}

// QueueBatchReproduction queues multiple crashes for reproduction testing
func (s *reproducibilityService) QueueBatchReproduction(ctx context.Context, crashIDs []string, priority int) error {
	s.logger.WithFields(logrus.Fields{
		"crash_count": len(crashIDs),
		"priority":    priority,
	}).Info("Queueing batch reproduction requests")

	var errors []error
	for _, crashID := range crashIDs {
		if err := s.QueueReproduction(ctx, crashID, priority); err != nil {
			errors = append(errors, fmt.Errorf("crash %s: %w", crashID, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to queue %d crashes", len(errors))
	}

	return nil
}

// GetQueueStatus returns the current queue status
func (s *reproducibilityService) GetQueueStatus() map[string]interface{} {
	workQueueSize := len(s.requestQueue)

	s.queueMu.Lock()
	priorityQueueSize := s.priorityQueue.Len()
	s.queueMu.Unlock()

	s.statusMu.RLock()
	activeRequests := len(s.requestStatus)
	var pendingCount, testingCount, completedCount int
	for _, req := range s.requestStatus {
		switch req.Status {
		case common.ReproducibilityStatusTesting:
			testingCount++
		case common.ReproducibilityStatusConfirmed, common.ReproducibilityStatusFailed, common.ReproducibilityStatusFlaky:
			completedCount++
		default:
			pendingCount++
		}
	}
	s.statusMu.RUnlock()

	return map[string]interface{}{
		"work_queue_size":     workQueueSize,
		"priority_queue_size": priorityQueueSize,
		"active_requests":     activeRequests,
		"pending_count":       pendingCount,
		"testing_count":       testingCount,
		"completed_count":     completedCount,
		"workers":             s.workers,
	}
}

// queueManager manages the priority queue and feeds work to workers
func (s *reproducibilityService) queueManager() {
	ticker := time.NewTicker(100 * time.Millisecond) // Check queue frequently
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Try to move items from priority queue to work channel
			s.queueMu.Lock()
			if s.priorityQueue.Len() > 0 {
				// Try to send to work channel (non-blocking)
				select {
				case s.requestQueue <- heap.Pop(s.priorityQueue).(*reproductionRequest):
					// Successfully moved item to work channel
				default:
					// Work channel is full, push back to priority queue
					item := heap.Pop(s.priorityQueue).(*reproductionRequest)
					heap.Push(s.priorityQueue, item)
				}
			}
			s.queueMu.Unlock()

		case <-s.done:
			return
		case <-s.ctx.Done():
			return
		}
	}
}

// monitorQueue monitors queue health and logs statistics
func (s *reproducibilityService) monitorQueue() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			workQueueSize := len(s.requestQueue)

			s.queueMu.Lock()
			priorityQueueSize := s.priorityQueue.Len()
			s.queueMu.Unlock()

			s.statusMu.RLock()
			activeRequests := len(s.requestStatus)
			s.statusMu.RUnlock()

			s.logger.WithFields(logrus.Fields{
				"work_queue_size":     workQueueSize,
				"priority_queue_size": priorityQueueSize,
				"active_requests":     activeRequests,
				"workers":             s.workers,
			}).Info("Reproducibility queue statistics")

		case <-s.done:
			return
		case <-s.ctx.Done():
			return
		}
	}
}
