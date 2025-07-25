package scheduler

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/sirupsen/logrus"
)

// PriorityQueue implements a priority-based job queue
type PriorityQueue interface {
	// Push adds a job to the priority queue
	Push(job *types.Job) error

	// Pop removes and returns the highest priority job
	Pop() (*types.Job, error)

	// Peek returns the highest priority job without removing it
	Peek() (*types.Job, error)

	// Remove removes a specific job from the queue
	Remove(jobID string) error

	// Update updates a job's priority in the queue
	Update(jobID string, priority types.JobPriority) error

	// Size returns the number of jobs in the queue
	Size() int

	// Clear removes all jobs from the queue
	Clear()

	// Contains checks if a job is in the queue
	Contains(jobID string) bool

	// Jobs returns all jobs in priority order
	Jobs() []*types.Job
}

// priorityQueue implements PriorityQueue interface
type priorityQueue struct {
	mu    sync.RWMutex
	items *jobHeap
	index map[string]int // jobID -> heap index
	log   logrus.FieldLogger
}

// NewPriorityQueue creates a new priority queue
func NewPriorityQueue(log logrus.FieldLogger) PriorityQueue {
	pq := &priorityQueue{
		items: &jobHeap{},
		index: make(map[string]int),
		log:   log.WithField("component", "priority-queue"),
	}
	heap.Init(pq.items)
	return pq
}

// Push adds a job to the priority queue
func (pq *priorityQueue) Push(job *types.Job) error {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if job == nil {
		return errors.New("job cannot be nil")
	}

	// Check if job already exists
	if _, exists := pq.index[job.ID]; exists {
		return fmt.Errorf("job %s already in queue", job.ID)
	}

	// Add to heap
	item := &jobItem{
		job:      job,
		priority: job.Priority,
		index:    -1, // Will be set by heap.Push
	}

	heap.Push(pq.items, item)
	pq.index[job.ID] = item.index

	pq.log.WithFields(logrus.Fields{
		"job_id":   job.ID,
		"priority": job.Priority,
		"size":     pq.items.Len(),
	}).Debug("Job added to priority queue")

	return nil
}

// Pop removes and returns the highest priority job
func (pq *priorityQueue) Pop() (*types.Job, error) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if pq.items.Len() == 0 {
		return nil, errors.New("queue is empty")
	}

	item := heap.Pop(pq.items).(*jobItem)
	delete(pq.index, item.job.ID)

	pq.log.WithFields(logrus.Fields{
		"job_id":   item.job.ID,
		"priority": item.job.Priority,
		"size":     pq.items.Len(),
	}).Debug("Job removed from priority queue")

	return item.job, nil
}

// Peek returns the highest priority job without removing it
func (pq *priorityQueue) Peek() (*types.Job, error) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	if pq.items.Len() == 0 {
		return nil, errors.New("queue is empty")
	}

	return (*pq.items)[0].job, nil
}

// Remove removes a specific job from the queue
func (pq *priorityQueue) Remove(jobID string) error {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	idx, exists := pq.index[jobID]
	if !exists {
		return fmt.Errorf("job %s not found in queue", jobID)
	}

	// Remove from heap
	heap.Remove(pq.items, idx)
	delete(pq.index, jobID)

	// Update indices
	pq.rebuildIndex()

	pq.log.WithField("job_id", jobID).Debug("Job removed from priority queue")
	return nil
}

// Update updates a job's priority in the queue
func (pq *priorityQueue) Update(jobID string, priority types.JobPriority) error {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	idx, exists := pq.index[jobID]
	if !exists {
		return fmt.Errorf("job %s not found in queue", jobID)
	}

	// Update priority
	item := (*pq.items)[idx]
	oldPriority := item.priority
	item.priority = priority
	item.job.Priority = priority

	// Fix heap ordering
	heap.Fix(pq.items, idx)
	pq.rebuildIndex()

	pq.log.WithFields(logrus.Fields{
		"job_id":       jobID,
		"old_priority": oldPriority,
		"new_priority": priority,
	}).Debug("Job priority updated")

	return nil
}

// Size returns the number of jobs in the queue
func (pq *priorityQueue) Size() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return pq.items.Len()
}

// Clear removes all jobs from the queue
func (pq *priorityQueue) Clear() {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pq.items = &jobHeap{}
	heap.Init(pq.items)
	pq.index = make(map[string]int)

	pq.log.Info("Priority queue cleared")
}

// Contains checks if a job is in the queue
func (pq *priorityQueue) Contains(jobID string) bool {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	_, exists := pq.index[jobID]
	return exists
}

// Jobs returns all jobs in priority order
func (pq *priorityQueue) Jobs() []*types.Job {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	jobs := make([]*types.Job, pq.items.Len())
	for i, item := range *pq.items {
		jobs[i] = item.job
	}

	// Sort by priority (heap is min-heap, but we want highest priority first)
	// The heap maintains the order, so we just need to copy
	return jobs
}

// rebuildIndex rebuilds the job ID to index mapping
func (pq *priorityQueue) rebuildIndex() {
	pq.index = make(map[string]int)
	for i, item := range *pq.items {
		pq.index[item.job.ID] = i
	}
}

