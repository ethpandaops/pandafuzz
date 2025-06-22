package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the PandaFuzz system
type Metrics struct {
	// HTTP metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec
	
	// Bot metrics
	BotsTotal           *prometheus.GaugeVec
	BotHeartbeats       *prometheus.CounterVec
	BotHeartbeatLatency *prometheus.HistogramVec
	BotTimeouts         *prometheus.CounterVec
	
	// Job metrics
	JobsTotal          *prometheus.CounterVec
	JobsActive         *prometheus.GaugeVec
	JobDuration        *prometheus.HistogramVec
	JobQueueSize       prometheus.Gauge
	JobAssignmentTime  *prometheus.HistogramVec
	
	// Fuzzing metrics
	CrashesTotal       *prometheus.CounterVec
	UniqueCrashes      *prometheus.GaugeVec
	CoverageEdges      *prometheus.GaugeVec
	CoverageNewEdges   *prometheus.CounterVec
	CorpusSize         *prometheus.GaugeVec
	ExecRate           *prometheus.GaugeVec
	
	// System metrics
	DatabaseQueries     *prometheus.CounterVec
	DatabaseLatency     *prometheus.HistogramVec
	DatabaseErrors      *prometheus.CounterVec
	CircuitBreakerState *prometheus.GaugeVec
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics() *Metrics {
	return &Metrics{
		// HTTP metrics
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "endpoint", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pandafuzz_http_request_duration_seconds",
				Help:    "HTTP request latency",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "endpoint"},
		),
		HTTPResponseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pandafuzz_http_response_size_bytes",
				Help:    "HTTP response size in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 7),
			},
			[]string{"method", "endpoint"},
		),
		
		// Bot metrics
		BotsTotal: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pandafuzz_bots_total",
				Help: "Total number of bots by status",
			},
			[]string{"status"},
		),
		BotHeartbeats: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_bot_heartbeats_total",
				Help: "Total number of bot heartbeats received",
			},
			[]string{"bot_id", "status"},
		),
		BotHeartbeatLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pandafuzz_bot_heartbeat_latency_seconds",
				Help:    "Time since last bot heartbeat",
				Buckets: prometheus.ExponentialBuckets(1, 2, 10),
			},
			[]string{"bot_id"},
		),
		BotTimeouts: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_bot_timeouts_total",
				Help: "Total number of bot timeouts",
			},
			[]string{"bot_id"},
		),
		
		// Job metrics
		JobsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_jobs_total",
				Help: "Total number of jobs by status",
			},
			[]string{"status", "fuzzer"},
		),
		JobsActive: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pandafuzz_jobs_active",
				Help: "Number of currently active jobs",
			},
			[]string{"fuzzer"},
		),
		JobDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pandafuzz_job_duration_seconds",
				Help:    "Job execution duration",
				Buckets: prometheus.ExponentialBuckets(60, 2, 10),
			},
			[]string{"fuzzer", "status"},
		),
		JobQueueSize: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "pandafuzz_job_queue_size",
				Help: "Number of jobs waiting for assignment",
			},
		),
		JobAssignmentTime: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pandafuzz_job_assignment_time_seconds",
				Help:    "Time from job creation to assignment",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"fuzzer"},
		),
		
		// Fuzzing metrics
		CrashesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_crashes_total",
				Help: "Total number of crashes found",
			},
			[]string{"job_id", "type", "unique"},
		),
		UniqueCrashes: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pandafuzz_unique_crashes",
				Help: "Number of unique crashes per job",
			},
			[]string{"job_id"},
		),
		CoverageEdges: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pandafuzz_coverage_edges",
				Help: "Current coverage edge count",
			},
			[]string{"job_id"},
		),
		CoverageNewEdges: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_coverage_new_edges_total",
				Help: "Total new coverage edges discovered",
			},
			[]string{"job_id"},
		),
		CorpusSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pandafuzz_corpus_size_bytes",
				Help: "Current corpus size in bytes",
			},
			[]string{"job_id"},
		),
		ExecRate: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pandafuzz_exec_rate",
				Help: "Fuzzing executions per second",
			},
			[]string{"job_id", "bot_id"},
		),
		
		// System metrics
		DatabaseQueries: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_database_queries_total",
				Help: "Total number of database queries",
			},
			[]string{"operation", "table"},
		),
		DatabaseLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pandafuzz_database_latency_seconds",
				Help:    "Database query latency",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation", "table"},
		),
		DatabaseErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_database_errors_total",
				Help: "Total number of database errors",
			},
			[]string{"operation", "error_type"},
		),
		CircuitBreakerState: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pandafuzz_circuit_breaker_state",
				Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
			},
			[]string{"name"},
		),
	}
}