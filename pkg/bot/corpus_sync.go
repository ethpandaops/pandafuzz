package bot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// CorpusSyncer defines the interface for corpus synchronization
type CorpusSyncer interface {
	// InitializeJobCorpus downloads campaign corpus files for a job
	InitializeJobCorpus(ctx context.Context, job *common.Job, targetDir string) error
}

// CorpusSyncClient handles corpus synchronization with the master
type CorpusSyncClient struct {
	client     *RetryClient
	campaignID string
	botID      string
	syncDir    string
	logger     logrus.FieldLogger

	// Control
	stopCh chan struct{}
	doneCh chan struct{}
	mu     sync.RWMutex

	// Sync state
	lastSync     time.Time
	syncedFiles  map[string]bool // Hash -> synced
	pendingFiles []*common.CorpusFile
}

// NewCorpusSyncClient creates a new corpus sync client
func NewCorpusSyncClient(client *RetryClient, campaignID, botID, syncDir string, logger logrus.FieldLogger) *CorpusSyncClient {
	return &CorpusSyncClient{
		client:      client,
		campaignID:  campaignID,
		botID:       botID,
		syncDir:     syncDir,
		logger:      logger.WithField("component", "corpus_sync"),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
		syncedFiles: make(map[string]bool),
	}
}

// Start starts the corpus sync client
func (csc *CorpusSyncClient) Start(ctx context.Context) error {
	csc.logger.WithFields(logrus.Fields{
		"campaign_id": csc.campaignID,
		"bot_id":      csc.botID,
		"sync_dir":    csc.syncDir,
	}).Info("Starting corpus sync client")

	// Create sync directory
	if err := os.MkdirAll(csc.syncDir, 0755); err != nil {
		return fmt.Errorf("failed to create sync directory: %w", err)
	}

	// Start sync loop
	go csc.syncLoop(ctx)

	return nil
}

// Stop stops the corpus sync client
func (csc *CorpusSyncClient) Stop() error {
	csc.logger.Info("Stopping corpus sync client")

	close(csc.stopCh)

	select {
	case <-csc.doneCh:
		csc.logger.Info("Corpus sync client stopped")
	case <-time.After(5 * time.Second):
		csc.logger.Warn("Corpus sync client stop timeout")
	}

	return nil
}

// syncLoop performs periodic corpus synchronization
func (csc *CorpusSyncClient) syncLoop(ctx context.Context) {
	defer close(csc.doneCh)

	// Initial sync
	if err := csc.downloadNewFiles(ctx); err != nil {
		csc.logger.WithError(err).Error("Initial corpus sync failed")
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-csc.stopCh:
			return
		case <-ticker.C:
			if err := csc.downloadNewFiles(ctx); err != nil {
				csc.logger.WithError(err).Error("Corpus sync failed")
			}
		}
	}
}

