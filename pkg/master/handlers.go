package master

import (
	"net/http"
	"time"
)

// Health endpoint handlers

// handleHealth provides health check endpoint
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now(),
		"uptime":    time.Since(s.stats.StartTime),
		"version":   "1.0.0", // TODO: Get from build info
	}
	
	// Check component health
	if err := s.state.HealthCheck(); err != nil {
		health["status"] = "unhealthy"
		health["database_error"] = err.Error()
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	
	if err := s.timeoutManager.HealthCheck(); err != nil {
		health["status"] = "unhealthy"
		health["timeout_manager_error"] = err.Error()
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	
	s.writeJSONResponse(w, health)
}

// handleMetrics provides metrics endpoint
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := map[string]interface{}{
		"server":          s.GetStats(),
		"state":           s.state.GetStats(),
		"timeouts":        s.timeoutManager.GetStats(),
		"database":        s.state.GetDatabaseStats(),
		"circuit_breaker": s.circuitBreaker.GetStats(),
	}
	
	s.writeJSONResponse(w, metrics)
}

// handleStatus provides detailed system status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	botTimeouts, jobTimeouts := s.timeoutManager.GetActiveTimeouts()
	
	status := map[string]interface{}{
		"server": map[string]interface{}{
			"running":    s.running,
			"start_time": s.stats.StartTime,
			"uptime":     time.Since(s.stats.StartTime),
		},
		"bots": map[string]interface{}{
			"total":         len(s.state.bots),
			"active_timeouts": botTimeouts,
		},
		"jobs": map[string]interface{}{
			"total":         len(s.state.jobs),
			"active_timeouts": jobTimeouts,
		},
		"database": s.state.GetDatabaseStats(),
	}
	
	s.writeJSONResponse(w, status)
}

// Helper functions

// updateRequestMetrics updates server metrics
func (s *Server) updateRequestMetrics(duration time.Duration, isError bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if isError {
		s.stats.ErrorCount++
	}
	
	// Update average latency (simple moving average)
	if s.stats.AverageLatency == 0 {
		s.stats.AverageLatency = duration
	} else {
		s.stats.AverageLatency = (s.stats.AverageLatency + duration) / 2
	}
}

// writeJSONResponse writes a JSON response
func (s *Server) writeJSONResponse(w http.ResponseWriter, data interface{}) {
	s.responseWriter.WriteJSONOK(w, data)
}

// writeErrorResponse writes an error response
func (s *Server) writeErrorResponse(w http.ResponseWriter, statusCode int, message string, err error) {
	s.responseWriter.WriteErrorWithStatus(w, statusCode, message, err)
}