package aflplusplus

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCoverageExtractor(t *testing.T) {
	tests := []struct {
		name string
		log  logrus.FieldLogger
	}{
		{
			name: "with_logger",
			log:  logrus.New(),
		},
		{
			name: "with_nil_logger",
			log:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewCoverageExtractor(tt.log)
			assert.NotNil(t, extractor)
			assert.NotNil(t, extractor.log)
		})
	}
}

func TestExtractBitmapCoverage(t *testing.T) {
	tests := []struct {
		name      string
		outputDir string
		setupFunc func(string) error
		wantErr   bool
		wantEdges bool
	}{
		{
			name:      "empty_output_dir",
			outputDir: "",
			wantErr:   true,
		},
		{
			name:      "with_bitmap_file",
			outputDir: "test_output",
			setupFunc: createTestBitmapFile,
			wantEdges: true,
		},
		{
			name:      "with_fuzzer_stats_only",
			outputDir: "test_output_stats",
			setupFunc: createTestFuzzerStatsFile,
			wantEdges: true,
		},
		{
			name:      "nonexistent_directory",
			outputDir: "nonexistent",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewCoverageExtractor(nil)
			ctx := context.Background()

			// Setup test environment
			if tt.setupFunc != nil {
				tempDir, err := os.MkdirTemp("", tt.outputDir)
				require.NoError(t, err)
				defer os.RemoveAll(tempDir)

				err = tt.setupFunc(tempDir)
				require.NoError(t, err)
				tt.outputDir = tempDir
			}

			data, err := extractor.ExtractBitmapCoverage(ctx, tt.outputDir)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, data)
			assert.Equal(t, tt.outputDir, data.OutputDir)
			assert.NotZero(t, data.CollectedAt)

			if tt.wantEdges {
				assert.Greater(t, data.Edges, uint64(0))
			}
		})
	}
}

func TestConvertToLCOV(t *testing.T) {
	tests := []struct {
		name         string
		data         *CoverageData
		targetBinary string
		wantErr      bool
		wantContent  bool
	}{
		{
			name:    "nil_data",
			data:    nil,
			wantErr: true,
		},
		{
			name: "empty_target_binary",
			data: &CoverageData{
				Edges: 10,
			},
			targetBinary: "",
			wantErr:      true,
		},
		{
			name: "valid_data",
			data: &CoverageData{
				Edges:           50,
				TotalEdges:      1000,
				CoveragePercent: 5.0,
				OutputDir:       "",
			},
			targetBinary: "/test/binary",
			wantContent:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewCoverageExtractor(nil)
			ctx := context.Background()

			content, err := extractor.ConvertToLCOV(ctx, tt.data, tt.targetBinary)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.wantContent {
				assert.Contains(t, content, "TN:AFL++ Coverage Report")
				assert.Contains(t, content, "SF:"+tt.targetBinary)
				assert.Contains(t, content, "end_of_record")
				// Check for line coverage entries
				assert.Contains(t, content, "DA:")
				assert.Contains(t, content, "LF:")
				assert.Contains(t, content, "LH:")
			}
		})
	}
}

func TestConvertToJSON(t *testing.T) {
	tests := []struct {
		name        string
		data        *CoverageData
		wantErr     bool
		wantContent bool
	}{
		{
			name:    "nil_data",
			data:    nil,
			wantErr: true,
		},
		{
			name: "valid_data",
			data: &CoverageData{
				Edges:           100,
				TotalEdges:      2000,
				CoveragePercent: 5.0,
				PathsTotal:      50,
				PathsPending:    10,
				QueueSize:       40,
				RunTime:         time.Minute * 5,
				FuzzerVersion:   "4.05c",
				TargetBinary:    "/test/binary",
				OutputDir:       "",
				Metadata:        map[string]string{"test": "value"},
			},
			wantContent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewCoverageExtractor(nil)
			ctx := context.Background()

			content, err := extractor.ConvertToJSON(ctx, tt.data)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.wantContent {
				assert.Contains(t, content, "afl++_coverage")
				assert.Contains(t, content, "edges_covered")
				assert.Contains(t, content, "coverage_percent")
				assert.Contains(t, content, strconv.FormatUint(tt.data.Edges, 10))
				assert.Contains(t, content, "metadata")
			}
		})
	}
}

