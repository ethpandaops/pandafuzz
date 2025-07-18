package master

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

const (
	// Maximum file size for corpus uploads (100MB)
	maxCorpusFileSize = 100 << 20
	// Maximum number of files in a single upload request
	maxFilesPerUpload = 100
	// Buffer size for streaming operations
	streamBufferSize = 32 * 1024
	// Maximum path depth to prevent deep nesting attacks
	maxPathDepth = 10
)

// validateCorpusFilename validates and sanitizes a corpus filename
func (s *Server) validateCorpusFilename(filename string) (string, error) {
	// Remove any directory traversal attempts
	filename = filepath.Base(filename)

	// Additional sanitization
	filename = strings.ReplaceAll(filename, "..", "")
	filename = strings.TrimSpace(filename)

	// Check for empty filename
	if filename == "" || filename == "." || filename == "/" {
		return "", fmt.Errorf("invalid filename")
	}

	// Check filename length
	if len(filename) > 255 {
		return "", fmt.Errorf("filename too long")
	}

	// Check for null bytes
	if strings.Contains(filename, "\x00") {
		return "", fmt.Errorf("filename contains null bytes")
	}

	// Validate path depth
	parts := strings.Split(filename, string(filepath.Separator))
	if len(parts) > maxPathDepth {
		return "", fmt.Errorf("path too deep")
	}

	return filename, nil
}

// validateStoragePath ensures the path is within allowed boundaries
func (s *Server) validateStoragePath(path string) error {
	// Clean the path
	cleanPath := filepath.Clean(path)

	// Get absolute path
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Get storage base path
	storageBase := s.getStorageBasePath()
	absStorageBase, err := filepath.Abs(storageBase)
	if err != nil {
		return fmt.Errorf("invalid storage base path: %w", err)
	}

	// Ensure path is within storage directory
	if !strings.HasPrefix(absPath, absStorageBase) {
		return fmt.Errorf("path outside storage directory")
	}

	return nil
}

// streamToFile safely streams content to a file with size limits
func (s *Server) streamToFile(reader io.Reader, targetPath string, maxSize int64) error {
	// Create temporary file in the same directory
	tempFile, err := os.CreateTemp(filepath.Dir(targetPath), ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Use limited reader to enforce size limit
	limitedReader := io.LimitReader(reader, maxSize+1)

	// Stream with buffer
	buffer := make([]byte, streamBufferSize)
	totalWritten := int64(0)

	for {
		n, err := limitedReader.Read(buffer)
		if n > 0 {
			written, writeErr := tempFile.Write(buffer[:n])
			if writeErr != nil {
				return fmt.Errorf("write error: %w", writeErr)
			}
			totalWritten += int64(written)
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}
	}

	// Check if we exceeded the size limit
	if totalWritten > maxSize {
		return fmt.Errorf("file size exceeds limit of %d bytes", maxSize)
	}

	// Close temp file before rename
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempFile.Name(), targetPath); err != nil {
		return fmt.Errorf("failed to move file: %w", err)
	}

	return nil
}

