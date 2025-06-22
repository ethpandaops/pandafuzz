package master

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
)

// handleJobCreateWithUpload handles job creation with binary upload
func (s *Server) handleJobCreateWithUpload(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form with 64MB max size (to accommodate binaries)
	err := r.ParseMultipartForm(64 << 20)
	if err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Failed to parse multipart form", err)
		return
	}

	// Parse job metadata from form
	jobMetadata := r.FormValue("job_metadata")
	if jobMetadata == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "job_metadata field is required", nil)
		return
	}

	var req JobRequest
	if err := json.Unmarshal([]byte(jobMetadata), &req); err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "Invalid job metadata JSON", err)
		return
	}

	// Validate required fields
	if req.Name == "" || req.Fuzzer == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Name and fuzzer are required", nil)
		return
	}

	// Get the uploaded binary file
	file, header, err := r.FormFile("target_binary")
	if err != nil {
		s.writeErrorResponse(w, http.StatusBadRequest, "target_binary file is required", err)
		return
	}
	defer file.Close()

	// Sanitize filename
	filename := filepath.Base(header.Filename)
	filename = strings.ReplaceAll(filename, "..", "")
	
	// Create binaries storage directory if it doesn't exist
	// Ensure we have an absolute path
	basePath := s.config.Storage.BasePath
	if !filepath.IsAbs(basePath) {
		// If relative, make it absolute based on current working directory
		if absPath, err := filepath.Abs(basePath); err == nil {
			basePath = absPath
		} else {
			// Fallback to /storage if we can't get absolute path
			basePath = "/storage"
		}
	}
	
	binariesDir := filepath.Join(basePath, "binaries")
	if err := os.MkdirAll(binariesDir, 0755); err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to create binaries directory", err)
		return
	}

	// Generate unique path for the binary
	timestamp := time.Now().Unix()
	binaryFilename := fmt.Sprintf("%d_%s", timestamp, filename)
	binaryPath := filepath.Join(binariesDir, binaryFilename)

	// Create destination file
	destFile, err := os.Create(binaryPath)
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to create binary file", err)
		return
	}
	defer destFile.Close()

	// Copy binary content
	written, err := io.Copy(destFile, file)
	if err != nil {
		os.Remove(binaryPath)
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to save binary file", err)
		return
	}

	// Make binary executable
	if err := os.Chmod(binaryPath, 0755); err != nil {
		os.Remove(binaryPath)
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to make binary executable", err)
		return
	}

	// Update target path to point to uploaded binary (ensure absolute path)
	absBinaryPath, err := filepath.Abs(binaryPath)
	if err != nil {
		absBinaryPath = binaryPath // Fallback to original if absolute conversion fails
	}
	req.Target = absBinaryPath
	
	// Log the binary path for debugging
	s.logger.WithFields(map[string]interface{}{
		"binary_path": absBinaryPath,
		"base_path": basePath,
		"original_filename": filename,
	}).Info("Binary uploaded and stored")

	// Handle seed corpus files if provided
	var seedCorpusPaths []string
	if corpusFiles, ok := r.MultipartForm.File["seed_corpus"]; ok {
		// Create corpus directory (using same base path as binaries)
		corpusDir := filepath.Join(basePath, "seed_corpus", fmt.Sprintf("%d", timestamp))
		if err := os.MkdirAll(corpusDir, 0755); err != nil {
			os.Remove(binaryPath)
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to create corpus directory", err)
			return
		}

		for _, fileHeader := range corpusFiles {
			corpusFile, err := fileHeader.Open()
			if err != nil {
				s.logger.WithError(err).Error("Failed to open corpus file")
				continue
			}
			defer corpusFile.Close()

			// Sanitize corpus filename
			corpusFilename := filepath.Base(fileHeader.Filename)
			corpusFilename = strings.ReplaceAll(corpusFilename, "..", "")
			corpusPath := filepath.Join(corpusDir, corpusFilename)

			// Save corpus file
			destCorpus, err := os.Create(corpusPath)
			if err != nil {
				s.logger.WithError(err).Error("Failed to create corpus file")
				continue
			}
			defer destCorpus.Close()

			if _, err := io.Copy(destCorpus, corpusFile); err != nil {
				s.logger.WithError(err).Error("Failed to copy corpus file")
				os.Remove(corpusPath)
				continue
			}

			seedCorpusPaths = append(seedCorpusPaths, corpusPath)
		}
	}

	// Update job config with seed corpus paths
	if len(seedCorpusPaths) > 0 {
		req.Config.SeedCorpus = seedCorpusPaths
	}

	// Create job using service layer
	jobReq := service.CreateJobRequest{
		Name:     req.Name,
		Target:   req.Target,
		Fuzzer:   req.Fuzzer,
		Duration: req.Duration,
		Config:   req.Config,
	}

	job, err := s.services.Job.CreateJob(r.Context(), jobReq)
	if err != nil {
		// Clean up uploaded files on failure
		os.Remove(binaryPath)
		for _, corpusPath := range seedCorpusPaths {
			os.Remove(corpusPath)
		}
		s.responseWriter.WriteError(w, err)
		return
	}

	s.logger.WithFields(map[string]interface{}{
		"job_id":      job.ID,
		"binary_path": binaryPath,
		"binary_size": written,
		"corpus_count": len(seedCorpusPaths),
	}).Info("Job created with uploaded binary")

	// Add additional info to response
	response := struct {
		*common.Job
		UploadedBinary struct {
			Path string `json:"path"`
			Size int64  `json:"size"`
		} `json:"uploaded_binary"`
		UploadedCorpus struct {
			Count int      `json:"count"`
			Paths []string `json:"paths"`
		} `json:"uploaded_corpus"`
	}{
		Job: job,
	}
	response.UploadedBinary.Path = binaryPath
	response.UploadedBinary.Size = written
	response.UploadedCorpus.Count = len(seedCorpusPaths)
	response.UploadedCorpus.Paths = seedCorpusPaths

	w.WriteHeader(http.StatusCreated)
	s.writeJSONResponse(w, response)
}