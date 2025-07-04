package bot

import (
	"bytes"
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
}

// BotRegisterResponse represents registration response from master
type BotRegisterResponse struct {
	BotID     string    `json:"bot_id"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Timeout   time.Time `json:"timeout"`
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

// NewRetryClient creates a new retry client for bot communication
func NewRetryClient(config *common.BotConfig, logger *logrus.Logger) (*RetryClient, error) {

	// Configure HTTP client with timeouts
	httpClient := &http.Client{
		Timeout: config.Timeouts.MasterCommunication,
		Transport: &http.Transport{
			MaxIdleConns:       10,
			IdleConnTimeout:    30 * time.Second,
			DisableCompression: false,
		},
	}

	// Setup retry policies
	commRetryPolicy := config.Retry.Communication
	if commRetryPolicy.MaxRetries == 0 {
		commRetryPolicy = common.NetworkRetryPolicy
	}

	updateRetryPolicy := config.Retry.UpdateRecovery
	if updateRetryPolicy.MaxRetries == 0 {
		updateRetryPolicy = common.UpdateRetryPolicy
	}

	// Setup circuit breaker
	circuitBreaker := common.NewCircuitBreaker(5, 60*time.Second)

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

	var response BotRegisterResponse
	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("POST", "/api/v1/bots/register", request, &response)
		})
	})

	if err != nil {
		return nil, common.NewNetworkError("register_bot", err)
	}

	rc.logger.WithFields(logrus.Fields{
		"bot_id":       response.BotID,
		"status":       response.Status,
		"capabilities": capabilities,
	}).Info("Bot registered successfully")

	return &response, nil
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
		"status":        status,
		"current_job":   currentJob,
		"last_activity": time.Now(),
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

// GetJob requests a job assignment from the master
func (rc *RetryClient) GetJob(botID string) (*common.Job, error) {
	var response json.RawMessage

	err := rc.retryManager.Execute(func() error {
		return rc.circuitBreaker.Execute(func() error {
			return rc.doRequest("GET", fmt.Sprintf("/api/v1/bots/%s/job", botID), nil, &response)
		})
	})

	if err != nil {
		return nil, common.NewNetworkError("get_job", err)
	}

	// Parse response to check if it's a job or a status message
	var statusResponse map[string]any
	if err := json.Unmarshal(response, &statusResponse); err == nil {
		if status, exists := statusResponse["status"]; exists && status == "no_jobs_available" {
			rc.logger.Debug("No jobs available from master")
			return nil, nil
		}
	}

	// Parse as job
	var job common.Job
	if err := json.Unmarshal(response, &job); err != nil {
		return nil, common.NewNetworkError("parse_job_response", err)
	}

	rc.logger.WithFields(logrus.Fields{
		"bot_id":   botID,
		"job_id":   job.ID,
		"job_name": job.Name,
		"fuzzer":   job.Fuzzer,
	}).Info("Job received from master")

	return &job, nil
}

// CompleteJob notifies the master of job completion and waits for acknowledgment
func (rc *RetryClient) CompleteJob(botID string, success bool, message string) error {
	request := map[string]any{
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
			return rc.doRequest("POST", fmt.Sprintf("/api/v1/bots/%s/job/complete", botID), request, &response)
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

// Close closes the HTTP client and releases resources
func (rc *RetryClient) Close() error {
	// Close idle connections
	if transport, ok := rc.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	return nil
}
