package bot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// APIServer provides HTTP endpoints for the master to query bot status
type APIServer struct {
	agent      *Agent
	router     *mux.Router
	httpServer *http.Server
	port       int
	mu         sync.RWMutex
	jobCache   map[string]*JobStatus // Cache of recent job statuses
	logger     *logrus.Logger
}

// JobStatus represents the status of a job on the bot
type JobStatus struct {
	JobID      string    `json:"job_id"`
	Status     string    `json:"status"` // running, completed, failed
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time,omitempty"`
	Success    bool      `json:"success,omitempty"`
	Message    string    `json:"message,omitempty"`
	Output     string    `json:"output,omitempty"`
	CrashCount int       `json:"crash_count,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// HealthResponse represents the bot's health status
type HealthResponse struct {
	Status        string    `json:"status"` // healthy, unhealthy
	BotID         string    `json:"bot_id"`
	CurrentJob    string    `json:"current_job,omitempty"`
	JobStatus     string    `json:"job_status,omitempty"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	Uptime        string    `json:"uptime"`
	Version       string    `json:"version"`
}

// NewAPIServer creates a new API server for the bot
func NewAPIServer(agent *Agent, port int) *APIServer {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	
	s := &APIServer{
		agent:    agent,
		port:     port,
		jobCache: make(map[string]*JobStatus),
		logger:   logger,
	}

	s.router = mux.NewRouter()
	s.setupRoutes()

	return s
}

// setupRoutes configures the API routes
func (s *APIServer) setupRoutes() {
	api := s.router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/health", s.handleHealth).Methods("GET")
	api.HandleFunc("/job/{jobID}/status", s.handleJobStatus).Methods("GET")
	api.HandleFunc("/jobs", s.handleListJobs).Methods("GET")
	api.HandleFunc("/metrics", s.handleMetrics).Methods("GET")
}

// Start starts the API server
func (s *APIServer) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	s.logger.WithField("addr", addr).Info("Starting bot API server")
	
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.WithError(err).Error("API server failed")
		}
	}()
	
	return nil
}

// handleHealth returns the bot's health status
func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := "healthy"
	var currentJob string
	var jobStatus string

	if s.agent.currentJob != nil {
		currentJob = s.agent.currentJob.ID
		jobStatus = "running"
		
		// Check if job is stuck
		if time.Since(s.agent.jobStartTime) > time.Hour*24 {
			status = "unhealthy"
			jobStatus = "stuck"
		}
	}

	response := HealthResponse{
		Status:        status,
		BotID:         s.agent.ID,
		CurrentJob:    currentJob,
		JobStatus:     jobStatus,
		LastHeartbeat: s.agent.lastHeartbeat,
		Uptime:        time.Since(s.agent.startTime).String(),
		Version:       s.agent.version,
	}

	s.writeJSONResponse(w, response)
}

// handleJobStatus returns the status of a specific job
func (s *APIServer) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["jobID"]
	
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check current job
	if s.agent.currentJob != nil && s.agent.currentJob.ID == jobID {
		status := &JobStatus{
			JobID:     jobID,
			Status:    "running",
			StartTime: s.agent.jobStartTime,
			UpdatedAt: time.Now(),
		}
		s.writeJSONResponse(w, status)
		return
	}

	// Check cache
	if status, ok := s.jobCache[jobID]; ok {
		s.writeJSONResponse(w, status)
		return
	}

	s.writeErrorResponse(w, http.StatusNotFound, "job not found")
}

// handleListJobs returns all jobs known to the bot
func (s *APIServer) handleListJobs(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]*JobStatus, 0, len(s.jobCache)+1)

	// Add current job if any
	if s.agent.currentJob != nil {
		jobs = append(jobs, &JobStatus{
			JobID:     s.agent.currentJob.ID,
			Status:    "running",
			StartTime: s.agent.jobStartTime,
			UpdatedAt: time.Now(),
		})
	}

	// Add cached jobs
	for _, status := range s.jobCache {
		jobs = append(jobs, status)
	}

	response := map[string]interface{}{
		"jobs": jobs,
	}
	s.writeJSONResponse(w, response)
}

// handleMetrics returns bot metrics
func (s *APIServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := map[string]interface{}{
		"bot_id":          s.agent.ID,
		"jobs_completed":  s.agent.jobsCompleted,
		"jobs_failed":     s.agent.jobsFailed,
		"total_crashes":   s.agent.totalCrashes,
		"uptime_seconds":  time.Since(s.agent.startTime).Seconds(),
		"current_job":     "",
		"cache_size":      len(s.jobCache),
	}

	if s.agent.currentJob != nil {
		metrics["current_job"] = s.agent.currentJob.ID
		metrics["job_runtime_seconds"] = time.Since(s.agent.jobStartTime).Seconds()
	}

	s.writeJSONResponse(w, metrics)
}

// UpdateJobStatus updates the status of a job in the cache
func (s *APIServer) UpdateJobStatus(jobID string, status *JobStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	status.UpdatedAt = time.Now()
	s.jobCache[jobID] = status

	// Cleanup old entries (keep last 100)
	if len(s.jobCache) > 100 {
		oldest := time.Now()
		var oldestID string
		for id, job := range s.jobCache {
			if job.UpdatedAt.Before(oldest) {
				oldest = job.UpdatedAt
				oldestID = id
			}
		}
		delete(s.jobCache, oldestID)
	}
}

// MarkJobCompleted marks a job as completed in the cache
func (s *APIServer) MarkJobCompleted(jobID string, success bool, message string, output string) {
	status := &JobStatus{
		JobID:     jobID,
		Status:    "completed",
		StartTime: s.agent.jobStartTime,
		EndTime:   time.Now(),
		Success:   success,
		Message:   message,
		Output:    output,
	}
	
	if s.agent.currentJob != nil {
		status.CrashCount = s.agent.currentJobCrashes
	}
	
	s.UpdateJobStatus(jobID, status)
}

// MarkJobFailed marks a job as failed in the cache
func (s *APIServer) MarkJobFailed(jobID string, err error) {
	status := &JobStatus{
		JobID:     jobID,
		Status:    "failed",
		StartTime: s.agent.jobStartTime,
		EndTime:   time.Now(),
		Success:   false,
		Message:   err.Error(),
	}
	
	s.UpdateJobStatus(jobID, status)
}

// writeJSONResponse writes a JSON response
func (s *APIServer) writeJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.WithError(err).Error("Failed to encode JSON response")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// writeErrorResponse writes an error response
func (s *APIServer) writeErrorResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	
	response := map[string]string{
		"error": message,
	}
	
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.WithError(err).Error("Failed to encode error response")
	}
}