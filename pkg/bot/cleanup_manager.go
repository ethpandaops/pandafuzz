package bot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// JobCleanupManager handles cleanup of job artifacts and resources
type JobCleanupManager struct {
	config *common.BotConfig
	logger *logrus.Logger
	policy *common.CleanupPolicy
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	ticker *time.Ticker
	stats  CleanupStats
}

// CleanupStats tracks cleanup operation statistics
type CleanupStats struct {
	LastRun            time.Time `json:"last_run"`
	JobsCleaned        int64     `json:"jobs_cleaned"`
	CrashFilesCleaned  int64     `json:"crash_files_cleaned"`
	CorpusFilesCleaned int64     `json:"corpus_files_cleaned"`
	LogsCleaned        int64     `json:"logs_cleaned"`
	BytesFreed         int64     `json:"bytes_freed"`
	Errors             int64     `json:"errors"`
}

// NewCleanupManager creates a new cleanup manager
func NewCleanupManager(config *common.BotConfig, logger *logrus.Logger) *JobCleanupManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &JobCleanupManager{
		config: config,
		logger: logger,
		policy: &common.CleanupPolicy{
			MaxJobAge:       24 * time.Hour,
			MaxCrashAge:     7 * 24 * time.Hour,
			MaxCorpusSize:   1024 * 1024 * 1024,      // 1GB
			MaxDiskUsage:    10 * 1024 * 1024 * 1024, // 10GB
			CleanupInterval: 1 * time.Hour,
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetPolicy updates the cleanup policy
func (m *JobCleanupManager) SetPolicy(policy *common.CleanupPolicy) error {
	if policy == nil {
		return common.NewValidationError("set_cleanup_policy", fmt.Errorf("policy cannot be nil"))
	}

	// Validate policy
	if policy.MaxJobAge <= 0 {
		return common.NewValidationError("set_cleanup_policy", fmt.Errorf("max job age must be positive"))
	}
	if policy.MaxCrashAge <= 0 {
		return common.NewValidationError("set_cleanup_policy", fmt.Errorf("max crash age must be positive"))
	}
	if policy.MaxCorpusSize <= 0 {
		return common.NewValidationError("set_cleanup_policy", fmt.Errorf("max corpus size must be positive"))
	}
	if policy.MaxDiskUsage <= 0 {
		return common.NewValidationError("set_cleanup_policy", fmt.Errorf("max disk usage must be positive"))
	}
	if policy.CleanupInterval <= 0 {
		return common.NewValidationError("set_cleanup_policy", fmt.Errorf("cleanup interval must be positive"))
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.policy = policy

	// Restart scheduled cleanup if running
	if m.ticker != nil {
		m.ticker.Stop()
		m.ticker = time.NewTicker(policy.CleanupInterval)
	}

	m.logger.WithFields(logrus.Fields{
		"max_job_age":      policy.MaxJobAge,
		"max_crash_age":    policy.MaxCrashAge,
		"max_corpus_size":  policy.MaxCorpusSize,
		"max_disk_usage":   policy.MaxDiskUsage,
		"cleanup_interval": policy.CleanupInterval,
	}).Info("Updated cleanup policy")

	return nil
}

// RunCleanup performs a cleanup operation immediately
func (m *JobCleanupManager) RunCleanup(ctx context.Context) error {
	m.mu.Lock()
	m.stats.LastRun = time.Now()
	m.mu.Unlock()

	m.logger.Info("Starting cleanup operation")

	startTime := time.Now()
	var totalFreed int64
	var totalErrors int

	// Clean job directories
	freed, err := m.cleanJobDirectories(ctx)
	if err != nil {
		m.logger.WithError(err).Error("Failed to clean job directories")
		totalErrors++
	} else {
		totalFreed += freed
	}

	// Clean crash files
	freed, err = m.cleanCrashFiles(ctx)
	if err != nil {
		m.logger.WithError(err).Error("Failed to clean crash files")
		totalErrors++
	} else {
		totalFreed += freed
	}

	// Clean corpus files
	freed, err = m.cleanCorpusFiles(ctx)
	if err != nil {
		m.logger.WithError(err).Error("Failed to clean corpus files")
		totalErrors++
	} else {
		totalFreed += freed
	}

	// Clean logs
	freed, err = m.cleanLogs(ctx)
	if err != nil {
		m.logger.WithError(err).Error("Failed to clean logs")
		totalErrors++
	} else {
		totalFreed += freed
	}

	// Update stats
	m.mu.Lock()
	m.stats.BytesFreed += totalFreed
	m.stats.Errors += int64(totalErrors)
	m.mu.Unlock()

	duration := time.Since(startTime)
	m.logger.WithFields(logrus.Fields{
		"duration":    duration,
		"bytes_freed": totalFreed,
		"errors":      totalErrors,
	}).Info("Cleanup operation completed")

	if totalErrors > 0 {
		return common.NewSystemError("cleanup_operation", fmt.Errorf("cleanup completed with %d errors", totalErrors))
	}

	return nil
}

// ScheduleCleanup starts periodic cleanup operations
func (m *JobCleanupManager) ScheduleCleanup() error {
	m.mu.Lock()
	if m.ticker != nil {
		m.mu.Unlock()
		return common.NewSystemError("schedule_cleanup", fmt.Errorf("cleanup already scheduled"))
	}

	m.ticker = time.NewTicker(m.policy.CleanupInterval)
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer m.ticker.Stop()

		for {
			select {
			case <-m.ctx.Done():
				return
			case <-m.ticker.C:
				if err := m.RunCleanup(m.ctx); err != nil {
					m.logger.WithError(err).Error("Scheduled cleanup failed")
				}
			}
		}
	}()

	m.logger.WithField("interval", m.policy.CleanupInterval).Info("Scheduled periodic cleanup")
	return nil
}

// Stop stops the cleanup manager
func (m *JobCleanupManager) Stop() {
	m.logger.Info("Stopping cleanup manager")
	m.cancel()
	m.wg.Wait()
}

// GetStats returns cleanup statistics
func (m *JobCleanupManager) GetStats() CleanupStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats
}

// cleanJobDirectories cleans old job directories
func (m *JobCleanupManager) cleanJobDirectories(ctx context.Context) (int64, error) {
	workDir := m.config.Fuzzing.WorkDir
	if workDir == "" {
		return 0, nil
	}

	cutoffTime := time.Now().Add(-m.policy.MaxJobAge)
	var totalFreed int64
	var jobsCleaned int64

	entries, err := os.ReadDir(workDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, common.NewStorageError("read_work_dir", err)
	}

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return totalFreed, ctx.Err()
		default:
		}

		if !entry.IsDir() {
			continue
		}

		jobDir := filepath.Join(workDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			m.logger.WithError(err).WithField("dir", jobDir).Warn("Failed to get directory info")
			continue
		}

		// Check if directory is old enough to clean
		if info.ModTime().Before(cutoffTime) {
			size, err := m.getDirectorySize(jobDir)
			if err != nil {
				m.logger.WithError(err).WithField("dir", jobDir).Warn("Failed to get directory size")
			}

			if err := os.RemoveAll(jobDir); err != nil {
				m.logger.WithError(err).WithField("dir", jobDir).Error("Failed to remove job directory")
				continue
			}

			totalFreed += size
			jobsCleaned++
			m.logger.WithFields(logrus.Fields{
				"dir":  jobDir,
				"age":  time.Since(info.ModTime()),
				"size": size,
			}).Debug("Removed old job directory")
		}
	}

	m.mu.Lock()
	m.stats.JobsCleaned += jobsCleaned
	m.mu.Unlock()

	return totalFreed, nil
}