// downloadNewFiles downloads new corpus files from master
func (csc *CorpusSyncClient) downloadNewFiles(ctx context.Context) error {
	startTime := time.Now()

	csc.logger.WithFields(logrus.Fields{
		"campaign_id": csc.campaignID,
		"bot_id":      csc.botID,
	}).Debug("Starting corpus sync check")

	// Request sync from master
	syncReq := map[string]string{
		"bot_id": csc.botID,
	}

	reqBody, err := json.Marshal(syncReq)
	if err != nil {
		csc.logger.WithError(err).Error("Failed to marshal sync request")
		return fmt.Errorf("failed to marshal sync request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/campaigns/%s/corpus/sync", csc.client.masterURL, csc.campaignID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(reqBody)))
	if err != nil {
		csc.logger.WithError(err).Error("Failed to create sync request")
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	csc.client.applyAuthHeaders(req)

	resp, err := csc.client.httpClient.Do(req)
	if err != nil {
		csc.logger.WithError(err).Error("Failed to sync corpus with master")
		return fmt.Errorf("failed to sync corpus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("sync failed with status %d: %s", resp.StatusCode, body)
		csc.logger.WithError(err).WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"response":    string(body),
		}).Error("Corpus sync request failed")
		return err
	}

	// Parse response
	var syncResp struct {
		Files     []*common.CorpusFile `json:"files"`
		FileCount int                  `json:"file_count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		csc.logger.WithError(err).Error("Failed to decode sync response")
		return fmt.Errorf("failed to decode sync response: %w", err)
	}

	if len(syncResp.Files) == 0 {
		csc.logger.Debug("No new corpus files to sync")
		csc.lastSync = time.Now()
		return nil
	}

	csc.logger.WithField("file_count", len(syncResp.Files)).Info("Downloading corpus files")

	// Download each file
	downloaded := 0
	totalBytes := int64(0)
	errors := 0

	for _, file := range syncResp.Files {
		if err := csc.downloadCorpusFile(ctx, file); err != nil {
			csc.logger.WithError(err).WithFields(logrus.Fields{
				"file_hash": file.Hash,
				"file_name": file.Filename,
			}).Error("Failed to download corpus file")
			errors++
			continue
		}
		downloaded++
		totalBytes += file.Size

		// Mark as synced
		csc.mu.Lock()
		csc.syncedFiles[file.Hash] = true
		csc.mu.Unlock()
	}

	duration := time.Since(startTime)
	csc.logger.WithFields(logrus.Fields{
		"requested":   len(syncResp.Files),
		"downloaded":  downloaded,
		"errors":      errors,
		"total_bytes": totalBytes,
		"duration":    duration.Seconds(),
		"rate":        fmt.Sprintf("%.1f files/sec", float64(downloaded)/duration.Seconds()),
	}).Info("Corpus sync completed")

	csc.lastSync = time.Now()
	return nil
}

// downloadCorpusFile downloads a single corpus file
func (csc *CorpusSyncClient) downloadCorpusFile(ctx context.Context, file *common.CorpusFile) error {
	// Check if already downloaded
	csc.mu.RLock()
	alreadySynced := csc.syncedFiles[file.Hash]
	csc.mu.RUnlock()

	if alreadySynced {
		return nil
	}

	// Download file
	url := fmt.Sprintf("%s/api/v1/campaigns/%s/corpus/files/%s",
		csc.client.masterURL, csc.campaignID, file.Hash)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	csc.client.applyAuthHeaders(req)

	resp, err := csc.client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Read file content
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read file content: %w", err)
	}

	// Save to disk
	filePath := filepath.Join(csc.syncDir, file.Filename)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	csc.logger.WithFields(logrus.Fields{
		"filename": file.Filename,
		"hash":     file.Hash,
		"size":     file.Size,
	}).Debug("Downloaded corpus file")

	return nil
}

// reportNewCoverage reports a new coverage-increasing corpus file
func (csc *CorpusSyncClient) reportNewCoverage(ctx context.Context, filename string, coverage int64) error {
	// Read file content
	filePath := filepath.Join(csc.syncDir, filename)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Calculate hash
	hash := calculateFileHash(content)

	// Check if already reported
	csc.mu.RLock()
	alreadySynced := csc.syncedFiles[hash]
	csc.mu.RUnlock()

	if alreadySynced {
		return nil // Already known to master
	}

	// Create corpus file entry
	corpusFile := &common.CorpusFile{
		CampaignID:  csc.campaignID,
		BotID:       csc.botID,
		Filename:    filename,
		Hash:        hash,
		Size:        int64(len(content)),
		Coverage:    coverage,
		NewCoverage: coverage, // Bot reports all coverage as new
		CreatedAt:   time.Now(),
	}

	// Queue for upload
	csc.mu.Lock()
	csc.pendingFiles = append(csc.pendingFiles, corpusFile)
	csc.mu.Unlock()

	// Trigger immediate upload
	go csc.uploadPendingFiles(context.Background())

	return nil
}

// uploadPendingFiles uploads pending corpus files to master
func (csc *CorpusSyncClient) uploadPendingFiles(ctx context.Context) {
	csc.mu.Lock()
	if len(csc.pendingFiles) == 0 {
		csc.mu.Unlock()
		return
	}

	// Take pending files
	files := csc.pendingFiles
	csc.pendingFiles = nil
	csc.mu.Unlock()

	csc.logger.WithField("file_count", len(files)).Info("Uploading corpus files")

	// Create multipart upload
	// In a real implementation, this would create a proper multipart form
	// For now, we'll simulate by making individual requests
	uploaded := 0
	for _, file := range files {
		if err := csc.uploadCorpusFile(ctx, file); err != nil {
			csc.logger.WithError(err).WithField("file_hash", file.Hash).Error("Failed to upload corpus file")
			// Re-queue failed upload
			csc.mu.Lock()
			csc.pendingFiles = append(csc.pendingFiles, file)
			csc.mu.Unlock()
			continue
		}
		uploaded++

		// Mark as synced
		csc.mu.Lock()
		csc.syncedFiles[file.Hash] = true
		csc.mu.Unlock()
	}

	csc.logger.WithFields(logrus.Fields{
		"attempted": len(files),
		"uploaded":  uploaded,
	}).Info("Corpus upload completed")
}

// uploadCorpusFile uploads a single corpus file
func (csc *CorpusSyncClient) uploadCorpusFile(ctx context.Context, file *common.CorpusFile) error {
	// Note: Currently uploading only metadata, not file content.
	// File content upload via multipart form will be added when needed.
	filePath := filepath.Join(csc.syncDir, file.Filename)
	_ = filePath // Mark as used for now

	// Create upload request using corpus metadata endpoint
	reqBody, err := json.Marshal(file)
	if err != nil {
		return fmt.Errorf("failed to marshal corpus file: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/campaigns/%s/corpus", csc.client.masterURL, csc.campaignID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(reqBody)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	csc.client.applyAuthHeaders(req)

	resp, err := csc.client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload corpus file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, body)
	}

	return nil
}

// GetSyncedCount returns the number of synced files
func (csc *CorpusSyncClient) GetSyncedCount() int {
	csc.mu.RLock()
	defer csc.mu.RUnlock()
	return len(csc.syncedFiles)
}

// GetPendingCount returns the number of pending uploads
func (csc *CorpusSyncClient) GetPendingCount() int {
	csc.mu.RLock()
	defer csc.mu.RUnlock()
	return len(csc.pendingFiles)
}

// GetLastSyncTime returns the last successful sync time
func (csc *CorpusSyncClient) GetLastSyncTime() time.Time {
	csc.mu.RLock()
	defer csc.mu.RUnlock()
	return csc.lastSync
}

// InitializeJobCorpus downloads campaign corpus files for a job if configured
func (csc *CorpusSyncClient) InitializeJobCorpus(ctx context.Context, job *common.Job, targetDir string) error {
	// Check if job is configured to use campaign corpus
	if !job.UseCampaignCorpus || job.CampaignID == nil || *job.CampaignID == "" {
		csc.logger.WithField("job_id", job.ID).Debug("Job not configured to use campaign corpus")
		return nil
	}

	campaignID := *job.CampaignID
	csc.logger.WithFields(logrus.Fields{
		"job_id":      job.ID,
		"campaign_id": campaignID,
		"target_dir":  targetDir,
	}).Info("Initializing job with campaign corpus")

	// Create corpus directory
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create corpus directory: %w", err)
	}

	// Request corpus files for the campaign
	url := fmt.Sprintf("%s/api/v1/campaigns/%s/corpus/files", csc.client.masterURL, campaignID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	csc.client.applyAuthHeaders(req)

	resp, err := csc.client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to get corpus files: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		csc.logger.WithField("campaign_id", campaignID).Info("No corpus files found for campaign")
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get corpus files with status %d: %s", resp.StatusCode, body)
	}

	// Parse response
	var corpusFiles struct {
		Files     []*common.CorpusFile `json:"files"`
		FileCount int                  `json:"file_count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&corpusFiles); err != nil {
		return fmt.Errorf("failed to decode corpus files response: %w", err)
	}

	if len(corpusFiles.Files) == 0 {
		csc.logger.Info("Campaign has no corpus files")
		return nil
	}

	csc.logger.WithField("file_count", len(corpusFiles.Files)).Info("Downloading campaign corpus files")

	// Download each file
	downloaded := 0
	errors := 0
	startTime := time.Now()

	for i, file := range corpusFiles.Files {
		// Log progress for large downloads
		if i > 0 && i%100 == 0 {
			elapsed := time.Since(startTime)
			rate := float64(i) / elapsed.Seconds()
			remaining := len(corpusFiles.Files) - i
			eta := time.Duration(float64(remaining) / rate * float64(time.Second))

			csc.logger.WithFields(logrus.Fields{
				"progress":   fmt.Sprintf("%d/%d", i, len(corpusFiles.Files)),
				"rate":       fmt.Sprintf("%.1f files/sec", rate),
				"eta":        eta.Round(time.Second),
				"downloaded": downloaded,
				"errors":     errors,
			}).Info("Corpus download progress")
		}

		// Download file
		if err := csc.downloadJobCorpusFile(ctx, campaignID, file, targetDir); err != nil {
			csc.logger.WithError(err).WithField("file_hash", file.Hash).Error("Failed to download corpus file")
			errors++
			continue
		}
		downloaded++
	}

	duration := time.Since(startTime)
	csc.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"job_id":      job.ID,
		"requested":   len(corpusFiles.Files),
		"downloaded":  downloaded,
		"errors":      errors,
		"duration":    duration.Round(time.Millisecond),
		"rate":        fmt.Sprintf("%.1f files/sec", float64(downloaded)/duration.Seconds()),
	}).Info("Campaign corpus initialization completed")

	return nil
}

