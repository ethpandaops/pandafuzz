package master

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// LogEntry represents a single log line
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Source    string    `json:"source"`
	Message   string    `json:"message"`
}

// JobLogsResponse represents the response for job logs
type JobLogsResponse struct {
	JobID       string      `json:"job_id"`
	JobName     string      `json:"job_name"`
	Status      string      `json:"status"`
	TotalLines  int         `json:"total_lines"`
	Logs        []LogEntry  `json:"logs"`
	HasMore     bool        `json:"has_more"`
	NextOffset  int         `json:"next_offset,omitempty"`
}

// handleJobLogsV2 handles job log retrieval with better implementation
func (s *Server) handleJobLogsV2(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if jobID == "" {
		s.writeErrorResponse(w, http.StatusBadRequest, "Job ID is required", nil)
		return
	}

	// Get query parameters
	query := r.URL.Query()
	limit := 1000 // Default limit
	offset := 0
	
	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 10000 {
			limit = parsed
		}
	}
	
	if o := query.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Get job details
	job, err := s.services.Job.GetJob(r.Context(), jobID)
	if err != nil {
		s.responseWriter.WriteError(w, err)
		return
	}

	// Check if job is in a final state
	finalStates := []common.JobStatus{
		common.JobStatusCompleted,
		common.JobStatusFailed,
		common.JobStatusTimedOut,
		common.JobStatusCancelled,
	}

	isJobFinished := false
	for _, state := range finalStates {
		if job.Status == state {
			isJobFinished = true
			break
		}
	}

	// First try to read logs from master storage (pushed by bots)
	storagePath := filepath.Join(s.config.Storage.BasePath, "logs", jobID, "job.log")
	logPath := storagePath
	
	// If not in storage, check if job is running locally and try work directory
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		// Fall back to work directory for local jobs
		logPath = filepath.Join(job.WorkDir, "job.log")
	}
	
	logs, totalLines, hasMore, err := readJobLogs(logPath, offset, limit)
	if err != nil {
		// If log file doesn't exist
		if os.IsNotExist(err) {
			// Return empty logs with job info
			response := JobLogsResponse{
				JobID:      job.ID,
				JobName:    job.Name,
				Status:     string(job.Status),
				TotalLines: 0,
				Logs:       []LogEntry{},
				HasMore:    false,
			}
			
			// Add a message about log availability
			if !isJobFinished {
				response.Logs = append(response.Logs, LogEntry{
					Timestamp: time.Now(),
					Level:     "info",
					Source:    "system",
					Message:   "Job is still running. Logs will be available after completion.",
				})
			} else {
				// Job is finished but no logs - this might indicate a crash
				response.Logs = append(response.Logs, LogEntry{
					Timestamp: time.Now(),
					Level:     "error",
					Source:    "system",
					Message:   "No logs found for this job. This may indicate the job failed to start properly.",
				})
				
				// Add job status information
				if job.Status == common.JobStatusFailed {
					response.Logs = append(response.Logs, LogEntry{
						Timestamp: time.Now(),
						Level:     "error",
						Source:    "system",
						Message:   fmt.Sprintf("Job status: FAILED. Target: %s", job.Target),
					})
				}
				
				// Check if neither storage nor work directory exist
				storageLogExists := false
				if _, err := os.Stat(storagePath); err == nil {
					storageLogExists = true
				}
				workDirLogExists := false
				if _, err := os.Stat(filepath.Join(job.WorkDir, "job.log")); err == nil {
					workDirLogExists = true
				}
				
				if !storageLogExists && !workDirLogExists {
					response.Logs = append(response.Logs, LogEntry{
						Timestamp: time.Now(),
						Level:     "error",
						Source:    "system",
						Message:   "No logs found in storage or work directory. The job may have failed during initialization.",
					})
				}
			}
			
			s.writeJSONResponse(w, response)
			return
		}
		
		s.writeErrorResponse(w, http.StatusInternalServerError, "Failed to read job logs", err)
		return
	}

	// Prepare response
	response := JobLogsResponse{
		JobID:      job.ID,
		JobName:    job.Name,
		Status:     string(job.Status),
		TotalLines: totalLines,
		Logs:       logs,
		HasMore:    hasMore,
	}

	if hasMore {
		response.NextOffset = offset + len(logs)
	}

	s.writeJSONResponse(w, response)
}

// readJobLogs reads logs from file with pagination
func readJobLogs(logPath string, offset, limit int) ([]LogEntry, int, bool, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return nil, 0, false, err
	}
	defer file.Close()

	// Get file info to check if it's empty
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, 0, false, err
	}
	
	if fileInfo.Size() == 0 {
		// Return a single entry indicating empty log file
		logs := []LogEntry{{
			Timestamp: fileInfo.ModTime(),
			Level:     "warning",
			Source:    "system",
			Message:   "Log file exists but is empty. The job may have crashed before logging started.",
		}}
		return logs, 1, false, nil
	}

	logs := make([]LogEntry, 0, limit)
	scanner := bufio.NewScanner(file)
	// Set a larger buffer for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	
	// Count total lines first (for small files)
	// For large files, this should be optimized
	totalLines := 0
	for scanner.Scan() {
		totalLines++
	}
	
	if err := scanner.Err(); err != nil {
		return nil, 0, false, fmt.Errorf("error counting lines: %v", err)
	}
	
	// Reset file pointer
	file.Seek(0, 0)
	scanner = bufio.NewScanner(file)
	scanner.Buffer(buf, 1024*1024)
	
	// Skip to offset
	currentLine := 0
	for currentLine < offset && scanner.Scan() {
		currentLine++
	}
	
	// Read logs up to limit
	count := 0
	for scanner.Scan() && count < limit {
		line := scanner.Text()
		entry := parseLogLine(line)
		logs = append(logs, entry)
		count++
		currentLine++
	}
	
	hasMore := currentLine < totalLines
	
	if err := scanner.Err(); err != nil {
		return nil, 0, false, err
	}
	
	return logs, totalLines, hasMore, nil
}

