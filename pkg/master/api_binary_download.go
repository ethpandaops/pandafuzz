package master

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

// handleBinaryDownload handles binary download requests from bots
func (s *Server) handleBinaryDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	// Get job details
	job, err := s.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		s.responseWriter.WriteError(w, err)
		return
	}

	// Verify bot authorization
	botID := r.Header.Get("X-Bot-ID")
	if botID == "" || job.AssignedBot == nil || *job.AssignedBot != botID {
		s.writeErrorResponse(w, http.StatusForbidden, "Unauthorized to download binary for this job", nil)
		return
	}

	// Get binary path
	binaryPath := job.Target
	
	// If the path is relative, check in storage
	if !filepath.IsAbs(binaryPath) {
		// Check if it's a stored binary (e.g., storage/binaries/timestamp_filename)
		storagePath := filepath.Join(s.config.Storage.BasePath, binaryPath)
		if _, err := os.Stat(storagePath); err == nil {
			binaryPath = storagePath
		} else {
			// Try without storage prefix
			if strings.HasPrefix(binaryPath, "storage/") {
				trimmedPath := strings.TrimPrefix(binaryPath, "storage/")
				storagePath = filepath.Join(s.config.Storage.BasePath, trimmedPath)
				if _, err := os.Stat(storagePath); err == nil {
					binaryPath = storagePath
				}
			}
		}
	}

	// Open binary file
	file, err := os.Open(binaryPath)
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"job_id": jobID,
			"bot_id": botID,
			"binary_path": binaryPath,
		}).Error("Failed to open binary file")
		
		if os.IsNotExist(err) {
			s.writeErrorResponse(w, http.StatusNotFound, "Binary file not found", nil)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to open binary file", err)
		}
		return
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to stat binary file", err)
		return
	}

	// Set headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(binaryPath)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	w.Header().Set("X-Binary-Name", filepath.Base(job.Target))

	// Stream the file
	http.ServeContent(w, r, filepath.Base(binaryPath), fileInfo.ModTime(), file)

	s.logger.WithFields(logrus.Fields{
		"job_id": jobID,
		"bot_id": botID,
		"binary_size": fileInfo.Size(),
		"binary_name": filepath.Base(binaryPath),
	}).Info("Binary downloaded successfully")
}

// handleCorpusDownload handles corpus download requests from bots
func (s *Server) handleCorpusDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	// Get job details
	job, err := s.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		s.responseWriter.WriteError(w, err)
		return
	}

	// Verify bot authorization
	botID := r.Header.Get("X-Bot-ID")
	if botID == "" || job.AssignedBot == nil || *job.AssignedBot != botID {
		s.writeErrorResponse(w, http.StatusForbidden, "Unauthorized to download corpus for this job", nil)
		return
	}

	// Check if corpus exists for this job
	corpusPath := filepath.Join(s.config.Storage.BasePath, "corpus", jobID, "seed_corpus.zip")
	
	// Try alternate path
	if _, err := os.Stat(corpusPath); os.IsNotExist(err) {
		corpusPath = filepath.Join(s.config.Storage.BasePath, "corpus", jobID, "corpus.zip")
	}
	
	// Open corpus file
	file, err := os.Open(corpusPath)
	if err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"job_id": jobID,
			"bot_id": botID,
			"corpus_path": corpusPath,
		}).Error("Failed to open corpus file")
		
		if os.IsNotExist(err) {
			s.writeErrorResponse(w, http.StatusNotFound, "No corpus file found for this job", nil)
		} else {
			s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to open corpus file", err)
		}
		return
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to stat corpus file", err)
		return
	}

	// Set headers for zip file
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"corpus_%s.zip\"", jobID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	// Stream the file
	http.ServeContent(w, r, filepath.Base(corpusPath), fileInfo.ModTime(), file)

	s.logger.WithFields(logrus.Fields{
		"job_id": jobID,
		"bot_id": botID,
		"corpus_size": fileInfo.Size(),
		"corpus_name": filepath.Base(corpusPath),
	}).Info("Corpus downloaded successfully")
}