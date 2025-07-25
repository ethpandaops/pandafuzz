package v1

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/interfaces/api/rest/v1/handlers/bot"
	"github.com/ethpandaops/pandafuzz/pkg/interfaces/api/rest/v1/handlers/campaign"
	"github.com/ethpandaops/pandafuzz/pkg/interfaces/api/rest/v1/handlers/corpus"
	"github.com/ethpandaops/pandafuzz/pkg/interfaces/api/rest/v1/handlers/crash"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// RouterConfig contains configuration for the V1 API router
type RouterConfig struct {
	// Services
	BotService      service.BotService
	JobService      service.JobService
	ResultService   service.ResultService
	CampaignService common.CampaignService
	CorpusService   common.CorpusService
	Storage         common.Storage

	// Logger
	Logger logrus.FieldLogger

	// Middleware configuration
	EnableCORS      bool
	AllowedOrigins  []string
	RequestTimeout  string
	EnableMetrics   bool
	EnableRateLimit bool
	RateLimitRPS    int
}

// Router manages the V1 API routes
type Router struct {
	config   *RouterConfig
	router   *mux.Router
	handlers *Handlers
}

// Handlers contains all handler instances
type Handlers struct {
	Bot      *bot.Handler
	Campaign *campaign.Handler
	Crash    *crash.Handler
	Corpus   *corpus.Handler
}

// NewRouter creates a new V1 API router
func NewRouter(config *RouterConfig) *Router {
	if config.Logger == nil {
		config.Logger = logrus.New()
	}

	// Create handlers
	handlers := &Handlers{
		Bot: bot.NewHandler(
			config.BotService,
			config.JobService,
			config.Logger,
		),
		Campaign: campaign.NewHandler(
			config.CampaignService,
			config.CorpusService,
			config.Logger,
		),
		Crash: crash.NewHandler(
			config.ResultService,
			config.Storage,
			config.Logger,
		),
		Corpus: corpus.NewHandler(
			config.CorpusService,
			config.CampaignService,
			config.BotService,
			config.Storage,
			config.Logger,
		),
	}

	return &Router{
		config:   config,
		handlers: handlers,
	}
}

// SetupRoutes configures all V1 API routes on the provided router
func (r *Router) SetupRoutes(router *mux.Router) {
	r.router = router

	// Apply middleware
	r.applyMiddleware()

	// Bot routes
	r.setupBotRoutes()

	// Campaign routes
	r.setupCampaignRoutes()

	// Crash routes
	r.setupCrashRoutes()

	// Corpus routes
	r.setupCorpusRoutes()

	// Job routes
	r.setupJobRoutes()

	// Result routes
	r.setupResultRoutes()

	// System routes
	r.setupSystemRoutes()
}

// applyMiddleware applies middleware to the router
func (r *Router) applyMiddleware() {
	// Request ID middleware
	r.router.Use(RequestIDMiddleware())

	// Logging middleware
	r.router.Use(LoggingMiddleware(r.config.Logger))

	// Error handling middleware
	r.router.Use(ErrorHandlingMiddleware(r.config.Logger))

	// CORS middleware
	if r.config.EnableCORS {
		origins := r.config.AllowedOrigins
		if len(origins) == 0 {
			origins = []string{"*"}
		}
		r.router.Use(CORSMiddleware(origins))
	}

	// Content type middleware
	r.router.Use(ContentTypeMiddleware())
}

// setupBotRoutes configures bot-related routes
func (r *Router) setupBotRoutes() {
	// Bot lifecycle management
	r.router.HandleFunc("/bots/register", r.handlers.Bot.Register).Methods("POST")
	r.router.HandleFunc("/bots/{id}", r.handlers.Bot.Get).Methods("GET")
	r.router.HandleFunc("/bots/{id}", r.handlers.Bot.Delete).Methods("DELETE")
	r.router.HandleFunc("/bots/{id}/heartbeat", r.handlers.Bot.Heartbeat).Methods("POST")
	r.router.HandleFunc("/bots/{id}/job", r.handlers.Bot.GetJob).Methods("GET")
	r.router.HandleFunc("/bots/{id}/job/complete", r.handlers.Bot.CompleteJob).Methods("POST")
	r.router.HandleFunc("/bots", r.handlers.Bot.List).Methods("GET")
	r.router.HandleFunc("/bots/{id}/resources", r.handlers.Bot.GetResourceMetrics).Methods("GET")
}

// setupCampaignRoutes configures campaign-related routes
func (r *Router) setupCampaignRoutes() {
	// Campaign management
	r.router.HandleFunc("/campaigns", r.handlers.Campaign.Create).Methods("POST")
	r.router.HandleFunc("/campaigns", r.handlers.Campaign.List).Methods("GET")
	r.router.HandleFunc("/campaigns/{id}", r.handlers.Campaign.Get).Methods("GET")
	r.router.HandleFunc("/campaigns/{id}", r.handlers.Campaign.Update).Methods("PUT", "PATCH")
	r.router.HandleFunc("/campaigns/{id}", r.handlers.Campaign.Delete).Methods("DELETE")
	r.router.HandleFunc("/campaigns/{id}/restart", r.handlers.Campaign.Restart).Methods("POST")
	r.router.HandleFunc("/campaigns/{id}/stats", r.handlers.Campaign.GetStats).Methods("GET")
	r.router.HandleFunc("/campaigns/{id}/binary", r.handlers.Campaign.UploadBinary).Methods("POST")
	r.router.HandleFunc("/campaigns/{id}/corpus", r.handlers.Campaign.UploadCorpus).Methods("POST")
}

