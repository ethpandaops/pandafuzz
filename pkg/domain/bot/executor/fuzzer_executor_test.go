package executor

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
	fuzzertypes "github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/types"
	jobtypes "github.com/ethpandaops/pandafuzz/pkg/domain/job/types"
)

type fakeFuzzer struct {
	stats           *fuzzertypes.FuzzerStats
	crashCh         chan *fuzzertypes.CrashInfo
	progressCh      chan *fuzzertypes.ProgressUpdate
	stopAfter       time.Duration
	running         atomic.Bool
	configureCalled bool
	corpusPath      string
	outputPath      string
}

func newFakeFuzzer(stopAfter time.Duration) *fakeFuzzer {
	return &fakeFuzzer{
		stats: &fuzzertypes.FuzzerStats{
			TotalExecutions: 100,
			ExecsPerSecond:  10,
			CorpusSize:      5,
			Coverage:        12.5,
			CrashesFound:    2,
		},
		crashCh:    make(chan *fuzzertypes.CrashInfo),
		progressCh: make(chan *fuzzertypes.ProgressUpdate),
		stopAfter:  stopAfter,
	}
}

func (f *fakeFuzzer) Start(ctx context.Context) error {
	f.running.Store(true)
	if f.stopAfter > 0 {
		go func() {
			time.Sleep(f.stopAfter)
			f.running.Store(false)
		}()
	}
	return nil
}

func (f *fakeFuzzer) Stop() error {
	f.running.Store(false)
	return nil
}

func (f *fakeFuzzer) GetStats() (*fuzzertypes.FuzzerStats, error) {
	return f.stats, nil
}

func (f *fakeFuzzer) GetCrashes() <-chan *fuzzertypes.CrashInfo {
	return f.crashCh
}

func (f *fakeFuzzer) GetProgress() <-chan *fuzzertypes.ProgressUpdate {
	return f.progressCh
}

func (f *fakeFuzzer) IsRunning() bool {
	return f.running.Load()
}

func (f *fakeFuzzer) GetType() string {
	return "afl++"
}

func (f *fakeFuzzer) GetVersion() string {
	return "test"
}

func (f *fakeFuzzer) SetCorpus(path string) error {
	f.corpusPath = path
	return nil
}

func (f *fakeFuzzer) SetOutput(path string) error {
	f.outputPath = path
	return nil
}

func (f *fakeFuzzer) Configure(config *fuzzertypes.FuzzerConfig) error {
	f.configureCalled = true
	return nil
}

type fakeFuzzerFactory struct {
	fuzzer    fuzzertypes.Fuzzer
	supported bool
	lastType  string
	lastTarget string
	lastArgs  []string
}

func (f *fakeFuzzerFactory) CreateFuzzer(fuzzerType string, target string, args []string) (fuzzertypes.Fuzzer, error) {
	f.lastType = fuzzerType
	f.lastTarget = target
	f.lastArgs = args
	return f.fuzzer, nil
}

func (f *fakeFuzzerFactory) GetSupportedTypes() []string {
	if !f.supported {
		return nil
	}
	return []string{"afl++"}
}

func (f *fakeFuzzerFactory) IsSupported(fuzzerType string) bool {
	return f.supported && fuzzerType == "afl++"
}

type fakeRegistry struct {
	assignCalls   int
	completeCalls int
	failCalls     int
	results       map[string]interface{}
}

func (r *fakeRegistry) AssignWork(ctx context.Context, botID, jobID, jobType string, metadata map[string]interface{}) error {
	r.assignCalls++
	return nil
}

func (r *fakeRegistry) CompleteWork(ctx context.Context, botID, jobID string, results map[string]interface{}) error {
	r.completeCalls++
	r.results = results
	return nil
}

func (r *fakeRegistry) FailWork(ctx context.Context, botID, jobID, errorMsg, reason string) error {
	r.failCalls++
	return nil
}

func TestFuzzerExecutor_Execute_Success(t *testing.T) {
	t.Parallel()

	config := &ExecutorConfig{
		JobTimeout:        100 * time.Millisecond,
		HeartbeatInterval: 5 * time.Millisecond,
	}

	fuzzer := newFakeFuzzer(10 * time.Millisecond)
	factory := &fakeFuzzerFactory{
		fuzzer:    fuzzer,
		supported: true,
	}
	registry := &fakeRegistry{}

	exec, err := NewFuzzerExecutor(config, nil, factory, registry, nil)
	require.NoError(t, err)

	bot, err := types.NewAgent("bot-1", "Bot One", []types.Capability{types.CapabilityFuzzing})
	require.NoError(t, err)

	job, err := jobtypes.NewJob("job", "afl++", "/bin/true", "/tmp/corpus", "/tmp/output")
	require.NoError(t, err)
	job.Status = jobtypes.StatusQueued
	job.UpdatedAt = time.Now()

	err = exec.Execute(context.Background(), bot, job)
	require.NoError(t, err)

	require.Equal(t, 1, registry.assignCalls)
	require.Equal(t, 1, registry.completeCalls)
	require.Equal(t, 0, registry.failCalls)
	require.True(t, fuzzer.configureCalled)
	require.Equal(t, "/tmp/corpus", fuzzer.corpusPath)
	require.Equal(t, "/tmp/output", fuzzer.outputPath)
	require.Equal(t, "afl++", factory.lastType)
	require.Equal(t, "/bin/true", factory.lastTarget)
	require.NotNil(t, registry.results)
	require.Equal(t, uint64(2), registry.results["crashes_found"])
	require.Equal(t, float64(12.5), registry.results["coverage"])
}

func TestFuzzerExecutor_Execute_InvalidBotCapability(t *testing.T) {
	t.Parallel()

	exec, err := NewFuzzerExecutor(&ExecutorConfig{JobTimeout: time.Second}, nil, &fakeFuzzerFactory{}, &fakeRegistry{}, nil)
	require.NoError(t, err)

	bot, err := types.NewAgent("bot-2", "Bot Two", []types.Capability{types.CapabilityAnalysis})
	require.NoError(t, err)

	job, err := jobtypes.NewJob("job", "afl++", "/bin/true", "/tmp/corpus", "/tmp/output")
	require.NoError(t, err)
	job.Status = jobtypes.StatusQueued
	job.UpdatedAt = time.Now()

	err = exec.Execute(context.Background(), bot, job)
	require.Error(t, err)
}
