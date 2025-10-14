package master

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/persistence/sqlite"
	apiv3 "github.com/ethpandaops/pandafuzz/pkg/master/api_v3"
	"github.com/ethpandaops/pandafuzz/pkg/master/repository"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
)

// setupRouter configures the HTTP router with all routes and middleware
func (s *Server) setupRouter() error {
	// Create Chi router if API v1 is initialized, otherwise use Gorilla mux
	if s.apiV1 != nil {
		s.setupChiRouter()
	} else {
		s.setupMuxRouter()
	}

	s.logger.Info("HTTP router configured with all routes")
	return nil
}

// setupChiRouter configures the Chi router with API v1 integration
func (s *Server) setupChiRouter() error {
	s.chiRouter = chi.NewRouter()

	// Add Chi middleware
	s.chiRouter.Use(middleware.Logger)
	s.chiRouter.Use(middleware.Recoverer)

	// Add custom middleware
	if s.config.Server.EnableCORS {
		s.chiRouter.Use(s.corsMiddlewareForChi())
	}

	// Add rate limiting if configured
	if s.config.Server.RateLimitRPS > 0 {
		s.chiRouter.Use(s.rateLimitMiddlewareForChi())
	}

	// Health and metrics endpoints (direct on Chi router)
	s.chiRouter.Get("/health", s.handleHealth)
	s.chiRouter.Get("/status", s.handleStatus)

	// Prometheus metrics endpoint
	if s.config.Monitoring.Enabled {
		s.chiRouter.Handle("/metrics", promhttp.Handler())
	} else {
		s.chiRouter.Get("/metrics", s.handleMetrics)
	}

	// Mount API v1 routes
	if s.apiV1 != nil {
		s.chiRouter.Mount("/api/v1", s.apiV1.GetRouter())
		s.logger.Info("API v1 routes mounted on Chi router")
	}

	// API v2 routes (backwards compatibility) - mount as subrouter
	s.chiRouter.Route("/api/v2", func(r chi.Router) {
		// Convert mux routes to chi - for now, we'll create a basic fallback
		s.logger.Info("API v2 routes will be handled through backwards compatibility")
		// TODO: Implement v2 route conversion or keep them on separate mux
	})

	// API v3 routes (if they exist)
	s.chiRouter.Route("/api/v3", func(r chi.Router) {
		s.setupAPIv3RoutesOnChi(r)
	})

	// Serve static files for web UI
	s.setupStaticFileServingOnChi()

	return nil
}

// setupMuxRouter configures the Gorilla mux router (fallback)
func (s *Server) setupMuxRouter() error {
	s.router = mux.NewRouter()

	// Add standard middleware
	s.router.Use(s.loggingMiddleware)
	s.router.Use(s.metricsMiddleware)
	s.router.Use(s.recoveryMiddleware)

	// Add request size limit middleware to prevent OOM attacks
	// Default to 1MB for regular requests, with specific limits for upload endpoints
	s.router.Use(s.requestSizeLimitMiddleware(1 * 1024 * 1024))

	// Add timeout middleware with global timeout
	// Use the configured timeout or default to 30 seconds
	timeout := s.config.Timeouts.HTTPRequest
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	s.router.Use(s.timeoutMiddleware(timeout))

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

	// API v2 routes
	apiV2 := s.router.PathPrefix("/api/v2").Subrouter()
	s.setupAPIv2Routes(apiV2)

	// API v3 routes
	s.setupAPIv3Routes()

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

	return nil
}

