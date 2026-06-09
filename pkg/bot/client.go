package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// RetryClient implements HTTP client with retry logic for bot-to-master communication
type RetryClient struct {
	httpClient     *http.Client
	retryManager   *common.RetryManager
	updateRetryMgr *common.RetryManager
	circuitBreaker *common.CircuitBreaker
	masterURL      string
	logger         *logrus.Logger
	config         *common.BotConfig
	botID          string // Track bot ID for coverage reporting
}

// BotRegisterResponse represents registration response from master
// This is adapted from the API v1 Bot response format
type BotRegisterResponse struct {
	BotID     string    `json:"bot_id"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Timeout   time.Time `json:"timeout"`
}

// apiV1BotResponse represents the actual Bot response from API v1
// Used internally to parse the API response and convert to BotRegisterResponse
type apiV1BotResponse struct {
	Id            string    `json:"id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	Hostname      string    `json:"hostname"`
	IsOnline      bool      `json:"is_online"`
	RegisteredAt  time.Time `json:"registered_at"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
}

// JobResponse represents job assignment response
type JobResponse struct {
	Status  string      `json:"status,omitempty"`
	Message string      `json:"message,omitempty"`
	Job     *common.Job `json:",omitempty"`
}

// LogPushResponse represents log push response
type LogPushResponse struct {
	Status string `json:"status"`
	JobID  string `json:"job_id"`
	Size   int    `json:"size"`
}

// UploadURLRequest represents a request for a presigned upload URL
type UploadURLRequest struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	Hash     string `json:"hash"`
}

// UploadURLResponse represents the response with presigned upload URL
type UploadURLResponse struct {
	URL       string            `json:"url"`
	Status    string            `json:"status,omitempty"`
	ExpiresIn int               `json:"expires_in"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// NewRetryClient creates a new retry client for bot communication
func NewRetryClient(config *common.BotConfig, logger *logrus.Logger) (*RetryClient, error) {
	return NewRetryClientWithHTTPClient(config, logger, nil)
}

// NewRetryClientWithHTTPClient creates a retry client with a custom HTTP client.
func NewRetryClientWithHTTPClient(config *common.BotConfig, logger *logrus.Logger, httpClient *http.Client) (*RetryClient, error) {
	// Configure HTTP client with timeouts when not provided
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: config.Timeouts.MasterCommunication,
			Transport: &http.Transport{
				MaxIdleConns:       10,
				IdleConnTimeout:    30 * time.Second,
				DisableCompression: false,
			},
		}
	}

	// Setup retry policies
	commRetryPolicy := config.Retry.Communication
	if commRetryPolicy.MaxRetries == 0 {
		commRetryPolicy = common.NetworkRetryPolicy
		commRetryPolicy.MaxRetries = 10 // Increase retries
	}

	updateRetryPolicy := config.Retry.UpdateRecovery
	if updateRetryPolicy.MaxRetries == 0 {
		updateRetryPolicy = common.UpdateRetryPolicy
	}

	// Setup circuit breaker
	circuitBreaker := common.NewCircuitBreaker(10, 60*time.Second) // Increase failure threshold

	return &RetryClient{
		httpClient:     httpClient,
		retryManager:   common.NewRetryManager(commRetryPolicy),
		updateRetryMgr: common.NewRetryManager(updateRetryPolicy),
		circuitBreaker: circuitBreaker,
		masterURL:      config.MasterURL,
		logger:         logger,
		config:         config,
	}, nil
}