func TestGenerateCoverageReport(t *testing.T) {
	tests := []struct {
		name         string
		outputDir    string
		targetBinary string
		format       string
		setupFunc    func(string) error
		wantErr      bool
	}{
		{
			name:      "empty_output_dir",
			outputDir: "",
			wantErr:   true,
		},
		{
			name:         "empty_target_binary",
			outputDir:    "test",
			targetBinary: "",
			wantErr:      true,
		},
		{
			name:         "invalid_format",
			outputDir:    "test",
			targetBinary: "/test/binary",
			format:       "invalid",
			wantErr:      true,
		},
		{
			name:         "lcov_format",
			outputDir:    "test_lcov",
			targetBinary: "/test/binary",
			format:       "lcov",
			setupFunc:    createTestBitmapFile,
		},
		{
			name:         "json_format",
			outputDir:    "test_json",
			targetBinary: "/test/binary",
			format:       "json",
			setupFunc:    createTestBitmapFile,
		},
		{
			name:         "both_formats",
			outputDir:    "test_both",
			targetBinary: "/test/binary",
			format:       "both",
			setupFunc:    createTestBitmapFile,
		},
		{
			name:         "auto_format",
			outputDir:    "test_auto",
			targetBinary: "/test/binary",
			format:       "auto",
			setupFunc:    createTestBitmapFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewCoverageExtractor(nil)
			ctx := context.Background()

			var outputDir = tt.outputDir

			// Setup test environment
			if tt.setupFunc != nil {
				tempDir, err := os.MkdirTemp("", tt.outputDir)
				require.NoError(t, err)
				defer os.RemoveAll(tempDir)

				err = tt.setupFunc(tempDir)
				require.NoError(t, err)
				outputDir = tempDir
			}

			data, err := extractor.GenerateCoverageReport(ctx, outputDir, tt.targetBinary, tt.format)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, data)
			assert.Equal(t, outputDir, data.OutputDir)
			assert.Equal(t, tt.targetBinary, data.TargetBinary)

			// Check generated files based on format
			switch tt.format {
			case "lcov":
				if data.LCOVFile != "" {
					assert.FileExists(t, data.LCOVFile)
				}
			case "json":
				if data.JSONFile != "" {
					assert.FileExists(t, data.JSONFile)
				}
			case "both", "auto", "":
				if data.LCOVFile != "" {
					assert.FileExists(t, data.LCOVFile)
				}
				if data.JSONFile != "" {
					assert.FileExists(t, data.JSONFile)
				}
			}
		})
	}
}

func TestAnalyzeBitmap(t *testing.T) {
	tests := []struct {
		name    string
		bitmap  []byte
		wantErr bool
	}{
		{
			name:    "empty_bitmap",
			bitmap:  []byte{},
			wantErr: true,
		},
		{
			name:   "valid_bitmap",
			bitmap: []byte{0xFF, 0x00, 0x0F, 0xF0}, // Mixed pattern
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := NewCoverageExtractor(nil)
			data := &CoverageData{
				Bitmap:   tt.bitmap,
				Metadata: make(map[string]string),
			}

			err := extractor.analyzeBitmap(data)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Greater(t, data.TotalEdges, uint64(0))
			assert.GreaterOrEqual(t, data.CoveragePercent, float64(0))
			assert.LessOrEqual(t, data.CoveragePercent, float64(100))
			assert.NotEmpty(t, data.Metadata["bitmap_size_bytes"])
		})
	}
}

