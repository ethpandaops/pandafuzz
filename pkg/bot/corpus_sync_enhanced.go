package bot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

const (
	// Download configuration
	maxConcurrentDownloads = 4
	downloadTimeout        = 5 * time.Minute
	retryAttempts          = 3
	corpusSyncRetryDelay   = 2 * time.Second

	// File limits
	maxCorpusFileSize = 100 << 20 // 100MB
	bufferSize        = 32 * 1024 // 32KB

	// Progress reporting
	progressReportInterval = 10 * time.Second
)

// EnhancedS3CorpusSyncer provides robust corpus synchronization with S3
type EnhancedS3CorpusSyncer struct {
	client       *RetryClient
	localStorage string
	logger       logrus.FieldLogger

	// Progress tracking
	downloadStats struct {
		totalFiles     int64
		completedFiles int64
		failedFiles    int64
		totalBytes     int64
		completedBytes int64
	}

	// Rate limiting
	rateLimiter chan struct{}
}

// NewEnhancedS3CorpusSyncer creates a new enhanced S3 corpus syncer
func NewEnhancedS3CorpusSyncer(client *RetryClient, localStorage string, logger logrus.FieldLogger) *EnhancedS3CorpusSyncer {
	return &EnhancedS3CorpusSyncer{
		client:       client,
		localStorage: localStorage,
		logger:       logger.WithField("component", "enhanced_s3_corpus_sync"),
		rateLimiter:  make(chan struct{}, maxConcurrentDownloads),
	}
}

// InitializeJobCorpus downloads campaign corpus files with enhanced error handling
func (s *EnhancedS3CorpusSyncer) InitializeJobCorpus(ctx context.Context, job *common.Job, targetDir string) error {
	// Check if job is configured to use campaign corpus
	if !job.UseCampaignCorpus || job.CampaignID == nil || *job.CampaignID == "" {
		s.logger.WithField("job_id", job.ID).Debug("Job not configured to use campaign corpus")
		return nil
	}

	campaignID := *job.CampaignID
	s.logger.WithFields(logrus.Fields{
		"job_id":      job.ID,
		"campaign_id": campaignID,
		"target_dir":  targetDir,
	}).Info("Initializing job with campaign corpus using enhanced S3 sync")

	// Validate and create target directory
	if err := s.validateAndCreateDir(targetDir); err != nil {
		return fmt.Errorf("failed to prepare target directory: %w", err)
	}

	// Get corpus files list with retry
	files, err := s.getCorpusFilesWithRetry(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("failed to get corpus files: %w", err)
	}

	if len(files) == 0 {
		s.logger.Info("Campaign has no corpus files")
		return nil
	}

	// Reset statistics
	atomic.StoreInt64(&s.downloadStats.totalFiles, int64(len(files)))
	atomic.StoreInt64(&s.downloadStats.completedFiles, 0)
	atomic.StoreInt64(&s.downloadStats.failedFiles, 0)
	atomic.StoreInt64(&s.downloadStats.totalBytes, 0)
	atomic.StoreInt64(&s.downloadStats.completedBytes, 0)

	// Calculate total size
	for _, file := range files {
		atomic.AddInt64(&s.downloadStats.totalBytes, file.Size)
	}

	// Start progress reporter
	progressCtx, cancelProgress := context.WithCancel(ctx)
	defer cancelProgress()
	go s.reportProgress(progressCtx, campaignID, job.ID)

	// Download files with parallelism control
	err = s.downloadFilesParallel(ctx, campaignID, files, targetDir)

	// Final statistics
	completed := atomic.LoadInt64(&s.downloadStats.completedFiles)
	failed := atomic.LoadInt64(&s.downloadStats.failedFiles)

	s.logger.WithFields(logrus.Fields{
		"campaign_id":  campaignID,
		"job_id":       job.ID,
		"total_files":  len(files),
		"downloaded":   completed,
		"failed":       failed,
		"success_rate": fmt.Sprintf("%.1f%%", float64(completed)/float64(len(files))*100),
	}).Info("Campaign corpus initialization completed")

	if failed > 0 && completed == 0 {
		return fmt.Errorf("failed to download any corpus files")
	}

	return nil
}

// validateAndCreateDir validates and creates the target directory
func (s *EnhancedS3CorpusSyncer) validateAndCreateDir(dir string) error {
	// Clean the path
	cleanPath := filepath.Clean(dir)

	// Check for path traversal attempts
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid directory path: contains '..'")
	}

	// Create directory with proper permissions
	if err := os.MkdirAll(cleanPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Verify we can write to the directory
	testFile := filepath.Join(cleanPath, ".test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("directory not writable: %w", err)
	}
	os.Remove(testFile)

	return nil
}

// getCorpusFilesWithRetry gets corpus files with retry logic
func (s *EnhancedS3CorpusSyncer) getCorpusFilesWithRetry(ctx context.Context, campaignID string) ([]*common.CorpusFile, error) {
	var files []*common.CorpusFile
	var lastErr error

	for attempt := 1; attempt <= retryAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		files, lastErr = s.client.GetCorpusFiles(attemptCtx, campaignID)
		cancel()

		if lastErr == nil {
			return files, nil
		}

		// Check if context was cancelled
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		s.logger.WithError(lastErr).WithField("attempt", attempt).Warn("Failed to get corpus files, retrying")

		if attempt < retryAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(corpusSyncRetryDelay * time.Duration(attempt)):
				// Exponential backoff
			}
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", retryAttempts, lastErr)
}

