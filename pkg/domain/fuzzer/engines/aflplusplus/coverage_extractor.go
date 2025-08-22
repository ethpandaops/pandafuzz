package aflplusplus

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// CoverageData represents AFL++ coverage information extracted from fuzzing output
type CoverageData struct {
	// Core coverage metrics
	Edges           uint64  `json:"edges"`            // Number of covered edges
	TotalEdges      uint64  `json:"total_edges"`      // Total number of possible edges
	CoveragePercent float64 `json:"coverage_percent"` // Coverage percentage
	Bitmap          []byte  `json:"bitmap,omitempty"` // Raw bitmap data

	// AFL++ specific metrics
	PathsTotal   uint64 `json:"paths_total"`   // Total paths discovered
	PathsPending uint64 `json:"paths_pending"` // Pending paths
	PathsFavored uint64 `json:"paths_favored"` // Favored paths

	// Timing information
	CollectedAt time.Time     `json:"collected_at"` // When coverage was collected
	RunTime     time.Duration `json:"run_time"`     // Total fuzzer runtime

	// Metadata
	FuzzerVersion string            `json:"fuzzer_version"` // AFL++ version
	TargetBinary  string            `json:"target_binary"`  // Target binary path
	OutputDir     string            `json:"output_dir"`     // AFL++ output directory
	QueueSize     uint64            `json:"queue_size"`     // Number of queue files
	Metadata      map[string]string `json:"metadata"`       // Additional metadata

	// Coverage files
	BitmapFile string `json:"bitmap_file,omitempty"` // Path to bitmap file
	LCOVFile   string `json:"lcov_file,omitempty"`   // Generated LCOV file
	JSONFile   string `json:"json_file,omitempty"`   // Generated JSON file
	AFLCovDir  string `json:"afl_cov_dir,omitempty"` // afl-cov output directory
}

// ToReportMap converts CoverageData to a map suitable for API reporting
func (cd *CoverageData) ToReportMap() map[string]interface{} {
	return map[string]interface{}{
		"edges":            cd.Edges,
		"total_edges":      cd.TotalEdges,
		"coverage_percent": cd.CoveragePercent,
		"paths_total":      cd.PathsTotal,
		"paths_pending":    cd.PathsPending,
		"paths_favored":    cd.PathsFavored,
		"fuzzer_version":   cd.FuzzerVersion,
		"queue_size":       cd.QueueSize,
		"run_time_seconds": cd.RunTime.Seconds(),
		"metadata":         cd.Metadata,
	}
}

// CoverageExtractor handles AFL++ coverage extraction and report generation
type CoverageExtractor struct {
	log logrus.FieldLogger
}

// NewCoverageExtractor creates a new coverage extractor instance
func NewCoverageExtractor(log logrus.FieldLogger) *CoverageExtractor {
	if log == nil {
		log = logrus.New()
	}

	return &CoverageExtractor{
		log: log.WithField("component", "afl++_coverage_extractor"),
	}
}

// ExtractBitmapCoverage reads and analyzes AFL++ bitmap coverage from the output directory
func (ce *CoverageExtractor) ExtractBitmapCoverage(ctx context.Context, outputDir string) (*CoverageData, error) {
	if outputDir == "" {
		return nil, errors.New("output directory cannot be empty")
	}

	ce.log.WithField("output_dir", outputDir).Debug("Extracting bitmap coverage")

	data := &CoverageData{
		OutputDir:   outputDir,
		CollectedAt: time.Now(),
		Metadata:    make(map[string]string),
	}

	// Look for bitmap file in various locations
	bitmapPaths := []string{
		filepath.Join(outputDir, "fuzz_bitmap"),
		filepath.Join(outputDir, "main", "fuzz_bitmap"),
		filepath.Join(outputDir, "secondary", "fuzz_bitmap"),
		filepath.Join(outputDir, "default", "fuzz_bitmap"),
	}

	var bitmapFile string
	var bitmapData []byte

	for _, path := range bitmapPaths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if _, err := os.Stat(path); err == nil {
			bitmapFile = path
			var readErr error
			bitmapData, readErr = os.ReadFile(path)
			if readErr != nil {
				ce.log.WithError(readErr).WithField("bitmap_file", path).Warn("Failed to read bitmap file")
				continue
			}
			break
		}
	}

	if bitmapFile == "" {
		ce.log.Debug("No bitmap file found, attempting to extract from fuzzer_stats")
		return ce.extractFromFuzzerStats(ctx, outputDir)
	}

	data.BitmapFile = bitmapFile
	data.Bitmap = bitmapData

	// Calculate bitmap coverage statistics
	if err := ce.analyzeBitmap(data); err != nil {
		return nil, fmt.Errorf("failed to analyze bitmap: %w", err)
	}

	// Extract additional statistics from fuzzer_stats
	if err := ce.extractFuzzerStats(ctx, outputDir, data); err != nil {
		ce.log.WithError(err).Warn("Failed to extract fuzzer stats")
	}

	// Count queue files
	if err := ce.countQueueFiles(ctx, outputDir, data); err != nil {
		ce.log.WithError(err).Warn("Failed to count queue files")
	}

	ce.log.WithFields(logrus.Fields{
		"edges":            data.Edges,
		"total_edges":      data.TotalEdges,
		"coverage_percent": data.CoveragePercent,
		"queue_size":       data.QueueSize,
	}).Info("Successfully extracted bitmap coverage")

	return data, nil
}