// setupAPIv3Routes configures API v3 routes
func (s *Server) setupAPIv3Routes() {
	s.logger.Info("Setting up API v3 routes")

	// Check that we're using SQLiteStorage
	if _, ok := s.state.db.(*storage.SQLiteStorage); !ok {
		s.logger.Error("Database is not SQLiteStorage, API v3 coverage features will not be available")
		return
	}

	// Create a new SQLite connection for the coverage repository
	// This will connect to the same database file
	dbPath := s.config.Database.Path
	if dbPath == "" {
		dbPath = "pandafuzz.db"
	}

	connConfig := sqlite.ConnectionConfig{
		FilePath:              dbPath,
		MaxOpenConnections:    10,
		MaxIdleConnections:    10,
		ConnectionMaxIdleTime: 5 * time.Minute,
		ConnectionMaxLifetime: time.Hour,
		EnableForeignKeys:     true,
		EnableWAL:             true,
		BusyTimeout:           5000,
		CacheSize:             -2000,
	}

	sqliteConn, err := sqlite.NewConnection(connConfig, s.logger)
	if err != nil {
		s.logger.WithError(err).Error("Failed to create SQLite connection for coverage repository")
		return
	}

	// Create coverage repository adapter for v1 table
	// Use the v1 adapter to read from existing coverage table instead of coverage_reports
	s.logger.Info("Creating coverage repository v1 adapter")
	coverageRepo, err := repository.NewCoverageRepositoryV1Adapter(sqliteConn, nil, s.logger)
	if err != nil {
		s.logger.WithError(err).Error("Failed to create coverage repository adapter")
		return
	}
	s.logger.Info("Coverage repository v1 adapter created successfully")

	// Create integration config
	config := apiv3.DefaultIntegrationConfig()
	config.EnableCORS = true
	config.AllowedOrigins = []string{"*"}
	config.EnableBackwardsCompatibility = false // We have separate v2 routes
	config.EnableDeprecationWarnings = false

	// Create version info
	versionInfo := &common.VersionInfo{
		Version:   s.version,
		BuildTime: s.buildTime,
		GitCommit: s.gitCommit,
	}

	// Create integration
	integration := apiv3.NewIntegration(s.services, coverageRepo, s.storageBackend, s.state.db, s.logger, config, versionInfo)

	// Register routes
	integration.RegisterRoutes(s.router)

	s.logger.Info("API v3 routes registered successfully")
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
	router.HandleFunc("/results/coverage-report", s.handleSubmitCoverageReport).Methods("POST")
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
	router.HandleFunc("/jobs/available-corpora", s.handleListAvailableCorpora).Methods("GET")
	router.HandleFunc("/jobs/{id}", s.handleJobGet).Methods("GET")
	router.HandleFunc("/jobs/{id}/cancel", s.handleJobCancel).Methods("PUT")
	router.HandleFunc("/jobs/{id}/status", s.handleJobStatusUpdate).Methods("PUT")
	router.HandleFunc("/jobs/{id}/coverage", s.handleGetCoverageReport).Methods("GET")
	router.HandleFunc("/jobs/{id}/coverage/raw", s.handleGetRawCoverageFiles).Methods("GET")
	router.HandleFunc("/jobs/{id}/coverage/raw/{fileType}", s.handleDownloadRawCoverageFile).Methods("GET")
	router.HandleFunc("/jobs/{id}/coverage/raw/all/zip", s.handleGetAllRawFiles).Methods("GET")
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
	router.HandleFunc("/system/maintenance", s.handleMaintenanceTrigger).Methods("POST")
	router.HandleFunc("/timeouts", s.handleTimeoutsList).Methods("GET")
	router.HandleFunc("/timeouts/{type}/{id}", s.handleTimeoutForce).Methods("POST")

	// Streaming and maintenance endpoints
	router.HandleFunc("/jobs/{id}/progress", s.handleJobProgress).Methods("GET")
	router.HandleFunc("/results/batch", s.handleBatchResults).Methods("POST")
	router.HandleFunc("/bots/{id}/resources", s.handleResourceMetrics).Methods("GET")

	// Campaign management routes (v1 for backward compatibility)
	router.HandleFunc("/campaigns", s.handleCreateCampaign).Methods("POST")
	router.HandleFunc("/campaigns", s.handleListCampaigns).Methods("GET")
	router.HandleFunc("/campaigns/{id}", s.handleGetCampaign).Methods("GET")
	router.HandleFunc("/campaigns/{id}", s.handleUpdateCampaign).Methods("PUT", "PATCH")
	router.HandleFunc("/campaigns/{id}", s.handleDeleteCampaign).Methods("DELETE")
	router.HandleFunc("/campaigns/{id}/restart", s.handleRestartCampaign).Methods("POST")
	router.HandleFunc("/campaigns/{id}/stats", s.handleGetCampaignStats).Methods("GET")
	router.HandleFunc("/campaigns/{id}/binary", s.handleUploadCampaignBinary).Methods("POST")
	router.HandleFunc("/campaigns/{id}/corpus", s.handleUploadCampaignCorpus).Methods("POST")

	// Corpus routes
	router.HandleFunc("/campaigns/{id}/corpus/evolution", s.handleGetCorpusEvolution).Methods("GET")
	router.HandleFunc("/campaigns/{id}/corpus/sync", s.handleSyncCorpus).Methods("POST")
	router.HandleFunc("/campaigns/{id}/corpus/share", s.handleShareCorpus).Methods("POST")
	router.HandleFunc("/campaigns/{id}/corpus/files", s.handleListCorpusFiles).Methods("GET")
	router.HandleFunc("/campaigns/{id}/corpus/files/{hash}", s.handleDownloadCorpusFile).Methods("GET")

	// Crash analysis routes
	router.HandleFunc("/campaigns/{id}/crashes", s.handleGetCrashGroups).Methods("GET")
	router.HandleFunc("/crashes/{id}/stacktrace", s.handleGetStackTrace).Methods("GET")

	// Corpus quarantine routes
	router.HandleFunc("/campaigns/{campaignID}/quarantine", s.handleQuarantineCorpusFile).Methods("POST")
	router.HandleFunc("/campaigns/{campaignID}/quarantine", s.handleGetQuarantinedFiles).Methods("GET")
	router.HandleFunc("/campaigns/{campaignID}/quarantine/restore", s.handleRestoreQuarantinedFile).Methods("POST")
	router.HandleFunc("/campaigns/{campaignID}/quarantine/delete", s.handleDeleteQuarantinedFile).Methods("DELETE")
	router.HandleFunc("/quarantine/rules", s.handleGetQuarantineRules).Methods("GET")
	router.HandleFunc("/quarantine/rules", s.handleSetQuarantineRule).Methods("PUT")
	router.HandleFunc("/quarantine/thresholds", s.handleSetQuarantineThresholds).Methods("PUT")
	router.HandleFunc("/corpus/files/{fileID}/metrics", s.handleUpdateCorpusFileMetrics).Methods("PUT")

	// Crash reproducibility routes
	router.HandleFunc("/crashes/{crashID}/reproduce", s.handleCrashReproduce).Methods("POST")
	router.HandleFunc("/crashes/{crashID}/reproduction", s.handleGetCrashReproduction).Methods("GET")
	router.HandleFunc("/reproduction/results", s.handleSubmitReproductionResult).Methods("POST")
	router.HandleFunc("/crashes/{crashID}/reproduction/results", s.handleGetReproductionResults).Methods("GET")

	// Corpus promotion routes
	router.HandleFunc("/corpus/promote", s.handleCorpusPromote).Methods("POST")
	// Note: /jobs/{id}/corpus GET route already exists above

	// Corpus collection routes
	router.HandleFunc("/corpus/collections", s.handleListCorpusCollections).Methods("GET")
	router.HandleFunc("/corpus/collections", s.handleCreateCorpusCollection).Methods("POST")
	router.HandleFunc("/corpus/collections/{id}", s.handleGetCorpusCollection).Methods("GET")
	router.HandleFunc("/corpus/collections/{id}", s.handleDeleteCorpusCollection).Methods("DELETE")
	router.HandleFunc("/corpus/collections/{id}/upload", s.handleUploadCorpusToCollection).Methods("POST")
	router.HandleFunc("/corpus/collections/{id}/files", s.handleGetCollectionFiles).Methods("GET")
	router.HandleFunc("/corpus/collections/{id}/files/{fileId}/download", s.handleDownloadCollectionFile).Methods("GET")

	// Crash minimization routes
	s.registerMinimizationRoutes(router)

	// S3 presigned URL routes for corpus
	router.HandleFunc("/corpus/{id}/files/{hash}/download-url", s.handleGetCorpusDownloadURL).Methods("GET")
	router.HandleFunc("/corpus/{id}/upload-url", s.handleGetCorpusUploadURL).Methods("POST")

	// Queue management routes (for asynq mode)
	router.HandleFunc("/queue/stats", s.handleQueueStats).Methods("GET")
	router.HandleFunc("/queue/stats/{queue}", s.handleQueueStatsDetail).Methods("GET")
	router.HandleFunc("/queue/pause", s.handleQueuePause).Methods("POST")
	router.HandleFunc("/queue/resume", s.handleQueueResume).Methods("POST")
	router.HandleFunc("/queue/purge/{queue}", s.handleQueuePurge).Methods("DELETE")

	// Analytics routes
	router.HandleFunc("/analytics/coverage-trend", s.handleGetCoverageTrend).Methods("GET")
	router.HandleFunc("/analytics/crash-timeline", s.handleGetCrashTimeline).Methods("GET")
	router.HandleFunc("/analytics/fuzzer-comparison", s.handleGetFuzzerComparison).Methods("GET")
	router.HandleFunc("/campaigns/{id}/insights", s.handleGetCampaignInsights).Methods("GET")

	s.logger.Info("API v1 routes configured")
}