// RegisterBot registers the bot with the master
func (rc *RetryClient) RegisterBot(botID string, capabilities []string, apiEndpoint string) (*BotRegisterResponse, error) {
	// Validate capabilities
	if len(capabilities) == 0 {
		return nil, common.NewValidationError("register_bot", fmt.Errorf("no capabilities provided"))
	}

	request := map[string]any{
		"hostname":     rc.getHostname(),
		"name":         rc.config.Name,
		"capabilities": capabilities,
		"api_endpoint": apiEndpoint,
	}

	rc.logger.WithFields(logrus.Fields{
		"hostname":     rc.getHostname(),
		"name":         rc.config.Name,
		"capabilities": capabilities,
		"api_endpoint": apiEndpoint,
	}).Debug("Sending bot registration request")

	// Parse the API v1 Bot response and convert to BotRegisterResponse
	var apiResponse apiV1BotResponse
	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			// API v1 uses POST /api/v1/bots for bot registration
			return rc.doRequest("POST", "/api/v1/bots", request, &apiResponse)
		})
	})

	if err != nil {
		return nil, common.NewNetworkError("register_bot", err)
	}

	// Convert API v1 response to BotRegisterResponse format
	response := &BotRegisterResponse{
		BotID:     apiResponse.Id,
		Status:    "registered",
		Timestamp: apiResponse.RegisteredAt,
		Timeout:   apiResponse.LastHeartbeat.Add(30 * time.Second), // Default timeout
	}

	// Store the bot ID for later use
	if response.BotID != "" {
		rc.botID = response.BotID
	}

	rc.logger.WithFields(logrus.Fields{
		"bot_id":       response.BotID,
		"status":       response.Status,
		"capabilities": capabilities,
	}).Info("Bot registered successfully")

	return response, nil
}

// DeregisterBot deregisters the bot from the master
func (rc *RetryClient) DeregisterBot(botID string) error {
	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("DELETE", fmt.Sprintf("/api/v1/bots/%s", botID), nil, nil)
		})
	})

	if err != nil {
		return common.NewNetworkError("deregister_bot", err)
	}

	rc.logger.WithField("bot_id", botID).Info("Bot deregistered successfully")
	return nil
}

// SendHeartbeat sends a heartbeat to the master
func (rc *RetryClient) SendHeartbeat(botID string, status common.BotStatus, currentJob *string) error {
	request := map[string]any{
		"status": status,
	}
	// Add current_job_id if provided (API v1 uses current_job_id field name)
	if currentJob != nil {
		request["current_job_id"] = *currentJob
	}

	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("POST", fmt.Sprintf("/api/v1/bots/%s/heartbeat", botID), request, nil)
		})
	})

	if err != nil {
		return common.NewNetworkError("send_heartbeat", err)
	}

	rc.logger.WithFields(logrus.Fields{
		"bot_id":      botID,
		"status":      status,
		"current_job": currentJob,
	}).Debug("Heartbeat sent successfully")

	return nil
}

// apiV1JobResponse represents the job assignment response from API v1
// The API wraps the job in a structure with lease information
type apiV1JobResponse struct {
	Job            *apiV1Job  `json:"job"`
	LeaseToken     string     `json:"lease_token"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at"`
	Message        string     `json:"message"`
}

// apiV1Job represents the job fields in the API v1 response
type apiV1Job struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Fuzzer         string            `json:"fuzzer"`
	Target         string            `json:"target"`
	TargetArgs     []string          `json:"target_args"`
	Status         string            `json:"status"`
	AssignedBotID  *string           `json:"assigned_bot_id"`
	WorkDir        string            `json:"work_dir"`
	CorpusDir      string            `json:"corpus_dir"`
	Duration       int               `json:"duration"` // in seconds
	Timeout        int               `json:"timeout"`  // in seconds
	MemoryLimit    int64             `json:"memory_limit"`
	CreatedAt      time.Time         `json:"created_at"`
	StartedAt      *time.Time        `json:"started_at"`
	CompletedAt    *time.Time        `json:"completed_at"`
	EnableCoverage bool              `json:"enable_coverage"`
	CoverageFormat string            `json:"coverage_format"`
	Metadata       map[string]string `json:"metadata"`
}