// cleanCrashFiles cleans old crash files
func (m *JobCleanupManager) cleanCrashFiles(ctx context.Context) (int64, error) {
	workDir := m.config.Fuzzing.WorkDir
	if workDir == "" {
		return 0, nil
	}

	cutoffTime := time.Now().Add(-m.policy.MaxCrashAge)
	var totalFreed int64
	var crashesCleaned int64

	// Walk through all job directories
	err := filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors and continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Look for crash files
		if !info.IsDir() && filepath.Base(path)[:6] == "crash-" && info.ModTime().Before(cutoffTime) {
			size := info.Size()
			if err := os.Remove(path); err != nil {
				m.logger.WithError(err).WithField("file", path).Warn("Failed to remove crash file")
				return nil
			}

			totalFreed += size
			crashesCleaned++
			m.logger.WithFields(logrus.Fields{
				"file": path,
				"age":  time.Since(info.ModTime()),
				"size": size,
			}).Debug("Removed old crash file")
		}

		return nil
	})

	if err != nil && err != context.Canceled {
		return totalFreed, common.NewStorageError("walk_crash_files", err)
	}

	m.mu.Lock()
	m.stats.CrashFilesCleaned += crashesCleaned
	m.mu.Unlock()

	return totalFreed, nil
}