// analyzeBitmap analyzes the raw bitmap data to calculate coverage metrics
func (ce *CoverageExtractor) analyzeBitmap(data *CoverageData) error {
	if len(data.Bitmap) == 0 {
		return errors.New("bitmap data is empty")
	}

	totalBits := uint64(len(data.Bitmap) * 8)
	setBits := uint64(0)
	edges := uint64(0)

	// Count set bits and estimate edges
	for _, b := range data.Bitmap {
		for i := 0; i < 8; i++ {
			if (b>>i)&1 == 1 {
				setBits++
				edges++
			}
		}
	}

	data.Edges = edges
	data.TotalEdges = totalBits

	if totalBits > 0 {
		data.CoveragePercent = float64(setBits) / float64(totalBits) * 100.0
	}

	// Store bitmap statistics in metadata
	data.Metadata["bitmap_size_bytes"] = strconv.Itoa(len(data.Bitmap))
	data.Metadata["total_bits"] = strconv.FormatUint(totalBits, 10)
	data.Metadata["set_bits"] = strconv.FormatUint(setBits, 10)
	data.Metadata["bitmap_density"] = fmt.Sprintf("%.2f%%", data.CoveragePercent)

	return nil
}

// extractFromFuzzerStats extracts coverage data from fuzzer_stats file when bitmap is not available
func (ce *CoverageExtractor) extractFromFuzzerStats(ctx context.Context, outputDir string) (*CoverageData, error) {
	data := &CoverageData{
		OutputDir:   outputDir,
		CollectedAt: time.Now(),
		Metadata:    make(map[string]string),
	}

	if err := ce.extractFuzzerStats(ctx, outputDir, data); err != nil {
		return nil, fmt.Errorf("failed to extract from fuzzer_stats: %w", err)
	}

	// Estimate coverage from paths when bitmap is not available
	if data.PathsTotal > 0 && data.PathsPending < data.PathsTotal {
		coveredPaths := data.PathsTotal - data.PathsPending
		data.CoveragePercent = float64(coveredPaths) / float64(data.PathsTotal) * 100.0
		data.Edges = coveredPaths // Approximate edges with covered paths
	}

	ce.log.WithField("source", "fuzzer_stats").Info("Extracted coverage from fuzzer statistics")
	return data, nil
}