// downloadJobCorpusFile downloads a single corpus file for job initialization
func (csc *CorpusSyncClient) downloadJobCorpusFile(ctx context.Context, campaignID string, file *common.CorpusFile, targetDir string) error {
	// Download file content
	url := fmt.Sprintf("%s/api/v1/campaigns/%s/corpus/files/%s",
		csc.client.masterURL, campaignID, file.Hash)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	csc.client.applyAuthHeaders(req)

	resp, err := csc.client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Read file content
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read file content: %w", err)
	}

	// Verify hash
	actualHash := calculateFileHash(content)
	if actualHash != file.Hash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", file.Hash, actualHash)
	}

	// Save to disk
	filePath := filepath.Join(targetDir, file.Filename)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	csc.logger.WithFields(logrus.Fields{
		"filename": file.Filename,
		"hash":     file.Hash,
		"size":     file.Size,
	}).Debug("Downloaded corpus file for job")

	return nil
}

// calculateFileHash calculates SHA256 hash of file content
func calculateFileHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// NewCorpusSyncer creates a corpus syncer based on storage backend configuration
func NewCorpusSyncer(client *RetryClient, campaignID, botID, syncDir string, logger logrus.FieldLogger) CorpusSyncer {
	// Check environment variable to determine storage backend
	// This could be enhanced to query the master for its storage configuration
	useS3 := os.Getenv("PANDAFUZZ_USE_S3_BACKEND") == "true"

	if useS3 {
		logger.Info("Using S3 backend for corpus synchronization")
		return NewS3CorpusSyncer(client, syncDir, logger)
	}

	logger.Info("Using filesystem backend for corpus synchronization")
	return NewCorpusSyncClient(client, campaignID, botID, syncDir, logger)
}