// GetJob requests a job assignment from the master
func (rc *RetryClient) GetJob(botID string) (*common.Job, error) {
	var response json.RawMessage

	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			// API v1 uses POST /api/v1/bots/{id}/jobs/next for job assignment
			return rc.doRequest("POST", fmt.Sprintf("/api/v1/bots/%s/jobs/next", botID), nil, &response)
		})
	})

	if err != nil {
		return nil, common.NewNetworkError("get_job", err)
	}

	// Parse as API v1 job response with wrapper
	var apiResponse apiV1JobResponse
	if err := json.Unmarshal(response, &apiResponse); err != nil {
		return nil, common.NewNetworkError("parse_job_response", err)
	}

	// Check if no job was available
	if apiResponse.Job == nil {
		rc.logger.Debug("No jobs available from master")
		return nil, nil
	}

	// Convert API job to common.Job
	job := &common.Job{
		ID:             apiResponse.Job.ID,
		Name:           apiResponse.Job.Name,
		Fuzzer:         apiResponse.Job.Fuzzer,
		Target:         apiResponse.Job.Target,
		Status:         common.JobStatus(apiResponse.Job.Status),
		WorkDir:        apiResponse.Job.WorkDir,
		CreatedAt:      apiResponse.Job.CreatedAt,
		StartedAt:      apiResponse.Job.StartedAt,
		CompletedAt:    apiResponse.Job.CompletedAt,
		EnableCoverage: apiResponse.Job.EnableCoverage,
		CoverageFormat: apiResponse.Job.CoverageFormat,
		Config: common.JobConfig{
			Duration:    time.Duration(apiResponse.Job.Duration) * time.Second,
			Timeout:     time.Duration(apiResponse.Job.Timeout) * time.Second,
			MemoryLimit: apiResponse.Job.MemoryLimit,
		},
	}

	// Set AssignedBot if present
	if apiResponse.Job.AssignedBotID != nil {
		job.AssignedBot = apiResponse.Job.AssignedBotID
	}

	rc.logger.WithFields(logrus.Fields{
		"bot_id":   botID,
		"job_id":   job.ID,
		"job_name": job.Name,
		"fuzzer":   job.Fuzzer,
	}).Info("Job received from master")

	return job, nil
}

// AckJobWithToken acknowledges job assignment with lease token
func (rc *RetryClient) AckJobWithToken(botID, jobID, leaseToken string) error {
	request := map[string]interface{}{
		"bot_id":      botID,
		"job_id":      jobID,
		"lease_token": leaseToken,
		"status":      "starting",
	}

	var response struct {
		Acknowledged   bool       `json:"acknowledged"`
		LeaseExpiresAt *time.Time `json:"lease_expires_at"`
		Message        string     `json:"message"`
	}

	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("POST", fmt.Sprintf("/api/v1/jobs/%s/ack", jobID), request, &response)
		})
	})

	if err != nil {
		return common.NewNetworkError("ack_job", err)
	}

	if !response.Acknowledged {
		return common.NewValidationError("ack_job", fmt.Errorf("acknowledgment failed: %s", response.Message))
	}

	rc.logger.WithFields(logrus.Fields{
		"bot_id":           botID,
		"job_id":           jobID,
		"lease_expires_at": response.LeaseExpiresAt,
	}).Info("Job acknowledged with lease")

	return nil
}

// SendHeartbeatWithToken sends heartbeat to renew job lease
func (rc *RetryClient) SendHeartbeatWithToken(botID, jobID, leaseToken string) (*time.Time, error) {
	request := map[string]interface{}{
		"bot_id":      botID,
		"job_id":      jobID,
		"lease_token": leaseToken,
	}

	var response struct {
		Success        bool       `json:"success"`
		LeaseExpiresAt *time.Time `json:"lease_expires_at"`
		Message        string     `json:"message"`
	}

	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("POST", fmt.Sprintf("/api/v1/jobs/%s/heartbeat", jobID), request, &response)
		})
	})

	if err != nil {
		return nil, common.NewNetworkError("heartbeat_job", err)
	}

	if !response.Success {
		return nil, common.NewValidationError("heartbeat_job", fmt.Errorf("heartbeat failed: %s", response.Message))
	}

	return response.LeaseExpiresAt, nil
}

// CompleteJob notifies the master of job completion and waits for acknowledgment
func (rc *RetryClient) CompleteJob(botID, jobID string, success bool, message string) error {
	request := map[string]any{
		"job_id":    jobID,
		"success":   success,
		"timestamp": time.Now(),
		"message":   message,
	}

	// Create a response object to capture the acknowledgment
	var response struct {
		Acknowledged bool   `json:"acknowledged"`
		JobID        string `json:"job_id"`
		Message      string `json:"message"`
	}

	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			// API v1 uses POST /api/v1/bots/{id}/jobs/complete for job completion
			return rc.doRequest("POST", fmt.Sprintf("/api/v1/bots/%s/jobs/complete", botID), request, &response)
		})
	})

	if err != nil {
		return common.NewNetworkError("complete_job", err)
	}

	// Check if master acknowledged the completion
	if !response.Acknowledged {
		rc.logger.WithFields(logrus.Fields{
			"bot_id":  botID,
			"success": success,
			"message": response.Message,
		}).Error("Master did not acknowledge job completion")
		return common.NewNetworkError("complete_job", fmt.Errorf("master did not acknowledge completion: %s", response.Message))
	}

	rc.logger.WithFields(logrus.Fields{
		"bot_id":  botID,
		"job_id":  response.JobID,
		"success": success,
		"message": message,
	}).Info("Job completion acknowledged by master")

	return nil
}