// parseLogLine parses a log line into a LogEntry
func parseLogLine(line string) LogEntry {
	// Try to parse structured log format
	// Expected format: 2025-06-20T14:03:21Z level=info source=bot msg="Message here"
	
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     "info",
		Source:    "fuzzer",
		Message:   line,
	}
	
	// Simple parsing - can be improved with proper log format
	parts := strings.SplitN(line, " ", 4)
	if len(parts) >= 1 {
		// Try to parse timestamp
		if t, err := time.Parse(time.RFC3339, parts[0]); err == nil {
			entry.Timestamp = t
			
			// Parse remaining parts
			if len(parts) > 1 {
				remaining := strings.Join(parts[1:], " ")
				
				// Extract level
				if strings.Contains(remaining, "level=") {
					if idx := strings.Index(remaining, "level="); idx >= 0 {
						levelEnd := strings.IndexAny(remaining[idx+6:], " \t")
						if levelEnd > 0 {
							entry.Level = remaining[idx+6 : idx+6+levelEnd]
						}
					}
				}
				
				// Extract source
				if strings.Contains(remaining, "source=") {
					if idx := strings.Index(remaining, "source="); idx >= 0 {
						sourceEnd := strings.IndexAny(remaining[idx+7:], " \t")
						if sourceEnd > 0 {
							entry.Source = remaining[idx+7 : idx+7+sourceEnd]
						}
					}
				}
				
				// Extract message
				if strings.Contains(remaining, "msg=") {
					if idx := strings.Index(remaining, "msg="); idx >= 0 {
						msg := remaining[idx+4:]
						// Remove quotes if present
						msg = strings.Trim(msg, "\"")
						entry.Message = msg
					}
				} else {
					// Use the whole remaining part as message
					entry.Message = remaining
				}
			}
		}
	}
	
	return entry
}

// handleJobLogStream handles real-time log streaming (WebSocket or SSE)
func (s *Server) handleJobLogStream(w http.ResponseWriter, r *http.Request) {
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

	// Set up Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeErrorResponse(w, http.StatusInternalServerError, "Streaming not supported", nil)
		return
	}

	// Send initial event
	fmt.Fprintf(w, "event: connected\ndata: {\"job_id\": \"%s\", \"status\": \"%s\"}\n\n", job.ID, job.Status)
	flusher.Flush()

	// Determine log file path - check storage first, then work directory
	storagePath := filepath.Join(s.config.Storage.BasePath, "logs", jobID, "job.log")
	logPath := storagePath
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		// Fall back to work directory for local jobs
		logPath = filepath.Join(job.WorkDir, "job.log")
	}
	
	// Tail the log file
	// This is a simplified implementation - in production, use a proper tail library
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastOffset := int64(0)
	
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Check if job is still running
			currentJob, err := s.services.Job.GetJob(r.Context(), jobID)
			if err != nil {
				fmt.Fprintf(w, "event: error\ndata: {\"error\": \"Failed to get job status\"}\n\n")
				flusher.Flush()
				return
			}

			// Read new lines from log file
			file, err := os.Open(logPath)
			if err != nil {
				if !os.IsNotExist(err) {
					fmt.Fprintf(w, "event: error\ndata: {\"error\": \"Failed to read logs\"}\n\n")
					flusher.Flush()
				}
				continue
			}

			// Seek to last position
			file.Seek(lastOffset, 0)
			reader := bufio.NewReader(file)
			
			hasNewLines := false
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					if err != io.EOF {
						fmt.Fprintf(w, "event: error\ndata: {\"error\": \"Failed to read log line\"}\n\n")
						flusher.Flush()
					}
					break
				}
				
				// Parse and send log entry
				entry := parseLogLine(strings.TrimSpace(line))
				data, _ := json.Marshal(entry)
				fmt.Fprintf(w, "event: log\ndata: %s\n\n", data)
				hasNewLines = true
			}
			
			// Update offset
			newOffset, _ := file.Seek(0, io.SeekCurrent)
			lastOffset = newOffset
			file.Close()
			
			if hasNewLines {
				flusher.Flush()
			}

			// Check if job is finished
			if currentJob.Status == common.JobStatusCompleted ||
			   currentJob.Status == common.JobStatusFailed ||
			   currentJob.Status == common.JobStatusCancelled ||
			   currentJob.Status == common.JobStatusTimedOut {
				fmt.Fprintf(w, "event: finished\ndata: {\"status\": \"%s\"}\n\n", currentJob.Status)
				flusher.Flush()
				return
			}
		}
	}
}