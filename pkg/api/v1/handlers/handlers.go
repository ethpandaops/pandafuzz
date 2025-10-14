package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/adapters"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/api/v1/middleware"
)

// Handlers provides HTTP endpoint handlers for the PandaFuzz API v1
type Handlers struct {
	adapter    *adapters.CompositeAdapter
	middleware *middleware.Stack
	logger     logrus.FieldLogger
}

// NewHandlers creates a new handlers instance
func NewHandlers(
	adapter *adapters.CompositeAdapter,
	middleware *middleware.Stack,
	logger logrus.FieldLogger,
) *Handlers {
	return &Handlers{
		adapter:    adapter,
		middleware: middleware,
		logger:     logger.WithField("component", "handlers"),
	}
}

// RegisterRoutes registers all API routes with the provided Chi router
func (h *Handlers) RegisterRoutes(r chi.Router) {
	// Apply middleware stack
	if h.middleware != nil {
		// Apply all middleware to the API routes
		r.Use(h.middleware.Recovery())
		r.Use(h.middleware.RequestLogger())
		r.Use(h.middleware.CORS())
		r.Use(h.middleware.Tracing())
		r.Use(h.middleware.RequestMetrics())
		r.Use(h.middleware.RateLimit())
		r.Use(h.middleware.ValidateRequest())
		r.Use(h.middleware.JWTAuth())
	}

	// Health and system endpoints (no auth required)
	r.Get("/health", h.HandleHealth)
	r.Get("/ready", h.HandleReady)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Bot endpoints
		r.Route("/bots", func(r chi.Router) {
			r.Get("/", h.HandleListBots)
			r.Post("/", h.HandleCreateBot)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.HandleGetBot)
				r.Put("/", h.HandleUpdateBot)
				r.Delete("/", h.HandleDeleteBot)
				r.Post("/heartbeat", h.HandleBotHeartbeat)
				r.Get("/jobs", h.HandleGetBotJobs)
			})
		})

		// Job endpoints
		r.Route("/jobs", func(r chi.Router) {
			r.Get("/", h.HandleListJobs)
			r.Post("/", h.HandleCreateJob)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.HandleGetJob)
				r.Put("/", h.HandleUpdateJob)
				r.Delete("/", h.HandleDeleteJob)
				r.Post("/ack", h.HandleJobAck)
				r.Post("/heartbeat", h.HandleJobHeartbeat)
				r.Get("/logs", h.HandleGetJobLogs)
				r.Get("/coverage", h.HandleGetJobCoverage)
				r.Get("/artifacts", h.HandleGetJobArtifacts)
				r.Get("/coverage/reports/{reportId}", h.HandleDownloadCoverageReport)
			})
		})

		// Campaign endpoints
		r.Route("/campaigns", func(r chi.Router) {
			r.Get("/", h.HandleListCampaigns)
			r.Post("/", h.HandleCreateCampaign)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.HandleGetCampaign)
				r.Put("/", h.HandleUpdateCampaign)
				r.Delete("/", h.HandleDeleteCampaign)
				r.Post("/start", h.HandleStartCampaign)
				r.Post("/stop", h.HandleStopCampaign)
				r.Get("/stats", h.HandleGetCampaignStats)
			})
		})

		// Corpus endpoints
		r.Route("/corpus", func(r chi.Router) {
			r.Get("/", h.HandleListCorpus)
			r.Post("/", h.HandleUploadCorpus)
			r.Post("/sync", h.HandleSyncCorpus)
			r.Post("/select", h.HandleSelectCorpus)
			r.Get("/quarantine", h.HandleListQuarantinedCorpus)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.HandleGetCorpusEntry)
				r.Delete("/", h.HandleDeleteCorpusEntry)
				r.Get("/download", h.HandleDownloadCorpusFile)
			})
		})

		// Crash endpoints
		r.Route("/crashes", func(r chi.Router) {
			r.Get("/", h.HandleListCrashes)
			r.Post("/", h.HandleCreateCrash)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.HandleGetCrash)
				r.Post("/minimize", h.HandleMinimizeCrash)
				r.Post("/reproduce", h.HandleReproduceCrash)
				r.Post("/deduplicate", h.HandleDeduplicateCrash)
			})
		})

		// Analytics endpoints
		r.Route("/analytics", func(r chi.Router) {
			r.Get("/", h.HandleGetAnalytics)
			r.Get("/metrics", h.HandleGetMetrics)
			r.Get("/coverage", h.HandleGetCoverageTrends)
			r.Get("/performance", h.HandleGetPerformanceStats)
		})

		// Batch operations endpoint
		r.Post("/batch", h.HandleBatchOperations)

		// SSE events endpoint
		r.Get("/events", h.HandleEvents)
	})
}