// ReportCrash reports a crash to the master
func (rc *RetryClient) ReportCrash(crash *common.CrashResult) error {
	// Log detailed crash information before sending
	rc.logger.WithFields(logrus.Fields{
		"crash_id":  crash.ID,
		"job_id":    crash.JobID,
		"bot_id":    crash.BotID,
		"hash":      crash.Hash,
		"type":      crash.Type,
		"signal":    crash.Signal,
		"exit_code": crash.ExitCode,
		"size":      crash.Size,
		"file_path": crash.FilePath,
		"is_unique": crash.IsUnique,
		"timestamp": crash.Timestamp,
	}).Info("Sending crash report to master")

	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("POST", "/api/v1/results/crash", crash, nil)
		})
	})

	if err != nil {
		rc.logger.WithError(err).WithFields(logrus.Fields{
			"crash_id": crash.ID,
			"job_id":   crash.JobID,
			"hash":     crash.Hash,
		}).Error("Failed to report crash to master")
		return common.NewNetworkError("report_crash", err)
	}

	rc.logger.WithFields(logrus.Fields{
		"crash_id": crash.ID,
		"job_id":   crash.JobID,
		"hash":     crash.Hash,
		"type":     crash.Type,
		"size":     crash.Size,
	}).Info("Crash successfully reported to master")

	return nil
}

// ReportCoverage reports coverage data to the master
func (rc *RetryClient) ReportCoverage(coverage *common.CoverageResult) error {
	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("POST", "/api/v1/results/coverage", coverage, nil)
		})
	})

	if err != nil {
		return common.NewNetworkError("report_coverage", err)
	}

	rc.logger.WithFields(logrus.Fields{
		"coverage_id": coverage.ID,
		"job_id":      coverage.JobID,
		"edges":       coverage.Edges,
		"new_edges":   coverage.NewEdges,
	}).Debug("Coverage reported to master")

	return nil
}

// ReportCoverageData reports detailed coverage data to the master
func (rc *RetryClient) ReportCoverageData(coverageData map[string]interface{}) error {
	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("POST", "/api/v1/results/coverage-report", coverageData, nil)
		})
	})

	if err != nil {
		return common.NewNetworkError("report_coverage_data", err)
	}

	rc.logger.WithFields(logrus.Fields{
		"job_id":    coverageData["job_id"],
		"report_id": coverageData["report_id"],
	}).Debug("Coverage data reported to master")

	return nil
}

// ReportCorpusUpdate reports corpus updates to the master
func (rc *RetryClient) ReportCorpusUpdate(corpus *common.CorpusUpdate) error {
	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("POST", "/api/v1/results/corpus", corpus, nil)
		})
	})

	if err != nil {
		return common.NewNetworkError("report_corpus_update", err)
	}

	rc.logger.WithFields(logrus.Fields{
		"corpus_id":  corpus.ID,
		"job_id":     corpus.JobID,
		"file_count": len(corpus.Files),
		"total_size": corpus.TotalSize,
	}).Debug("Corpus update reported to master")

	return nil
}

// ReportStatus reports general status to the master
func (rc *RetryClient) ReportStatus(status map[string]any) error {
	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("POST", "/api/v1/results/status", status, nil)
		})
	})

	if err != nil {
		return common.NewNetworkError("report_status", err)
	}

	rc.logger.WithField("status", status).Debug("Status reported to master")
	return nil
}

// WaitForMasterRecovery waits for master to become available during updates
func (rc *RetryClient) WaitForMasterRecovery() error {
	rc.logger.Info("Waiting for master recovery")

	err := rc.updateRetryMgr.Execute(func() error {
		return rc.Ping()
	})

	if err != nil {
		rc.logger.WithError(err).Error("Master recovery timeout")
		return common.NewNetworkError("wait_for_master_recovery", err)
	}

	rc.logger.Info("Master recovery completed")
	return nil
}