// extractFuzzerStats reads fuzzer statistics from fuzzer_stats file
func (ce *CoverageExtractor) extractFuzzerStats(ctx context.Context, outputDir string, data *CoverageData) error {
	statsPaths := []string{
		filepath.Join(outputDir, "fuzzer_stats"),
		filepath.Join(outputDir, "main", "fuzzer_stats"),
		filepath.Join(outputDir, "secondary", "fuzzer_stats"),
		filepath.Join(outputDir, "default", "fuzzer_stats"),
	}

	var statsFile string
	for _, path := range statsPaths {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if _, err := os.Stat(path); err == nil {
			statsFile = path
			break
		}
	}

	if statsFile == "" {
		return errors.New("fuzzer_stats file not found")
	}

	file, err := os.Open(statsFile)
	if err != nil {
		return fmt.Errorf("failed to open fuzzer_stats file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	stats := make(map[string]string)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.Contains(line, ":") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			stats[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading fuzzer_stats: %w", err)
	}

	// Parse relevant statistics
	if val, ok := stats["paths_total"]; ok {
		if parsed, err := strconv.ParseUint(val, 10, 64); err == nil {
			data.PathsTotal = parsed
		}
	}

	if val, ok := stats["pending_paths"]; ok {
		if parsed, err := strconv.ParseUint(val, 10, 64); err == nil {
			data.PathsPending = parsed
		}
	}

	if val, ok := stats["pending_favs"]; ok {
		if parsed, err := strconv.ParseUint(val, 10, 64); err == nil {
			data.PathsFavored = parsed
		}
	}

	if val, ok := stats["run_time"]; ok {
		if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
			data.RunTime = time.Duration(parsed) * time.Second
		}
	}

	if val, ok := stats["afl_version"]; ok {
		data.FuzzerVersion = val
	}

	// Extract edges_found from AFL++ stats
	if val, ok := stats["edges_found"]; ok {
		if parsed, err := strconv.ParseUint(val, 10, 64); err == nil {
			data.Edges = parsed
		}
	}

	// Extract total_edges if available
	if val, ok := stats["total_edges"]; ok {
		if parsed, err := strconv.ParseUint(val, 10, 64); err == nil {
			data.TotalEdges = parsed
			// Calculate coverage percentage from edges
			if data.TotalEdges > 0 && data.Edges > 0 {
				data.CoveragePercent = float64(data.Edges) / float64(data.TotalEdges) * 100.0
			}
		}
	}

	// Extract bitmap_cvg if available
	if val, ok := stats["bitmap_cvg"]; ok {
		// Remove the % sign and parse
		val = strings.TrimSuffix(val, "%")
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			// Only use bitmap_cvg if we don't have edge-based coverage
			if data.CoveragePercent == 0 {
				data.CoveragePercent = parsed
			}
		}
	}

	// Store all stats in metadata
	for k, v := range stats {
		data.Metadata["afl_"+k] = v
	}

	return nil
}

// countQueueFiles counts the number of queue files in the output directory
func (ce *CoverageExtractor) countQueueFiles(ctx context.Context, outputDir string, data *CoverageData) error {
	queuePaths := []string{
		filepath.Join(outputDir, "queue"),
		filepath.Join(outputDir, "main", "queue"),
		filepath.Join(outputDir, "secondary", "queue"),
		filepath.Join(outputDir, "default", "queue"),
	}

	for _, queueDir := range queuePaths {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if _, err := os.Stat(queueDir); err != nil {
			continue
		}

		entries, err := os.ReadDir(queueDir)
		if err != nil {
			ce.log.WithError(err).WithField("queue_dir", queueDir).Debug("Failed to read queue directory")
			continue
		}

		count := uint64(0)
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "id:") {
				count++
			}
		}

		data.QueueSize = count
		data.Metadata["queue_dir"] = queueDir
		break
	}

	return nil
}

// ConvertToLCOV generates a synthetic LCOV format report from coverage data
func (ce *CoverageExtractor) ConvertToLCOV(ctx context.Context, data *CoverageData, targetBinary string) (string, error) {
	if data == nil {
		return "", errors.New("coverage data cannot be nil")
	}

	if targetBinary == "" {
		return "", errors.New("target binary cannot be empty")
	}

	ce.log.WithFields(logrus.Fields{
		"target_binary": targetBinary,
		"edges":         data.Edges,
	}).Debug("Converting coverage to LCOV format")

	data.TargetBinary = targetBinary

	// Generate synthetic LCOV content
	var lcovContent strings.Builder

	// Test name and source file info
	lcovContent.WriteString("TN:AFL++ Coverage Report\n")
	lcovContent.WriteString(fmt.Sprintf("SF:%s\n", targetBinary))

	// Generate synthetic line coverage based on edge coverage
	// Each edge is mapped to a pseudo-line number
	if data.Edges > 0 {
		// Generate function coverage (estimated)
		funcCount := data.Edges / 10 // Conservative estimate: 10 edges per function
		if funcCount == 0 {
			funcCount = 1
		}

		for i := uint64(1); i <= funcCount; i++ {
			lcovContent.WriteString(fmt.Sprintf("FN:%d,func_%d\n", i*10, i))
		}

		lcovContent.WriteString(fmt.Sprintf("FNF:%d\n", funcCount))
		lcovContent.WriteString(fmt.Sprintf("FNH:%d\n", funcCount))

		// Generate line coverage based on edges
		for i := uint64(1); i <= data.Edges; i++ {
			// Simulate execution count (always 1 for covered edges in AFL++)
			lcovContent.WriteString(fmt.Sprintf("DA:%d,1\n", i))
		}

		lcovContent.WriteString(fmt.Sprintf("LF:%d\n", data.Edges))
		lcovContent.WriteString(fmt.Sprintf("LH:%d\n", data.Edges))
	}

	// Branch coverage (approximate from bitmap density)
	if data.CoveragePercent > 0 {
		branchesFound := uint64(float64(data.Edges) * 1.5) // Estimate branches
		branchesHit := uint64(float64(branchesFound) * (data.CoveragePercent / 100.0))

		for i := uint64(1); i <= branchesFound; i++ {
			taken := 0
			if i <= branchesHit {
				taken = 1
			}
			lcovContent.WriteString(fmt.Sprintf("BRDA:%d,0,%d,%d\n", i, i%2, taken))
		}

		lcovContent.WriteString(fmt.Sprintf("BRF:%d\n", branchesFound))
		lcovContent.WriteString(fmt.Sprintf("BRH:%d\n", branchesHit))
	}

	lcovContent.WriteString("end_of_record\n")

	// Write to file if output directory exists
	if data.OutputDir != "" {
		lcovPath := filepath.Join(data.OutputDir, "coverage.lcov")
		if err := os.WriteFile(lcovPath, []byte(lcovContent.String()), 0644); err != nil {
			ce.log.WithError(err).Warn("Failed to write LCOV file")
		} else {
			data.LCOVFile = lcovPath
			ce.log.WithField("lcov_file", lcovPath).Info("Generated LCOV coverage report")
		}
	}

	return lcovContent.String(), nil
}