// downloadFilesParallel downloads files with controlled parallelism
func (s *EnhancedS3CorpusSyncer) downloadFilesParallel(ctx context.Context, campaignID string, files []*common.CorpusFile, targetDir string) error {
	var wg sync.WaitGroup
	errorChan := make(chan error, len(files))

	// Create a context that can be cancelled if too many errors occur
	downloadCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, file := range files {
		// Check if context is cancelled
		if downloadCtx.Err() != nil {
			break
		}

		// Acquire rate limit token
		select {
		case s.rateLimiter <- struct{}{}:
			wg.Add(1)
			go func(f *common.CorpusFile) {
				defer wg.Done()
				defer func() { <-s.rateLimiter }()

				if err := s.downloadSingleFile(downloadCtx, campaignID, f, targetDir); err != nil {
					atomic.AddInt64(&s.downloadStats.failedFiles, 1)
					errorChan <- fmt.Errorf("file %s: %w", f.Hash, err)

					// Cancel all downloads if too many failures
					if atomic.LoadInt64(&s.downloadStats.failedFiles) > int64(len(files)/10) {
						s.logger.Error("Too many download failures, cancelling remaining downloads")
						cancel()
					}
				} else {
					atomic.AddInt64(&s.downloadStats.completedFiles, 1)
					atomic.AddInt64(&s.downloadStats.completedBytes, f.Size)
				}
			}(file)
		case <-downloadCtx.Done():
			break
		}
	}

	// Wait for all downloads to complete
	wg.Wait()
	close(errorChan)

	// Collect errors
	var errors []error
	for err := range errorChan {
		errors = append(errors, err)
		if len(errors) >= 10 {
			// Log only first 10 errors to avoid spam
			break
		}
	}

	if len(errors) > 0 {
		for _, err := range errors {
			s.logger.Error(err)
		}
	}

	return nil
}

// downloadSingleFile downloads a single corpus file with retry logic
func (s *EnhancedS3CorpusSyncer) downloadSingleFile(ctx context.Context, campaignID string, file *common.CorpusFile, targetDir string) error {
	targetPath := filepath.Join(targetDir, file.Filename)

	// Check if file already exists and has correct hash
	if s.fileExistsWithHash(targetPath, file.Hash) {
		s.logger.WithField("filename", file.Filename).Debug("File already exists with correct hash, skipping")
		return nil
	}

	var lastErr error
	for attempt := 1; attempt <= retryAttempts; attempt++ {
		// Get presigned URL
		downloadURL, err := s.getDownloadURLWithTimeout(ctx, campaignID, file.Hash)
		if err != nil {
			lastErr = err
			continue
		}

		// Download file
		err = s.downloadFromURL(ctx, downloadURL, targetPath, file.Hash, file.Size)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if context was cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}

		s.logger.WithError(err).WithFields(logrus.Fields{
			"filename": file.Filename,
			"attempt":  attempt,
		}).Warn("Download failed, retrying")

		if attempt < retryAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(corpusSyncRetryDelay * time.Duration(attempt)):
				// Exponential backoff
			}
		}
	}

	return fmt.Errorf("download failed after %d attempts: %w", retryAttempts, lastErr)
}

// fileExistsWithHash checks if a file exists and has the expected hash
func (s *EnhancedS3CorpusSyncer) fileExistsWithHash(path, expectedHash string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	// Quick size check
	if info.Size() == 0 {
		return false
	}

	// Calculate actual hash
	actualHash, err := s.calculateFileHash(path)
	if err != nil {
		s.logger.WithError(err).WithField("path", path).Warn("Failed to calculate file hash")
		return false
	}

	return actualHash == expectedHash
}

// calculateFileHash calculates SHA256 hash of a file
func (s *EnhancedS3CorpusSyncer) calculateFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	buffer := make([]byte, bufferSize)

	if _, err := io.CopyBuffer(hasher, file, buffer); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// getDownloadURLWithTimeout gets a presigned download URL with timeout
func (s *EnhancedS3CorpusSyncer) getDownloadURLWithTimeout(ctx context.Context, campaignID, fileHash string) (string, error) {
	urlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return s.client.GetCorpusDownloadURL(urlCtx, campaignID, fileHash)
}

