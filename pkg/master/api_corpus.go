package master

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/storage"
	"github.com/gorilla/mux"
)

// CorpusFile represents a corpus file
type CorpusFile struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Hash       string    `json:"hash"`
	UploadedAt time.Time `json:"uploaded_at"`
}

// CorpusStats represents corpus statistics
type CorpusStats struct {
	TotalFiles   int       `json:"total_files"`
	TotalSize    int64     `json:"total_size"`
	LastUpdated  time.Time `json:"last_updated"`
	UniqueHashes int       `json:"unique_hashes"`
}

// handleGetJobCorpus returns the list of corpus files for a job (using enhanced corpus service)
func (s *Server) handleGetJobCorpus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

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

	// Check if corpus service is available
	if s.services == nil || s.services.Corpus == nil {
		s.logger.Warn("Corpus service not initialized")
		// Return empty corpus instead of error for backward compatibility
		response := map[string]any{
			"job_id":      jobID,
			"campaign_id": job.CampaignID,
			"files":       []CorpusFile{},
			"count":       0,
		}
		s.writeJSONResponse(w, response)
		return
	}

	// Get corpus files from the corpus service
	corpusFiles, err := s.services.Corpus.GetCorpusForJob(r.Context(), jobID)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get corpus files", err)
		return
	}

	// Convert to API response format
	files := make([]CorpusFile, 0, len(corpusFiles))
	for _, cf := range corpusFiles {
		files = append(files, CorpusFile{
			Name:       cf.Filename,
			Size:       cf.Size,
			Hash:       cf.Hash,
			UploadedAt: cf.CreatedAt,
		})
	}

	response := map[string]any{
		"job_id":      jobID,
		"campaign_id": job.CampaignID,
		"files":       files,
		"count":       len(files),
	}

	s.writeJSONResponse(w, response)
}

// handleUploadJobCorpus handles corpus file upload
func (s *Server) handleUploadJobCorpus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	// Get job to verify it exists
	job, err := s.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		s.responseWriter.WriteError(w, err)
		return
	}

	// Parse multipart form
	err = r.ParseMultipartForm(32 << 20) // 32MB max
	if err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Failed to parse multipart form", err)
		return
	}

	// Get corpus directory
	corpusDir := filepath.Join(job.WorkDir, "corpus")

	// Create corpus directory if it doesn't exist
	if err := os.MkdirAll(corpusDir, 0755); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to create corpus directory", err)
		return
	}

	uploadedFiles := []string{}

	// Process each uploaded file
	for _, headers := range r.MultipartForm.File {
		for _, header := range headers {
			// Open uploaded file
			file, err := header.Open()
			if err != nil {
				s.logger.WithError(err).Error("Failed to open uploaded file")
				continue
			}
			defer file.Close()

			// Sanitize filename
			filename := filepath.Base(header.Filename)
			filename = strings.ReplaceAll(filename, "..", "")

			// Create destination file
			destPath := filepath.Join(corpusDir, filename)
			destFile, err := os.Create(destPath)
			if err != nil {
				s.logger.WithError(err).Error("Failed to create destination file")
				continue
			}
			defer destFile.Close()

			// Copy file content
			_, err = io.Copy(destFile, file)
			if err != nil {
				s.logger.WithError(err).Error("Failed to copy file content")
				os.Remove(destPath)
				continue
			}

			uploadedFiles = append(uploadedFiles, filename)
		}
	}

	response := map[string]any{
		"job_id":         jobID,
		"uploaded_files": uploadedFiles,
		"count":          len(uploadedFiles),
	}

	w.WriteHeader(http.StatusCreated)
	s.writeJSONResponse(w, response)
}

// handleGetCorpusStats returns corpus statistics for a job
func (s *Server) handleGetCorpusStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	// Get job to verify it exists
	job, err := s.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		s.responseWriter.WriteError(w, err)
		return
	}

	// Get corpus directory
	corpusDir := filepath.Join(job.WorkDir, "corpus")

	stats := CorpusStats{
		TotalFiles:   0,
		TotalSize:    0,
		LastUpdated:  time.Time{},
		UniqueHashes: 0,
	}

	// Check if corpus directory exists
	if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
		s.writeJSONResponse(w, stats)
		return
	}

	// Calculate statistics
	hashes := make(map[string]bool)
	var lastModTime time.Time

	err = filepath.Walk(corpusDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		stats.TotalFiles++
		stats.TotalSize += info.Size()

		// Track latest modification time
		if info.ModTime().After(lastModTime) {
			lastModTime = info.ModTime()
		}

		// TODO: Calculate file hash for uniqueness
		// For now, use filename as a placeholder
		hashes[info.Name()] = true

		return nil
	})

	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to calculate corpus statistics", err)
		return
	}

	stats.LastUpdated = lastModTime
	stats.UniqueHashes = len(hashes)

	s.writeJSONResponse(w, stats)
}