func TestContextCancellation(t *testing.T) {
	extractor := NewCoverageExtractor(nil)

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tempDir, err := os.MkdirTemp("", "test_ctx")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// This should return context.Canceled error
	_, err = extractor.ExtractBitmapCoverage(ctx, tempDir)
	assert.Error(t, err)
}

// Helper functions for setting up test data

func createTestBitmapFile(outputDir string) error {
	// Create a test bitmap file with some coverage data
	bitmap := make([]byte, 1024)
	// Set some bits to simulate coverage
	for i := 0; i < len(bitmap); i += 4 {
		bitmap[i] = 0xFF // Set every 4th byte
	}

	bitmapPath := filepath.Join(outputDir, "fuzz_bitmap")
	return os.WriteFile(bitmapPath, bitmap, 0644)
}

func createTestFuzzerStatsFile(outputDir string) error {
	stats := `start_time        : 1672531200
	last_update       : 1672531500
	fuzzer_pid        : 12345
	cycles_done       : 5
	execs_done        : 10000
	execs_per_sec     : 100.50
	paths_total       : 150
	paths_favored     : 50
	paths_found       : 100
	paths_imported    : 25
	max_depth         : 10
	cur_path          : 75
	pending_favs      : 10
	pending_total     : 20
	variable_paths    : 5
	stability         : 100.00%
	bitmap_cvg        : 5.25%
	unique_crashes    : 3
	unique_hangs      : 1
	last_path         : 1672531450
	last_crash        : 1672531400
	last_hang         : 0
	execs_since_crash : 500
	exec_timeout      : 1000
	slowest_exec_ms   : 50
	peak_rss_mb       : 125
	afl_version       : 4.05c`

	statsPath := filepath.Join(outputDir, "fuzzer_stats")
	return os.WriteFile(statsPath, []byte(stats), 0644)
}

// Benchmark tests

func BenchmarkExtractBitmapCoverage(b *testing.B) {
	extractor := NewCoverageExtractor(nil)
	ctx := context.Background()

	// Setup test directory
	tempDir, err := os.MkdirTemp("", "benchmark")
	require.NoError(b, err)
	defer os.RemoveAll(tempDir)

	err = createTestBitmapFile(tempDir)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := extractor.ExtractBitmapCoverage(ctx, tempDir)
		require.NoError(b, err)
	}
}

func BenchmarkConvertToLCOV(b *testing.B) {
	extractor := NewCoverageExtractor(nil)
	ctx := context.Background()

	data := &CoverageData{
		Edges:           1000,
		TotalEdges:      10000,
		CoveragePercent: 10.0,
		Metadata:        make(map[string]string),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := extractor.ConvertToLCOV(ctx, data, "/test/binary")
		require.NoError(b, err)
	}
}

func BenchmarkConvertToJSON(b *testing.B) {
	extractor := NewCoverageExtractor(nil)
	ctx := context.Background()

	data := &CoverageData{
		Edges:           1000,
		TotalEdges:      10000,
		CoveragePercent: 10.0,
		PathsTotal:      500,
		PathsPending:    50,
		QueueSize:       450,
		RunTime:         time.Hour,
		FuzzerVersion:   "4.05c",
		Metadata:        make(map[string]string),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := extractor.ConvertToJSON(ctx, data)
		require.NoError(b, err)
	}
}

// Test edge cases and error conditions

