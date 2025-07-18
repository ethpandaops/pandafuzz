package monitoring

import (
	"context"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

// StorageMetrics tracks metrics for storage operations
type StorageMetrics struct {
	// Operation counters
	operationTotal  *prometheus.CounterVec
	operationErrors *prometheus.CounterVec

	// Operation duration histograms
	operationDuration *prometheus.HistogramVec

	// File size histograms
	fileSize *prometheus.HistogramVec

	// Presigned URL generation
	presignedURLTotal    *prometheus.CounterVec
	presignedURLDuration *prometheus.HistogramVec

	// Storage backend health
	healthCheckTotal    *prometheus.CounterVec
	healthCheckDuration *prometheus.HistogramVec

	// Corpus sync metrics
	corpusSyncFiles    *prometheus.CounterVec
	corpusSyncBytes    *prometheus.CounterVec
	corpusSyncDuration *prometheus.HistogramVec

	// Active operations gauge
	activeOperations *prometheus.GaugeVec

	logger logrus.FieldLogger
}

// NewStorageMetrics creates a new storage metrics collector
func NewStorageMetrics(logger logrus.FieldLogger) *StorageMetrics {
	return &StorageMetrics{
		operationTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_storage_operations_total",
				Help: "Total number of storage operations",
			},
			[]string{"backend", "operation", "bucket"},
		),
		operationErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_storage_errors_total",
				Help: "Total number of storage operation errors",
			},
			[]string{"backend", "operation", "bucket", "error_type"},
		),
		operationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pandafuzz_storage_operation_duration_seconds",
				Help:    "Duration of storage operations in seconds",
				Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10},
			},
			[]string{"backend", "operation", "bucket"},
		),
		fileSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pandafuzz_storage_file_size_bytes",
				Help:    "Size of files in storage operations",
				Buckets: prometheus.ExponentialBuckets(1024, 2, 20), // 1KB to 1GB
			},
			[]string{"backend", "operation", "bucket"},
		),
		presignedURLTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_storage_presigned_urls_total",
				Help: "Total number of presigned URLs generated",
			},
			[]string{"backend", "type", "bucket"},
		),
		presignedURLDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pandafuzz_storage_presigned_url_duration_seconds",
				Help:    "Duration of presigned URL generation",
				Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
			},
			[]string{"backend", "type", "bucket"},
		),
		healthCheckTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_storage_health_checks_total",
				Help: "Total number of storage health checks",
			},
			[]string{"backend", "status"},
		),
		healthCheckDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pandafuzz_storage_health_check_duration_seconds",
				Help:    "Duration of storage health checks",
				Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
			},
			[]string{"backend"},
		),
		corpusSyncFiles: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_corpus_sync_files_total",
				Help: "Total number of corpus files synced",
			},
			[]string{"direction", "campaign_id", "bot_id"},
		),
		corpusSyncBytes: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pandafuzz_corpus_sync_bytes_total",
				Help: "Total bytes synced for corpus files",
			},
			[]string{"direction", "campaign_id", "bot_id"},
		),
		corpusSyncDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pandafuzz_corpus_sync_duration_seconds",
				Help:    "Duration of corpus sync operations",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"direction", "campaign_id", "bot_id"},
		),
		activeOperations: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pandafuzz_storage_active_operations",
				Help: "Number of active storage operations",
			},
			[]string{"backend", "operation"},
		),
		logger: logger.WithField("component", "storage_metrics"),
	}
}

// RecordOperation records a storage operation with timing and error tracking
func (m *StorageMetrics) RecordOperation(backend, operation, bucket string, size int64, err error, duration time.Duration) {
	labels := prometheus.Labels{
		"backend":   backend,
		"operation": operation,
		"bucket":    bucket,
	}

	// Record operation count
	m.operationTotal.With(labels).Inc()

	// Record duration
	m.operationDuration.With(labels).Observe(duration.Seconds())

	// Record file size if applicable
	if size > 0 && (operation == "store" || operation == "retrieve") {
		m.fileSize.With(labels).Observe(float64(size))
	}

	// Record errors
	if err != nil {
		errorType := classifyError(err)
		m.operationErrors.With(prometheus.Labels{
			"backend":    backend,
			"operation":  operation,
			"bucket":     bucket,
			"error_type": errorType,
		}).Inc()

		m.logger.WithFields(logrus.Fields{
			"backend":    backend,
			"operation":  operation,
			"bucket":     bucket,
			"error":      err.Error(),
			"error_type": errorType,
			"duration":   duration.Seconds(),
			"size":       size,
		}).Error("Storage operation failed")
	} else {
		m.logger.WithFields(logrus.Fields{
			"backend":   backend,
			"operation": operation,
			"bucket":    bucket,
			"duration":  duration.Seconds(),
			"size":      size,
		}).Debug("Storage operation completed")
	}
}

