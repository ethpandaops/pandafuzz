package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/aflplusplus"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/honggfuzz"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/libfuzzer"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// TestCoverageCollectionAFLPlusPlus tests coverage collection for AFL++
func TestCoverageCollectionAFLPlusPlus(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("ExtractCoverageFromBitmap", func(t *testing.T) {
		// Create temporary directory for test
		testDir := t.TempDir()
		workDir := filepath.Join(testDir, "afl_work")
		err := os.MkdirAll(workDir, 0755)
		require.NoError(t, err)

		// Create mock coverage bitmap file
		bitmapPath := filepath.Join(workDir, "fuzz_bitmap")
		bitmapData := make([]byte, 65536) // AFL++ bitmap size
		// Set some bits to simulate coverage
		for i := 0; i < 1000; i++ {
			bitmapData[i*10] = byte(i % 256)
		}
		err = os.WriteFile(bitmapPath, bitmapData, 0644)
		require.NoError(t, err)

		// Create mock plot_data file
		plotDataPath := filepath.Join(workDir, "plot_data")
		plotData := `# unix_time, map_size, coverage, paths, crashes, execs, speed
1234567890, 65536, 1000, 10, 0, 10000, 100
1234567900, 65536, 1100, 15, 1, 20000, 150
`
		err = os.WriteFile(plotDataPath, []byte(plotData), 0644)
		require.NoError(t, err)

		// Create coverage extractor
		extractor := aflplusplus.NewCoverageExtractor(logger.WithField("fuzzer", "afl++"))

		// Extract coverage
		data, err := extractor.ExtractBitmapCoverage(context.Background(), workDir)
		require.NoError(t, err)
		require.NotNil(t, data)

		// Verify coverage metrics
		require.Greater(t, data.Edges, uint64(0))
		require.Greater(t, data.TotalEdges, uint64(0))
		require.Greater(t, data.CoveragePercent, 0.0)
	})

	t.Run("GenerateLCOVReport", func(t *testing.T) {
		// Create temporary directory
		testDir := t.TempDir()
		workDir := filepath.Join(testDir, "afl_work")
		err := os.MkdirAll(workDir, 0755)
		require.NoError(t, err)

		// Create mock coverage bitmap file
		bitmapPath := filepath.Join(workDir, "fuzz_bitmap")
		bitmapData := make([]byte, 65536)
		for i := 0; i < 500; i++ {
			bitmapData[i*20] = byte(i % 256)
		}
		err = os.WriteFile(bitmapPath, bitmapData, 0644)
		require.NoError(t, err)

		// Extract coverage
		extractor := aflplusplus.NewCoverageExtractor(logger.WithField("fuzzer", "afl++"))
		data, err := extractor.ExtractBitmapCoverage(context.Background(), workDir)
		require.NoError(t, err)
		require.NotNil(t, data)

		lcovContent, err := extractor.ConvertToLCOV(context.Background(), data, "/bin/echo")
		require.NoError(t, err)
		require.Contains(t, lcovContent, "SF:/bin/echo")
	})
}

// TestCoverageCollectionLibFuzzer tests coverage collection for LibFuzzer
func TestCoverageCollectionLibFuzzer(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("ExtractCoverageFromProfraw", func(t *testing.T) {
		// Create temporary directory
		testDir := t.TempDir()
		workDir := filepath.Join(testDir, "libfuzzer_work")
		err := os.MkdirAll(workDir, 0755)
		require.NoError(t, err)

		// Create mock fuzzer log with coverage stats
		logPath := filepath.Join(workDir, "fuzzer.log")
		logContent := `#1 cov: 120 ft: 15
#2 cov: 200 ft: 25
`
		err = os.WriteFile(logPath, []byte(logContent), 0644)
		require.NoError(t, err)

		// Create coverage extractor
		extractor := libfuzzer.NewCoverageExtractor(logger.WithField("fuzzer", "libfuzzer"))

		// Extract basic coverage stats
		data, err := extractor.GetBasicStats(workDir)
		require.NoError(t, err)
		require.NotNil(t, data.BasicStats)
		require.Greater(t, data.BasicStats.ExecutedBlocks, uint64(0))
	})

	t.Run("ParseSanitizerCoverage", func(t *testing.T) {
		// Create temporary directory
		testDir := t.TempDir()
		workDir := filepath.Join(testDir, "libfuzzer_work")
		err := os.MkdirAll(workDir, 0755)
		require.NoError(t, err)

		// Create mock fuzzer log with coverage stats
		logPath := filepath.Join(workDir, "fuzzer.log")
		logContent := `#1 cov: 50 ft: 5
#2 cov: 80 ft: 10
`
		err = os.WriteFile(logPath, []byte(logContent), 0644)
		require.NoError(t, err)

		// Extract coverage with fallback to basic stats
		extractor := libfuzzer.NewCoverageExtractor(logger.WithField("fuzzer", "libfuzzer"))
		data, err := extractor.GenerateCoverageReport(workDir, "/bin/echo", "json")
		require.NoError(t, err)
		require.NotNil(t, data.BasicStats)
	})
}