// handleUploadJobCorpusEnhanced handles corpus file upload with enhanced security
func (s *Server) handleUploadJobCorpusEnhanced(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	// Add request ID for tracing
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = "upload-" + time.Now().Format("20060102-150405")
	}

	logger := s.logger.WithFields(logrus.Fields{
		"job_id":     jobID,
		"request_id": requestID,
		"method":     "handleUploadJobCorpusEnhanced",
	})

	// Get job to verify it exists
	job, err := s.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		if common.IsNotFoundError(err) {
			s.writeErrorResponse(w, http.StatusNotFound, "Job not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get job", err)
		}
		return
	}

	// Check if job is in a valid state for corpus upload
	if job.Status == common.JobStatusCompleted || job.Status == common.JobStatusFailed {
		s.writeErrorResponse(w, http.StatusBadRequest, "Cannot upload corpus to completed/failed job", nil)
		return
	}

	// Set max request size
	r.Body = http.MaxBytesReader(w, r.Body, maxCorpusFileSize*maxFilesPerUpload)

	// Parse multipart form with size limit
	if err := r.ParseMultipartForm(maxCorpusFileSize); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Failed to parse multipart form", err)
		return
	}
	defer r.MultipartForm.RemoveAll()

	// Get corpus directory
	corpusDir := filepath.Join(job.WorkDir, "corpus")

	// Validate corpus directory path
	if err := s.validateStoragePath(corpusDir); err != nil {
		logger.WithError(err).Error("Invalid corpus directory path")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Invalid storage path", err)
		return
	}

	// Create corpus directory if it doesn't exist
	if err := os.MkdirAll(corpusDir, 0755); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to create corpus directory", err)
		return
	}

	uploadedFiles := []string{}
	errors := []string{}

	// Count total files
	totalFiles := 0
	for _, headers := range r.MultipartForm.File {
		totalFiles += len(headers)
	}

	if totalFiles > maxFilesPerUpload {
		s.writeErrorResponse(w, http.StatusBadRequest,
			fmt.Sprintf("Too many files. Maximum %d files per upload", maxFilesPerUpload), nil)
		return
	}

	// Process each uploaded file
	for fieldName, headers := range r.MultipartForm.File {
		for i, header := range headers {
			// Validate filename
			sanitizedFilename, err := s.validateCorpusFilename(header.Filename)
			if err != nil {
				errors = append(errors, fmt.Sprintf("file %d in field %s: %v", i, fieldName, err))
				logger.WithError(err).WithField("filename", header.Filename).Warn("Invalid filename")
				continue
			}

			// Check file size
			if header.Size > maxCorpusFileSize {
				errors = append(errors, fmt.Sprintf("%s: file too large (%d bytes)", sanitizedFilename, header.Size))
				continue
			}

			// Open uploaded file
			file, err := header.Open()
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: failed to open", sanitizedFilename))
				logger.WithError(err).Error("Failed to open uploaded file")
				continue
			}

			// Create destination path
			destPath := filepath.Join(corpusDir, sanitizedFilename)

			// Validate destination path
			if err := s.validateStoragePath(destPath); err != nil {
				file.Close()
				errors = append(errors, fmt.Sprintf("%s: invalid destination path", sanitizedFilename))
				logger.WithError(err).Error("Invalid destination path")
				continue
			}

			// Stream file content with size validation
			err = s.streamToFile(file, destPath, maxCorpusFileSize)
			file.Close()

			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", sanitizedFilename, err))
				logger.WithError(err).Error("Failed to save file")
				continue
			}

			uploadedFiles = append(uploadedFiles, sanitizedFilename)

			// Log successful upload
			logger.WithFields(logrus.Fields{
				"filename": sanitizedFilename,
				"size":     header.Size,
			}).Debug("Corpus file uploaded")
		}
	}

	// Update corpus metadata if service is available
	if s.services != nil && s.services.Corpus != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		for _, filename := range uploadedFiles {
			filePath := filepath.Join(corpusDir, filename)

			// Calculate file hash
			if hash, err := s.calculateFileHash(filePath); err == nil {
				campaignID := ""
				if job.CampaignID != nil {
					campaignID = *job.CampaignID
				}

				corpusFile := &common.CorpusFile{
					ID:         fmt.Sprintf("file-%s-%d", jobID, time.Now().UnixNano()),
					CampaignID: campaignID,
					JobID:      jobID,
					BotID:      "", // Jobs don't have BotID
					Filename:   filename,
					Hash:       hash,
					Size:       0, // Will be set by corpus service
					IsSeed:     true,
					CreatedAt:  time.Now(),
				}

				if err := s.services.Corpus.AddFile(ctx, corpusFile); err != nil {
					logger.WithError(err).WithField("filename", filename).Warn("Failed to register corpus file")
				}
			}
		}
	}

	response := map[string]any{
		"job_id":          jobID,
		"uploaded_files":  uploadedFiles,
		"count":           len(uploadedFiles),
		"total_attempted": totalFiles,
		"request_id":      requestID,
	}

	if len(errors) > 0 {
		response["errors"] = errors
		response["error_count"] = len(errors)
	}

	// Set appropriate status code
	statusCode := http.StatusCreated
	if len(uploadedFiles) == 0 && len(errors) > 0 {
		statusCode = http.StatusBadRequest
	} else if len(errors) > 0 {
		statusCode = http.StatusPartialContent
	}

	w.WriteHeader(statusCode)
	s.writeJSONResponse(w, response)
}