// handleDownloadCorpusFile downloads a specific corpus file
func (s *Server) handleDownloadCorpusFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	campaignID := vars["id"]
	hash := vars["hash"]

	if campaignID == "" || hash == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Campaign ID and file hash are required", nil)
		return
	}

	// Get the corpus file metadata from database
	var corpusFile *common.CorpusFile
	if db, ok := s.state.db.(*storage.SQLiteStorage); ok {
		files, err := db.GetCorpusFiles(r.Context(), campaignID)
		if err != nil {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to get corpus files", err)
			return
		}

		// Find the file by hash
		for _, f := range files {
			if f.Hash == hash {
				corpusFile = f
				break
			}
		}
	} else {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Storage interface not available", nil)
		return
	}

	if corpusFile == nil {
		s.writeErrorResponse(w, http.StatusNotFound, "Corpus file not found", nil)
		return
	}

	// Try to retrieve from MinIO/S3 storage backend
	if s.storageBackend != nil {
		storageKey := fmt.Sprintf("corpus/%s/%s", campaignID, hash)
		data, err := s.storageBackend.Retrieve(r.Context(), storageKey)
		if err != nil {
			// Fallback to filesystem for backward compatibility
			s.logger.WithError(err).Debug("Failed to retrieve from storage backend, trying filesystem")

			// Try to get from filesystem (for jobs that store locally)
			job, jobErr := s.services.Job.GetJob(r.Context(), campaignID)
			if jobErr == nil && job.WorkDir != "" {
				filePath := filepath.Join(job.WorkDir, "corpus", corpusFile.Filename)
				file, err := os.Open(filePath)
				if err != nil {
					s.writeErrorResponse(w, http.StatusNotFound, "Corpus file not found", err)
					return
				}
				defer file.Close()

				info, _ := file.Stat()
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", corpusFile.Filename))
				w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

				if _, err := io.Copy(w, file); err != nil {
					s.logger.WithError(err).Error("Failed to send corpus file")
				}
				return
			} else {
				s.writeErrorResponse(w, http.StatusNotFound, "Corpus file not found", err)
				return
			}
		}
		defer data.Close()

		// Set headers
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", corpusFile.Filename))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", corpusFile.Size))

		// Copy the file content to response
		if _, err := io.Copy(w, data); err != nil {
			s.logger.WithError(err).Error("Failed to send corpus file")
		}
	} else {
		// No storage backend configured, try filesystem only
		job, err := s.services.Job.GetJob(r.Context(), campaignID)
		if err != nil {
			s.writeErrorResponse(w, http.StatusNotFound, "Job not found", err)
			return
		}

		filePath := filepath.Join(job.WorkDir, "corpus", corpusFile.Filename)
		file, err := os.Open(filePath)
		if err != nil {
			s.writeErrorResponse(w, http.StatusNotFound, "Corpus file not found", err)
			return
		}
		defer file.Close()

		info, _ := file.Stat()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", corpusFile.Filename))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

		if _, err := io.Copy(w, file); err != nil {
			s.logger.WithError(err).Error("Failed to send corpus file")
		}
	}
}

// handleDeleteCorpusFile deletes a specific corpus file
func (s *Server) handleDeleteCorpusFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]
	filename := vars["filename"]

	// Get job to verify it exists
	job, err := s.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		s.responseWriter.WriteError(w, err)
		return
	}

	// Check if job is running
	if job.Status == common.JobStatusRunning {
		s.writeErrorResponse(w, http.StatusBadRequest, "Cannot delete corpus files while job is running", nil)
		return
	}

	// Sanitize filename
	filename = filepath.Base(filename)
	filename = strings.ReplaceAll(filename, "..", "")

	// Get file path
	filePath := filepath.Join(job.WorkDir, "corpus", filename)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		s.writeErrorResponse(w, http.StatusNotFound, "Corpus file not found", nil)
		return
	}

	// Delete file
	if err := os.Remove(filePath); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete corpus file", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleGetCorpusDownloadURL generates presigned URL for direct download
func (s *Server) handleGetCorpusDownloadURL(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	corpusID := vars["id"]
	fileHash := vars["hash"]

	if corpusID == "" || fileHash == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Corpus ID and file hash required", nil)
		return
	}

	// Check if storage backend is available
	if s.storageBackend == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Storage backend not initialized", nil)
		return
	}

	// Generate S3 key
	key := fmt.Sprintf("corpus/%s/%s/%s", corpusID, fileHash[:2], fileHash)

	// Generate presigned URL (1 hour expiry)
	url, err := s.storageBackend.GetPresignedURL(r.Context(), key, 1*time.Hour)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to generate download URL", err)
		return
	}

	response := map[string]interface{}{
		"url":        url,
		"expires_in": 3600, // seconds
		"method":     "GET",
	}

	s.writeJSONResponse(w, response)
}

// handleGetCorpusUploadURL generates presigned URL for direct upload
func (s *Server) handleGetCorpusUploadURL(w http.ResponseWriter, r *http.Request) {
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

	// Check if storage backend is available
	if s.storageBackend == nil {
		s.writeErrorResponse(w, http.StatusServiceUnavailable, "Storage backend not initialized", nil)
		return
	}

	// Check size limit
	if s.config.Storage.MaxFileSize > 0 && req.Size > s.config.Storage.MaxFileSize {
		s.writeErrorResponse(w, http.StatusBadRequest, "File too large", nil)
		return
	}

	// Generate S3 key
	key := fmt.Sprintf("corpus/%s/%s/%s", corpusID, req.Hash[:2], req.Hash)

	// Check if already exists
	exists, err := s.storageBackend.Exists(r.Context(), key)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to check existence", err)
		return
	}

	if exists {
		// File already exists, no need to upload
		response := map[string]interface{}{
			"status":  "exists",
			"message": "File already exists in corpus",
		}
		s.writeJSONResponse(w, response)
		return
	}

	// Generate presigned upload URL
	url, err := s.storageBackend.PutPresignedURL(r.Context(), key, 1*time.Hour)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to generate upload URL", err)
		return
	}

	response := map[string]interface{}{
		"url":        url,
		"expires_in": 3600,
		"method":     "PUT",
		"headers": map[string]string{
			"Content-Type":   "application/octet-stream",
			"Content-Length": fmt.Sprintf("%d", req.Size),
		},
	}

	s.writeJSONResponse(w, response)
}
