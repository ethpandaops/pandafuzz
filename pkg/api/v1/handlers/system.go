package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// HandleGetSystemStatus handles GET /api/v1/system/status
func (h *Handlers) HandleGetSystemStatus(w http.ResponseWriter, r *http.Request) {
	h.adapter.GetSystemStatus(w, r)
}

// HandleGetSystemStats handles GET /api/v1/system/stats
func (h *Handlers) HandleGetSystemStats(w http.ResponseWriter, r *http.Request) {
	h.adapter.GetSystemStats(w, r)
}

// HandleGetVersion handles GET /api/v1/system/version
func (h *Handlers) HandleGetVersion(w http.ResponseWriter, r *http.Request) {
	h.adapter.GetVersion(w, r)
}

// HandleTriggerRecovery handles POST /api/v1/system/recovery
func (h *Handlers) HandleTriggerRecovery(w http.ResponseWriter, r *http.Request) {
	h.adapter.TriggerRecovery(w, r)
}

// HandleTriggerMaintenance handles POST /api/v1/system/maintenance
func (h *Handlers) HandleTriggerMaintenance(w http.ResponseWriter, r *http.Request) {
	h.adapter.TriggerMaintenance(w, r)
}

// HandleListTimeouts handles GET /api/v1/system/timeouts
func (h *Handlers) HandleListTimeouts(w http.ResponseWriter, r *http.Request) {
	h.adapter.ListTimeouts(w, r)
}

// HandleForceTimeout handles POST /api/v1/system/timeouts/{type}/{id}
func (h *Handlers) HandleForceTimeout(w http.ResponseWriter, r *http.Request) {
	timeoutType := chi.URLParam(r, "type")
	id := chi.URLParam(r, "id")
	h.adapter.ForceTimeout(w, r, timeoutType, id)
}

// HandleDetailedHealthCheck handles GET /api/v1/system/health/detailed
func (h *Handlers) HandleDetailedHealthCheck(w http.ResponseWriter, r *http.Request) {
	h.adapter.DetailedHealthCheck(w, r)
}

// HandleGetNextJob handles POST /api/v1/bots/{id}/jobs/next
func (h *Handlers) HandleGetNextJob(w http.ResponseWriter, r *http.Request) {
	botId := h.extractBotID(r)
	h.adapter.GetNextJob(w, r, botId)
}

// HandleCompleteJob handles POST /api/v1/bots/{id}/jobs/complete
func (h *Handlers) HandleCompleteJob(w http.ResponseWriter, r *http.Request) {
	botId := h.extractBotID(r)
	h.adapter.CompleteJob(w, r, botId)
}

// HandleGetBotMetrics handles GET /api/v1/bots/{id}/metrics
func (h *Handlers) HandleGetBotMetrics(w http.ResponseWriter, r *http.Request) {
	botId := h.extractBotID(r)
	h.adapter.GetBotMetrics(w, r, botId)
}

// HandleGetJobProgress handles GET /api/v1/jobs/{id}/progress
func (h *Handlers) HandleGetJobProgress(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)
	h.adapter.GetJobProgress(w, r, jobId)
}

// HandleGetJobCrashes handles GET /api/v1/jobs/{id}/crashes
func (h *Handlers) HandleGetJobCrashes(w http.ResponseWriter, r *http.Request) {
	jobId := h.extractJobID(r)
	h.adapter.GetJobCrashes(w, r, jobId)
}

// HandleGetCrashInput handles GET /api/v1/crashes/{id}/input
func (h *Handlers) HandleGetCrashInput(w http.ResponseWriter, r *http.Request) {
	crashId := h.extractCrashID(r)
	h.adapter.GetCrashInput(w, r, crashId)
}

// Result submission handlers (from v3)

// HandleSubmitBatchResults handles POST /api/v1/results/batch
func (h *Handlers) HandleSubmitBatchResults(w http.ResponseWriter, r *http.Request) {
	h.adapter.SubmitBatchResults(w, r)
}

// HandleSubmitCrashResult handles POST /api/v1/results/crash
func (h *Handlers) HandleSubmitCrashResult(w http.ResponseWriter, r *http.Request) {
	h.adapter.SubmitCrashResult(w, r)
}

// HandleSubmitCoverageResult handles POST /api/v1/results/coverage
func (h *Handlers) HandleSubmitCoverageResult(w http.ResponseWriter, r *http.Request) {
	h.adapter.SubmitCoverageResult(w, r)
}

// HandleSubmitCorpusUpdate handles POST /api/v1/results/corpus
func (h *Handlers) HandleSubmitCorpusUpdate(w http.ResponseWriter, r *http.Request) {
	h.adapter.SubmitCorpusUpdate(w, r)
}