// setupCrashRoutes configures crash-related routes
func (r *Router) setupCrashRoutes() {
	// Crash submission and retrieval
	r.router.HandleFunc("/results/crash", r.handlers.Crash.SubmitCrash).Methods("POST")
	r.router.HandleFunc("/results/crashes", r.handlers.Crash.List).Methods("GET")
	r.router.HandleFunc("/results/crashes/{id}", r.handlers.Crash.Get).Methods("GET")
	r.router.HandleFunc("/results/crashes/{id}/input", r.handlers.Crash.GetCrashInput).Methods("GET")
	r.router.HandleFunc("/jobs/{id}/crashes", r.handlers.Crash.GetJobCrashes).Methods("GET")

	// Crash analysis
	r.router.HandleFunc("/campaigns/{id}/crashes", r.handlers.Crash.GetCrashGroups).Methods("GET")
	r.router.HandleFunc("/crashes/{crash_id}/stacktrace", r.handlers.Crash.GetStackTrace).Methods("GET")

	// Batch submission
	r.router.HandleFunc("/results/batch", r.handlers.Crash.BatchSubmit).Methods("POST")
}

// setupCorpusRoutes configures corpus-related routes
func (r *Router) setupCorpusRoutes() {
	// Corpus management
	r.router.HandleFunc("/campaigns/{id}/corpus/evolution", r.handlers.Corpus.GetEvolution).Methods("GET")
	r.router.HandleFunc("/campaigns/{id}/corpus/sync", r.handlers.Corpus.SyncCorpus).Methods("POST")
	r.router.HandleFunc("/campaigns/{id}/corpus/share", r.handlers.Corpus.ShareCorpus).Methods("POST")
	r.router.HandleFunc("/campaigns/{id}/corpus/files", r.handlers.Corpus.ListFiles).Methods("GET")
	r.router.HandleFunc("/campaigns/{id}/corpus/import", r.handlers.Corpus.ImportCorpus).Methods("POST")
	r.router.HandleFunc("/campaigns/{id}/corpus/cleanup", r.handlers.Corpus.CleanupOrphaned).Methods("POST")

	// Corpus promotion
	r.router.HandleFunc("/corpus/promote", r.handlers.Corpus.PromoteCrash).Methods("POST")

	// Result submission
	r.router.HandleFunc("/results/coverage", r.handlers.Corpus.SubmitCoverage).Methods("POST")
	r.router.HandleFunc("/results/corpus", r.handlers.Corpus.SubmitUpdate).Methods("POST")
}

// setupJobRoutes configures job-related routes
func (r *Router) setupJobRoutes() {
	// Job management routes would be implemented here
	// For now, these are placeholders as they require a separate job handler

	// Commented out to avoid conflicts with master server routes
	// Job creation and management
	// r.router.HandleFunc("/jobs", r.notImplemented).Methods("POST")
	// r.router.HandleFunc("/jobs", r.notImplemented).Methods("GET")
	// r.router.HandleFunc("/jobs/{id}", r.notImplemented).Methods("GET")
	// r.router.HandleFunc("/jobs/{id}/cancel", r.notImplemented).Methods("PUT")
	// r.router.HandleFunc("/jobs/{id}/logs", r.notImplemented).Methods("GET")
	// r.router.HandleFunc("/jobs/{id}/progress", r.notImplemented).Methods("GET")
}

// setupResultRoutes configures result-related routes
func (r *Router) setupResultRoutes() {
	// Status updates
	r.router.HandleFunc("/results/status", r.notImplemented).Methods("POST")
}

// setupSystemRoutes configures system-related routes
func (r *Router) setupSystemRoutes() {
	// System management routes would be implemented here
	// For now, these are placeholders as they require a separate system handler

	r.router.HandleFunc("/system/stats", r.notImplemented).Methods("GET")
	r.router.HandleFunc("/system/recovery", r.notImplemented).Methods("POST")
	r.router.HandleFunc("/system/maintenance", r.notImplemented).Methods("POST")
	r.router.HandleFunc("/timeouts", r.notImplemented).Methods("GET")
	r.router.HandleFunc("/timeouts/{type}/{id}", r.notImplemented).Methods("POST")
}

// notImplemented is a placeholder handler for routes not yet implemented
func (r *Router) notImplemented(w http.ResponseWriter, req *http.Request) {
	response := ErrorResponse{
		Error:     "Not Implemented",
		Message:   "This endpoint is not yet implemented",
		Timestamp: time.Now(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(response)
}

// GetHandlers returns the handlers for testing or direct access
func (r *Router) GetHandlers() *Handlers {
	return r.handlers
}
