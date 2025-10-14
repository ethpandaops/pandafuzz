package honggfuzz

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// CoverageData represents Honggfuzz coverage information
type CoverageData struct {
	// Core coverage metrics
	BranchCount        uint64  `json:"branch_count"`
	BranchHits         uint64  `json:"branch_hits"`
	BranchCoverage     float64 `json:"branch_coverage_percent"`
	BasicBlockCount    uint64  `json:"basic_block_count"`
	BasicBlockHits     uint64  `json:"basic_block_hits"`
	BasicBlockCoverage float64 `json:"basic_block_coverage_percent"`
	EdgeCount          uint64  `json:"edge_count"`
	EdgeHits           uint64  `json:"edge_hits"`
	EdgeCoverage       float64 `json:"edge_coverage_percent"`

	// Honggfuzz specific metrics
	CorpusSize       uint64 `json:"corpus_size"`
	UniqueSignals    uint64 `json:"unique_signals"`
	CrashedCount     uint64 `json:"crashed_count"`
	TimeoutCount     uint64 `json:"timeout_count"`
	UniqueHangsCount uint64 `json:"unique_hangs_count"`

	// Performance metrics
	ExecsPerSecond  float64       `json:"execs_per_second"`
	TotalExecutions uint64        `json:"total_executions"`
	RunTime         time.Duration `json:"run_time"`

	// Timing information
	CollectedAt time.Time `json:"collected_at"`

	// Metadata
	FuzzerVersion string            `json:"fuzzer_version"`
	TargetBinary  string            `json:"target_binary"`
	OutputDir     string            `json:"output_dir"`
	Metadata      map[string]string `json:"metadata"`

	// Coverage files
	SancovDir  string `json:"sancov_dir,omitempty"`
	ReportFile string `json:"report_file,omitempty"`
	LCOVFile   string `json:"lcov_file,omitempty"`
	JSONFile   string `json:"json_file,omitempty"`
}

// ToReportMap converts CoverageData to a map suitable for API reporting
func (cd *CoverageData) ToReportMap() map[string]interface{} {
	return map[string]interface{}{
		"branch_count":         cd.BranchCount,
		"branch_hits":          cd.BranchHits,
		"branch_coverage":      cd.BranchCoverage,
		"basic_block_count":    cd.BasicBlockCount,
		"basic_block_hits":     cd.BasicBlockHits,
		"basic_block_coverage": cd.BasicBlockCoverage,
		"edge_count":           cd.EdgeCount,
		"edge_hits":            cd.EdgeHits,
		"edge_coverage":        cd.EdgeCoverage,
		"corpus_size":          cd.CorpusSize,
		"unique_signals":       cd.UniqueSignals,
		"crashed_count":        cd.CrashedCount,
		"timeout_count":        cd.TimeoutCount,
		"unique_hangs_count":   cd.UniqueHangsCount,
		"execs_per_second":     cd.ExecsPerSecond,
		"total_executions":     cd.TotalExecutions,
		"run_time_seconds":     cd.RunTime.Seconds(),
		"fuzzer_version":       cd.FuzzerVersion,
		"metadata":             cd.Metadata,
	}
}

// CoverageExtractor handles Honggfuzz coverage extraction and report generation
type CoverageExtractor struct {
	log logrus.FieldLogger
}

// NewCoverageExtractor creates a new coverage extractor instance
func NewCoverageExtractor(log logrus.FieldLogger) *CoverageExtractor {
	if log == nil {
		log = logrus.New()
	}

	return &CoverageExtractor{
		log: log.WithField("component", "honggfuzz_coverage_extractor"),
	}
}

// ExtractCoverageFromReport parses Honggfuzz report files to extract coverage data
func (ce *CoverageExtractor) ExtractCoverageFromReport(ctx context.Context, outputDir string) (*CoverageData, error) {
	if outputDir == "" {
		return nil, errors.New("output directory cannot be empty")
	}

	ce.log.WithField("output_dir", outputDir).Debug("Extracting coverage from Honggfuzz report")

	data := &CoverageData{
		OutputDir:   outputDir,
		CollectedAt: time.Now(),
		Metadata:    make(map[string]string),
	}

	// Look for report file in various locations
	reportPaths := []string{
		filepath.Join(outputDir, "HONGGFUZZ.REPORT.TXT"),
		filepath.Join(outputDir, "honggfuzz.report.txt"),
		filepath.Join(outputDir, "report.txt"),
		filepath.Join(outputDir, "fuzzer_stats"),
	}

	var reportFile string
	for _, path := range reportPaths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if _, err := os.Stat(path); err == nil {
			reportFile = path
			break
		}
	}

	if reportFile == "" {
		ce.log.Debug("No report file found, attempting to extract from log output")
		return ce.extractFromLogOutput(ctx, outputDir)
	}

	data.ReportFile = reportFile

	// Parse the report file
	if err := ce.parseReportFile(ctx, reportFile, data); err != nil {
		return nil, fmt.Errorf("failed to parse report file: %w", err)
	}

	// Extract sancov data if available
	if err := ce.extractSancovData(ctx, outputDir, data); err != nil {
		ce.log.WithError(err).Warn("Failed to extract sancov data")
	}

	// Count corpus files
	if err := ce.countCorpusFiles(ctx, outputDir, data); err != nil {
		ce.log.WithError(err).Warn("Failed to count corpus files")
	}

	ce.log.WithFields(logrus.Fields{
		"branch_coverage":      data.BranchCoverage,
		"basic_block_coverage": data.BasicBlockCoverage,
		"edge_coverage":        data.EdgeCoverage,
		"corpus_size":          data.CorpusSize,
	}).Info("Successfully extracted Honggfuzz coverage")

	return data, nil
}