// Ping checks connectivity to the master
func (rc *RetryClient) Ping() error {
	return rc.doRequest("GET", "/health", nil, nil)
}

// GetStats returns client statistics
func (rc *RetryClient) GetStats() map[string]any {
	return map[string]any{
		"circuit_breaker": rc.circuitBreaker.GetStats(),
		"master_url":      rc.masterURL,
		"client_timeout":  rc.httpClient.Timeout,
	}
}

// doRequest performs an HTTP request with proper error handling
func (rc *RetryClient) doRequest(method, path string, requestBody any, responseBody any) error {
	url := rc.masterURL + path

	// Prepare request body
	var body io.Reader
	if requestBody != nil {
		jsonData, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		body = bytes.NewBuffer(jsonData)

		// Debug log registration requests
		if method == "POST" && path == "/api/v1/bots/register" {
			rc.logger.WithFields(logrus.Fields{
				"method": method,
				"path":   path,
				"url":    url,
				"body":   string(jsonData),
			}).Debug("Sending registration request")
		}
	}

	// Create request
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("PandaFuzz-Bot/%s", rc.config.ID))
	rc.applyAuthHeaders(req)

	// Make request
	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode == http.StatusServiceUnavailable {
		return common.NewNetworkError("service_unavailable", fmt.Errorf("service unavailable (503)"))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Try to parse error response
		var errorResp map[string]any
		if json.Unmarshal(respData, &errorResp) == nil {
			if errorMsg, exists := errorResp["error"]; exists {
				return fmt.Errorf("server error (%d): %v", resp.StatusCode, errorMsg)
			}
		}
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respData))
	}

	// Parse response body if needed
	if responseBody != nil && len(respData) > 0 {
		if err := json.Unmarshal(respData, responseBody); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// getHostname returns the hostname for bot identification
func (rc *RetryClient) getHostname() string {
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return "unknown"
}

func (rc *RetryClient) applyAuthHeaders(req *http.Request) {
	if rc.config.APIKey != "" {
		req.Header.Set("X-API-Key", rc.config.APIKey)
	}
}

// PushJobLogs pushes job logs to the master
func (rc *RetryClient) PushJobLogs(jobID, botID string, logFilePath string) error {
	// Read log file
	logContent, err := os.ReadFile(logFilePath)
	if err != nil {
		return fmt.Errorf("failed to read log file: %v", err)
	}

	// Create request
	url := fmt.Sprintf("%s/api/v1/jobs/%s/logs/push", rc.masterURL, jobID)
	req, err := http.NewRequest("POST", url, bytes.NewReader(logContent))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers for raw content upload
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-Bot-ID", botID)
	rc.applyAuthHeaders(req)

	// Execute with retry
	var response LogPushResponse
	err = rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			resp, err := rc.httpClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %v", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read response: %v", err)
			}

			if resp.StatusCode != http.StatusCreated {
				return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
			}

			if err := json.Unmarshal(body, &response); err != nil {
				return fmt.Errorf("failed to parse response: %v", err)
			}

			return nil
		})
	})

	if err != nil {
		return fmt.Errorf("failed to push logs after retries: %v", err)
	}

	rc.logger.WithFields(logrus.Fields{
		"job_id":   jobID,
		"bot_id":   botID,
		"log_size": len(logContent),
	}).Info("Successfully pushed job logs to master")

	return nil
}

