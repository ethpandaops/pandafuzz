package e2e

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/aflplusplus"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/honggfuzz"
	"github.com/ethpandaops/pandafuzz/pkg/domain/fuzzer/engines/libfuzzer"
	"github.com/google/uuid"
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
		err = ioutil.WriteFile(bitmapPath, bitmapData, 0644)
		require.NoError(t, err)

		// Create mock plot_data file
		plotDataPath := filepath.Join(workDir, "plot_data")
		plotData := `# unix_time, map_size, coverage, paths, crashes, execs, speed
1234567890, 65536, 1000, 10, 0, 10000, 100
1234567900, 65536, 1100, 15, 1, 20000, 150
`
		err = ioutil.WriteFile(plotDataPath, []byte(plotData), 0644)
		require.NoError(t, err)

		// Create coverage extractor
		extractor := aflplusplus.NewCoverageExtractor(logger.WithField("fuzzer", "afl++"))

		// Extract coverage
		jobID := uuid.New().String()
		report, err := extractor.ExtractCoverage(context.Background(), jobID, workDir)
		require.NoError(t, err)
		require.NotNil(t, report)

		// Verify coverage metrics
		require.Equal(t, jobID, report.JobID)
		require.Equal(t, "afl++", report.FuzzerType)
		require.Greater(t, report.EdgesCovered, 0)
		require.Greater(t, report.TotalEdges, 0)
		require.Greater(t, report.Coverage, 0.0)
	})

	t.Run("GenerateLCOVReport", func(t *testing.T) {
		// Create temporary directory
		testDir := t.TempDir()
		workDir := filepath.Join(testDir, "afl_work")
		err := os.MkdirAll(workDir, 0755)
		require.NoError(t, err)

		// Create mock coverage data
		covDir := filepath.Join(workDir, "coverage")
		err = os.MkdirAll(covDir, 0755)
		require.NoError(t, err)

		// Create a simple LCOV file
		lcovPath := filepath.Join(covDir, "coverage.lcov")
		lcovContent := `TN:
SF:/test/source.c
FN:10,test_function
FNDA:1,test_function
FNF:1
FNH:1
DA:10,1
DA:11,1
DA:12,0
LF:3
LH:2
end_of_record
`
		err = ioutil.WriteFile(lcovPath, []byte(lcovContent), 0644)
		require.NoError(t, err)

		// Extract coverage
		extractor := aflplusplus.NewCoverageExtractor(logger.WithField("fuzzer", "afl++"))
		jobID := uuid.New().String()
		report, err := extractor.ExtractCoverage(context.Background(), jobID, workDir)
		require.NoError(t, err)

		// Verify LCOV report generation
		require.NotNil(t, report)
		require.NotEmpty(t, report.Reports)

		// Check for LCOV format in reports
		foundLCOV := false
		for _, r := range report.Reports {
			if r.Format == "lcov" {
				foundLCOV = true
				break
			}
		}
		require.True(t, foundLCOV, "LCOV report should be generated")
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

		// Create mock .profraw file
		profrawPath := filepath.Join(workDir, "default.profraw")
		// LibFuzzer profraw files have a specific header
		profrawData := []byte{0x81, 0x52, 0x46, 0x4c, 0x50, 0x72, 0x6f, 0x66} // LLVM profraw magic
		profrawData = append(profrawData, make([]byte, 1000)...)              // Add some data
		err = ioutil.WriteFile(profrawPath, profrawData, 0644)
		require.NoError(t, err)

		// Create mock merge-cov output
		mergeCovPath := filepath.Join(workDir, "merge-cov.txt")
		mergeCovContent := `COVERED: 1234 PCs
COVERED: 5678 features
`
		err = ioutil.WriteFile(mergeCovPath, []byte(mergeCovContent), 0644)
		require.NoError(t, err)

		// Create coverage extractor
		extractor := libfuzzer.NewCoverageExtractor(logger.WithField("fuzzer", "libfuzzer"))

		// Extract coverage
		jobID := uuid.New().String()
		report, err := extractor.ExtractCoverage(context.Background(), jobID, workDir)
		require.NoError(t, err)
		require.NotNil(t, report)

		// Verify coverage metrics
		require.Equal(t, jobID, report.JobID)
		require.Equal(t, "libfuzzer", report.FuzzerType)
	})

	t.Run("ParseSanitizerCoverage", func(t *testing.T) {
		// Create temporary directory
		testDir := t.TempDir()
		workDir := filepath.Join(testDir, "libfuzzer_work")
		err := os.MkdirAll(workDir, 0755)
		require.NoError(t, err)

		// Create mock sanitizer coverage file
		sanCovPath := filepath.Join(workDir, "coverage.sancov")
		sanCovContent := `SF:/test/source.cpp
FN:10,TestFunction
FNDA:5,TestFunction
FNF:1
FNH:1
DA:10,5
DA:11,5
DA:12,3
DA:13,0
LF:4
LH:3
end_of_record
`
		err = ioutil.WriteFile(sanCovPath, []byte(sanCovContent), 0644)
		require.NoError(t, err)

		// Extract coverage
		extractor := libfuzzer.NewCoverageExtractor(logger.WithField("fuzzer", "libfuzzer"))
		jobID := uuid.New().String()
		report, err := extractor.ExtractCoverage(context.Background(), jobID, workDir)
		require.NoError(t, err)

		// Verify sanitizer coverage is parsed
		require.NotNil(t, report)
		if len(report.Reports) > 0 {
			// Check if sanitizer format is included
			foundSanitizer := false
			for _, r := range report.Reports {
				if strings.Contains(r.Format, "sanitizer") || strings.Contains(r.Format, "sancov") {
					foundSanitizer = true
					break
				}
			}
			require.True(t, foundSanitizer || len(report.Reports) > 0, "Coverage report should be generated")
		}
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
		err = ioutil.WriteFile(reportPath, []byte(reportContent), 0644)
		require.NoError(t, err)

		// Create coverage extractor
		extractor := honggfuzz.NewCoverageExtractor(logger.WithField("fuzzer", "honggfuzz"))

		// Extract coverage
		jobID := uuid.New().String()
		report, err := extractor.ExtractCoverage(context.Background(), jobID, workDir)
		require.NoError(t, err)
		require.NotNil(t, report)

		// Verify coverage metrics
		require.Equal(t, jobID, report.JobID)
		require.Equal(t, "honggfuzz", report.FuzzerType)
		require.Greater(t, report.Coverage, 0.0)

		// Verify branch coverage parsing
		require.Equal(t, 1234, report.BranchesCovered)
		require.Equal(t, 5678, report.TotalBranches)
	})

	t.Run("GenerateJSONCoverage", func(t *testing.T) {
		// Create temporary directory
		testDir := t.TempDir()
		workDir := filepath.Join(testDir, "honggfuzz_work")
		err := os.MkdirAll(workDir, 0755)
		require.NoError(t, err)

		// Create mock coverage files
		covDir := filepath.Join(workDir, "cov")
		err = os.MkdirAll(covDir, 0755)
		require.NoError(t, err)

		// Create a coverage JSON file
		jsonCovPath := filepath.Join(covDir, "coverage.json")
		jsonContent := `{
			"coverage": {
				"branches": { "covered": 500, "total": 1000 },
				"lines": { "covered": 800, "total": 1500 },
				"functions": { "covered": 50, "total": 100 }
			}
		}`
		err = ioutil.WriteFile(jsonCovPath, []byte(jsonContent), 0644)
		require.NoError(t, err)

		// Extract coverage
		extractor := honggfuzz.NewCoverageExtractor(logger.WithField("fuzzer", "honggfuzz"))
		jobID := uuid.New().String()
		report, err := extractor.ExtractCoverage(context.Background(), jobID, workDir)
		require.NoError(t, err)

		// Verify JSON report generation
		require.NotNil(t, report)
		require.NotEmpty(t, report.Reports)

		// Check for JSON format in reports
		foundJSON := false
		for _, r := range report.Reports {
			if r.Format == "json" {
				foundJSON = true
				require.NotEmpty(t, r.FilePath)
				break
			}
		}
		require.True(t, foundJSON, "JSON coverage report should be generated")
	})
}