// parseReportFile parses a Honggfuzz report file
func (ce *CoverageExtractor) parseReportFile(ctx context.Context, reportFile string, data *CoverageData) error {
	file, err := os.Open(reportFile)
	if err != nil {
		return fmt.Errorf("failed to open report file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Regular expressions for parsing various metrics
	branchRe := regexp.MustCompile(`(?i)branches?\s*:\s*(\d+)\s*/\s*(\d+)`)
	blockRe := regexp.MustCompile(`(?i)basic\s*blocks?\s*:\s*(\d+)\s*/\s*(\d+)`)
	edgeRe := regexp.MustCompile(`(?i)edges?\s*:\s*(\d+)\s*/\s*(\d+)`)
	corpusRe := regexp.MustCompile(`(?i)corpus\s*size\s*:\s*(\d+)`)
	crashRe := regexp.MustCompile(`(?i)crashed\s*:\s*(\d+)`)
	hangRe := regexp.MustCompile(`(?i)unique\s*hangs?\s*:\s*(\d+)`)
	execsRe := regexp.MustCompile(`(?i)execs?\s*per\s*sec\s*:\s*([\d.]+)`)
	totalExecsRe := regexp.MustCompile(`(?i)total\s*execs?\s*:\s*(\d+)`)
	signalsRe := regexp.MustCompile(`(?i)unique\s*signals?\s*:\s*(\d+)`)
	timeoutRe := regexp.MustCompile(`(?i)timeouts?\s*:\s*(\d+)`)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()

		// Parse branch coverage
		if matches := branchRe.FindStringSubmatch(line); len(matches) == 3 {
			if hits, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				data.BranchHits = hits
			}
			if total, err := strconv.ParseUint(matches[2], 10, 64); err == nil {
				data.BranchCount = total
				if total > 0 {
					data.BranchCoverage = float64(data.BranchHits) / float64(total) * 100.0
				}
			}
		}

		// Parse basic block coverage
		if matches := blockRe.FindStringSubmatch(line); len(matches) == 3 {
			if hits, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				data.BasicBlockHits = hits
			}
			if total, err := strconv.ParseUint(matches[2], 10, 64); err == nil {
				data.BasicBlockCount = total
				if total > 0 {
					data.BasicBlockCoverage = float64(data.BasicBlockHits) / float64(total) * 100.0
				}
			}
		}

		// Parse edge coverage
		if matches := edgeRe.FindStringSubmatch(line); len(matches) == 3 {
			if hits, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				data.EdgeHits = hits
			}
			if total, err := strconv.ParseUint(matches[2], 10, 64); err == nil {
				data.EdgeCount = total
				if total > 0 {
					data.EdgeCoverage = float64(data.EdgeHits) / float64(total) * 100.0
				}
			}
		}

		// Parse corpus size
		if matches := corpusRe.FindStringSubmatch(line); len(matches) == 2 {
			if size, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				data.CorpusSize = size
			}
		}

		// Parse crash count
		if matches := crashRe.FindStringSubmatch(line); len(matches) == 2 {
			if count, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				data.CrashedCount = count
			}
		}

		// Parse unique hangs
		if matches := hangRe.FindStringSubmatch(line); len(matches) == 2 {
			if count, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				data.UniqueHangsCount = count
			}
		}

		// Parse executions per second
		if matches := execsRe.FindStringSubmatch(line); len(matches) == 2 {
			if rate, err := strconv.ParseFloat(matches[1], 64); err == nil {
				data.ExecsPerSecond = rate
			}
		}

		// Parse total executions
		if matches := totalExecsRe.FindStringSubmatch(line); len(matches) == 2 {
			if total, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				data.TotalExecutions = total
			}
		}

		// Parse unique signals
		if matches := signalsRe.FindStringSubmatch(line); len(matches) == 2 {
			if count, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				data.UniqueSignals = count
			}
		}

		// Parse timeout count
		if matches := timeoutRe.FindStringSubmatch(line); len(matches) == 2 {
			if count, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				data.TimeoutCount = count
			}
		}

		// Store raw line in metadata
		data.Metadata["raw_"+strings.ReplaceAll(strings.ToLower(line[:min(20, len(line))]), " ", "_")] = line
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading report file: %w", err)
	}

	// Calculate runtime from total executions and exec rate
	if data.ExecsPerSecond > 0 && data.TotalExecutions > 0 {
		seconds := float64(data.TotalExecutions) / data.ExecsPerSecond
		data.RunTime = time.Duration(seconds) * time.Second
	}

	return nil
}