// setupStaticFileServing configures static file serving for the web UI
func (s *Server) setupStaticFileServing() {
	// Check if web UI directory exists
	webStaticDir := "./web/static"
	if _, err := os.Stat(webStaticDir); os.IsNotExist(err) {
		s.logger.WithField("dir", webStaticDir).Warn("Web UI static directory not found, skipping static file serving")
		return
	}

	// Serve CSS files
	cssDir := "./web/css"
	if _, err := os.Stat(cssDir); err == nil {
		s.router.PathPrefix("/css/").Handler(http.StripPrefix("/css/", http.FileServer(http.Dir(cssDir)))).Methods("GET")
	}

	// Serve JS files
	jsDir := "./web/js"
	if _, err := os.Stat(jsDir); err == nil {
		s.router.PathPrefix("/js/").Handler(http.StripPrefix("/js/", http.FileServer(http.Dir(jsDir)))).Methods("GET")
	}

	// Create file server for static HTML files
	fileServer := http.FileServer(http.Dir(webStaticDir))

	// SPA handler - serves HTML files
	spaHandler := &spaFileHandler{
		staticPath: webStaticDir,
		fileServer: fileServer,
	}

	// Serve static HTML files - match everything except /api, /metrics, /css, /js
	s.router.PathPrefix("/").Handler(spaHandler).Methods("GET")

	s.logger.WithField("dir", webStaticDir).Info("Static file serving configured for web UI")
}

