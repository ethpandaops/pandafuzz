package bot

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// CollectAndStoreRawAFLFiles collects and stores raw AFL++ output files
func (cc *CoverageCollector) CollectAndStoreRawAFLFiles(ctx context.Context, job *common.Job) error {
	if !job.EnableCoverage {
		cc.logger.WithField("job_id", job.ID).Debug("Coverage collection disabled for job")
		return nil
	}

	cc.logger.WithFields(logrus.Fields{
		"job_id": job.ID,
		"fuzzer": job.Fuzzer,
	}).Info("Collecting raw AFL++ coverage files")

	// Only process AFL++ jobs
	if job.Fuzzer != "afl++" && job.Fuzzer != "aflplusplus" && job.Fuzzer != "afl" {
		cc.logger.WithField("fuzzer", job.Fuzzer).Debug("Not an AFL++ job, skipping raw file collection")
		return nil
	}

	// AFL++ output directory structure: WorkDir/output/afl_output/default/
	outputDir := filepath.Join(job.WorkDir, "output", "afl_output")

	// Check for the default subdirectory first
	defaultDir := filepath.Join(outputDir, "default")
	if _, err := os.Stat(defaultDir); err == nil {
		outputDir = defaultDir
	}

	// Files to collect
	files := map[string]string{
		"fuzzer_stats": filepath.Join(outputDir, "fuzzer_stats"),
		"plot_data":    filepath.Join(outputDir, "plot_data"),
		"fuzz_bitmap":  filepath.Join(outputDir, "fuzz_bitmap"),
	}

	storedFiles := make(map[string]string)
	timestamp := time.Now().Unix()

	for fileType, filePath := range files {
		// Check if file exists
		if _, err := os.Stat(filePath); err != nil {
			cc.logger.WithFields(logrus.Fields{
				"job_id":    job.ID,
				"file_type": fileType,
				"file_path": filePath,
			}).Debug("Raw coverage file not found, skipping")
			continue
		}

		// Read file
		data, err := os.ReadFile(filePath)
		if err != nil {
			cc.logger.WithError(err).WithFields(logrus.Fields{
				"job_id":    job.ID,
				"file_type": fileType,
				"file_path": filePath,
			}).Warn("Failed to read raw coverage file")
			continue
		}

		// Store file with retry logic
		storagePath := fmt.Sprintf("coverage/%s/%s-%d.raw", job.ID, fileType, timestamp)

		// Retry storage operation up to 3 times with exponential backoff
		var storeErr error
	retryLoop:
		for attempt := 1; attempt <= 3; attempt++ {
			reader := bytes.NewReader(data)
			storeErr = cc.storage.Store(ctx, storagePath, reader, int64(len(data)))
			if storeErr == nil {
				break
			}

			cc.logger.WithError(storeErr).WithFields(logrus.Fields{
				"job_id":       job.ID,
				"file_type":    fileType,
				"storage_path": storagePath,
				"attempt":      attempt,
			}).Warn("Storage attempt failed, retrying...")

			if attempt < 3 {
				// Exponential backoff: 1s, 2s, 4s
				backoff := time.Duration(1<<uint(attempt-1)) * time.Second
				select {
				case <-ctx.Done():
					storeErr = ctx.Err()
					break retryLoop
				case <-time.After(backoff):
					continue
				}
			}
		}

		if storeErr != nil {
			cc.logger.WithError(storeErr).WithFields(logrus.Fields{
				"job_id":       job.ID,
				"file_type":    fileType,
				"storage_path": storagePath,
			}).Error("Failed to store raw coverage file after retries")
			continue
		}

		storedFiles[fileType] = storagePath

		cc.logger.WithFields(logrus.Fields{
			"job_id":       job.ID,
			"file_type":    fileType,
			"storage_path": storagePath,
			"size":         len(data),
		}).Info("Stored raw AFL++ coverage file")
	}

	// Report to master with all file paths
	if len(storedFiles) > 0 {
		if err := cc.reportRawCoverageFiles(ctx, job, storedFiles); err != nil {
			cc.logger.WithError(err).WithField("job_id", job.ID).Warn("Failed to report raw coverage files to master")
		}
	}

	return nil
}

// reportRawCoverageFiles reports raw coverage file paths to master
func (cc *CoverageCollector) reportRawCoverageFiles(ctx context.Context, job *common.Job, files map[string]string) error {
	cc.logger.WithFields(logrus.Fields{
		"job_id": job.ID,
		"files":  files,
	}).Debug("Reporting raw coverage files to master")

	// Get bot ID from API client if available
	botID := "bot-default"
	if cc.api != nil {
		if botClient, ok := cc.api.(*RetryClient); ok && botClient.botID != "" {
			botID = botClient.botID
		}
	}

	// Calculate total size first
	totalSize := int64(0)
	for _, path := range files {
		if path != "" {
			// Get file info from storage
			metadata, err := cc.storage.GetMetadata(ctx, path)
			if err == nil {
				totalSize += metadata.Size
			}
		}
	}

	// Build report with proper structure for master's CoverageReportRequest
	report := map[string]interface{}{
		"job_id":       job.ID,
		"bot_id":       botID,
		"report_id":    fmt.Sprintf("raw_coverage_%s_%d", job.ID, time.Now().Unix()),
		"format":       "raw",
		"collected_at": time.Now(),
		"size":         totalSize,
		// Raw coverage fields must be nested inside coverage_data
		"coverage_data": map[string]interface{}{
			"file_type":         "raw",
			"fuzzer_stats_path": files["fuzzer_stats"],
			"plot_data_path":    files["plot_data"],
			"fuzz_bitmap_path":  files["fuzz_bitmap"],
			"size":              totalSize,
		},
	}

	// Report to master with timeout
	reportCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- cc.api.ReportCoverageData(report)
	}()

	select {
	case <-reportCtx.Done():
		return reportCtx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("failed to report raw coverage files to master: %w", err)
		}
	}

	cc.logger.WithField("job_id", job.ID).Info("Raw coverage files successfully reported to master")
	return nil
}

// RetrieveRawFile retrieves a raw coverage file from storage
func (cc *CoverageCollector) RetrieveRawFile(ctx context.Context, storagePath string) (io.ReadCloser, int64, error) {
	reader, err := cc.storage.Retrieve(ctx, storagePath)
	if err != nil {
		return nil, 0, err
	}

	// Get metadata to retrieve size
	metadata, err := cc.storage.GetMetadata(ctx, storagePath)
	if err != nil {
		// If we can't get metadata, return the reader with size 0
		return reader, 0, nil
	}

	return reader, metadata.Size, nil
}