// S3CorpusSyncer handles corpus synchronization using S3 presigned URLs
type S3CorpusSyncer struct {
	client       *RetryClient
	localStorage string
	logger       logrus.FieldLogger
}

// NewS3CorpusSyncer creates a new S3 corpus syncer
func NewS3CorpusSyncer(client *RetryClient, localStorage string, logger logrus.FieldLogger) *S3CorpusSyncer {
	return &S3CorpusSyncer{
		client:       client,
		localStorage: localStorage,
		logger:       logger.WithField("component", "s3_corpus_sync"),
	}
}

// downloadCorpusWithPresignedURLs downloads corpus files using presigned URLs
func (s *S3CorpusSyncer) downloadCorpusWithPresignedURLs(ctx context.Context, campaignID string) error {
	// Get list of corpus files from master
	files, err := s.client.GetCorpusFiles(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("failed to get corpus list: %w", err)
	}

	// Create download tasks
	type downloadTask struct {
		file *common.CorpusFile
		url  string
	}

	tasks := make([]downloadTask, 0, len(files))

	// Get presigned URLs for each file
	for _, file := range files {
		// Check if already exists locally
		localPath := filepath.Join(s.localStorage, file.Hash)
		if _, err := os.Stat(localPath); err == nil {
			continue // Already have this file
		}

		// Get presigned URL
		url, err := s.client.GetCorpusDownloadURL(ctx, campaignID, file.Hash)
		if err != nil {
			s.logger.WithError(err).WithField("hash", file.Hash).Warn("Failed to get download URL")
			continue
		}

		tasks = append(tasks, downloadTask{file: file, url: url})
	}

	if len(tasks) == 0 {
		s.logger.Info("Corpus already up to date")
		return nil
	}

	// Download files in parallel
	sem := make(chan struct{}, 4) // Limit concurrent downloads
	var wg sync.WaitGroup

	for _, task := range tasks {
		sem <- struct{}{}
		wg.Add(1)

		go func(t downloadTask) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := s.downloadFile(ctx, t.url, t.file.Hash); err != nil {
				s.logger.WithError(err).WithField("hash", t.file.Hash).Error("Failed to download corpus file")
			}
		}(task)
	}

	wg.Wait()

	s.logger.WithField("files", len(tasks)).Info("Corpus download completed")
	return nil
}

// downloadFile downloads a single file from a presigned URL
func (s *S3CorpusSyncer) downloadFile(ctx context.Context, url, hash string) error {
	// Download directly from S3 using presigned URL
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	// Save to local storage
	localPath := filepath.Join(s.localStorage, hash)
	tempPath := localPath + ".tmp"

	// Ensure directory exists
	if err := os.MkdirAll(s.localStorage, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	file, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Copy with progress tracking
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		os.Remove(tempPath)
		return err
	}

	// Atomic rename
	return os.Rename(tempPath, localPath)
}

