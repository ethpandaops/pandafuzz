package monitoring

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// Collector provides methods to collect and export metrics
type Collector struct {
	metrics *Metrics
	logger  *logrus.Logger
}

// NewCollector creates a new metrics collector
func NewCollector(logger *logrus.Logger) *Collector {
	if logger == nil {
		logger = logrus.New()
	}
	
	return &Collector{
		metrics: NewMetrics(),
		logger:  logger,
	}
}

// GetMetrics returns the metrics instance
func (c *Collector) GetMetrics() *Metrics {
	return c.metrics
}

// HTTPMiddleware returns a middleware that collects HTTP metrics
func (c *Collector) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Wrap response writer to capture status and size
		wrapped := &responseWriterWrapper{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			bytesWritten:   0,
		}
		
		// Get route pattern from mux
		route := mux.CurrentRoute(r)
		pattern := "unknown"
		if route != nil {
			pattern, _ = route.GetPathTemplate()
		}
		
		// Process request
		next.ServeHTTP(wrapped, r)
		
		// Record metrics
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(wrapped.statusCode)
		
		c.metrics.HTTPRequestsTotal.WithLabelValues(r.Method, pattern, status).Inc()
		c.metrics.HTTPRequestDuration.WithLabelValues(r.Method, pattern).Observe(duration)
		c.metrics.HTTPResponseSize.WithLabelValues(r.Method, pattern).Observe(float64(wrapped.bytesWritten))
		
		c.logger.WithFields(logrus.Fields{
			"method":   r.Method,
			"endpoint": pattern,
			"status":   status,
			"duration": duration,
			"size":     wrapped.bytesWritten,
		}).Debug("HTTP request processed")
	})
}

// RecordBotHeartbeat records bot heartbeat metrics
func (c *Collector) RecordBotHeartbeat(botID string, status string, lastSeen time.Time) {
	c.metrics.BotHeartbeats.WithLabelValues(botID, status).Inc()
	
	// Calculate time since last heartbeat
	latency := time.Since(lastSeen).Seconds()
	c.metrics.BotHeartbeatLatency.WithLabelValues(botID).Observe(latency)
}

// RecordBotTimeout records a bot timeout event
func (c *Collector) RecordBotTimeout(botID string) {
	c.metrics.BotTimeouts.WithLabelValues(botID).Inc()
}

// UpdateBotGauge updates the bot count by status
func (c *Collector) UpdateBotGauge(statusCounts map[string]int) {
	for status, count := range statusCounts {
		c.metrics.BotsTotal.WithLabelValues(status).Set(float64(count))
	}
}

// RecordJobCreated records job creation
func (c *Collector) RecordJobCreated(fuzzer string) {
	c.metrics.JobsTotal.WithLabelValues("created", fuzzer).Inc()
}

// RecordJobCompleted records job completion
func (c *Collector) RecordJobCompleted(fuzzer string, success bool, duration time.Duration) {
	status := "completed"
	if !success {
		status = "failed"
	}
	
	c.metrics.JobsTotal.WithLabelValues(status, fuzzer).Inc()
	c.metrics.JobDuration.WithLabelValues(fuzzer, status).Observe(duration.Seconds())
}

// UpdateJobQueueSize updates the job queue size metric
func (c *Collector) UpdateJobQueueSize(size int) {
	c.metrics.JobQueueSize.Set(float64(size))
}

// UpdateActiveJobs updates active job gauges
func (c *Collector) UpdateActiveJobs(activeByFuzzer map[string]int) {
	for fuzzer, count := range activeByFuzzer {
		c.metrics.JobsActive.WithLabelValues(fuzzer).Set(float64(count))
	}
}

// RecordCrash records a crash finding
func (c *Collector) RecordCrash(jobID, crashType string, isUnique bool) {
	unique := "duplicate"
	if isUnique {
		unique = "unique"
	}
	
	c.metrics.CrashesTotal.WithLabelValues(jobID, crashType, unique).Inc()
	
	if isUnique {
		// Increment unique crash gauge
		c.metrics.UniqueCrashes.WithLabelValues(jobID).Inc()
	}
}

// UpdateCoverage updates coverage metrics
func (c *Collector) UpdateCoverage(jobID string, edges, newEdges int64) {
	c.metrics.CoverageEdges.WithLabelValues(jobID).Set(float64(edges))
	if newEdges > 0 {
		c.metrics.CoverageNewEdges.WithLabelValues(jobID).Add(float64(newEdges))
	}
}

// UpdateCorpusSize updates corpus size metric
func (c *Collector) UpdateCorpusSize(jobID string, sizeBytes int64) {
	c.metrics.CorpusSize.WithLabelValues(jobID).Set(float64(sizeBytes))
}

// UpdateExecRate updates fuzzing execution rate
func (c *Collector) UpdateExecRate(jobID, botID string, execPerSec float64) {
	c.metrics.ExecRate.WithLabelValues(jobID, botID).Set(execPerSec)
}

// RecordDatabaseOperation records database operation metrics
func (c *Collector) RecordDatabaseOperation(operation, table string, duration time.Duration, err error) {
	c.metrics.DatabaseQueries.WithLabelValues(operation, table).Inc()
	c.metrics.DatabaseLatency.WithLabelValues(operation, table).Observe(duration.Seconds())
	
	if err != nil {
		errorType := "unknown"
		// Categorize error types
		if err.Error() == "database is locked" {
			errorType = "locked"
		} else if err.Error() == "no such table" {
			errorType = "missing_table"
		}
		c.metrics.DatabaseErrors.WithLabelValues(operation, errorType).Inc()
	}
}

// UpdateCircuitBreakerState updates circuit breaker state metric
func (c *Collector) UpdateCircuitBreakerState(name, state string) {
	value := 0.0
	switch state {
	case "open":
		value = 1.0
	case "half-open":
		value = 2.0
	}
	c.metrics.CircuitBreakerState.WithLabelValues(name).Set(value)
}

// StartMetricsServer starts a separate HTTP server for metrics
func (c *Collector) StartMetricsServer(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()
	
	c.logger.WithField("addr", addr).Info("Starting metrics server")
	return server.ListenAndServe()
}

// responseWriterWrapper wraps http.ResponseWriter to capture metrics
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterWrapper) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += n
	return n, err
}