// ConvertToJSON generates a JSON coverage report from coverage data
func (ce *CoverageExtractor) ConvertToJSON(ctx context.Context, data *CoverageData) (string, error) {
	if data == nil {
		return "", errors.New("coverage data cannot be nil")
	}

	ce.log.WithField("edges", data.Edges).Debug("Converting coverage to JSON format")

	// Create JSON structure with comprehensive coverage information
	report := map[string]interface{}{
		"version":      "1.0",
		"type":         "afl++_coverage",
		"generated_at": time.Now().Format(time.RFC3339),
		"summary": map[string]interface{}{
			"edges_covered":    data.Edges,
			"total_edges":      data.TotalEdges,
			"coverage_percent": data.CoveragePercent,
			"paths_total":      data.PathsTotal,
			"paths_pending":    data.PathsPending,
			"paths_favored":    data.PathsFavored,
			"queue_size":       data.QueueSize,
			"runtime_seconds":  data.RunTime.Seconds(),
		},
		"target": map[string]interface{}{
			"binary":         data.TargetBinary,
			"output_dir":     data.OutputDir,
			"fuzzer_version": data.FuzzerVersion,
		},
		"files": map[string]interface{}{
			"bitmap_file": data.BitmapFile,
			"lcov_file":   data.LCOVFile,
		},
		"metadata": data.Metadata,
		"collection_info": map[string]interface{}{
			"collected_at": data.CollectedAt.Format(time.RFC3339),
			"method":       "bitmap_analysis",
		},
	}

	// Add bitmap analysis if available
	if len(data.Bitmap) > 0 {
		report["bitmap"] = map[string]interface{}{
			"size_bytes":   len(data.Bitmap),
			"density":      data.CoveragePercent,
			"edges_active": data.Edges,
		}
	}

	jsonBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	jsonContent := string(jsonBytes)

	// Write to file if output directory exists
	if data.OutputDir != "" {
		jsonPath := filepath.Join(data.OutputDir, "coverage.json")
		if err := os.WriteFile(jsonPath, jsonBytes, 0644); err != nil {
			ce.log.WithError(err).Warn("Failed to write JSON file")
		} else {
			data.JSONFile = jsonPath
			ce.log.WithField("json_file", jsonPath).Info("Generated JSON coverage report")
		}
	}

	return jsonContent, nil
}

