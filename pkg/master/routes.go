package master

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// setupRouter configures the HTTP router with all routes and middleware
func (s *Server) setupRouter() error {
	s.router = mux.NewRouter()
	
	// Add standard middleware
	s.router.Use(s.loggingMiddleware)
	s.router.Use(s.metricsMiddleware)
	s.router.Use(s.recoveryMiddleware)
	// Temporarily disable timeout middleware - it may be causing issues with static file serving
	// s.router.Use(s.timeoutMiddleware)
	
	// Add Prometheus monitoring middleware if enabled
	if s.services != nil && s.config.Monitoring.Enabled {
		collector := s.services.Monitoring.GetCollector()
		s.router.Use(collector.HTTPMiddleware)
	}
	
	// Add CORS middleware if enabled
	if s.config.Server.EnableCORS {
		s.router.Use(s.corsMiddleware)
	}
	
	// Add rate limiting if configured
	if s.config.Server.RateLimitRPS > 0 {
		s.router.Use(s.rateLimitMiddleware)
	}
	
	// Add custom middleware
	for _, middleware := range s.middleware {
		s.router.Use(mux.MiddlewareFunc(middleware))
	}
	
	// API v1 routes
	apiV1 := s.router.PathPrefix("/api/v1").Subrouter()
	s.setupAPIRoutes(apiV1)
	
	// Health and metrics endpoints
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")
	s.router.HandleFunc("/status", s.handleStatus).Methods("GET")
	
	// Prometheus metrics endpoint
	if s.config.Monitoring.Enabled {
		s.router.Handle("/metrics", promhttp.Handler()).Methods("GET")
	} else {
		// Use the existing metrics handler for basic metrics
		s.router.HandleFunc("/metrics", s.handleMetrics).Methods("GET")
	}
	
	// Serve static files for web UI
	s.setupStaticFileServing()
	
	s.logger.Info("HTTP router configured with all routes")
	return nil
}

// setupAPIRoutes configures API v1 routes
func (s *Server) setupAPIRoutes(router *mux.Router) {
	// Bot lifecycle management
	router.HandleFunc("/bots/register", s.handleBotRegister).Methods("POST")
	router.HandleFunc("/bots/{id}", s.handleBotGet).Methods("GET")
	router.HandleFunc("/bots/{id}", s.handleBotDelete).Methods("DELETE")
	router.HandleFunc("/bots/{id}/heartbeat", s.handleBotHeartbeat).Methods("POST")
	router.HandleFunc("/bots/{id}/job", s.handleBotGetJob).Methods("GET")
	router.HandleFunc("/bots/{id}/job/complete", s.handleBotCompleteJob).Methods("POST")
	router.HandleFunc("/bots", s.handleBotList).Methods("GET")
	
	// Result communication (Bot -> Master)
	router.HandleFunc("/results/crash", s.handleResultCrash).Methods("POST")
	router.HandleFunc("/results/coverage", s.handleResultCoverage).Methods("POST")
	router.HandleFunc("/results/corpus", s.handleResultCorpus).Methods("POST")
	router.HandleFunc("/results/status", s.handleResultStatus).Methods("POST")
	
	// Result retrieval (Admin/UI)
	router.HandleFunc("/results/crashes", s.handleGetCrashes).Methods("GET")
	router.HandleFunc("/results/crashes/{id}", s.handleGetCrash).Methods("GET")
	router.HandleFunc("/results/crashes/{id}/input", s.handleGetCrashInput).Methods("GET")
	router.HandleFunc("/jobs/{id}/crashes", s.handleGetJobCrashes).Methods("GET")
	
	// Job management (Admin)
	router.HandleFunc("/jobs", s.handleJobCreate).Methods("POST")
	router.HandleFunc("/jobs/upload", s.handleJobCreateWithUpload).Methods("POST")
	router.HandleFunc("/jobs", s.handleJobList).Methods("GET")
	router.HandleFunc("/jobs/{id}", s.handleJobGet).Methods("GET")
	router.HandleFunc("/jobs/{id}/cancel", s.handleJobCancel).Methods("PUT")
	router.HandleFunc("/jobs/{id}/logs", s.handleJobLogsV2).Methods("GET")
	router.HandleFunc("/jobs/{id}/logs/stream", s.handleJobLogStream).Methods("GET")
	router.HandleFunc("/jobs/{id}/logs/push", s.handleLogPush).Methods("POST")
	router.HandleFunc("/jobs/{id}/logs/exists", s.handleLogExists).Methods("GET")
	
	// Binary and corpus download for bots
	router.HandleFunc("/jobs/{id}/binary/download", s.handleBinaryDownload).Methods("GET")
	router.HandleFunc("/jobs/{id}/corpus/download", s.handleCorpusDownload).Methods("GET")
	
	// Corpus management endpoints
	router.HandleFunc("/jobs/{id}/corpus", s.handleGetJobCorpus).Methods("GET")
	router.HandleFunc("/jobs/{id}/corpus", s.handleUploadJobCorpus).Methods("POST")
	router.HandleFunc("/jobs/{id}/corpus/stats", s.handleGetCorpusStats).Methods("GET")
	router.HandleFunc("/jobs/{id}/corpus/{filename}", s.handleDownloadCorpusFile).Methods("GET")
	router.HandleFunc("/jobs/{id}/corpus/{filename}", s.handleDeleteCorpusFile).Methods("DELETE")
	
	// System status and management
	router.HandleFunc("/system/stats", s.handleSystemStats).Methods("GET")
	router.HandleFunc("/system/recovery", s.handleSystemRecovery).Methods("POST")
	router.HandleFunc("/timeouts", s.handleTimeoutsList).Methods("GET")
	router.HandleFunc("/timeouts/{type}/{id}", s.handleTimeoutForce).Methods("POST")
	
	s.logger.Info("API v1 routes configured")
}

// setupStaticFileServing configures static file serving for the web UI
func (s *Server) setupStaticFileServing() {
	// Check if web UI directory exists
	webDir := "./web/build"
	if _, err := os.Stat(webDir); os.IsNotExist(err) {
		s.logger.WithField("dir", webDir).Warn("Web UI build directory not found, skipping static file serving")
		return
	}
	
	// Create file server
	fileServer := http.FileServer(http.Dir(webDir))
	
	// SPA handler - serves index.html for non-API routes
	spaHandler := &spaFileHandler{
		staticPath: webDir,
		fileServer: fileServer,
	}
	
	// Serve static files - match everything except /api and /metrics
	s.router.PathPrefix("/").Handler(spaHandler).Methods("GET")
	
	s.logger.WithField("dir", webDir).Info("Static file serving configured for web UI")
}

// spaFileHandler serves static files and handles SPA routing
type spaFileHandler struct {
	staticPath string
	fileServer http.Handler
}

func (h *spaFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if it's an API or metrics request
	if strings.HasPrefix(r.URL.Path, "/api/") || 
	   strings.HasPrefix(r.URL.Path, "/metrics") || 
	   strings.HasPrefix(r.URL.Path, "/health") ||
	   strings.HasPrefix(r.URL.Path, "/status") {
		// These are handled by other routes
		http.NotFound(w, r)
		return
	}
	
	// Get the absolute path to prevent directory traversal
	path := filepath.Join(h.staticPath, r.URL.Path)
	
	// Check if file exists
	_, err := os.Stat(path)
	if os.IsNotExist(err) || r.URL.Path == "/" {
		// File doesn't exist or root path, serve index.html
		http.ServeFile(w, r, filepath.Join(h.staticPath, "index.html"))
		return
	}
	
	// Serve the requested file
	h.fileServer.ServeHTTP(w, r)
}