// DownloadJobBinary downloads the binary for a job
func (rc *RetryClient) DownloadJobBinary(jobID, botID string, targetPath string) error {
	url := fmt.Sprintf("%s/api/v1/jobs/%s/binary/download", rc.masterURL, jobID)

	rc.logger.WithFields(logrus.Fields{
		"job_id":      jobID,
		"bot_id":      botID,
		"url":         url,
		"target_path": targetPath,
	}).Info("Starting binary download")

	var downloadErr error
	err := rc.retryManager.Execute(func() error {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %v", err)
		}

		req.Header.Set("X-Bot-ID", botID)
		rc.applyAuthHeaders(req)

		return rc.circuitBreaker.Execute(func() error {
			resp, err := rc.httpClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %v", err)
			}
			defer resp.Body.Close()

			rc.logger.WithFields(logrus.Fields{
				"job_id":         jobID,
				"bot_id":         botID,
				"status":         resp.StatusCode,
				"content_length": resp.ContentLength,
			}).Debug("Binary download response received")

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
			}

			// Create target directory
			targetDir := filepath.Dir(targetPath)
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return fmt.Errorf("failed to create target directory: %v", err)
			}

			// Remove any existing file first
			os.Remove(targetPath)

			// Create target file
			file, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("failed to create target file: %v", err)
			}
			defer file.Close()

			// Copy content
			written, err := io.Copy(file, resp.Body)
			if err != nil {
				file.Close()
				os.Remove(targetPath)
				return fmt.Errorf("failed to write binary: %v", err)
			}

			// Flush to disk
			if err := file.Sync(); err != nil {
				file.Close()
				os.Remove(targetPath)
				return fmt.Errorf("failed to sync binary to disk: %v", err)
			}

			// Verify we actually wrote something
			if written == 0 {
				file.Close()
				os.Remove(targetPath)
				return fmt.Errorf("downloaded binary is empty (0 bytes)")
			}

			// Close file before chmod
			file.Close()

			// Make binary executable
			if err := os.Chmod(targetPath, 0755); err != nil {
				os.Remove(targetPath)
				return fmt.Errorf("failed to make binary executable: %v", err)
			}

			// Verify file exists after download
			if stat, err := os.Stat(targetPath); err != nil {
				return fmt.Errorf("binary file missing after download: %v", err)
			} else if stat.Size() != written {
				return fmt.Errorf("binary size mismatch: wrote %d bytes but file is %d bytes", written, stat.Size())
			}

			rc.logger.WithFields(logrus.Fields{
				"job_id": jobID,
				"bot_id": botID,
				"size":   written,
				"target": targetPath,
			}).Info("Binary downloaded successfully")

			downloadErr = nil
			return nil
		})
	})

	if err != nil {
		rc.logger.WithError(err).WithFields(logrus.Fields{
			"job_id":      jobID,
			"bot_id":      botID,
			"target_path": targetPath,
		}).Error("Binary download failed")
		return fmt.Errorf("failed to download binary: %v", err)
	}

	if downloadErr != nil {
		rc.logger.WithError(downloadErr).WithFields(logrus.Fields{
			"job_id":      jobID,
			"bot_id":      botID,
			"target_path": targetPath,
		}).Error("Binary download error")
		return downloadErr
	}

	// Final verification
	if stat, err := os.Stat(targetPath); err != nil {
		rc.logger.WithError(err).WithField("target_path", targetPath).Error("Binary file not found after download")
		return fmt.Errorf("binary file not found after download: %v", err)
	} else if stat.Size() == 0 {
		rc.logger.WithField("target_path", targetPath).Error("Binary file is empty after download")
		return fmt.Errorf("binary file is empty after download")
	}

	return nil
}

// DownloadJobCorpus downloads the seed corpus for a job
func (rc *RetryClient) DownloadJobCorpus(jobID, botID string, targetPath string) error {
	url := fmt.Sprintf("%s/api/v1/jobs/%s/corpus/download", rc.masterURL, jobID)

	var downloadErr error
	err := rc.retryManager.Execute(func() error {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %v", err)
		}

		req.Header.Set("X-Bot-ID", botID)
		rc.applyAuthHeaders(req)

		return rc.circuitBreaker.Execute(func() error {
			resp, err := rc.httpClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusNotFound {
				// No corpus is okay
				downloadErr = nil
				return nil
			}

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
			}

			// Create target directory
			targetDir := filepath.Dir(targetPath)
			if err := os.MkdirAll(targetDir, 0755); err != nil {
				return fmt.Errorf("failed to create target directory: %v", err)
			}

			// Create target file
			file, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("failed to create target file: %v", err)
			}
			defer file.Close()

			// Copy content
			written, err := io.Copy(file, resp.Body)
			if err != nil {
				return fmt.Errorf("failed to write corpus: %v", err)
			}

			rc.logger.WithFields(logrus.Fields{
				"job_id": jobID,
				"bot_id": botID,
				"size":   written,
				"target": targetPath,
			}).Info("Corpus downloaded successfully")

			downloadErr = nil
			return nil
		})
	})

	if err != nil {
		return fmt.Errorf("failed to download corpus: %v", err)
	}
	return downloadErr
}