// extractFromLogOutput extracts coverage from log files when report is not available
func (ce *CoverageExtractor) extractFromLogOutput(ctx context.Context, outputDir string) (*CoverageData, error) {
	data := &CoverageData{
		OutputDir:   outputDir,
		CollectedAt: time.Now(),
		Metadata:    make(map[string]string),
	}

	// Look for log files
	logPaths := []string{
		filepath.Join(outputDir, "honggfuzz.log"),
		filepath.Join(outputDir, "fuzzer.log"),
		filepath.Join(outputDir, "output.log"),
	}

	for _, logPath := range logPaths {
		if _, err := os.Stat(logPath); err == nil {
			if err := ce.parseLogFile(ctx, logPath, data); err == nil {
				ce.log.WithField("source", "log_file").Info("Extracted coverage from log file")
				return data, nil
			}
		}
	}

	// If no files found, return basic data
	ce.log.Debug("No log files found, returning minimal coverage data")
	return data, nil
}

// parseLogFile parses a Honggfuzz log file for coverage information
func (ce *CoverageExtractor) parseLogFile(ctx context.Context, logFile string, data *CoverageData) error {
	file, err := os.Open(logFile)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Parse similar to report file but from log output
	// Honggfuzz logs contain periodic status updates with coverage info
	coverageRe := regexp.MustCompile(`(?i)coverage:\s*([\d.]+)%`)
	corpusRe := regexp.MustCompile(`(?i)corpus:\s*(\d+)`)
	crashRe := regexp.MustCompile(`(?i)crashes:\s*(\d+)`)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()

		// Parse coverage percentage
		if matches := coverageRe.FindStringSubmatch(line); len(matches) == 2 {
			if coverage, err := strconv.ParseFloat(matches[1], 64); err == nil {
				// Use as edge coverage by default
				data.EdgeCoverage = coverage
			}
		}

		// Parse corpus size
		if matches := corpusRe.FindStringSubmatch(line); len(matches) == 2 {
			if size, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				data.CorpusSize = size
			}
		}

		// Parse crash count
		if matches := crashRe.FindStringSubmatch(line); len(matches) == 2 {
			if count, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				data.CrashedCount = count
			}
		}
	}

	return scanner.Err()
}

// extractSancovData extracts sanitizer coverage data if available
func (ce *CoverageExtractor) extractSancovData(ctx context.Context, outputDir string, data *CoverageData) error {
	sancovDir := filepath.Join(outputDir, "sancov")
	if _, err := os.Stat(sancovDir); os.IsNotExist(err) {
		// Try alternative location
		sancovDir = filepath.Join(outputDir, "coverage")
		if _, err := os.Stat(sancovDir); os.IsNotExist(err) {
			return errors.New("sancov directory not found")
		}
	}

	data.SancovDir = sancovDir

	// Look for sancov files
	files, err := os.ReadDir(sancovDir)
	if err != nil {
		return fmt.Errorf("failed to read sancov directory: %w", err)
	}

	sancovCount := 0
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".sancov") {
			sancovCount++
		}
	}

	data.Metadata["sancov_file_count"] = strconv.Itoa(sancovCount)

	// If sancov-merge tool is available, merge coverage files
	if _, err := exec.LookPath("sancov"); err == nil {
		ce.log.Debug("sancov tool found, attempting to merge coverage files")
		// Implement sancov merging if needed
	}

	return nil
}

