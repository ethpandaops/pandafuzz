package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jobrepo "github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testDB creates a temporary SQLite database for testing
func testDB(t *testing.T) *sql.DB {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath+"?cache=shared&mode=rwc&_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000")
	require.NoError(t, err)

	// Set connection pool settings
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(0)

	// Create tables
	err = createTestTables(db)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(tmpDir)
	})

	return db
}

// createTestTables creates the required tables for testing
func createTestTables(db *sql.DB) error {
	schema := `
		CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			target TEXT NOT NULL,
			fuzzer TEXT NOT NULL,
			type TEXT,
			status TEXT NOT NULL,
			assigned_bot TEXT,
			created_at DATETIME NOT NULL,
			started_at DATETIME,
			completed_at DATETIME,
			timeout_at DATETIME NOT NULL,
			work_dir TEXT NOT NULL,
			config TEXT,
			progress INTEGER DEFAULT 0,
			priority INTEGER DEFAULT 50,
			metadata TEXT,
			campaign_id TEXT,
			collection_id TEXT,
			use_campaign_corpus INTEGER DEFAULT 0,
			enable_coverage BOOLEAN DEFAULT FALSE,
			coverage_format TEXT,
			coverage_report_id TEXT,
			lease_token VARCHAR(64),
			lease_expires_at DATETIME,
			last_heartbeat DATETIME,
			updated_at DATETIME,
			description TEXT,
			scheduled_at DATETIME,
			queued_at DATETIME,
			dequeue_count INTEGER DEFAULT 0,
			retry_count INTEGER DEFAULT 0,
			max_retries INTEGER DEFAULT 3,
			retry_delay INTEGER DEFAULT 0,
			locked_by TEXT,
			locked_at DATETIME,
			lock_expires_at DATETIME,
			error_message TEXT,
			execution_time INTEGER DEFAULT 0,
			corpus_path TEXT,
			output_path TEXT
		);

		CREATE TABLE IF NOT EXISTS job_dependencies (
			job_id TEXT NOT NULL,
			depends_on_job_id TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (job_id, depends_on_job_id)
		);

		CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
		CREATE INDEX IF NOT EXISTS idx_jobs_locked_by ON jobs(locked_by);
	`
	_, err := db.Exec(schema)
	return err
}

// testLogger creates a logger for testing
func testLogger() logrus.FieldLogger {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetOutput(os.Stdout)
	return logger.WithField("test", true)
}

// createTestJob creates a test job with the given parameters
func createTestJob(id, name string, status jobtypes.JobStatus) *jobtypes.Job {
	now := time.Now().UTC()
	return &jobtypes.Job{
		ID:           id,
		Name:         name,
		Status:       status,
		FuzzerType:   "libfuzzer",
		TargetBinary: "/bin/test",
		CreatedAt:    now,
		UpdatedAt:    now,
		Priority:     jobtypes.PriorityNormal,
		CorpusPath:   "/tmp/corpus",
		OutputPath:   "/tmp/output",
		MaxRetries:   3,
	}
}

func TestJobRepository_Create(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()
	job := createTestJob("test-create-1", "Test Job", jobtypes.StatusPending)

	err := repo.Create(ctx, job)
	require.NoError(t, err)

	// Verify job was created
	retrieved, err := repo.Get(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, retrieved.ID)
	assert.Equal(t, job.Name, retrieved.Name)
	assert.Equal(t, job.Status, retrieved.Status)
}

func TestJobRepository_Get(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()
	job := createTestJob("test-get-1", "Test Job", jobtypes.StatusPending)

	err := repo.Create(ctx, job)
	require.NoError(t, err)

	t.Run("existing job", func(t *testing.T) {
		retrieved, err := repo.Get(ctx, job.ID)
		require.NoError(t, err)
		assert.Equal(t, job.ID, retrieved.ID)
	})

	t.Run("non-existent job", func(t *testing.T) {
		_, err := repo.Get(ctx, "non-existent")
		assert.ErrorIs(t, err, sqlite.ErrNotFound)
	})
}