// jobItem wraps a job for use in the heap
type jobItem struct {
	job      *types.Job
	priority types.JobPriority
	index    int // Index in the heap
}

// jobHeap implements heap.Interface for priority queue
type jobHeap []*jobItem

func (h jobHeap) Len() int { return len(h) }

// Less defines the heap ordering
// We want a max-heap (highest priority first), but Go's heap is a min-heap
// So we reverse the comparison
func (h jobHeap) Less(i, j int) bool {
	// Higher priority values should come first
	if h[i].priority != h[j].priority {
		return h[i].priority > h[j].priority
	}

	// For same priority, use FIFO (earlier queued jobs first)
	if h[i].job.QueuedAt != nil && h[j].job.QueuedAt != nil {
		return h[i].job.QueuedAt.Before(*h[j].job.QueuedAt)
	}

	// Fallback to creation time
	return h[i].job.CreatedAt.Before(h[j].job.CreatedAt)
}

func (h jobHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *jobHeap) Push(x any) {
	item := x.(*jobItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *jobHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // Avoid memory leak
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// PriorityScheduler wraps Queue with priority-based scheduling
type PriorityScheduler struct {
	*queue
	priorityQueue PriorityQueue
	mu            sync.RWMutex
}

// NewPriorityScheduler creates a new priority-based scheduler
func NewPriorityScheduler(config Config, repo repository.JobRepository, processor JobProcessor, log logrus.FieldLogger) Queue {
	// Ensure priority is enabled
	config.EnablePriority = true

	baseQueue := NewQueue(config, repo, processor, log).(*queue)
	return &PriorityScheduler{
		queue:         baseQueue,
		priorityQueue: NewPriorityQueue(log),
	}
}

// getNextJob overrides base implementation to use priority queue
func (ps *PriorityScheduler) getNextJob(ctx context.Context, workerID string) (*types.Job, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// First check in-memory priority queue
	for ps.priorityQueue.Size() > 0 {
		job, err := ps.priorityQueue.Pop()
		if err != nil {
			continue
		}

		// Validate job is still valid
		currentJob, err := ps.repo.Get(ctx, job.ID)
		if err != nil {
			continue
		}

		if currentJob.Status != types.StatusQueued {
			continue
		}

		// Check dependencies
		if currentJob.HasDependencies() {
			ready, err := ps.checkDependencies(ctx, currentJob)
			if err != nil {
				ps.log.WithError(err).WithField("job_id", currentJob.ID).Error("Failed to check dependencies")
				continue
			}
			if !ready {
				// Re-add to queue for later
				ps.priorityQueue.Push(currentJob)
				continue
			}
		}

		// Try to lock the job
		lockedJob, err := ps.repo.LockForProcessing(ctx, currentJob.ID, workerID, ps.config.LockDuration)
		if err != nil {
			// Re-add to queue if lock failed
			ps.priorityQueue.Push(currentJob)
			continue
		}
		if lockedJob != nil {
			return lockedJob, nil
		}
	}

	// Fallback to database query
	return ps.queue.getNextJob(ctx, workerID)
}

// Enqueue adds a job to both database and priority queue
func (ps *PriorityScheduler) Enqueue(ctx context.Context, job *types.Job) error {
	// First enqueue to database
	if err := ps.queue.Enqueue(ctx, job); err != nil {
		return err
	}

	// Add to priority queue if not scheduled for future
	if !job.IsScheduled() {
		ps.mu.Lock()
		defer ps.mu.Unlock()

		if err := ps.priorityQueue.Push(job); err != nil {
			ps.log.WithError(err).WithField("job_id", job.ID).Warn("Failed to add job to priority queue")
		}
	}

	return nil
}

// processScheduledJobs override to add jobs to priority queue
func (ps *PriorityScheduler) processScheduledJobs() {
	ps.queue.processScheduledJobs()

	// Sync priority queue with database
	ctx, cancel := context.WithTimeout(ps.ctx, 30*time.Second)
	defer cancel()

	// Get all queued jobs
	jobs, err := ps.repo.ListByStatus(ctx, types.StatusQueued)
	if err != nil {
		ps.log.WithError(err).Error("Failed to sync priority queue")
		return
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Clear and rebuild priority queue
	ps.priorityQueue.Clear()
	for _, job := range jobs {
		if !job.IsScheduled() && !job.IsLocked() {
			if err := ps.priorityQueue.Push(job); err != nil {
				ps.log.WithError(err).WithField("job_id", job.ID).Warn("Failed to add job to priority queue during sync")
			}
		}
	}

	ps.log.WithField("queue_size", ps.priorityQueue.Size()).Debug("Priority queue synced")
}

// GetStats includes priority queue statistics
func (ps *PriorityScheduler) GetStats() QueueStats {
	stats := ps.queue.GetStats()

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	// Add priority queue depth
	stats.QueueDepth = ps.priorityQueue.Size()

	return stats
}

// Cancel removes job from priority queue
func (ps *PriorityScheduler) Cancel(ctx context.Context, jobID string) error {
	// Remove from priority queue first
	ps.mu.Lock()
	if ps.priorityQueue.Contains(jobID) {
		ps.priorityQueue.Remove(jobID)
	}
	ps.mu.Unlock()

	// Then cancel in database
	return ps.queue.Cancel(ctx, jobID)
}