// uploadNewCorpusFile uploads a new corpus file using presigned URL
func (s *S3CorpusSyncer) uploadNewCorpusFile(ctx context.Context, campaignID string, filePath string) error {
	// Calculate hash
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])

	// Get presigned upload URL
	uploadReq := UploadURLRequest{
		Filename: filepath.Base(filePath),
		Size:     int64(len(data)),
		Hash:     hashStr,
	}

	uploadInfo, err := s.client.GetCorpusUploadURL(ctx, campaignID, uploadReq)
	if err != nil {
		return err
	}

	if uploadInfo.Status == "exists" {
		s.logger.WithField("hash", hashStr).Debug("File already exists in corpus")
		return nil
	}

	// Upload directly to S3
	req, err := http.NewRequestWithContext(ctx, "PUT", uploadInfo.URL, bytes.NewReader(data))
	if err != nil {
		return err
	}

	// Set required headers
	for k, v := range uploadInfo.Headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload failed: %s", resp.Status)
	}

	// Notify master of new corpus file
	return s.client.RegisterCorpusFile(ctx, campaignID, hashStr, filePath)
}

// InitializeJobCorpus downloads campaign corpus files for a job using S3 presigned URLs
func (s *S3CorpusSyncer) InitializeJobCorpus(ctx context.Context, job *common.Job, targetDir string) error {
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
	}).Info("Initializing job with campaign corpus using S3")

	// Create corpus directory
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create corpus directory: %w", err)
	}

	// Get corpus files list
	files, err := s.client.GetCorpusFiles(ctx, campaignID)
	if err != nil {
		return fmt.Errorf("failed to get corpus files: %w", err)
	}

	if len(files) == 0 {
		s.logger.Info("Campaign has no corpus files")
		return nil
	}

	s.logger.WithField("file_count", len(files)).Info("Downloading campaign corpus files")

	// Download files in parallel
	type downloadTask struct {
		file *common.CorpusFile
		url  string
	}

	tasks := make([]downloadTask, 0, len(files))

	// Get presigned URLs for all files
	for _, file := range files {
		url, err := s.client.GetCorpusDownloadURL(ctx, campaignID, file.Hash)
		if err != nil {
			s.logger.WithError(err).WithField("hash", file.Hash).Warn("Failed to get download URL")
			continue
		}
		tasks = append(tasks, downloadTask{file: file, url: url})
	}

	// Download in parallel with limit
	sem := make(chan struct{}, 4) // Limit concurrent downloads
	var wg sync.WaitGroup
	downloaded := 0
	errors := 0
	startTime := time.Now()

	for i, task := range tasks {
		// Log progress for large downloads
		if i > 0 && i%100 == 0 {
			elapsed := time.Since(startTime)
			rate := float64(i) / elapsed.Seconds()
			remaining := len(tasks) - i
			eta := time.Duration(float64(remaining) / rate * float64(time.Second))

			s.logger.WithFields(logrus.Fields{
				"progress":   fmt.Sprintf("%d/%d", i, len(tasks)),
				"rate":       fmt.Sprintf("%.1f files/sec", rate),
				"eta":        eta.Round(time.Second),
				"downloaded": downloaded,
				"errors":     errors,
			}).Info("Corpus download progress")
		}

		sem <- struct{}{}
		wg.Add(1)

		go func(t downloadTask) {
			defer wg.Done()
			defer func() { <-sem }()

			// Download to target directory with original filename
			targetPath := filepath.Join(targetDir, t.file.Filename)
			if err := s.downloadFileToPath(ctx, t.url, targetPath, t.file.Hash); err != nil {
				s.logger.WithError(err).WithField("file_hash", t.file.Hash).Error("Failed to download corpus file")
				errors++
				return
			}
			downloaded++
		}(task)
	}

	wg.Wait()

	duration := time.Since(startTime)
	s.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"job_id":      job.ID,
		"requested":   len(files),
		"downloaded":  downloaded,
		"errors":      errors,
		"duration":    duration.Round(time.Millisecond),
		"rate":        fmt.Sprintf("%.1f files/sec", float64(downloaded)/duration.Seconds()),
	}).Info("Campaign corpus initialization completed")

	return nil
}

// downloadFileToPath downloads a file from presigned URL to a specific path and verifies hash
func (s *S3CorpusSyncer) downloadFileToPath(ctx context.Context, url, targetPath, expectedHash string) error {
	// Download directly from S3 using presigned URL
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	// Read content
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Verify hash
	actualHash := calculateFileHash(content)
	if actualHash != expectedHash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	// Write to target path
	tempPath := targetPath + ".tmp"
	if err := os.WriteFile(tempPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Atomic rename
	return os.Rename(tempPath, targetPath)
}
