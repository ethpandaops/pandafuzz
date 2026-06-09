package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

func newTestSQLiteStorage(t *testing.T) (*SQLiteStorage, func()) {
	t.Helper()

	dir := t.TempDir()
	cfg := common.DatabaseConfig{
		Path: filepath.Join(dir, "pandafuzz-test.db"),
	}

	logger := logrus.New()
	db, err := NewSQLiteStorage(cfg, logger)
	require.NoError(t, err)

	storage, ok := db.(*SQLiteStorage)
	require.True(t, ok)

	cleanup := func() {
		_ = storage.Close(context.Background())
	}

	return storage, cleanup
}

func TestSQLiteStorage_CreateGetListJob(t *testing.T) {
	storage, cleanup := newTestSQLiteStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	job := &common.Job{
		ID:        "job-1",
		Name:      "fuzz-job",
		Target:    "/bin/true",
		Fuzzer:    "afl++",
		Status:    common.JobStatusPending,
		CreatedAt: now,
		TimeoutAt: now.Add(time.Hour),
		WorkDir:   "/tmp/job-1",
		Config: common.JobConfig{
			Duration:    10 * time.Minute,
			MemoryLimit: 128 * 1024 * 1024,
		},
		Progress: 10,
	}

	require.NoError(t, storage.CreateJob(ctx, job))

	fetched, err := storage.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, job.Name, fetched.Name)
	require.Equal(t, job.Target, fetched.Target)
	require.Equal(t, job.Fuzzer, fetched.Fuzzer)
	require.Equal(t, job.Status, fetched.Status)
	require.Equal(t, job.Progress, fetched.Progress)
	require.Equal(t, job.Config.Duration, fetched.Config.Duration)
	require.Equal(t, job.Config.MemoryLimit, fetched.Config.MemoryLimit)

	jobs, err := storage.ListJobs(ctx, 10, 0, string(common.JobStatusPending))
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, job.ID, jobs[0].ID)
}

func TestSQLiteStorage_UpdateJob(t *testing.T) {
	storage, cleanup := newTestSQLiteStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	job := &common.Job{
		ID:        "job-2",
		Name:      "update-job",
		Target:    "/bin/false",
		Fuzzer:    "libfuzzer",
		Status:    common.JobStatusPending,
		CreatedAt: now,
		TimeoutAt: now.Add(time.Hour),
		WorkDir:   "/tmp/job-2",
		Config:    common.JobConfig{Duration: 5 * time.Minute},
	}

	require.NoError(t, storage.CreateJob(ctx, job))

	updates := map[string]interface{}{
		"status":   common.JobStatusRunning,
		"progress": 60,
		"config": common.JobConfig{
			Duration:    15 * time.Minute,
			MemoryLimit: 64 * 1024 * 1024,
		},
	}

	require.NoError(t, storage.UpdateJob(ctx, job.ID, updates))

	fetched, err := storage.GetJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, common.JobStatusRunning, fetched.Status)
	require.Equal(t, 60, fetched.Progress)
	require.Equal(t, 15*time.Minute, fetched.Config.Duration)
	require.Equal(t, int64(64*1024*1024), fetched.Config.MemoryLimit)
}

func TestSQLiteStorage_CreateListCampaign(t *testing.T) {
	storage, cleanup := newTestSQLiteStorage(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()

	campaign := &common.Campaign{
		ID:           "camp-1",
		Name:         "Campaign One",
		Description:  "test campaign",
		Status:       common.CampaignStatusPending,
		TargetBinary: "/bin/true",
		BinaryHash:   "deadbeef",
		CreatedAt:    now,
		UpdatedAt:    now,
		AutoRestart:  true,
		MaxDuration:  2 * time.Hour,
		MaxJobs:      3,
		JobTemplate:  common.JobConfig{Duration: 30 * time.Minute},
		SharedCorpus: true,
		Tags:         []string{"smoke", "unit"},
	}

	require.NoError(t, storage.CreateCampaign(ctx, campaign))

	fetched, err := storage.GetCampaign(ctx, campaign.ID)
	require.NoError(t, err)
	require.Equal(t, campaign.Name, fetched.Name)
	require.Equal(t, campaign.Status, fetched.Status)
	require.Equal(t, campaign.TargetBinary, fetched.TargetBinary)
	require.Equal(t, campaign.JobTemplate.Duration, fetched.JobTemplate.Duration)
	require.Equal(t, campaign.Tags, fetched.Tags)

	campaigns, err := storage.ListCampaigns(ctx, 10, 0, string(common.CampaignStatusPending))
	require.NoError(t, err)
	require.Len(t, campaigns, 1)
	require.Equal(t, campaign.ID, campaigns[0].ID)
}