// DownloadCrashInput downloads crash input data from the master
func (rc *RetryClient) DownloadCrashInput(crashID, botID string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/v1/results/crashes/%s/input", rc.masterURL, crashID)

	rc.logger.WithFields(logrus.Fields{
		"crash_id": crashID,
		"bot_id":   botID,
		"url":      url,
	}).Debug("Downloading crash input")

	var input []byte
	err := rc.retryManager.Execute(func() error {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %v", err)
		}

		req.Header.Set("X-Bot-ID", botID)
		rc.applyAuthHeaders(req)

		return rc.circuitBreaker.Execute(func() error {
			resp, err := rc.httpClient.Do(req)
			if err != nil {
				return fmt.Errorf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
			}

			input, err = io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("failed to read response body: %v", err)
			}

			rc.logger.WithFields(logrus.Fields{
				"crash_id": crashID,
				"bot_id":   botID,
				"size":     len(input),
			}).Debug("Crash input downloaded successfully")

			return nil
		})
	})

	if err != nil {
		return nil, fmt.Errorf("failed to download crash input: %v", err)
	}

	return input, nil
}

// ReportReproductionResult reports a reproduction result to the master
func (rc *RetryClient) ReportReproductionResult(result *common.ReproductionResult) error {
	rc.logger.WithFields(logrus.Fields{
		"result_id":        result.ID,
		"crash_id":         result.CrashID,
		"reproduced":       result.Reproduced,
		"matches_original": result.MatchesOriginal,
	}).Debug("Reporting reproduction result to master")

	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("POST", "/api/v1/reproduction/result", result, nil)
		})
	})

	if err != nil {
		return common.NewNetworkError("report_reproduction_result", err)
	}

	rc.logger.WithField("result_id", result.ID).Info("Reproduction result reported successfully")
	return nil
}

// GetReproductionRequest fetches a reproduction request from the master
func (rc *RetryClient) GetReproductionRequest(botID string) (*common.ReproductionRequest, error) {
	var response json.RawMessage

	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("GET", fmt.Sprintf("/api/v1/bots/%s/reproduction", botID), nil, &response)
		})
	})

	if err != nil {
		return nil, common.NewNetworkError("get_reproduction_request", err)
	}

	// Check if response is empty (no reproduction requests available)
	if len(response) == 0 || string(response) == "null" {
		return nil, nil
	}

	// Try to unmarshal as reproduction request
	var reproRequest common.ReproductionRequest
	if err := json.Unmarshal(response, &reproRequest); err != nil {
		// Try to unmarshal as error response
		var errResp map[string]string
		if jsonErr := json.Unmarshal(response, &errResp); jsonErr == nil {
			if errResp["status"] == "no_reproduction_request" {
				return nil, nil
			}
		}
		return nil, fmt.Errorf("failed to unmarshal reproduction request: %v", err)
	}

	rc.logger.WithFields(logrus.Fields{
		"request_id": reproRequest.ID,
		"crash_id":   reproRequest.CrashID,
		"bot_id":     botID,
	}).Info("Received reproduction request")

	return &reproRequest, nil
}

// ReportJobStarted reports that a job has started to transition it from "assigned" to "running"
func (rc *RetryClient) ReportJobStarted(jobID string) error {
	rc.logger.WithField("job_id", jobID).Debug("Reporting job started to master")

	// Use a simple status update request
	request := struct {
		Status string `json:"status"`
	}{
		Status: "running",
	}

	url := fmt.Sprintf("/api/v1/jobs/%s/status", jobID)
	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("PUT", url, request, nil)
		})
	})

	if err != nil {
		return common.NewNetworkError("report_job_started", err)
	}

	rc.logger.WithField("job_id", jobID).Info("Successfully reported job started")
	return nil
}

// GetCorpusFiles gets the list of corpus files for a campaign
func (rc *RetryClient) GetCorpusFiles(ctx context.Context, campaignID string) ([]*common.CorpusFile, error) {
	var response struct {
		Files     []*common.CorpusFile `json:"files"`
		FileCount int                  `json:"file_count"`
	}

	url := fmt.Sprintf("/api/v1/campaigns/%s/corpus/files", campaignID)
	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("GET", url, nil, &response)
		})
	})

	if err != nil {
		return nil, common.NewNetworkError("get_corpus_files", err)
	}

	return response.Files, nil
}