// RunAFLCov executes afl-cov tool for detailed coverage analysis (optional)
func (ce *CoverageExtractor) RunAFLCov(ctx context.Context, outputDir, targetBinary, queueDir string) (*CoverageData, error) {
	if outputDir == "" || targetBinary == "" {
		return nil, errors.New("output directory and target binary cannot be empty")
	}

	// Check if afl-cov is available
	aflCovBinary := "afl-cov"
	if _, err := exec.LookPath(aflCovBinary); err != nil {
		ce.log.Debug("afl-cov not found, trying afl-cov-fast")
		aflCovBinary = "afl-cov-fast"
		if _, err := exec.LookPath(aflCovBinary); err != nil {
			return nil, fmt.Errorf("afl-cov tools not available: %w", err)
		}
	}

	ce.log.WithFields(logrus.Fields{
		"binary":     aflCovBinary,
		"output_dir": outputDir,
		"target":     targetBinary,
	}).Info("Running afl-cov for detailed coverage analysis")

	// Create coverage output directory
	covDir := filepath.Join(outputDir, "afl-cov-analysis")
	if err := os.MkdirAll(covDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create coverage directory: %w", err)
	}

	// Build afl-cov arguments
	args := []string{
		"-d", outputDir,
		"-e", targetBinary,
		"-c", covDir,
		"--coverage-at-exit",
		"--lcov-web-all",
	}

	// Add queue directory if specified
	if queueDir != "" {
		args = append(args, "--queue-dir", queueDir)
	}

	// Execute afl-cov with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, aflCovBinary, args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("afl-cov timed out after 120 seconds")
		}
		ce.log.WithError(err).WithField("output", string(output)).Warn("afl-cov execution failed")
		// Don't fail completely, continue with basic extraction
	}

	// Extract basic coverage data
	data, err := ce.ExtractBitmapCoverage(ctx, outputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to extract coverage after afl-cov: %w", err)
	}

	data.AFLCovDir = covDir
	data.TargetBinary = targetBinary
	data.Metadata["afl_cov_binary"] = aflCovBinary
	data.Metadata["afl_cov_output"] = string(output)

	// Check for generated LCOV file
	lcovFile := filepath.Join(covDir, "coverage.info")
	if _, err := os.Stat(lcovFile); err == nil {
		data.LCOVFile = lcovFile
		data.Metadata["afl_cov_lcov"] = lcovFile
	}

	ce.log.WithField("cov_dir", covDir).Info("afl-cov analysis completed")
	return data, nil
}

// GenerateCoverageReport is the main entry point for generating coverage reports
func (ce *CoverageExtractor) GenerateCoverageReport(ctx context.Context, outputDir, targetBinary, format string) (*CoverageData, error) {
	if outputDir == "" {
		return nil, errors.New("output directory cannot be empty")
	}

	if targetBinary == "" {
		return nil, errors.New("target binary cannot be empty")
	}

	validFormats := map[string]bool{
		"lcov": true,
		"json": true,
		"both": true,
		"auto": true,
	}

	if format == "" {
		format = "auto"
	}

	if !validFormats[format] {
		return nil, fmt.Errorf("invalid format: %s (valid: lcov, json, both, auto)", format)
	}

	ce.log.WithFields(logrus.Fields{
		"output_dir":    outputDir,
		"target_binary": targetBinary,
		"format":        format,
	}).Info("Generating AFL++ coverage report")

	// First, try to use afl-cov for comprehensive analysis
	data, err := ce.RunAFLCov(ctx, outputDir, targetBinary, "")
	if err != nil {
		ce.log.WithError(err).Debug("afl-cov failed, falling back to bitmap extraction")

		// Fallback to bitmap extraction
		data, err = ce.ExtractBitmapCoverage(ctx, outputDir)
		if err != nil {
			return nil, fmt.Errorf("failed to extract coverage: %w", err)
		}
		data.TargetBinary = targetBinary
	}

	// Generate requested formats
	switch format {
	case "lcov":
		if _, err := ce.ConvertToLCOV(ctx, data, targetBinary); err != nil {
			ce.log.WithError(err).Error("Failed to generate LCOV report")
		}

	case "json":
		if _, err := ce.ConvertToJSON(ctx, data); err != nil {
			ce.log.WithError(err).Error("Failed to generate JSON report")
		}

	case "both", "auto":
		if _, err := ce.ConvertToLCOV(ctx, data, targetBinary); err != nil {
			ce.log.WithError(err).Error("Failed to generate LCOV report")
		}
		if _, err := ce.ConvertToJSON(ctx, data); err != nil {
			ce.log.WithError(err).Error("Failed to generate JSON report")
		}
	}

	ce.log.WithFields(logrus.Fields{
		"edges":            data.Edges,
		"coverage_percent": data.CoveragePercent,
		"queue_size":       data.QueueSize,
		"lcov_file":        data.LCOVFile,
		"json_file":        data.JSONFile,
	}).Info("Coverage report generation completed")

	return data, nil
}