// spaFileHandler serves static files and handles SPA routing
type spaFileHandler struct {
	staticPath string
	fileServer http.Handler
}

func (h *spaFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if it's an API, metrics, CSS, JS, or other API endpoint
	if strings.HasPrefix(r.URL.Path, "/api/") ||
		strings.HasPrefix(r.URL.Path, "/metrics") ||
		strings.HasPrefix(r.URL.Path, "/health") ||
		strings.HasPrefix(r.URL.Path, "/status") ||
		strings.HasPrefix(r.URL.Path, "/css/") ||
		strings.HasPrefix(r.URL.Path, "/js/") ||
		strings.HasPrefix(r.URL.Path, "/jobs") ||
		strings.HasPrefix(r.URL.Path, "/bots") ||
		strings.HasPrefix(r.URL.Path, "/results") ||
		strings.HasPrefix(r.URL.Path, "/campaigns") ||
		strings.HasPrefix(r.URL.Path, "/corpus") ||
		strings.HasPrefix(r.URL.Path, "/crashes") ||
		strings.HasPrefix(r.URL.Path, "/system") ||
		strings.HasPrefix(r.URL.Path, "/timeouts") ||
		strings.HasPrefix(r.URL.Path, "/analytics") ||
		strings.HasPrefix(r.URL.Path, "/quarantine") ||
		strings.HasPrefix(r.URL.Path, "/reproduction") {
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

// corsMiddlewareForChi returns CORS middleware for Chi router
func (s *Server) corsMiddlewareForChi() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitMiddlewareForChi returns rate limiting middleware for Chi router
func (s *Server) rateLimitMiddlewareForChi() func(http.Handler) http.Handler {
	// For now, this is a simple implementation
	// In production, you'd want to use a proper rate limiter
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: Implement proper rate limiting
			next.ServeHTTP(w, r)
		})
	}
}