// GetCorpusDownloadURL gets a presigned download URL for a corpus file
func (rc *RetryClient) GetCorpusDownloadURL(ctx context.Context, campaignID, fileHash string) (string, error) {
	var response struct {
		URL       string `json:"url"`
		ExpiresIn int    `json:"expires_in"`
		Method    string `json:"method"`
	}

	url := fmt.Sprintf("/api/v1/campaigns/%s/corpus/files/%s/download-url", campaignID, fileHash)
	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("GET", url, nil, &response)
		})
	})

	if err != nil {
		return "", common.NewNetworkError("get_corpus_download_url", err)
	}

	return response.URL, nil
}

// GetCorpusUploadURL gets a presigned upload URL for a corpus file
func (rc *RetryClient) GetCorpusUploadURL(ctx context.Context, campaignID string, request UploadURLRequest) (*UploadURLResponse, error) {
	var response UploadURLResponse

	url := fmt.Sprintf("/api/v1/campaigns/%s/corpus/upload-url", campaignID)
	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("POST", url, request, &response)
		})
	})

	if err != nil {
		return nil, common.NewNetworkError("get_corpus_upload_url", err)
	}

	return &response, nil
}

// RegisterCorpusFile registers a new corpus file with the master
func (rc *RetryClient) RegisterCorpusFile(ctx context.Context, campaignID, hash, filename string) error {
	request := map[string]string{
		"hash":     hash,
		"filename": filename,
	}

	url := fmt.Sprintf("/api/v1/campaigns/%s/corpus/register", campaignID)
	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("POST", url, request, nil)
		})
	})

	if err != nil {
		return common.NewNetworkError("register_corpus_file", err)
	}

	rc.logger.WithFields(logrus.Fields{
		"campaign_id": campaignID,
		"hash":        hash,
		"filename":    filename,
	}).Debug("Corpus file registered with master")

	return nil
}

// GetCorpusCollectionFiles retrieves the list of files in a corpus collection
func (rc *RetryClient) GetCorpusCollectionFiles(collectionID string) ([]*common.CorpusCollectionFile, error) {
	url := fmt.Sprintf("%s/api/v1/corpus/collections/%s/files", rc.masterURL, collectionID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, common.NewSystemError("create_request", err)
	}
	rc.applyAuthHeaders(req)

	// API returns wrapped response with files array
	var response struct {
		Files []*common.CorpusCollectionFile `json:"files"`
		Count int                            `json:"count"`
	}

	err = rc.retryManager.Execute(func() error {
		resp, err := rc.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("HTTP request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("master returned status %d: %s", resp.StatusCode, body)
		}

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return response.Files, nil
}

// DownloadCorpusCollectionFile downloads a specific file from a corpus collection
func (rc *RetryClient) DownloadCorpusCollectionFile(collectionID, fileID, targetPath string) error {
	url := fmt.Sprintf("%s/api/v1/corpus/collections/%s/files/%s/download", rc.masterURL, collectionID, fileID)

	// Ensure target directory exists
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Create temporary file
	tempFile, err := os.CreateTemp(targetDir, "corpus_download_*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()

	// Clean up temp file on error
	defer func() {
		if _, err := os.Stat(tempPath); err == nil {
			os.Remove(tempPath)
		}
	}()

	// Download with retry
	err = rc.retryManager.Execute(func() error {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}
		rc.applyAuthHeaders(req)

		resp, err := rc.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("HTTP request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("master returned status %d: %s", resp.StatusCode, body)
		}

		// Open temp file for writing
		out, err := os.Create(tempPath)
		if err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}
		defer out.Close()

		// Copy response body to file
		_, err = io.Copy(out, resp.Body)
		if err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Move temp file to final location
	if err := os.Rename(tempPath, targetPath); err != nil {
		// If rename fails, try copy instead
		if err := rc.copyFile(tempPath, targetPath); err != nil {
			return fmt.Errorf("failed to move file to final location: %w", err)
		}
		os.Remove(tempPath)
	}

	return nil
}

// copyFile copies a file from src to dst
func (rc *RetryClient) copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

// Close closes the HTTP client and releases resources
func (rc *RetryClient) Close() error {
	// Close idle connections
	if transport, ok := rc.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	return nil
}