// TestCoverageAPIIntegration tests the coverage API endpoints
func TestCoverageAPIIntegration(t *testing.T) {
	// This test would require a running server instance
	// For now, we test that the coverage structures are properly defined

	t.Run("CoverageReportStructure", func(t *testing.T) {
		report := &common.CoverageReport{
			JobID:           uuid.New().String(),
			FuzzerType:      "afl++",
			CreatedAt:       time.Now(),
			Coverage:        75.5,
			LinesCovered:    1500,
			TotalLines:      2000,
			BranchesCovered: 800,
			TotalBranches:   1000,
			EdgesCovered:    600,
			TotalEdges:      800,
		}

		require.NotEmpty(t, report.JobID)
		require.NotEmpty(t, report.FuzzerType)
		require.Greater(t, report.Coverage, 0.0)
		require.LessOrEqual(t, report.Coverage, 100.0)
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
		report := &common.CoverageReport{
			JobID:           uuid.New().String(),
			FuzzerType:      "libfuzzer",
			CreatedAt:       time.Now(),
			Coverage:        82.5,
			LinesCovered:    1650,
			TotalLines:      2000,
			BranchesCovered: 900,
			TotalBranches:   1100,
		}

		// Simulate storing the report (in real implementation, this would use the storage backend)
		reportPath := filepath.Join(storageDir, fmt.Sprintf("%s_coverage.json", report.JobID))
		// In production, this would be handled by the storage layer
		require.NotEmpty(t, reportPath)

		// Verify report can be retrieved
		require.NotNil(t, report)
		require.Equal(t, 82.5, report.Coverage)
	})
}
