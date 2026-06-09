package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/domain/job/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

var errNotImplementedRepo = errors.New("not implemented")

type unlockCall struct {
	jobID    string
	workerID string
}

type updateStatusCall struct {
	jobID string
	from  types.JobStatus
	to    types.JobStatus
}

type fakeJobRepo struct {
	listByStatusFn func(ctx context.Context, status types.JobStatus) ([]*types.Job, error)
	listFn         func(ctx context.Context, filter repository.JobFilter) ([]*types.Job, error)
	unlockCalls    []unlockCall
	updateCalls    []updateStatusCall
	incrementCalls []string
}

func (f *fakeJobRepo) Create(ctx context.Context, job *types.Job) error {
	return errNotImplementedRepo
}

func (f *fakeJobRepo) Get(ctx context.Context, id string) (*types.Job, error) {
	return nil, errNotImplementedRepo
}

func (f *fakeJobRepo) Update(ctx context.Context, job *types.Job) error {
	return errNotImplementedRepo
}

func (f *fakeJobRepo) Delete(ctx context.Context, id string) error {
	return errNotImplementedRepo
}

func (f *fakeJobRepo) List(ctx context.Context, filter repository.JobFilter) ([]*types.Job, error) {
	if f.listFn != nil {
		return f.listFn(ctx, filter)
	}
	return nil, nil
}

func (f *fakeJobRepo) ListByStatus(ctx context.Context, status types.JobStatus) ([]*types.Job, error) {
	if f.listByStatusFn != nil {
		return f.listByStatusFn(ctx, status)
	}
	return nil, nil
}

func (f *fakeJobRepo) ListPending(ctx context.Context, limit int) ([]*types.Job, error) {
	return nil, errNotImplementedRepo
}

func (f *fakeJobRepo) ListScheduled(ctx context.Context, before time.Time) ([]*types.Job, error) {
	return nil, errNotImplementedRepo
}

func (f *fakeJobRepo) CountByStatus(ctx context.Context) (map[types.JobStatus]int64, error) {
	return nil, errNotImplementedRepo
}

func (f *fakeJobRepo) UpdateStatus(ctx context.Context, id string, from, to types.JobStatus) error {
	f.updateCalls = append(f.updateCalls, updateStatusCall{jobID: id, from: from, to: to})
	return nil
}

func (f *fakeJobRepo) IncrementRetries(ctx context.Context, id string) error {
	f.incrementCalls = append(f.incrementCalls, id)
	return nil
}

func (f *fakeJobRepo) GetDependencies(ctx context.Context, jobID string) ([]*types.Job, error) {
	return nil, errNotImplementedRepo
}

func (f *fakeJobRepo) GetDependents(ctx context.Context, jobID string) ([]*types.Job, error) {
	return nil, errNotImplementedRepo
}

func (f *fakeJobRepo) AddDependency(ctx context.Context, jobID, dependsOnID string) error {
	return errNotImplementedRepo
}

func (f *fakeJobRepo) RemoveDependency(ctx context.Context, jobID, dependsOnID string) error {
	return errNotImplementedRepo
}

func (f *fakeJobRepo) LockForProcessing(ctx context.Context, jobID string, workerID string, lockDuration time.Duration) (*types.Job, error) {
	return nil, errNotImplementedRepo
}

func (f *fakeJobRepo) UnlockJob(ctx context.Context, jobID string, workerID string) error {
	f.unlockCalls = append(f.unlockCalls, unlockCall{jobID: jobID, workerID: workerID})
	return nil
}

func (f *fakeJobRepo) GetStaleJobs(ctx context.Context, staleDuration time.Duration) ([]*types.Job, error) {
	return nil, errNotImplementedRepo
}

func (f *fakeJobRepo) GetMetrics(ctx context.Context) (*repository.JobRepositoryMetrics, error) {
	return nil, errNotImplementedRepo
}

func TestJobRecoveryManager_ReleaseStuckJobs(t *testing.T) {
	t.Parallel()

	now := time.Now()
	startedAt := now.Add(-2 * time.Hour)
	updatedAt := now.Add(-20 * time.Minute)

	job := &types.Job{
		ID:        "job-stuck",
		Status:    types.StatusRunning,
		StartedAt: &startedAt,
		UpdatedAt: updatedAt,
		LockedBy:  "bot-1",
	}

	repo := &fakeJobRepo{
		listByStatusFn: func(ctx context.Context, status types.JobStatus) ([]*types.Job, error) {
			require.Equal(t, types.StatusRunning, status)
			return []*types.Job{job}, nil
		},
	}

	manager := NewJobRecoveryManager(repo, logrus.New())
	manager.SetStuckJobThreshold(1 * time.Minute)

	require.NoError(t, manager.ReleaseStuckJobs())
	require.Len(t, repo.unlockCalls, 1)
	require.Len(t, repo.updateCalls, 1)
	require.Equal(t, types.StatusRunning, repo.updateCalls[0].from)
	require.Equal(t, types.StatusPending, repo.updateCalls[0].to)

	stats := manager.GetStats()
	require.Equal(t, int64(1), stats.StuckJobsRecovered)
}

func TestJobRecoveryManager_ReassignTimedOutJobs(t *testing.T) {
	t.Parallel()

	now := time.Now()
	startedAt := now.Add(-2 * time.Hour)

	job := &types.Job{
		ID:          "job-timeout",
		Status:      types.StatusRunning,
		StartedAt:   &startedAt,
		MaxDuration: 10 * time.Minute,
		LockedBy:    "bot-2",
		RetryCount:  0,
		MaxRetries:  1,
	}

	repo := &fakeJobRepo{
		listFn: func(ctx context.Context, filter repository.JobFilter) ([]*types.Job, error) {
			return []*types.Job{job}, nil
		},
	}

	manager := NewJobRecoveryManager(repo, logrus.New())

	require.NoError(t, manager.ReassignTimedOutJobs())
	require.Len(t, repo.unlockCalls, 1)
	require.Len(t, repo.updateCalls, 2)
	require.Len(t, repo.incrementCalls, 1)

	require.Equal(t, types.StatusRunning, repo.updateCalls[0].from)
	require.Equal(t, types.StatusFailed, repo.updateCalls[0].to)
	require.Equal(t, types.StatusFailed, repo.updateCalls[1].from)
	require.Equal(t, types.StatusPending, repo.updateCalls[1].to)

	stats := manager.GetStats()
	require.Equal(t, int64(1), stats.TimedOutJobsRecovered)
}

func TestJobRecoveryManager_RecoverOrphanedJobs(t *testing.T) {
	t.Parallel()

	expired := time.Now().Add(-1 * time.Minute)
	job := &types.Job{
		ID:            "job-orphaned",
		Status:        types.StatusRunning,
		LockedBy:      "bot-3",
		LockExpiresAt: &expired,
	}

	repo := &fakeJobRepo{
		listByStatusFn: func(ctx context.Context, status types.JobStatus) ([]*types.Job, error) {
			if status == types.StatusRunning {
				return []*types.Job{job}, nil
			}
			return nil, nil
		},
	}

	manager := NewJobRecoveryManager(repo, logrus.New())

	require.NoError(t, manager.RecoverOrphanedJobs())
	require.Len(t, repo.unlockCalls, 1)
	require.Len(t, repo.updateCalls, 1)
	require.Equal(t, types.StatusRunning, repo.updateCalls[0].from)
	require.Equal(t, types.StatusPending, repo.updateCalls[0].to)

	stats := manager.GetStats()
	require.Equal(t, int64(1), stats.OrphanedJobsRecovered)
}