// countCorpusFiles counts the number of corpus files
func (ce *CoverageExtractor) countCorpusFiles(ctx context.Context, outputDir string, data *CoverageData) error {
	corpusDirs := []string{
		filepath.Join(outputDir, "corpus"),
		filepath.Join(outputDir, "input"),
		outputDir, // Sometimes corpus is directly in output dir
	}

	for _, corpusDir := range corpusDirs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		files, err := os.ReadDir(corpusDir)
		if err != nil {
			continue
		}

		count := uint64(0)
		for _, file := range files {
			if !file.IsDir() {
				// Count non-directory files as corpus entries
				count++
			}
		}

		if count > 0 {
			data.CorpusSize = count
			data.Metadata["corpus_dir"] = corpusDir
			break
		}
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
		"edge_coverage": data.EdgeCoverage,
	}).Debug("Converting coverage to LCOV format")

	data.TargetBinary = targetBinary

	// Generate synthetic LCOV content
	var lcovContent strings.Builder

	// Test name and source file info
	lcovContent.WriteString("TN:Honggfuzz Coverage Report\n")
	lcovContent.WriteString(fmt.Sprintf("SF:%s\n", targetBinary))

	// Generate synthetic function coverage based on basic blocks
	if data.BasicBlockHits > 0 {
		funcCount := data.BasicBlockHits / 5 // Estimate: 5 blocks per function
		if funcCount == 0 {
			funcCount = 1
		}

		for i := uint64(1); i <= funcCount; i++ {
			lcovContent.WriteString(fmt.Sprintf("FN:%d,func_%d\n", i*10, i))
		}

		lcovContent.WriteString(fmt.Sprintf("FNF:%d\n", funcCount))
		lcovContent.WriteString(fmt.Sprintf("FNH:%d\n", funcCount))
	}

	// Generate line coverage based on edges
	if data.EdgeHits > 0 {
		for i := uint64(1); i <= data.EdgeHits; i++ {
			lcovContent.WriteString(fmt.Sprintf("DA:%d,1\n", i))
		}

		lcovContent.WriteString(fmt.Sprintf("LF:%d\n", data.EdgeCount))
		lcovContent.WriteString(fmt.Sprintf("LH:%d\n", data.EdgeHits))
	}

	// Branch coverage
	if data.BranchHits > 0 {
		for i := uint64(1); i <= data.BranchCount; i++ {
			taken := 0
			if i <= data.BranchHits {
				taken = 1
			}
			lcovContent.WriteString(fmt.Sprintf("BRDA:%d,0,%d,%d\n", i, i%2, taken))
		}

		lcovContent.WriteString(fmt.Sprintf("BRF:%d\n", data.BranchCount))
		lcovContent.WriteString(fmt.Sprintf("BRH:%d\n", data.BranchHits))
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

	ce.log.WithField("edge_coverage", data.EdgeCoverage).Debug("Converting coverage to JSON format")

	// Create JSON structure with comprehensive coverage information
	report := map[string]interface{}{
		"version":      "1.0",
		"type":         "honggfuzz_coverage",
		"generated_at": time.Now().Format(time.RFC3339),
		"summary": map[string]interface{}{
			"branch_coverage":      data.BranchCoverage,
			"branch_hits":          data.BranchHits,
			"branch_count":         data.BranchCount,
			"basic_block_coverage": data.BasicBlockCoverage,
			"basic_block_hits":     data.BasicBlockHits,
			"basic_block_count":    data.BasicBlockCount,
			"edge_coverage":        data.EdgeCoverage,
			"edge_hits":            data.EdgeHits,
			"edge_count":           data.EdgeCount,
			"corpus_size":          data.CorpusSize,
			"unique_signals":       data.UniqueSignals,
			"crashed_count":        data.CrashedCount,
			"timeout_count":        data.TimeoutCount,
			"unique_hangs_count":   data.UniqueHangsCount,
			"runtime_seconds":      data.RunTime.Seconds(),
			"execs_per_second":     data.ExecsPerSecond,
			"total_executions":     data.TotalExecutions,
		},
		"target": map[string]interface{}{
			"binary":         data.TargetBinary,
			"output_dir":     data.OutputDir,
			"fuzzer_version": data.FuzzerVersion,
		},
		"files": map[string]interface{}{
			"report_file": data.ReportFile,
			"sancov_dir":  data.SancovDir,
			"lcov_file":   data.LCOVFile,
		},
		"metadata": data.Metadata,
		"collection_info": map[string]interface{}{
			"collected_at": data.CollectedAt.Format(time.RFC3339),
			"method":       "report_parsing",
		},
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
	}).Info("Generating Honggfuzz coverage report")

	// Extract coverage data from report files
	data, err := ce.ExtractCoverageFromReport(ctx, outputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to extract coverage: %w", err)
	}

	data.TargetBinary = targetBinary

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
		"edge_coverage":        data.EdgeCoverage,
		"branch_coverage":      data.BranchCoverage,
		"basic_block_coverage": data.BasicBlockCoverage,
		"corpus_size":          data.CorpusSize,
		"lcov_file":            data.LCOVFile,
		"json_file":            data.JSONFile,
	}).Info("Coverage report generation completed")

	return data, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