// handleDownloadCorpusFileEnhanced downloads a specific corpus file with security checks
func (s *Server) handleDownloadCorpusFileEnhanced(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]
	filename := vars["filename"]

	// Get job to verify it exists
	job, err := s.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		if common.IsNotFoundError(err) {
			s.writeErrorResponse(w, http.StatusNotFound, "Job not found", err)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get job", err)
		}
		return
	}

	// Validate filename
	sanitizedFilename, err := s.validateCorpusFilename(filename)
	if err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid filename", err)
		return
	}

	// Get file path
	filePath := filepath.Join(job.WorkDir, "corpus", sanitizedFilename)

	// Validate file path
	if err := s.validateStoragePath(filePath); err != nil {
		s.writeErrorResponse(w, http.StatusForbidden, "Access denied", err)
		return
	}

	// Check if file exists
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.writeErrorResponse(w, http.StatusNotFound, "Corpus file not found", nil)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to access corpus file", err)
		}
		return
	}

	// Check file size for sanity
	if info.Size() > maxCorpusFileSize {
		s.writeErrorResponse(w, http.StatusInternalServerError, "File too large for direct download", nil)
		return
	}

	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to open corpus file", err)
		return
	}
	defer file.Close()

	// Set headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", sanitizedFilename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("ETag", fmt.Sprintf("\"%s-%d\"", info.ModTime().Format(time.RFC3339), info.Size()))

	// Handle conditional requests
	if match := r.Header.Get("If-None-Match"); match != "" {
		etag := w.Header().Get("ETag")
		if match == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	// Stream file content
	buffer := make([]byte, streamBufferSize)
	_, err = io.CopyBuffer(w, file, buffer)
	if err != nil {
		s.logger.WithError(err).Error("Failed to send corpus file")
	}
}

// calculateFileHash calculates SHA256 hash of a file
func (s *Server) calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	buffer := make([]byte, streamBufferSize)

	if _, err := io.CopyBuffer(hasher, file, buffer); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// handleGetCorpusDownloadURLEnhanced generates presigned URL with enhanced validation
func (s *Server) handleGetCorpusDownloadURLEnhanced(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	corpusID := vars["id"]
	fileHash := vars["hash"]

	// Validate inputs
	if corpusID == "" || fileHash == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Corpus ID and file hash required", nil)
		return
	}

	// Validate hash format (SHA256)
	if len(fileHash) != 64 || !isHexString(fileHash) {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid file hash format", nil)
		return
	}

	// Check if storage backend is available
	if s.storageBackend == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Storage backend not initialized", nil)
		return
	}

	// Verify corpus/campaign exists
	if s.services != nil && s.services.Campaign != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if _, err := s.services.Campaign.Get(ctx, corpusID); err != nil {
			if err == common.ErrCampaignNotFound {
				s.writeErrorResponse(w, http.StatusNotFound, "Corpus not found", err)
			} else {
				s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to verify corpus", err)
			}
			return
		}
	}

	// Generate S3 key with proper structure
	key := fmt.Sprintf("corpus/%s/%s/%s", corpusID, fileHash[:2], fileHash)

	// Check if file exists in storage
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	exists, err := s.storageBackend.Exists(ctx, key)
	if err != nil {
		s.logger.WithError(err).WithField("key", key).Error("Failed to check file existence")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to check file existence", err)
		return
	}

	if !exists {
		s.writeErrorResponse(w, http.StatusNotFound, "File not found in storage", nil)
		return
	}

	// Generate presigned URL with appropriate expiry
	expiry := 1 * time.Hour
	if exp := r.URL.Query().Get("expiry"); exp != "" {
		if customExpiry, err := time.ParseDuration(exp); err == nil {
			// Limit custom expiry between 5 minutes and 24 hours
			if customExpiry >= 5*time.Minute && customExpiry <= 24*time.Hour {
				expiry = customExpiry
			}
		}
	}

	url, err := s.storageBackend.GetPresignedURL(ctx, key, expiry)
	if err != nil {
		s.logger.WithError(err).WithField("key", key).Error("Failed to generate download URL")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to generate download URL", err)
		return
	}

	response := map[string]interface{}{
		"url":        url,
		"expires_in": int(expiry.Seconds()),
		"method":     "GET",
		"corpus_id":  corpusID,
		"file_hash":  fileHash,
	}

	s.writeJSONResponse(w, response)
}