// cleanCorpusFiles manages corpus size limits
func (m *JobCleanupManager) cleanCorpusFiles(ctx context.Context) (int64, error) {
	workDir := m.config.Fuzzing.WorkDir
	if workDir == "" {
		return 0, nil
	}

	var totalFreed int64
	var corpusCleaned int64

	// Walk through all corpus directories
	err := filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors and continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if this is a corpus directory
		if info.IsDir() && filepath.Base(path) == "corpus" {
			size, err := m.getDirectorySize(path)
			if err != nil {
				m.logger.WithError(err).WithField("dir", path).Warn("Failed to get corpus size")
				return nil
			}

			// If corpus exceeds size limit, remove oldest files
			if size > m.policy.MaxCorpusSize {
				freed, err := m.trimCorpusDirectory(ctx, path, m.policy.MaxCorpusSize)
				if err != nil {
					m.logger.WithError(err).WithField("dir", path).Error("Failed to trim corpus")
					return nil
				}
				totalFreed += freed
				corpusCleaned++
			}
		}

		return nil
	})

	if err != nil && err != context.Canceled {
		return totalFreed, common.NewStorageError("walk_corpus_dirs", err)
	}

	m.mu.Lock()
	m.stats.CorpusFilesCleaned += corpusCleaned
	m.mu.Unlock()

	return totalFreed, nil
}

// cleanLogs cleans old log files
func (m *JobCleanupManager) cleanLogs(ctx context.Context) (int64, error) {
	workDir := m.config.Fuzzing.WorkDir
	if workDir == "" {
		return 0, nil
	}

	cutoffTime := time.Now().Add(-m.policy.MaxJobAge)
	var totalFreed int64
	var logsCleaned int64

	// Walk through all directories looking for log files
	err := filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors and continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Look for log files
		if !info.IsDir() && filepath.Ext(path) == ".log" && info.ModTime().Before(cutoffTime) {
			size := info.Size()
			if err := os.Remove(path); err != nil {
				m.logger.WithError(err).WithField("file", path).Warn("Failed to remove log file")
				return nil
			}

			totalFreed += size
			logsCleaned++
			m.logger.WithFields(logrus.Fields{
				"file": path,
				"age":  time.Since(info.ModTime()),
				"size": size,
			}).Debug("Removed old log file")
		}

		return nil
	})

	if err != nil && err != context.Canceled {
		return totalFreed, common.NewStorageError("walk_log_files", err)
	}

	m.mu.Lock()
	m.stats.LogsCleaned += logsCleaned
	m.mu.Unlock()

	return totalFreed, nil
}

// getDirectorySize calculates the total size of a directory
func (m *JobCleanupManager) getDirectorySize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// trimCorpusDirectory removes oldest files from corpus until it's under the size limit
func (m *JobCleanupManager) trimCorpusDirectory(ctx context.Context, corpusDir string, maxSize int64) (int64, error) {
	type fileInfo struct {
		path    string
		size    int64
		modTime time.Time
	}

	var files []fileInfo
	var totalSize int64

	// Collect all files in corpus
	err := filepath.Walk(corpusDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			files = append(files, fileInfo{
				path:    path,
				size:    info.Size(),
				modTime: info.ModTime(),
			})
			totalSize += info.Size()
		}
		return nil
	})

	if err != nil {
		return 0, err
	}

	if totalSize <= maxSize {
		return 0, nil
	}

	// Sort files by modification time (oldest first)
	for i := 0; i < len(files)-1; i++ {
		for j := i + 1; j < len(files); j++ {
			if files[i].modTime.After(files[j].modTime) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	var freedSpace int64
	targetSize := totalSize - maxSize

	// Remove oldest files until we're under the limit
	for _, f := range files {
		select {
		case <-ctx.Done():
			return freedSpace, ctx.Err()
		default:
		}

		if freedSpace >= targetSize {
			break
		}

		if err := os.Remove(f.path); err != nil {
			m.logger.WithError(err).WithField("file", f.path).Warn("Failed to remove corpus file")
			continue
		}

		freedSpace += f.size
		m.logger.WithFields(logrus.Fields{
			"file": f.path,
			"size": f.size,
			"age":  time.Since(f.modTime),
		}).Debug("Removed old corpus file")
	}

	return freedSpace, nil
}

// CheckDiskUsage checks if disk usage exceeds limits
func (m *JobCleanupManager) CheckDiskUsage() (bool, int64, error) {
	workDir := m.config.Fuzzing.WorkDir
	if workDir == "" {
		return false, 0, nil
	}

	size, err := m.getDirectorySize(workDir)
	if err != nil {
		return false, 0, common.NewStorageError("check_disk_usage", err)
	}

	m.mu.RLock()
	maxUsage := m.policy.MaxDiskUsage
	m.mu.RUnlock()

	return size > maxUsage, size, nil
}
