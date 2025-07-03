package bot

import (
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

// CorpusSyncClient handles corpus synchronization with the master
type CorpusSyncClient struct {
	client     *Client
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
func NewCorpusSyncClient(client *Client, campaignID, botID, syncDir string, logger logrus.FieldLogger) *CorpusSyncClient {
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
	// Request sync from master
	syncReq := map[string]string{
		"bot_id": csc.botID,
	}

	reqBody, err := json.Marshal(syncReq)
	if err != nil {
		return fmt.Errorf("failed to marshal sync request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/campaigns/%s/corpus/sync", csc.client.masterURL, csc.campaignID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(reqBody)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := csc.client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to sync corpus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sync failed with status %d: %s", resp.StatusCode, body)
	}

	// Parse response
	var syncResp struct {
		Files     []*common.CorpusFile `json:"files"`
		FileCount int                  `json:"file_count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return fmt.Errorf("failed to decode sync response: %w", err)
	}

	if len(syncResp.Files) == 0 {
		csc.logger.Debug("No new corpus files to sync")
		return nil
	}

	csc.logger.WithField("file_count", len(syncResp.Files)).Info("Downloading corpus files")

	// Download each file
	downloaded := 0
	for _, file := range syncResp.Files {
		if err := csc.downloadCorpusFile(ctx, file); err != nil {
			csc.logger.WithError(err).WithField("file_hash", file.Hash).Error("Failed to download corpus file")
			continue
		}
		downloaded++

		// Mark as synced
		csc.mu.Lock()
		csc.syncedFiles[file.Hash] = true
		csc.mu.Unlock()
	}

	csc.logger.WithFields(logrus.Fields{
		"requested":  len(syncResp.Files),
		"downloaded": downloaded,
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
	// Read file content
	filePath := filepath.Join(csc.syncDir, file.Filename)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Create upload request
	// In a real implementation, this would be a multipart form upload
	// For now, we'll use the corpus metadata endpoint
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

// calculateFileHash calculates SHA256 hash of file content
func calculateFileHash(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}
