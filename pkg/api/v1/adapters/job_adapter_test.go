package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
)

func TestJobAdapter_ListJobs_UsesServiceFilterAndPagination(t *testing.T) {
	t.Parallel()

	now := time.Now()
	jobs := []*common.Job{
		{
			ID:        "job-1",
			Name:      "Job One",
			Target:    "/bin/true",
			Fuzzer:    "afl++",
			Status:    common.JobStatusRunning,
			CreatedAt: now.Add(-time.Minute),
			TimeoutAt: now.Add(time.Hour),
			WorkDir:   "/tmp/job-1",
			Config:    common.JobConfig{Duration: 10 * time.Minute},
			Priority:  50,
		},
		{
			ID:        "job-2",
			Name:      "Job Two",
			Target:    "/bin/false",
			Fuzzer:    "afl++",
			Status:    common.JobStatusRunning,
			CreatedAt: now.Add(-2 * time.Minute),
			TimeoutAt: now.Add(time.Hour),
			WorkDir:   "/tmp/job-2",
			Config:    common.JobConfig{Duration: 5 * time.Minute},
			Priority:  25,
		},
	}

	jobService := &stubJobService{
		listFn: func(_ context.Context, filter service.JobFilter) ([]*common.Job, error) {
			return jobs, nil
		},
	}

	adapter := NewJobAdapter(nil, nil, jobService, nil, nil, nil, logrus.New(), 1024)

	limit := 2
	offset := 2
	status := generated.JobStatus("running")
	fuzzer := generated.FuzzerType("afl++")
	params := generated.ListJobsParams{
		Limit:  &limit,
		Offset: &offset,
		Status: &status,
		Fuzzer: &fuzzer,
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/jobs", nil)

	adapter.ListJobs(recorder, request, params)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotNil(t, jobService.lastFilter)
	require.Equal(t, 2, jobService.lastFilter.Page)
	require.Equal(t, 2, jobService.lastFilter.Limit)
	require.NotNil(t, jobService.lastFilter.Status)
	require.Equal(t, common.JobStatusRunning, *jobService.lastFilter.Status)
	require.NotNil(t, jobService.lastFilter.Fuzzer)
	require.Equal(t, "afl++", *jobService.lastFilter.Fuzzer)

	var response generated.JobListResponse
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	require.Len(t, response.Data, 2)
	require.Equal(t, 2, response.Pagination.Limit)
	require.Equal(t, 2, response.Pagination.Offset)
	require.Equal(t, 2, response.Pagination.Total)
	require.True(t, response.Pagination.HasMore)
}