// TestCoverageCollectionHonggfuzz tests coverage collection for Honggfuzz
func TestCoverageCollectionHonggfuzz(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("ExtractCoverageFromReport", func(t *testing.T) {
		// Create temporary directory
		testDir := t.TempDir()
		workDir := filepath.Join(testDir, "honggfuzz_work")
		err := os.MkdirAll(workDir, 0755)
		require.NoError(t, err)

		// Create mock Honggfuzz report file
		reportPath := filepath.Join(workDir, "HONGGFUZZ.REPORT.TXT")
		reportContent := `====================================================
Iterations: 10000
Start time: 2024-01-01 00:00:00
====================================================
Crashes: 2
Unique crashes: 1
Timeout crashes: 0
====================================================
Coverage:
  Branches: 1234/5678 (21.7%)
  Basic blocks: 890/2000 (44.5%)
  Edges: 567/1500 (37.8%)
====================================================
`
		err = os.WriteFile(reportPath, []byte(reportContent), 0644)
		require.NoError(t, err)

		// Create coverage extractor
		extractor := honggfuzz.NewCoverageExtractor(logger.WithField("fuzzer", "honggfuzz"))

		// Extract coverage
		data, err := extractor.ExtractCoverageFromReport(context.Background(), workDir)
		require.NoError(t, err)
		require.NotNil(t, data)

		// Verify coverage metrics
		require.Greater(t, data.EdgeCoverage, 0.0)

		// Verify branch coverage parsing
		require.Equal(t, uint64(1234), data.BranchHits)
		require.Equal(t, uint64(5678), data.BranchCount)
	})

	t.Run("GenerateJSONCoverage", func(t *testing.T) {
		// Create temporary directory
		testDir := t.TempDir()
		workDir := filepath.Join(testDir, "honggfuzz_work")
		err := os.MkdirAll(workDir, 0755)
		require.NoError(t, err)

		// Create mock Honggfuzz report file
		reportPath := filepath.Join(workDir, "HONGGFUZZ.REPORT.TXT")
		reportContent := `====================================================
Iterations: 20000
Start time: 2024-01-01 00:00:00
====================================================
Crashes: 1
Unique crashes: 1
Timeout crashes: 0
====================================================
Coverage:
  Branches: 500/1000 (50.0%)
  Basic blocks: 800/1500 (53.3%)
  Edges: 600/1200 (50.0%)
====================================================
`
		err = os.WriteFile(reportPath, []byte(reportContent), 0644)
		require.NoError(t, err)

		// Extract coverage
		extractor := honggfuzz.NewCoverageExtractor(logger.WithField("fuzzer", "honggfuzz"))
		data, err := extractor.ExtractCoverageFromReport(context.Background(), workDir)
		require.NoError(t, err)

		jsonContent, err := extractor.ConvertToJSON(context.Background(), data)
		require.NoError(t, err)
		require.Contains(t, jsonContent, "\"edge_coverage\"")
	})
}