// Compile-time check to ensure Handlers implements the necessary interfaces
var _ http.Handler = (*Handlers)(nil)

// ServeHTTP implements http.Handler interface
func (h *Handlers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// This should not be called directly; use RegisterRoutes with Chi router
	h.logger.Warn("ServeHTTP called directly, this should use RegisterRoutes with Chi router")
	http.NotFound(w, r)
}

// extractBotID extracts bot ID from URL parameters
func (h *Handlers) extractBotID(r *http.Request) generated.BotIdParam {
	idStr := chi.URLParam(r, "id")
	parsedUUID, _ := uuid.Parse(idStr)
	return generated.BotIdParam(openapi_types.UUID(parsedUUID))
}

// extractJobID extracts job ID from URL parameters
func (h *Handlers) extractJobID(r *http.Request) generated.JobIdParam {
	idStr := chi.URLParam(r, "id")
	parsedUUID, _ := uuid.Parse(idStr)
	return generated.JobIdParam(openapi_types.UUID(parsedUUID))
}

// extractCampaignID extracts campaign ID from URL parameters
func (h *Handlers) extractCampaignID(r *http.Request) generated.CampaignIdParam {
	idStr := chi.URLParam(r, "id")
	parsedUUID, _ := uuid.Parse(idStr)
	return generated.CampaignIdParam(openapi_types.UUID(parsedUUID))
}

// extractCorpusEntryID extracts corpus entry ID from URL parameters
func (h *Handlers) extractCorpusEntryID(r *http.Request) generated.CorpusEntryIdParam {
	idStr := chi.URLParam(r, "id")
	parsedUUID, _ := uuid.Parse(idStr)
	return generated.CorpusEntryIdParam(openapi_types.UUID(parsedUUID))
}

// extractCrashID extracts crash ID from URL parameters
func (h *Handlers) extractCrashID(r *http.Request) generated.CrashIdParam {
	idStr := chi.URLParam(r, "id")
	parsedUUID, _ := uuid.Parse(idStr)
	return generated.CrashIdParam(openapi_types.UUID(parsedUUID))
}

// extractReportID extracts report ID from URL parameters
func (h *Handlers) extractReportID(r *http.Request) generated.ReportIdParam {
	idStr := chi.URLParam(r, "reportId")
	parsedUUID, _ := uuid.Parse(idStr)
	return generated.ReportIdParam(openapi_types.UUID(parsedUUID))
}

// writeError is a helper method to write error responses
func (h *Handlers) writeError(w http.ResponseWriter, statusCode int, title string, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)

	// The generated.ProblemDetails is used here just for type reference
	// The actual JSON encoding is handled by the adapter
	_ = generated.ProblemDetails{
		Type:   "/errors/" + title,
		Title:  title,
		Status: statusCode,
		Detail: &detail,
	}

	// We let the adapter handle the actual error formatting
	h.logger.WithFields(logrus.Fields{
		"status": statusCode,
		"title":  title,
		"detail": detail,
	}).Error("handler error")
}
