package adapters

import (
	"context"
	"errors"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/job/scheduler"
	"github.com/ethpandaops/pandafuzz/pkg/service"
)

var errNotImplemented = errors.New("not implemented")

type stubJobService struct {
	listFn     func(ctx context.Context, filter service.JobFilter) ([]*common.Job, error)
	lastFilter *service.JobFilter
}

func (s *stubJobService) Start(ctx context.Context) error {
	return nil
}

func (s *stubJobService) Stop() error {
	return nil
}

func (s *stubJobService) CreateJob(ctx context.Context, req service.CreateJobRequest) (*common.Job, error) {
	return nil, errNotImplemented
}

func (s *stubJobService) GetJob(ctx context.Context, jobID string) (*common.Job, error) {
	return nil, errNotImplemented
}

func (s *stubJobService) ListJobs(ctx context.Context, filter service.JobFilter) ([]*common.Job, error) {
	s.lastFilter = &filter
	if s.listFn != nil {
		return s.listFn(ctx, filter)
	}
	return nil, nil
}

func (s *stubJobService) AssignJob(ctx context.Context, botID string) (*common.Job, error) {
	return nil, errNotImplemented
}

func (s *stubJobService) AssignNextJob(ctx context.Context, botID string) (*common.Job, error) {
	return nil, errNotImplemented
}

func (s *stubJobService) CompleteJob(ctx context.Context, jobID, botID string, success bool) error {
	return errNotImplemented
}

func (s *stubJobService) CancelJob(ctx context.Context, jobID string) error {
	return errNotImplemented
}

func (s *stubJobService) GetJobLogs(ctx context.Context, jobID string) ([]string, error) {
	return nil, errNotImplemented
}

func (s *stubJobService) GetJobCorpus(ctx context.Context, jobID string) ([]*common.CorpusFile, error) {
	return nil, errNotImplemented
}

func (s *stubJobService) StreamLogs(ctx context.Context, jobID string) (<-chan string, error) {
	return nil, errNotImplemented
}

func (s *stubJobService) GetLogs(ctx context.Context, jobID string) ([]string, error) {
	return nil, errNotImplemented
}

func (s *stubJobService) GetJobStats(ctx context.Context, jobID string) (*service.JobStats, error) {
	return nil, errNotImplemented
}

func (s *stubJobService) GetJobCrashes(ctx context.Context, jobID string) ([]*common.CrashResult, error) {
	return nil, errNotImplemented
}

func (s *stubJobService) GetQueueStats(ctx context.Context) (*service.QueueStats, error) {
	return nil, errNotImplemented
}

func (s *stubJobService) SetQueue(queue scheduler.Queue) {}

func (s *stubJobService) UpdateJob(ctx context.Context, job *common.Job) error {
	return errNotImplemented
}

func (s *stubJobService) CancelJobExecution(ctx context.Context, jobID string) error {
	return errNotImplemented
}

func (s *stubJobService) GetJobLogsPaginated(ctx context.Context, jobID string, limit, offset int) ([]string, int, error) {
	return nil, 0, errNotImplemented
}

func (s *stubJobService) StoreJobLogs(ctx context.Context, jobID string, logs []string) error {
	return errNotImplemented
}

func (s *stubJobService) GetJobCrashesPaginated(ctx context.Context, jobID string, limit, offset int) ([]*common.CrashResult, int, error) {
	return nil, 0, errNotImplemented
}

func (s *stubJobService) GetCoverageReport(ctx context.Context, jobID string) (*service.CoverageReport, error) {
	return nil, errNotImplemented
}

func (s *stubJobService) GetCoverageFile(ctx context.Context, jobID string, filePath string) ([]byte, error) {
	return nil, errNotImplemented
}

func (s *stubJobService) StoreCoverageFile(ctx context.Context, jobID string, filePath string, data []byte) error {
	return errNotImplemented
}

func (s *stubJobService) ListCoverageFiles(ctx context.Context, jobID string) ([]string, error) {
	return nil, errNotImplemented
}

type stubBotService struct {
	listFn     func(ctx context.Context, statusFilter *common.BotStatus) ([]*common.Bot, error)
	lastStatus *common.BotStatus
}

func (s *stubBotService) Start(ctx context.Context) error {
	return nil
}

func (s *stubBotService) Stop() error {
	return nil
}

func (s *stubBotService) RegisterBot(ctx context.Context, hostname string, name string, capabilities []string, apiEndpoint string) (*common.Bot, error) {
	return nil, errNotImplemented
}

func (s *stubBotService) GetBot(ctx context.Context, botID string) (*common.Bot, error) {
	return nil, errNotImplemented
}

func (s *stubBotService) DeleteBot(ctx context.Context, botID string) error {
	return errNotImplemented
}

func (s *stubBotService) UpdateHeartbeat(ctx context.Context, botID string, status common.BotStatus, currentJob *string) error {
	return errNotImplemented
}

func (s *stubBotService) ListBots(ctx context.Context, statusFilter *common.BotStatus) ([]*common.Bot, error) {
	s.lastStatus = statusFilter
	if s.listFn != nil {
		return s.listFn(ctx, statusFilter)
	}
	return nil, nil
}

func (s *stubBotService) GetAvailableBot(ctx context.Context, requiredCapabilities []string) (*common.Bot, error) {
	return nil, errNotImplemented
}

func (s *stubBotService) DeregisterBot(ctx context.Context, botID string) error {
	return errNotImplemented
}

func (s *stubBotService) Heartbeat(ctx context.Context, botID string) error {
	return errNotImplemented
}

func (s *stubBotService) GetCurrentJob(ctx context.Context, botID string) (*common.Job, error) {
	return nil, errNotImplemented
}

func (s *stubBotService) GetMetrics(ctx context.Context, botID string) (*service.BotMetrics, error) {
	return nil, errNotImplemented
}