// handleGetCorpusUploadURLEnhanced generates presigned URL for upload with validation
func (s *Server) handleGetCorpusUploadURLEnhanced(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	corpusID := vars["id"]

	var req struct {
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
		Hash     string `json:"hash"`
	}

	if err := s.decodeJSONBody(w, r, &req); err != nil {
		return
	}

	// Validate request
	if req.Hash == "" || req.Size <= 0 {
		s.writeErrorResponse(w, http.StatusBadRequest, "Hash and size required", nil)
		return
	}

	// Validate hash format
	if len(req.Hash) != 64 || !isHexString(req.Hash) {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid hash format", nil)
		return
	}

	// Validate filename if provided
	if req.Filename != "" {
		if _, err := s.validateCorpusFilename(req.Filename); err != nil {
			s.writeErrorResponse(w, http.StatusBadRequest, "Invalid filename", err)
			return
		}
	}

	// Check file size limit
	effectiveLimit := int64(maxCorpusFileSize)
	if s.config.Storage.MaxFileSize > 0 && s.config.Storage.MaxFileSize < effectiveLimit {
		effectiveLimit = s.config.Storage.MaxFileSize
	}

	if req.Size > effectiveLimit {
		s.writeErrorResponse(w, http.StatusBadRequest,
			fmt.Sprintf("File too large. Maximum size: %d bytes", effectiveLimit), nil)
		return
	}

	// Check if storage backend is available
	if s.storageBackend == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Storage backend not initialized", nil)
		return
	}

	// Verify corpus/campaign exists
	if s.services != nil && s.services.Campaign != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if _, err := s.services.Campaign.Get(ctx, corpusID); err != nil {
			if err == common.ErrCampaignNotFound {
				s.writeErrorResponse(w, http.StatusNotFound, "Corpus not found", err)
			} else {
				s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to verify corpus", err)
			}
			return
		}
	}

	// Generate S3 key
	key := fmt.Sprintf("corpus/%s/%s/%s", corpusID, req.Hash[:2], req.Hash)

	// Check if already exists
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	exists, err := s.storageBackend.Exists(ctx, key)
	if err != nil {
		s.logger.WithError(err).WithField("key", key).Error("Failed to check existence")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to check existence", err)
		return
	}

	if exists {
		// File already exists, no need to upload
		response := map[string]interface{}{
			"status":  "exists",
			"message": "File already exists in corpus",
			"hash":    req.Hash,
		}
		s.writeJSONResponse(w, response)
		return
	}

	// Generate presigned upload URL
	expiry := 1 * time.Hour
	url, err := s.storageBackend.PutPresignedURL(ctx, key, expiry)
	if err != nil {
		s.logger.WithError(err).WithField("key", key).Error("Failed to generate upload URL")
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to generate upload URL", err)
		return
	}

	// Prepare response with required headers
	response := map[string]interface{}{
		"url":        url,
		"expires_in": int(expiry.Seconds()),
		"method":     "PUT",
		"corpus_id":  corpusID,
		"file_hash":  req.Hash,
		"headers": map[string]string{
			"Content-Type":        "application/octet-stream",
			"Content-Length":      fmt.Sprintf("%d", req.Size),
			"x-amz-meta-filename": req.Filename,
			"x-amz-meta-hash":     req.Hash,
		},
	}

	s.writeJSONResponse(w, response)
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