func TestEdgeCases(t *testing.T) {
	extractor := NewCoverageExtractor(nil)
	_ = context.Background() // unused in some test cases

	t.Run("large_bitmap", func(t *testing.T) {
		// Test with large bitmap (64KB like AFL++)
		largeBitmap := make([]byte, 65536)
		// Set random bits
		for i := 0; i < len(largeBitmap); i += 100 {
			largeBitmap[i] = 0x55 // Alternating pattern
		}

		data := &CoverageData{
			Bitmap:   largeBitmap,
			Metadata: make(map[string]string),
		}

		err := extractor.analyzeBitmap(data)
		require.NoError(t, err)
		assert.Equal(t, uint64(65536*8), data.TotalEdges)
		assert.Greater(t, data.Edges, uint64(0))
	})

	t.Run("zero_coverage", func(t *testing.T) {
		// Test with all-zero bitmap
		zeroBitmap := make([]byte, 1024)
		data := &CoverageData{
			Bitmap:   zeroBitmap,
			Metadata: make(map[string]string),
		}

		err := extractor.analyzeBitmap(data)
		require.NoError(t, err)
		assert.Equal(t, uint64(0), data.Edges)
		assert.Equal(t, float64(0), data.CoveragePercent)
	})

	t.Run("full_coverage", func(t *testing.T) {
		// Test with all-set bitmap
		fullBitmap := make([]byte, 128)
		for i := range fullBitmap {
			fullBitmap[i] = 0xFF
		}

		data := &CoverageData{
			Bitmap:   fullBitmap,
			Metadata: make(map[string]string),
		}

		err := extractor.analyzeBitmap(data)
		require.NoError(t, err)
		assert.Equal(t, uint64(128*8), data.TotalEdges)
		assert.Equal(t, data.Edges, data.TotalEdges)
		assert.Equal(t, float64(100), data.CoveragePercent)
	})
}

// Test with different AFL++ output directory structures

func TestDifferentOutputStructures(t *testing.T) {
	extractor := NewCoverageExtractor(nil)
	ctx := context.Background()

	structures := []struct {
		name     string
		setupDir func(string) error
	}{
		{
			name:     "main_node",
			setupDir: setupMainNodeStructure,
		},
		{
			name:     "secondary_node",
			setupDir: setupSecondaryNodeStructure,
		},
		{
			name:     "default_structure",
			setupDir: createTestBitmapFile,
		},
	}

	for _, s := range structures {
		t.Run(s.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", s.name)
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			err = s.setupDir(tempDir)
			require.NoError(t, err)

			data, err := extractor.ExtractBitmapCoverage(ctx, tempDir)
			require.NoError(t, err)
			assert.NotNil(t, data)
			assert.Greater(t, data.Edges, uint64(0))
		})
	}
}

func setupMainNodeStructure(outputDir string) error {
	mainDir := filepath.Join(outputDir, "main")
	if err := os.MkdirAll(mainDir, 0755); err != nil {
		return err
	}

	// Create bitmap file in main directory
	bitmap := make([]byte, 1024)
	for i := 0; i < len(bitmap); i += 2 {
		bitmap[i] = 0xAA
	}
	bitmapPath := filepath.Join(mainDir, "fuzz_bitmap")
	if err := os.WriteFile(bitmapPath, bitmap, 0644); err != nil {
		return err
	}

	// Create fuzzer_stats in main directory
	stats := strings.ReplaceAll(`start_time:1672531200
	paths_total:200
	pending_paths:25
	afl_version:4.05c`, "\t", "")

	statsPath := filepath.Join(mainDir, "fuzzer_stats")
	return os.WriteFile(statsPath, []byte(stats), 0644)
}

func setupSecondaryNodeStructure(outputDir string) error {
	secondaryDir := filepath.Join(outputDir, "secondary")
	if err := os.MkdirAll(secondaryDir, 0755); err != nil {
		return err
	}

	// Create bitmap file in secondary directory
	bitmap := make([]byte, 512)
	for i := 0; i < len(bitmap); i += 3 {
		bitmap[i] = 0x77
	}
	bitmapPath := filepath.Join(secondaryDir, "fuzz_bitmap")
	if err := os.WriteFile(bitmapPath, bitmap, 0644); err != nil {
		return err
	}

	// Create fuzzer_stats in secondary directory
	stats := strings.ReplaceAll(`start_time:1672531200
	paths_total:150
	pending_paths:15
	afl_version:4.05c`, "\t", "")

	statsPath := filepath.Join(secondaryDir, "fuzzer_stats")
	return os.WriteFile(statsPath, []byte(stats), 0644)
}
