package master

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ethpandaops/pandafuzz/pkg/common"
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
	TotalFiles   int   `json:"total_files"`
	TotalSize    int64 `json:"total_size"`
	LastUpdated  time.Time `json:"last_updated"`
	UniqueHashes int   `json:"unique_hashes"`
}

// handleGetJobCorpus returns the list of corpus files for a job
func (s *Server) handleGetJobCorpus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	// Get job to verify it exists
	job, err := s.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		s.responseWriter.WriteError(w, err)
		return
	}

	// Get corpus directory for the job
	corpusDir := filepath.Join(job.WorkDir, "corpus")
	
	// Check if corpus directory exists
	if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
		// Return empty list if corpus directory doesn't exist
		response := map[string]any{
			"job_id": jobID,
			"files":  []CorpusFile{},
		}
		s.writeJSONResponse(w, response)
		return
	}

	// Read corpus files
	files := []CorpusFile{}
	err = filepath.Walk(corpusDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip directories
		if info.IsDir() {
			return nil
		}
		
		// Get relative path from corpus directory
		relPath, err := filepath.Rel(corpusDir, path)
		if err != nil {
			return err
		}
		
		files = append(files, CorpusFile{
			Name:       relPath,
			Size:       info.Size(),
			Hash:       "", // TODO: Calculate hash if needed
			UploadedAt: info.ModTime(),
		})
		
		return nil
	})
	
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to read corpus files", err)
		return
	}

	response := map[string]any{
		"job_id": jobID,
		"files":  files,
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
	jobID := vars["id"]
	filename := vars["filename"]

	// Get job to verify it exists
	job, err := s.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		s.responseWriter.WriteError(w, err)
		return
	}

	// Sanitize filename
	filename = filepath.Base(filename)
	filename = strings.ReplaceAll(filename, "..", "")
	
	// Get file path
	filePath := filepath.Join(job.WorkDir, "corpus", filename)
	
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

	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to open corpus file", err)
		return
	}
	defer file.Close()

	// Set headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	// Copy file to response
	_, err = io.Copy(w, file)
	if err != nil {
		s.logger.WithError(err).Error("Failed to send corpus file")
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