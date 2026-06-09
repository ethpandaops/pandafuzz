package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"path/filepath"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/aflplusplus"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/libfuzzer"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage/backend"
	"github.com/sirupsen/logrus"
)

// CoverageCollector orchestrates the entire coverage collection flow from generation to storage
type CoverageCollector struct {
	storage    backend.StorageBackend
	api        APIClient
	logger     logrus.FieldLogger
	aflExtract *aflplusplus.CoverageExtractor
	libExtract *libfuzzer.CoverageExtractor
}

// APIClient defines the interface for communicating with the master node
type APIClient interface {
	ReportCoverageData(coverageData map[string]interface{}) error
}

// CoverageReport represents the final coverage report structure
type CoverageReport struct {
	JobID       string                 `json:"job_id"`
	Format      string                 `json:"format"`
	Size        int64                  `json:"size"`
	StoragePath string                 `json:"storage_path"`
	CollectedAt time.Time              `json:"collected_at"`
	FuzzerType  string                 `json:"fuzzer_type"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// NewCoverageCollector creates a new coverage collector with the provided backends
func NewCoverageCollector(storage backend.StorageBackend, api APIClient, logger logrus.FieldLogger) *CoverageCollector {
	if logger == nil {
		logger = logrus.New().WithField("component", "coverage_collector")
	} else {
		logger = logger.WithField("component", "coverage_collector")
	}

	return &CoverageCollector{
		storage:    storage,
		api:        api,
		logger:     logger,
		aflExtract: aflplusplus.NewCoverageExtractor(logger),
		libExtract: libfuzzer.NewCoverageExtractor(logger),
	}
}

// CollectAndStore orchestrates the main coverage collection flow with retry logic
func (cc *CoverageCollector) CollectAndStore(ctx context.Context, job *common.Job, fuzzer types.Fuzzer) error {
	cc.logger.WithFields(logrus.Fields{
		"job_id":     job.ID,
		"fuzzer":     job.Fuzzer,
		"enable_cov": job.EnableCoverage,
		"format":     job.CoverageFormat,
	}).Info("Starting coverage collection")

	// Check if coverage is enabled for this job
	if !job.EnableCoverage {
		cc.logger.WithField("job_id", job.ID).Debug("Coverage collection disabled for job")
		return nil
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// AFL++ only uses raw coverage format
	if job.Fuzzer == "aflplusplus" || job.Fuzzer == "afl++" {
		cc.logger.WithField("job_id", job.ID).Info("AFL++ uses raw coverage format, collecting raw files")
		if err := cc.CollectAndStoreRawAFLFiles(ctx, job); err != nil {
			cc.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to collect raw AFL++ coverage files")
			return fmt.Errorf("failed to collect raw AFL++ coverage files: %w", err)
		}
		cc.logger.WithField("job_id", job.ID).Info("Raw AFL++ coverage collection completed successfully")
		return nil
	}

	// For other fuzzers, try multiple formats if none specified
	if job.CoverageFormat == "" {
		return cc.tryMultipleFormats(ctx, job, fuzzer)
	}

	// Generate coverage file for specified format (non-AFL++)
	data, format, err := cc.generateCoverageFile(ctx, job, fuzzer)
	if err != nil {
		cc.logger.WithError(err).WithFields(logrus.Fields{
			"job_id": job.ID,
			"format": job.CoverageFormat,
		}).Error("Failed to generate coverage file")
		return fmt.Errorf("failed to generate coverage file: %w", err)
	}

	// Store the coverage file with retry logic
	storagePath, err := cc.storeCoverageFile(ctx, job.ID, data, format)
	if err != nil {
		cc.logger.WithError(err).WithField("job_id", job.ID).Error("Failed to store coverage file")
		return fmt.Errorf("failed to store coverage file: %w", err)
	}

	// Report to master node
	if err := cc.reportCoverageToMaster(ctx, job, storagePath, int64(len(data))); err != nil {
		cc.logger.WithError(err).WithField("job_id", job.ID).Warn("Failed to report coverage to master")
		// Don't fail the whole operation if reporting fails
	}

	cc.logger.WithFields(logrus.Fields{
		"job_id":       job.ID,
		"storage_path": storagePath,
		"size":         len(data),
		"format":       format,
	}).Info("Coverage collection completed successfully")

	return nil
}

// generateCoverageFile calls the appropriate extractor based on fuzzer type
func (cc *CoverageCollector) generateCoverageFile(ctx context.Context, job *common.Job, fuzzer types.Fuzzer) ([]byte, string, error) {
	cc.logger.WithFields(logrus.Fields{
		"job_id":   job.ID,
		"fuzzer":   job.Fuzzer,
		"format":   job.CoverageFormat,
		"work_dir": job.WorkDir,
	}).Debug("Generating coverage file")

	var data []byte
	var format string
	var err error

	switch job.Fuzzer {
	case "aflplusplus", "afl++":
		// AFL++ should not reach here, it's handled earlier
		return nil, "", fmt.Errorf("AFL++ should use raw coverage collection")

	case "libfuzzer":
		format = job.CoverageFormat
		if format == "" {
			format = "json" // Default format for LibFuzzer
		}
		data, err = cc.generateLibFuzzerCoverage(ctx, job, format)

	case "honggfuzz":
		format = job.CoverageFormat
		if format == "" {
			format = "json" // Default format for Honggfuzz
		}
		data, err = cc.generateHonggfuzzCoverage(ctx, job, format)

	default:
		return nil, "", fmt.Errorf("unsupported fuzzer type: %s", job.Fuzzer)
	}

	if err != nil {
		return nil, "", fmt.Errorf("failed to generate %s coverage for %s: %w", format, job.Fuzzer, err)
	}

	if len(data) == 0 {
		return nil, "", fmt.Errorf("generated coverage data is empty")
	}

	return data, format, nil
}

// generateLibFuzzerCoverage generates coverage data for LibFuzzer
func (cc *CoverageCollector) generateLibFuzzerCoverage(ctx context.Context, job *common.Job, format string) ([]byte, error) {
	profdataPath := filepath.Join(job.WorkDir, "default.profdata")
	binaryPath := filepath.Join(job.WorkDir, filepath.Base(job.Target))

	// Extract profdata coverage using the LibFuzzer extractor
	coverageData, err := cc.libExtract.ExtractProfdataCoverage(profdataPath, binaryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract LibFuzzer profdata coverage: %w", err)
	}

	// Convert to requested format
	switch format {
	case "json":
		return json.Marshal(coverageData)
	case "lcov":
		lcovData, err := cc.libExtract.ConvertToLCOV(coverageData, binaryPath)
		if err != nil {
			cc.logger.WithError(err).Warn("Failed to convert to LCOV, returning JSON")
			return json.Marshal(coverageData)
		}
		return []byte(lcovData), nil
	default:
		return nil, fmt.Errorf("unsupported coverage format for LibFuzzer: %s", format)
	}
}

// generateHonggfuzzCoverage generates coverage data for Honggfuzz (basic implementation)
func (cc *CoverageCollector) generateHonggfuzzCoverage(ctx context.Context, job *common.Job, format string) ([]byte, error) {
	// Basic Honggfuzz coverage implementation
	// In a real implementation, this would use honggfuzz-specific coverage extraction
	cc.logger.WithField("job_id", job.ID).Debug("Generating basic Honggfuzz coverage report")

	basicReport := map[string]interface{}{
		"fuzzer_type":  "honggfuzz",
		"job_id":       job.ID,
		"collected_at": time.Now().Format(time.RFC3339),
		"work_dir":     job.WorkDir,
		"format":       format,
		"note":         "Basic coverage report - enhanced extraction not yet implemented",
	}

	return json.Marshal(basicReport)
}

// storeCoverageFile saves coverage data to storage with exponential backoff retry logic
func (cc *CoverageCollector) storeCoverageFile(ctx context.Context, jobID string, data []byte, format string) (string, error) {
	timestamp := time.Now().Unix()
	storagePath := fmt.Sprintf("coverage/%s/coverage-%d.%s", jobID, timestamp, format)

	cc.logger.WithFields(logrus.Fields{
		"job_id":       jobID,
		"storage_path": storagePath,
		"size":         len(data),
		"format":       format,
	}).Debug("Storing coverage file")

	// Retry logic with exponential backoff
	maxRetries := 3
	baseDelay := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Create reader from data
		reader := bytes.NewReader(data)

		// Attempt to store
		err := cc.storage.Store(ctx, storagePath, reader, int64(len(data)))
		if err == nil {
			cc.logger.WithFields(logrus.Fields{
				"job_id":       jobID,
				"storage_path": storagePath,
				"attempt":      attempt,
			}).Debug("Coverage file stored successfully")
			return storagePath, nil
		}

		cc.logger.WithError(err).WithFields(logrus.Fields{
			"job_id":       jobID,
			"storage_path": storagePath,
			"attempt":      attempt,
			"max_retries":  maxRetries,
		}).Warn("Failed to store coverage file, will retry")

		// Don't retry on last attempt
		if attempt == maxRetries {
			break
		}

		// Calculate delay with exponential backoff and jitter
		delay := time.Duration(attempt-1) * baseDelay * 2
		jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
		totalDelay := delay + jitter

		cc.logger.WithField("delay", totalDelay).Debug("Waiting before retry")

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(totalDelay):
			// Continue to next attempt
		}
	}

	return "", fmt.Errorf("failed to store coverage file after %d attempts", maxRetries)
}

// reportCoverageToMaster updates the master node with coverage file information
func (cc *CoverageCollector) reportCoverageToMaster(ctx context.Context, job *common.Job, storagePath string, size int64) error {
	cc.logger.WithFields(logrus.Fields{
		"job_id":       job.ID,
		"storage_path": storagePath,
		"size":         size,
	}).Debug("Reporting coverage to master")

	// Extract actual coverage metrics based on fuzzer type
	var coverageMetrics map[string]interface{}

	switch job.Fuzzer {
	case "afl++":
		// AFL++ output is in WorkDir/output/afl_output
		outputDir := filepath.Join(job.WorkDir, "output", "afl_output")
		coverageData, err := cc.aflExtract.ExtractBitmapCoverage(ctx, outputDir)
		if err != nil {
			cc.logger.WithError(err).Warn("Failed to extract AFL++ metrics for reporting")
			coverageMetrics = map[string]interface{}{
				"error":            "extraction_failed",
				"edges":            0,
				"coverage_percent": 0.0,
			}
		} else {
			coverageMetrics = coverageData.ToReportMap()
		}
	case "libfuzzer":
		// Extract LibFuzzer metrics
		profdataPath := filepath.Join(job.WorkDir, "default.profdata")
		binaryPath := filepath.Join(job.WorkDir, filepath.Base(job.Target))
		coverageData, err := cc.libExtract.ExtractProfdataCoverage(profdataPath, binaryPath)
		if err != nil {
			cc.logger.WithError(err).Warn("Failed to extract LibFuzzer metrics for reporting")
			coverageMetrics = map[string]interface{}{
				"error":            "extraction_failed",
				"edges":            0,
				"coverage_percent": 0.0,
			}
		} else {
			// Convert LibFuzzer coverage data to map format
			coverageMetrics = map[string]interface{}{
				"edges":            0,   // LibFuzzer doesn't directly provide edge count
				"coverage_percent": 0.0, // Default, will be extracted below
			}

			// Extract line coverage if available
			if coverageData.LineStats != nil {
				coverageMetrics["line_coverage"] = coverageData.LineStats.CoveragePercent
				coverageMetrics["total_lines"] = coverageData.LineStats.TotalLines
				coverageMetrics["covered_lines"] = coverageData.LineStats.CoveredLines
				if coverageData.LineStats.CoveragePercent > 0 {
					coverageMetrics["coverage_percent"] = coverageData.LineStats.CoveragePercent
				}
			}

			// Extract function coverage if available
			if coverageData.FunctionStats != nil {
				coverageMetrics["function_coverage"] = coverageData.FunctionStats.CoveragePercent
				coverageMetrics["total_functions"] = coverageData.FunctionStats.TotalFunctions
				coverageMetrics["covered_functions"] = coverageData.FunctionStats.CoveredFunctions
			}

			// Extract region/branch coverage if available
			if coverageData.RegionStats != nil {
				coverageMetrics["branch_coverage"] = coverageData.RegionStats.CoveragePercent
				coverageMetrics["total_regions"] = coverageData.RegionStats.TotalRegions
				coverageMetrics["covered_regions"] = coverageData.RegionStats.CoveredRegions
			}

			// Use basic stats if available and other stats are missing
			if coverageData.BasicStats != nil {
				if val, ok := coverageMetrics["coverage_percent"].(float64); !ok || val == 0 {
					if coverageData.BasicStats.CoverageScore > 0 {
						coverageMetrics["coverage_percent"] = coverageData.BasicStats.CoverageScore
					}
				}
				if coverageData.BasicStats.CoveredFeatures > 0 {
					coverageMetrics["edges"] = coverageData.BasicStats.CoveredFeatures
				}
			}
		}
	default:
		coverageMetrics = map[string]interface{}{
			"edges":            0,
			"coverage_percent": 0.0,
		}
	}

	// Get bot ID from API client if available
	botID := "bot-default"
	if cc.api != nil {
		// Try to get bot ID from the API client if it implements an interface with GetBotID
		if botClient, ok := cc.api.(*RetryClient); ok && botClient.botID != "" {
			botID = botClient.botID
		}
	}

	// Build complete report with actual coverage data
	report := map[string]interface{}{
		"job_id":            job.ID,
		"bot_id":            botID,
		"report_id":         fmt.Sprintf("coverage_%s_%d", job.ID, time.Now().Unix()),
		"format":            job.CoverageFormat,
		"coverage_data":     coverageMetrics, // CRITICAL: Include actual metrics
		"line_coverage":     0.0,             // Will be populated from coverage_data if available
		"function_coverage": 0.0,             // Will be populated from coverage_data if available
		"branch_coverage":   0.0,             // Will be populated from coverage_data if available
		"collected_at":      time.Now(),      // Send as time.Time, not string
		"storage_path":      storagePath,
		"size":              size,
	}

	// Extract line/function/branch coverage from coverageMetrics if available
	if val, ok := coverageMetrics["line_coverage"].(float64); ok {
		report["line_coverage"] = val
	}
	if val, ok := coverageMetrics["function_coverage"].(float64); ok {
		report["function_coverage"] = val
	}
	if val, ok := coverageMetrics["branch_coverage"].(float64); ok {
		report["branch_coverage"] = val
	}

	// Report to master with timeout
	reportCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Use a goroutine to respect context cancellation
	done := make(chan error, 1)
	go func() {
		done <- cc.api.ReportCoverageData(report)
	}()

	select {
	case <-reportCtx.Done():
		return reportCtx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("failed to report coverage to master: %w", err)
		}
	}

	cc.logger.WithField("job_id", job.ID).Info("Coverage successfully reported to master")
	return nil
}

// tryMultipleFormats attempts coverage collection in multiple formats and reports all successes/failures
func (cc *CoverageCollector) tryMultipleFormats(ctx context.Context, job *common.Job, fuzzer types.Fuzzer) error {
	cc.logger.WithField("job_id", job.ID).Debug("Trying multiple coverage formats")

	// For non-AFL++ fuzzers, try these formats
	formats := []string{"json", "lcov"}
	var lastErr error
	successCount := 0

	for _, format := range formats {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		cc.logger.WithFields(logrus.Fields{
			"job_id": job.ID,
			"format": format,
		}).Debug("Attempting coverage format")

		// Generate coverage data for this format
		data, actualFormat, err := cc.generateCoverageFile(ctx, job, fuzzer)
		if err != nil {
			cc.logger.WithError(err).WithFields(logrus.Fields{
				"job_id": job.ID,
				"format": format,
			}).Warn("Failed to generate coverage in format")
			lastErr = err
			continue
		}

		// Store the coverage file
		storagePath, err := cc.storeCoverageFile(ctx, job.ID, data, actualFormat)
		if err != nil {
			cc.logger.WithError(err).WithFields(logrus.Fields{
				"job_id": job.ID,
				"format": actualFormat,
			}).Warn("Failed to store coverage file")
			lastErr = err
			continue
		}

		// Report success to master
		if err := cc.reportCoverageToMaster(ctx, job, storagePath, int64(len(data))); err != nil {
			cc.logger.WithError(err).WithFields(logrus.Fields{
				"job_id": job.ID,
				"format": actualFormat,
			}).Warn("Failed to report coverage format to master")
		}

		cc.logger.WithFields(logrus.Fields{
			"job_id":       job.ID,
			"format":       actualFormat,
			"storage_path": storagePath,
			"size":         len(data),
		}).Info("Successfully collected coverage in format")

		successCount++
	}

	if successCount == 0 {
		return fmt.Errorf("failed to collect coverage in any format, last error: %w", lastErr)
	}

	cc.logger.WithFields(logrus.Fields{
		"job_id":        job.ID,
		"success_count": successCount,
		"total_formats": len(formats),
	}).Info("Coverage collection completed")

	return nil
}