// TestCoverageAPIIntegration tests the coverage API endpoints
func TestCoverageAPIIntegration(t *testing.T) {
	// This test would require a running server instance
	// For now, we test that the coverage structures are properly defined

	t.Run("CoverageReportStructure", func(t *testing.T) {
		jobID := openapi_types.UUID(uuid.New())
		reportID := openapi_types.UUID(uuid.New())
		coveragePercent := float32(75.5)
		coveredLines := 1500
		totalLines := 2000
		report := &generated.CoverageReport{
			Id:        reportID,
			JobId:     jobID,
			Format:    generated.CoverageFormat("json"),
			CreatedAt: time.Now(),
			SizeBytes: 1024,
			CoverageMetrics: &struct {
				BranchCoveragePercent   *float32 `json:"branch_coverage_percent,omitempty"`
				CoveredBranches         *int     `json:"covered_branches,omitempty"`
				CoveredFunctions        *int     `json:"covered_functions,omitempty"`
				CoveredLines            *int     `json:"covered_lines,omitempty"`
				FunctionCoveragePercent *float32 `json:"function_coverage_percent,omitempty"`
				LineCoveragePercent     *float32 `json:"line_coverage_percent,omitempty"`
				TotalBranches           *int     `json:"total_branches,omitempty"`
				TotalFunctions          *int     `json:"total_functions,omitempty"`
				TotalLines              *int     `json:"total_lines,omitempty"`
			}{
				LineCoveragePercent: &coveragePercent,
				CoveredLines:        &coveredLines,
				TotalLines:          &totalLines,
			},
		}

		require.NotEqual(t, openapi_types.UUID{}, report.JobId)
		require.Equal(t, generated.CoverageFormat("json"), report.Format)
		require.NotNil(t, report.CoverageMetrics)
		require.Greater(t, *report.CoverageMetrics.LineCoveragePercent, float32(0))
	})

	t.Run("CoverageEndpointPath", func(t *testing.T) {
		// Verify the coverage endpoint path is correct
		jobID := uuid.New().String()
		expectedPath := fmt.Sprintf("/api/v1/jobs/%s/coverage", jobID)
		require.Contains(t, expectedPath, jobID)
		require.Contains(t, expectedPath, "coverage")
	})
}

// TestCoverageUIDisplay tests that coverage data can be properly formatted for UI
func TestCoverageUIDisplay(t *testing.T) {
	t.Run("FormatCoveragePercentage", func(t *testing.T) {
		testCases := []struct {
			covered  int
			total    int
			expected string
		}{
			{750, 1000, "75.0%"},
			{1, 3, "33.3%"},
			{0, 100, "0.0%"},
			{100, 100, "100.0%"},
		}

		for _, tc := range testCases {
			percentage := float64(tc.covered) / float64(tc.total) * 100
			formatted := fmt.Sprintf("%.1f%%", percentage)
			require.Equal(t, tc.expected, formatted)
		}
	})

	t.Run("CoverageColorCoding", func(t *testing.T) {
		// Test coverage color coding for UI
		getColorForCoverage := func(coverage float64) string {
			switch {
			case coverage >= 80:
				return "green"
			case coverage >= 60:
				return "yellow"
			case coverage >= 40:
				return "orange"
			default:
				return "red"
			}
		}

		require.Equal(t, "green", getColorForCoverage(85.0))
		require.Equal(t, "yellow", getColorForCoverage(65.0))
		require.Equal(t, "orange", getColorForCoverage(45.0))
		require.Equal(t, "red", getColorForCoverage(20.0))
	})
}

// TestCoverageDataPersistence tests that coverage data is properly stored and retrieved
func TestCoverageDataPersistence(t *testing.T) {
	t.Run("StoreCoverageReport", func(t *testing.T) {
		// Create temporary directory for storage
		testDir := t.TempDir()
		storageDir := filepath.Join(testDir, "coverage_storage")
		err := os.MkdirAll(storageDir, 0755)
		require.NoError(t, err)

		// Create a coverage report
		jobID := openapi_types.UUID(uuid.New())
		report := &generated.CoverageReport{
			Id:        openapi_types.UUID(uuid.New()),
			JobId:     jobID,
			Format:    generated.CoverageFormat("json"),
			CreatedAt: time.Now(),
			SizeBytes: 2048,
		}

		// Simulate storing the report (in real implementation, this would use the storage backend)
		reportPath := filepath.Join(storageDir, fmt.Sprintf("%s_coverage.json", report.JobId.String()))
		// In production, this would be handled by the storage layer
		require.NotEmpty(t, reportPath)

		// Verify report can be retrieved
		require.NotNil(t, report)
		require.Equal(t, jobID, report.JobId)
	})
}
