package bot

import (
	"context"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/monitoring"
	"github.com/sirupsen/logrus"
)

// CorpusSyncMonitor provides monitoring hooks for corpus synchronization
type CorpusSyncMonitor struct {
	metrics *monitoring.StorageMetrics
	logger  logrus.FieldLogger
}

// NewCorpusSyncMonitor creates a new corpus sync monitor
func NewCorpusSyncMonitor(metrics *monitoring.StorageMetrics, logger logrus.FieldLogger) *CorpusSyncMonitor {
	return &CorpusSyncMonitor{
		metrics: metrics,
		logger:  logger.WithField("component", "corpus_sync_monitor"),
	}
}

// RecordDownload records a corpus file download
func (csm *CorpusSyncMonitor) RecordDownload(ctx context.Context, campaignID, botID string, fileCount int, totalBytes int64, err error, duration time.Duration) {
	if csm.metrics != nil {
		csm.metrics.RecordCorpusSync("download", campaignID, botID, fileCount, totalBytes, err, duration)
	}

	logger := monitoring.WithRequestID(ctx, csm.logger)

	if err != nil {
		logger.WithError(err).WithFields(logrus.Fields{
			"campaign_id": campaignID,
			"bot_id":      botID,
			"duration":    duration.Seconds(),
		}).Error("Corpus download failed")
	} else {
		logger.WithFields(logrus.Fields{
			"campaign_id": campaignID,
			"bot_id":      botID,
			"file_count":  fileCount,
			"total_bytes": totalBytes,
			"duration":    duration.Seconds(),
			"rate_mbps":   float64(totalBytes) / duration.Seconds() / 1024 / 1024,
		}).Info("Corpus download completed")
	}
}

// RecordUpload records a corpus file upload
func (csm *CorpusSyncMonitor) RecordUpload(ctx context.Context, campaignID, botID string, fileCount int, totalBytes int64, err error, duration time.Duration) {
	if csm.metrics != nil {
		csm.metrics.RecordCorpusSync("upload", campaignID, botID, fileCount, totalBytes, err, duration)
	}

	logger := monitoring.WithRequestID(ctx, csm.logger)

	if err != nil {
		logger.WithError(err).WithFields(logrus.Fields{
			"campaign_id": campaignID,
			"bot_id":      botID,
			"duration":    duration.Seconds(),
		}).Error("Corpus upload failed")
	} else {
		logger.WithFields(logrus.Fields{
			"campaign_id": campaignID,
			"bot_id":      botID,
			"file_count":  fileCount,
			"total_bytes": totalBytes,
			"duration":    duration.Seconds(),
			"rate_mbps":   float64(totalBytes) / duration.Seconds() / 1024 / 1024,
		}).Info("Corpus upload completed")
	}
}

// RecordSyncProgress logs intermediate sync progress
func (csm *CorpusSyncMonitor) RecordSyncProgress(ctx context.Context, campaignID, botID string, current, total int, bytesTransferred int64) {
	logger := monitoring.WithRequestID(ctx, csm.logger)

	percentage := float64(current) / float64(total) * 100

	logger.WithFields(logrus.Fields{
		"campaign_id":       campaignID,
		"bot_id":            botID,
		"current":           current,
		"total":             total,
		"percentage":        percentage,
		"bytes_transferred": bytesTransferred,
	}).Debug("Corpus sync progress")

	// Log milestone progress
	if current%100 == 0 || current == total {
		logger.WithFields(logrus.Fields{
			"campaign_id":       campaignID,
			"bot_id":            botID,
			"progress":          percentage,
			"files_processed":   current,
			"files_total":       total,
			"bytes_transferred": bytesTransferred,
		}).Info("Corpus sync milestone reached")
	}
}

// RecordSyncError logs sync errors with context
func (csm *CorpusSyncMonitor) RecordSyncError(ctx context.Context, campaignID, botID, operation string, err error) {
	logger := monitoring.WithRequestID(ctx, csm.logger)

	logger.WithError(err).WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"bot_id":      botID,
		"operation":   operation,
	}).Error("Corpus sync error")
}

// StartSyncOperation marks the beginning of a sync operation
func (csm *CorpusSyncMonitor) StartSyncOperation(ctx context.Context, campaignID, botID, operation string) func() {
	logger := monitoring.WithRequestID(ctx, csm.logger)

	startTime := time.Now()

	logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"bot_id":      botID,
		"operation":   operation,
	}).Info("Starting corpus sync operation")

	// Return cleanup function
	return func() {
		duration := time.Since(startTime)
		logger.WithFields(logrus.Fields{
			"campaign_id": campaignID,
			"bot_id":      botID,
			"operation":   operation,
			"duration":    duration.Seconds(),
		}).Info("Completed corpus sync operation")
	}
}
