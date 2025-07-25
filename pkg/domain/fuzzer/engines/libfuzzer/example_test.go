package libfuzzer_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/libfuzzer"
	"github.com/ethpandaops/pandafuzz/pkg/fuzzer"
)

// ExampleEngine demonstrates basic usage of the LibFuzzer engine
func ExampleEngine() {
	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create engine
	engine := libfuzzer.NewEngine(logger)

	// Configure
	config := fuzzer.FuzzConfig{
		JobID:         "example-job-001",
		Target:        "/path/to/libfuzzer_target",
		WorkDirectory: "/tmp/libfuzzer-example",
		Duration:      5 * time.Minute,
		MemoryLimit:   1024 * 1024 * 1024, // 1GB
		Timeout:       30 * time.Second,
		Dictionary:    "/path/to/dictionary.txt",
		MaxCrashes:    10,
		FuzzerOptions: map[string]any{
			"jobs":    2,
			"max_len": 500,
		},
	}

	if err := engine.Configure(config); err != nil {
		log.Fatal(err)
	}

	// Initialize
	if err := engine.Initialize(); err != nil {
		log.Fatal(err)
	}

	// Start fuzzing
	ctx := context.Background()
	if err := engine.Start(ctx); err != nil {
		log.Fatal(err)
	}

	// Let it run for a bit
	time.Sleep(30 * time.Second)

	// Get current stats
	stats := engine.GetStats()
	fmt.Printf("Executed %d tests, found %d crashes\n", stats.Executions, stats.UniqueCrashes)

	// Stop
	if err := engine.Stop(); err != nil {
		log.Fatal(err)
	}

	// Get final results
	results, err := engine.GetResults()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Fuzzing completed: %d crashes, %.2f%% coverage\n",
		results.Summary.UniqueCrashes,
		results.Summary.CoverageAchieved)
}

// ExampleEngine_eventHandling demonstrates using custom event handlers
func ExampleEngine_eventHandling() {
	logger := logrus.New()
	engine := libfuzzer.NewEngine(logger)

	// Custom event handler
	handler := &CustomEventHandler{
		logger: logger,
	}
	engine.SetEventHandler(handler)

	// Configure and run...
	// Output would show event handler callbacks
}

// CustomEventHandler implements fuzzer.EventHandler
type CustomEventHandler struct {
	logger logrus.FieldLogger
}

func (h *CustomEventHandler) OnStart(f fuzzer.Fuzzer) {
	h.logger.WithField("fuzzer", f.Name()).Info("Fuzzing started")
}

func (h *CustomEventHandler) OnStop(f fuzzer.Fuzzer, reason string) {
	h.logger.WithFields(logrus.Fields{
		"fuzzer": f.Name(),
		"reason": reason,
	}).Info("Fuzzing stopped")
}

func (h *CustomEventHandler) OnCrash(f fuzzer.Fuzzer, crash *common.CrashResult) {
	h.logger.WithFields(logrus.Fields{
		"fuzzer":   f.Name(),
		"crash_id": crash.ID,
		"type":     crash.Type,
		"size":     crash.Size,
	}).Error("Crash found")
}

func (h *CustomEventHandler) OnNewPath(f fuzzer.Fuzzer, path *fuzzer.CorpusEntry) {
	h.logger.WithFields(logrus.Fields{
		"fuzzer": f.Name(),
		"path":   path.FileName,
		"size":   path.Size,
	}).Debug("New path discovered")
}

func (h *CustomEventHandler) OnStats(f fuzzer.Fuzzer, stats fuzzer.FuzzerStats) {
	h.logger.WithFields(logrus.Fields{
		"fuzzer":      f.Name(),
		"executions":  stats.Executions,
		"exec_per_s":  stats.ExecPerSecond,
		"coverage":    stats.CoveredEdges,
		"crashes":     stats.UniqueCrashes,
		"corpus_size": stats.CorpusSize,
	}).Info("Statistics update")
}

func (h *CustomEventHandler) OnError(f fuzzer.Fuzzer, err error) {
	h.logger.WithFields(logrus.Fields{
		"fuzzer": f.Name(),
		"error":  err,
	}).Error("Fuzzer error")
}

func (h *CustomEventHandler) OnProgress(f fuzzer.Fuzzer, progress fuzzer.FuzzerProgress) {
	h.logger.WithFields(logrus.Fields{
		"fuzzer":   f.Name(),
		"phase":    progress.Phase,
		"progress": progress.ProgressPercent,
	}).Debug("Progress update")
}

// ExampleEngine_crashReproduction demonstrates crash reproduction
func ExampleEngine_crashReproduction() {
	logger := logrus.New()
	engine := libfuzzer.NewEngine(logger)

	// Configure engine first
	config := fuzzer.FuzzConfig{
		Target:        "/path/to/libfuzzer_target",
		WorkDirectory: "/tmp/libfuzzer-repro",
	}
	_ = engine.Configure(config)

	// Reproduce a crash
	crashInput := []byte("CRASH_INPUT_DATA")
	reproConfig := fuzzer.ReproductionConfig{
		OriginalCrashID:  "crash-12345",
		Timeout:          10 * time.Second,
		Attempts:         3,
		CollectDebugInfo: true,
		Environment: map[string]string{
			"ASAN_OPTIONS": "print_stats=1:check_initialization_order=1",
		},
	}

	ctx := context.Background()
	result, err := engine.ReproduceCrash(ctx, crashInput, reproConfig)
	if err != nil {
		log.Fatal(err)
	}

	if result.Reproduced {
		fmt.Printf("Crash reproduced: signal=%d, matches_original=%v\n",
			result.Signal, result.MatchesOriginal)
		fmt.Printf("Stack trace:\n%s\n", result.StackTrace)
	} else {
		fmt.Println("Failed to reproduce crash")
	}
}