// setupAPIv3RoutesOnChi configures API v3 routes on Chi router
func (s *Server) setupAPIv3RoutesOnChi(r chi.Router) {
	s.logger.Info("Setting up API v3 routes on Chi router")

	// Check that we're using SQLiteStorage
	if _, ok := s.state.db.(*storage.SQLiteStorage); !ok {
		s.logger.Error("Database is not SQLiteStorage, API v3 coverage features will not be available")
		return
	}

	// Create a new SQLite connection for the coverage repository
	dbPath := s.config.Database.Path
	if dbPath == "" {
		dbPath = "pandafuzz.db"
	}

	connConfig := sqlite.ConnectionConfig{
		FilePath:              dbPath,
		MaxOpenConnections:    10,
		MaxIdleConnections:    10,
		ConnectionMaxIdleTime: 5 * time.Minute,
		ConnectionMaxLifetime: time.Hour,
		EnableForeignKeys:     true,
		EnableWAL:             true,
		BusyTimeout:           5000,
		CacheSize:             -2000,
	}

	sqliteConn, err := sqlite.NewConnection(connConfig, s.logger)
	if err != nil {
		s.logger.WithError(err).Error("Failed to create SQLite connection for coverage repository")
		return
	}

	// Create coverage repository adapter for v1 table
	coverageRepo, err := repository.NewCoverageRepositoryV1Adapter(sqliteConn, nil, s.logger)
	if err != nil {
		s.logger.WithError(err).Error("Failed to create coverage repository adapter")
		return
	}

	// Create integration config
	config := apiv3.DefaultIntegrationConfig()
	config.EnableCORS = true
	config.AllowedOrigins = []string{"*"}
	config.EnableBackwardsCompatibility = false
	config.EnableDeprecationWarnings = false

	// Create version info
	versionInfo := &common.VersionInfo{
		Version:   s.version,
		BuildTime: s.buildTime,
		GitCommit: s.gitCommit,
	}

	// Create integration
	integration := apiv3.NewIntegration(s.services, coverageRepo, s.storageBackend, s.state.db, s.logger, config, versionInfo)

	// Convert the mux routes to chi routes
	// For now, we'll create a simple handler that bridges to the existing integration
	// Since Integration doesn't have GetRouter(), we need to register routes differently
	// For now, skip API v3 on Chi router
	// r.Mount("/", integration.GetHandler())
	s.logger.Warn("API v3 routes not registered on Chi router - requires migration")
	_ = integration // Avoid unused variable warning
}

// setupStaticFileServingOnChi configures static file serving for Chi router
func (s *Server) setupStaticFileServingOnChi() {
	// Check if web UI directory exists
	webStaticDir := "./web/static"
	if _, err := os.Stat(webStaticDir); os.IsNotExist(err) {
		s.logger.WithField("dir", webStaticDir).Warn("Web UI static directory not found, skipping static file serving")
		return
	}

	// Serve CSS files
	cssDir := "./web/css"
	if _, err := os.Stat(cssDir); err == nil {
		cssFileServer := http.FileServer(http.Dir(cssDir))
		s.chiRouter.Mount("/css", http.StripPrefix("/css", cssFileServer))
	}

	// Serve JS files
	jsDir := "./web/js"
	if _, err := os.Stat(jsDir); err == nil {
		jsFileServer := http.FileServer(http.Dir(jsDir))
		s.chiRouter.Mount("/js", http.StripPrefix("/js", jsFileServer))
	}

	// Create file server for static HTML files
	fileServer := http.FileServer(http.Dir(webStaticDir))

	// SPA handler - serves HTML files
	spaHandler := &spaFileHandler{
		staticPath: webStaticDir,
		fileServer: fileServer,
	}

	// Serve static HTML files - catch-all for everything not matched above
	s.chiRouter.Get("/*", spaHandler.ServeHTTP)

	s.logger.WithField("dir", webStaticDir).Info("Static file serving configured for web UI on Chi router")
}