// downloadFromURL downloads a file from a presigned URL with validation
func (s *EnhancedS3CorpusSyncer) downloadFromURL(ctx context.Context, url, targetPath, expectedHash string, expectedSize int64) error {
	// Create download context with timeout
	downloadCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	// Create HTTP request
	req, err := http.NewRequestWithContext(downloadCtx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Check content length if provided
	if resp.ContentLength > 0 {
		if resp.ContentLength != expectedSize {
			return fmt.Errorf("size mismatch: expected %d, got %d", expectedSize, resp.ContentLength)
		}
		if resp.ContentLength > maxCorpusFileSize {
			return fmt.Errorf("file too large: %d bytes", resp.ContentLength)
		}
	}

	// Create temporary file
	tempPath := targetPath + ".tmp." + fmt.Sprintf("%d", time.Now().UnixNano())
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempPath)
	defer tempFile.Close()

	// Download with hash calculation
	hasher := sha256.New()
	writer := io.MultiWriter(tempFile, hasher)

	// Limit the reader to prevent abuse
	limitedReader := io.LimitReader(resp.Body, maxCorpusFileSize+1)

	// Copy with progress tracking
	buffer := make([]byte, bufferSize)
	written, err := io.CopyBuffer(writer, limitedReader, buffer)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	// Check size
	if written > maxCorpusFileSize {
		return fmt.Errorf("file too large: %d bytes", written)
	}

	if expectedSize > 0 && written != expectedSize {
		return fmt.Errorf("size mismatch after download: expected %d, got %d", expectedSize, written)
	}

	// Verify hash
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	// Close temp file before rename
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, targetPath); err != nil {
		return fmt.Errorf("failed to move file: %w", err)
	}

	return nil
}

// reportProgress periodically reports download progress
func (s *EnhancedS3CorpusSyncer) reportProgress(ctx context.Context, campaignID, jobID string) {
	ticker := time.NewTicker(progressReportInterval)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			total := atomic.LoadInt64(&s.downloadStats.totalFiles)
			completed := atomic.LoadInt64(&s.downloadStats.completedFiles)
			failed := atomic.LoadInt64(&s.downloadStats.failedFiles)
			completedBytes := atomic.LoadInt64(&s.downloadStats.completedBytes)

			if total == 0 {
				continue
			}

			elapsed := time.Since(startTime)
			rate := float64(completed) / elapsed.Seconds()
			bytesRate := float64(completedBytes) / elapsed.Seconds()

			remaining := total - completed - failed
			eta := time.Duration(0)
			if rate > 0 && remaining > 0 {
				eta = time.Duration(float64(remaining) / rate * float64(time.Second))
			}

			progress := float64(completed+failed) / float64(total) * 100

			s.logger.WithFields(logrus.Fields{
				"campaign_id": campaignID,
				"job_id":      jobID,
				"progress":    fmt.Sprintf("%.1f%%", progress),
				"completed":   fmt.Sprintf("%d/%d", completed, total),
				"failed":      failed,
				"rate":        fmt.Sprintf("%.1f files/sec", rate),
				"bandwidth":   fmt.Sprintf("%.1f MB/sec", bytesRate/1024/1024),
				"eta":         eta.Round(time.Second).String(),
			}).Info("Corpus download progress")
		}
	}
}

// uploadNewCorpusFileEnhanced uploads a new corpus file with enhanced error handling
func (s *EnhancedS3CorpusSyncer) uploadNewCorpusFileEnhanced(ctx context.Context, campaignID string, filePath string) error {
	// Validate file
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	if info.Size() > maxCorpusFileSize {
		return fmt.Errorf("file too large: %d bytes (max: %d)", info.Size(), maxCorpusFileSize)
	}

	// Read file content
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Calculate hash
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// Prepare upload request
	uploadReq := UploadURLRequest{
		Filename: filepath.Base(filePath),
		Size:     info.Size(),
		Hash:     hashStr,
	}

	var lastErr error
	for attempt := 1; attempt <= retryAttempts; attempt++ {
		// Retry label moved here to avoid jumping over declarations
		if attempt > 1 {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			s.logger.WithError(lastErr).WithField("attempt", attempt).Warn("Upload failed, retrying")

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(corpusSyncRetryDelay * time.Duration(attempt-1)):
				// Exponential backoff
			}
		}

		// Get presigned upload URL
		urlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		uploadInfo, err := s.client.GetCorpusUploadURL(urlCtx, campaignID, uploadReq)
		cancel()

		if err != nil {
			lastErr = err
			continue
		}

		if uploadInfo.Status == "exists" {
			s.logger.WithField("hash", hashStr).Debug("File already exists in corpus")
			return nil
		}

		// Upload to S3
		uploadCtx, uploadCancel := context.WithTimeout(ctx, downloadTimeout)
		err = s.uploadToPresignedURL(uploadCtx, uploadInfo.URL, data, uploadInfo.Headers)
		uploadCancel()

		if err != nil {
			lastErr = err
			continue
		}

		// Register with master
		regCtx, regCancel := context.WithTimeout(ctx, 30*time.Second)
		err = s.client.RegisterCorpusFile(regCtx, campaignID, hashStr, filepath.Base(filePath))
		regCancel()

		if err != nil {
			lastErr = err
			continue
		}

		return nil
	}

	return fmt.Errorf("upload failed after %d attempts: %w", retryAttempts, lastErr)
}

// uploadToPresignedURL uploads data to a presigned URL
func (s *EnhancedS3CorpusSyncer) uploadToPresignedURL(ctx context.Context, url string, data []byte, headers map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Add content length if not set
	if req.Header.Get("Content-Length") == "" {
		req.ContentLength = int64(len(data))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