// RecordPresignedURL records presigned URL generation metrics
func (m *StorageMetrics) RecordPresignedURL(backend, urlType, bucket string, err error, duration time.Duration) {
	labels := prometheus.Labels{
		"backend": backend,
		"type":    urlType,
		"bucket":  bucket,
	}

	m.presignedURLTotal.With(labels).Inc()
	m.presignedURLDuration.With(labels).Observe(duration.Seconds())

	if err != nil {
		m.logger.WithFields(logrus.Fields{
			"backend":  backend,
			"type":     urlType,
			"bucket":   bucket,
			"error":    err.Error(),
			"duration": duration.Seconds(),
		}).Error("Presigned URL generation failed")
	} else {
		m.logger.WithFields(logrus.Fields{
			"backend":  backend,
			"type":     urlType,
			"bucket":   bucket,
			"duration": duration.Seconds(),
		}).Debug("Presigned URL generated")
	}
}

// RecordHealthCheck records storage health check metrics
func (m *StorageMetrics) RecordHealthCheck(backend string, err error, duration time.Duration) {
	status := "success"
	if err != nil {
		status = "failure"
	}

	m.healthCheckTotal.With(prometheus.Labels{
		"backend": backend,
		"status":  status,
	}).Inc()

	m.healthCheckDuration.With(prometheus.Labels{
		"backend": backend,
	}).Observe(duration.Seconds())

	if err != nil {
		m.logger.WithFields(logrus.Fields{
			"backend":  backend,
			"error":    err.Error(),
			"duration": duration.Seconds(),
		}).Error("Storage health check failed")
	} else {
		m.logger.WithFields(logrus.Fields{
			"backend":  backend,
			"duration": duration.Seconds(),
		}).Debug("Storage health check succeeded")
	}
}

// RecordCorpusSync records corpus synchronization metrics
func (m *StorageMetrics) RecordCorpusSync(direction, campaignID, botID string, fileCount int, totalBytes int64, err error, duration time.Duration) {
	labels := prometheus.Labels{
		"direction":   direction, // "upload" or "download"
		"campaign_id": campaignID,
		"bot_id":      botID,
	}

	if err == nil {
		m.corpusSyncFiles.With(labels).Add(float64(fileCount))
		m.corpusSyncBytes.With(labels).Add(float64(totalBytes))
	}

	m.corpusSyncDuration.With(labels).Observe(duration.Seconds())

	if err != nil {
		m.logger.WithFields(logrus.Fields{
			"direction":   direction,
			"campaign_id": campaignID,
			"bot_id":      botID,
			"error":       err.Error(),
			"duration":    duration.Seconds(),
		}).Error("Corpus sync failed")
	} else {
		m.logger.WithFields(logrus.Fields{
			"direction":   direction,
			"campaign_id": campaignID,
			"bot_id":      botID,
			"file_count":  fileCount,
			"total_bytes": totalBytes,
			"duration":    duration.Seconds(),
		}).Info("Corpus sync completed")
	}
}

// TrackActiveOperation tracks an active operation
func (m *StorageMetrics) TrackActiveOperation(backend, operation string) func() {
	labels := prometheus.Labels{
		"backend":   backend,
		"operation": operation,
	}

	m.activeOperations.With(labels).Inc()

	return func() {
		m.activeOperations.With(labels).Dec()
	}
}

// classifyError categorizes errors for better metrics
func classifyError(err error) string {
	errStr := err.Error()
	switch {
	case containsAny(errStr, "not found", "no such", "404"):
		return "not_found"
	case containsAny(errStr, "permission", "forbidden", "403"):
		return "permission_denied"
	case containsAny(errStr, "timeout", "deadline"):
		return "timeout"
	case containsAny(errStr, "connection", "network"):
		return "network"
	case containsAny(errStr, "space", "quota"):
		return "quota_exceeded"
	case containsAny(errStr, "invalid", "validation"):
		return "validation"
	default:
		return "unknown"
	}
}

// containsAny checks if the string contains any of the substrings
func containsAny(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if strings.Contains(strings.ToLower(s), substr) {
			return true
		}
	}
	return false
}

// WithRequestID adds request ID to logger context
func WithRequestID(ctx context.Context, logger logrus.FieldLogger) logrus.FieldLogger {
	if reqID := ctx.Value("request_id"); reqID != nil {
		return logger.WithField("request_id", reqID)
	}
	return logger
}