func TestJobRepository_Update(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()
	job := createTestJob("test-update-1", "Test Job", jobtypes.StatusPending)

	err := repo.Create(ctx, job)
	require.NoError(t, err)

	// Update job
	job.Name = "Updated Job Name"
	job.Status = jobtypes.StatusRunning
	now := time.Now().UTC()
	job.StartedAt = &now

	err = repo.Update(ctx, job)
	require.NoError(t, err)

	// Verify update
	retrieved, err := repo.Get(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Job Name", retrieved.Name)
	assert.Equal(t, jobtypes.StatusRunning, retrieved.Status)
}

func TestJobRepository_Delete(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()
	job := createTestJob("test-delete-1", "Test Job", jobtypes.StatusPending)

	err := repo.Create(ctx, job)
	require.NoError(t, err)

	err = repo.Delete(ctx, job.ID)
	require.NoError(t, err)

	_, err = repo.Get(ctx, job.ID)
	assert.ErrorIs(t, err, sqlite.ErrNotFound)
}

func TestJobRepository_List(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()

	// Create test jobs with different statuses
	for i := 0; i < 5; i++ {
		status := jobtypes.StatusPending
		if i%2 == 0 {
			status = jobtypes.StatusCompleted
		}
		job := createTestJob(fmt.Sprintf("test-list-%d", i), fmt.Sprintf("Test Job %d", i), status)
		require.NoError(t, repo.Create(ctx, job))
	}

	t.Run("list all", func(t *testing.T) {
		jobs, err := repo.List(ctx, jobrepo.JobFilter{Limit: 100})
		require.NoError(t, err)
		assert.Len(t, jobs, 5)
	})

	t.Run("filter by status", func(t *testing.T) {
		status := jobtypes.StatusPending
		jobs, err := repo.List(ctx, jobrepo.JobFilter{
			Status: &status,
			Limit:  100,
		})
		require.NoError(t, err)
		assert.Len(t, jobs, 2)
	})

	t.Run("pagination", func(t *testing.T) {
		jobs, err := repo.List(ctx, jobrepo.JobFilter{
			Limit:  2,
			Offset: 0,
		})
		require.NoError(t, err)
		assert.Len(t, jobs, 2)
	})
}

func TestJobRepository_ListPending(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()

	// Create pending jobs
	for i := 0; i < 3; i++ {
		job := createTestJob(fmt.Sprintf("test-pending-%d", i), fmt.Sprintf("Pending Job %d", i), jobtypes.StatusPending)
		require.NoError(t, repo.Create(ctx, job))
	}

	// Create running job (should not be returned)
	runningJob := createTestJob("test-running", "Running Job", jobtypes.StatusRunning)
	require.NoError(t, repo.Create(ctx, runningJob))

	jobs, err := repo.ListPending(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, jobs, 3)
}

func TestJobRepository_UpdateStatus(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()
	job := createTestJob("test-status-1", "Test Job", jobtypes.StatusPending)

	require.NoError(t, repo.Create(ctx, job))

	t.Run("valid transition", func(t *testing.T) {
		err := repo.UpdateStatus(ctx, job.ID, jobtypes.StatusPending, jobtypes.StatusQueued)
		require.NoError(t, err)

		retrieved, _ := repo.Get(ctx, job.ID)
		assert.Equal(t, jobtypes.StatusQueued, retrieved.Status)
	})

	t.Run("invalid transition", func(t *testing.T) {
		// Completed status cannot transition
		job2 := createTestJob("test-status-2", "Test Job 2", jobtypes.StatusCompleted)
		require.NoError(t, repo.Create(ctx, job2))

		err := repo.UpdateStatus(ctx, job2.ID, jobtypes.StatusCompleted, jobtypes.StatusRunning)
		assert.Error(t, err)
		assert.ErrorIs(t, err, sqlite.ErrInvalidTransition)
	})
}

func TestJobRepository_ConcurrentLocking(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()

	// Create test job
	job := createTestJob("lock-test", "Lock Test Job", jobtypes.StatusPending)
	require.NoError(t, repo.Create(ctx, job))

	// Concurrent lock attempts
	const numWorkers = 10
	var wg sync.WaitGroup
	var successCount atomic.Int32
	var failCount atomic.Int32

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			_, err := repo.LockForProcessing(ctx, "lock-test",
				fmt.Sprintf("worker-%d", workerID), time.Minute)
			if err == nil {
				successCount.Add(1)
			} else {
				failCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	// Only one worker should succeed
	assert.Equal(t, int32(1), successCount.Load(), "Expected exactly one successful lock")
	assert.Equal(t, int32(numWorkers-1), failCount.Load(), "Expected all other workers to fail")

	// Verify the job is locked
	retrieved, err := repo.Get(ctx, "lock-test")
	require.NoError(t, err)
	assert.NotEmpty(t, retrieved.LockedBy)
	assert.NotNil(t, retrieved.LockExpiresAt)
}

func TestJobRepository_LockExpiry(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()

	t.Run("expired lock on running job cannot be re-locked", func(t *testing.T) {
		// Create test job
		job := createTestJob("lock-expiry-test-1", "Lock Expiry Test Job 1", jobtypes.StatusPending)
		require.NoError(t, repo.Create(ctx, job))

		// Lock with very short duration
		lockDuration := 10 * time.Millisecond
		_, err := repo.LockForProcessing(ctx, job.ID, "worker-1", lockDuration)
		require.NoError(t, err)

		// Wait for lock to expire
		time.Sleep(20 * time.Millisecond)

		// Another worker CANNOT acquire the lock because job is still "running"
		// This is correct behavior - lock expiry alone doesn't mean the job should be re-run
		_, err = repo.LockForProcessing(ctx, job.ID, "worker-2", time.Minute)
		assert.Error(t, err) // Should fail - job is in running status
	})

	t.Run("stale jobs are detected", func(t *testing.T) {
		// Create a job and lock it
		job := createTestJob("lock-expiry-test-2", "Lock Expiry Test Job 2", jobtypes.StatusPending)
		require.NoError(t, repo.Create(ctx, job))

		lockDuration := 10 * time.Millisecond
		_, err := repo.LockForProcessing(ctx, job.ID, "worker-1", lockDuration)
		require.NoError(t, err)

		// Wait for lock to expire
		time.Sleep(20 * time.Millisecond)

		// GetStaleJobs should return this job as stale
		staleJobs, err := repo.GetStaleJobs(ctx, 5*time.Millisecond) // Very short threshold
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(staleJobs), 1)

		// Find our job in the stale list
		found := false
		for _, staleJob := range staleJobs {
			if staleJob.ID == job.ID {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected job to be in stale jobs list")
	})
}

func TestJobRepository_UnlockJob(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()

	job := createTestJob("unlock-test", "Unlock Test Job", jobtypes.StatusPending)
	require.NoError(t, repo.Create(ctx, job))

	t.Run("successful unlock", func(t *testing.T) {
		// Lock the job
		_, err := repo.LockForProcessing(ctx, job.ID, "worker-1", time.Minute)
		require.NoError(t, err)

		// Unlock the job
		err = repo.UnlockJob(ctx, job.ID, "worker-1")
		require.NoError(t, err)

		// Verify job is unlocked
		retrieved, _ := repo.Get(ctx, job.ID)
		assert.Empty(t, retrieved.LockedBy)
	})

	t.Run("wrong worker cannot unlock", func(t *testing.T) {
		// First reset the job status
		job.Status = jobtypes.StatusPending
		job.LockedBy = ""
		require.NoError(t, repo.Update(ctx, job))

		// Lock with worker-1
		_, err := repo.LockForProcessing(ctx, job.ID, "worker-1", time.Minute)
		require.NoError(t, err)

		// Try to unlock with worker-2
		err = repo.UnlockJob(ctx, job.ID, "worker-2")
		assert.ErrorIs(t, err, sqlite.ErrInvalidLockHolder)
	})
}

func TestJobRepository_GetStaleJobs(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()

	// Create and lock a job
	job := createTestJob("stale-test", "Stale Test Job", jobtypes.StatusPending)
	require.NoError(t, repo.Create(ctx, job))

	_, err := repo.LockForProcessing(ctx, job.ID, "worker-1", 10*time.Millisecond)
	require.NoError(t, err)

	// Wait for the lock to become stale
	time.Sleep(20 * time.Millisecond)

	// Get stale jobs
	staleJobs, err := repo.GetStaleJobs(ctx, 10*time.Millisecond)
	require.NoError(t, err)
	assert.Len(t, staleJobs, 1)
	assert.Equal(t, job.ID, staleJobs[0].ID)
}

func TestJobRepository_Dependencies(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()

	// Create test jobs
	job1 := createTestJob("dep-job-1", "Job 1", jobtypes.StatusPending)
	job2 := createTestJob("dep-job-2", "Job 2", jobtypes.StatusPending)
	require.NoError(t, repo.Create(ctx, job1))
	require.NoError(t, repo.Create(ctx, job2))

	t.Run("add and get dependencies", func(t *testing.T) {
		// job2 depends on job1
		err := repo.AddDependency(ctx, job2.ID, job1.ID)
		require.NoError(t, err)

		// Get dependencies of job1 (jobs that depend on it)
		deps, err := repo.GetDependencies(ctx, job1.ID)
		require.NoError(t, err)
		assert.Len(t, deps, 1)
		assert.Equal(t, job2.ID, deps[0].ID)

		// Get dependents of job2 (jobs it depends on)
		dependents, err := repo.GetDependents(ctx, job2.ID)
		require.NoError(t, err)
		assert.Len(t, dependents, 1)
		assert.Equal(t, job1.ID, dependents[0].ID)
	})

	t.Run("self-dependency not allowed", func(t *testing.T) {
		err := repo.AddDependency(ctx, job1.ID, job1.ID)
		assert.ErrorIs(t, err, sqlite.ErrDependencyCycle)
	})

	t.Run("remove dependency", func(t *testing.T) {
		err := repo.RemoveDependency(ctx, job2.ID, job1.ID)
		require.NoError(t, err)

		deps, err := repo.GetDependencies(ctx, job1.ID)
		require.NoError(t, err)
		assert.Len(t, deps, 0)
	})
}

func TestJobRepository_CountByStatus(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()

	// Create jobs with different statuses
	statuses := []jobtypes.JobStatus{
		jobtypes.StatusPending,
		jobtypes.StatusPending,
		jobtypes.StatusRunning,
		jobtypes.StatusCompleted,
		jobtypes.StatusCompleted,
		jobtypes.StatusCompleted,
	}

	for i, status := range statuses {
		job := createTestJob(fmt.Sprintf("count-test-%d", i), fmt.Sprintf("Job %d", i), status)
		require.NoError(t, repo.Create(ctx, job))
	}

	counts, err := repo.CountByStatus(ctx)
	require.NoError(t, err)

	assert.Equal(t, int64(2), counts[jobtypes.StatusPending])
	assert.Equal(t, int64(1), counts[jobtypes.StatusRunning])
	assert.Equal(t, int64(3), counts[jobtypes.StatusCompleted])
}

func TestJobRepository_IncrementRetries(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()

	job := createTestJob("retry-test", "Retry Test Job", jobtypes.StatusFailed)
	require.NoError(t, repo.Create(ctx, job))

	// Increment retries multiple times
	for i := 0; i < 3; i++ {
		err := repo.IncrementRetries(ctx, job.ID)
		require.NoError(t, err)
	}

	// Verify retry count
	retrieved, err := repo.Get(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, retrieved.RetryCount)
}

func TestJobRepository_GetMetrics(t *testing.T) {
	db := testDB(t)
	repo := sqlite.NewJobRepository(db, testLogger())

	ctx := context.Background()

	// Create some jobs
	for i := 0; i < 5; i++ {
		job := createTestJob(fmt.Sprintf("metrics-test-%d", i), fmt.Sprintf("Job %d", i), jobtypes.StatusPending)
		require.NoError(t, repo.Create(ctx, job))
	}

	metrics, err := repo.GetMetrics(ctx)
	require.NoError(t, err)

	assert.Equal(t, int64(5), metrics.TotalJobs)
	assert.Equal(t, int64(5), metrics.JobsByStatus[jobtypes.StatusPending])
}
