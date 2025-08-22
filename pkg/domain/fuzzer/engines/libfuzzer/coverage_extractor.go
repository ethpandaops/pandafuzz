package libfuzzer

import (
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

// CoverageData represents the coverage data extracted from LibFuzzer
type CoverageData struct {
	Timestamp    time.Time `json:"timestamp"`
	CollectedAt  string    `json:"collected_at"`
	Format       string    `json:"format"`
	WorkDir      string    `json:"work_dir"`
	BinaryPath   string    `json:"binary_path"`
	ProfilePath  string    `json:"profile_path,omitempty"`
	ProfdataPath string    `json:"profdata_path,omitempty"`
	OutputPath   string    `json:"output_path,omitempty"`
	HasProfdata  bool      `json:"has_profdata"`

	// Coverage statistics
	FunctionStats *FunctionCoverageStats `json:"function_stats,omitempty"`
	LineStats     *LineCoverageStats     `json:"line_stats,omitempty"`
	RegionStats   *RegionCoverageStats   `json:"region_stats,omitempty"`

	// Raw coverage data
	CoverageReport map[string]interface{} `json:"coverage_report,omitempty"`

	// Basic fallback stats
	BasicStats *BasicCoverageStats `json:"basic_stats,omitempty"`

	// Tool versions
	ToolVersions map[string]string `json:"tool_versions,omitempty"`
}

// FunctionCoverageStats represents function-level coverage statistics
type FunctionCoverageStats struct {
	TotalFunctions   uint64  `json:"total_functions"`
	CoveredFunctions uint64  `json:"covered_functions"`
	CoveragePercent  float64 `json:"coverage_percent"`
}

// LineCoverageStats represents line-level coverage statistics
type LineCoverageStats struct {
	TotalLines      uint64  `json:"total_lines"`
	CoveredLines    uint64  `json:"covered_lines"`
	CoveragePercent float64 `json:"coverage_percent"`
}

// RegionCoverageStats represents region-level coverage statistics
type RegionCoverageStats struct {
	TotalRegions    uint64  `json:"total_regions"`
	CoveredRegions  uint64  `json:"covered_regions"`
	CoveragePercent float64 `json:"coverage_percent"`
}

// BasicCoverageStats represents basic coverage statistics from fuzzer output
type BasicCoverageStats struct {
	ExecutedBlocks  uint64  `json:"executed_blocks,omitempty"`
	TotalPCs        uint64  `json:"total_pcs,omitempty"`
	CoveredFeatures uint64  `json:"covered_features,omitempty"`
	CoverageScore   float64 `json:"coverage_score,omitempty"`
}

// CoverageExtractor handles LLVM profdata format coverage extraction
type CoverageExtractor struct {
	log logrus.FieldLogger
}

// NewCoverageExtractor creates a new coverage extractor
func NewCoverageExtractor(log logrus.FieldLogger) *CoverageExtractor {
	if log == nil {
		log = logrus.New()
	}

	return &CoverageExtractor{
		log: log.WithField("component", "coverage_extractor"),
	}
}

// ExtractProfdataCoverage processes LLVM profdata if available
func (ce *CoverageExtractor) ExtractProfdataCoverage(profdataPath, binaryPath string) (*CoverageData, error) {
	data := &CoverageData{
		Timestamp:    time.Now(),
		CollectedAt:  time.Now().Format(time.RFC3339),
		ProfdataPath: profdataPath,
		BinaryPath:   binaryPath,
		HasProfdata:  false,
		ToolVersions: make(map[string]string),
	}

	// Check if profdata file exists
	if _, err := os.Stat(profdataPath); os.IsNotExist(err) {
		ce.log.WithField("profdata_path", profdataPath).Debug("Profdata file not found")
		return data, nil
	}

	// Verify binary exists and has coverage instrumentation
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("binary not found: %s", binaryPath)
	}

	// Check if binary has coverage instrumentation
	hasInstrumentation, err := ce.checkCoverageInstrumentation(binaryPath)
	if err != nil {
		ce.log.WithError(err).Warn("Failed to check coverage instrumentation")
	}

	if !hasInstrumentation {
		ce.log.WithField("binary", binaryPath).Debug("Binary does not appear to have coverage instrumentation")
		return data, nil
	}

	data.HasProfdata = true

	// Get tool versions for diagnostics
	ce.getToolVersions(data.ToolVersions)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Extract coverage statistics using llvm-cov
	if err := ce.extractCoverageStats(ctx, profdataPath, binaryPath, data); err != nil {
		ce.log.WithError(err).Warn("Failed to extract detailed coverage statistics")
		// Don't fail completely, we can still provide basic info
	}

	ce.log.WithFields(logrus.Fields{
		"profdata_path": profdataPath,
		"binary_path":   binaryPath,
		"has_stats":     data.FunctionStats != nil,
	}).Debug("Extracted profdata coverage")

	return data, nil
}

// ConvertToLCOV generates LCOV format with real line coverage if profdata exists
func (ce *CoverageExtractor) ConvertToLCOV(data *CoverageData, binaryPath string) (string, error) {
	if !data.HasProfdata {
		return "", errors.New("profdata not available for LCOV conversion")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use llvm-cov to export in LCOV format
	cmd := exec.CommandContext(ctx, "llvm-cov", "export", binaryPath,
		"-instr-profile", data.ProfdataPath, "-format=lcov")

	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("llvm-cov export to LCOV timed out after 30 seconds")
		}
		return "", fmt.Errorf("llvm-cov export to LCOV failed: %w, output: %s", err, string(output))
	}

	ce.log.Debug("Successfully converted coverage to LCOV format")
	return string(output), nil
}

// ConvertToJSON generates JSON report
func (ce *CoverageExtractor) ConvertToJSON(data *CoverageData) (string, error) {
	if !data.HasProfdata {
		// Return basic JSON with available data
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal coverage data to JSON: %w", err)
		}
		return string(jsonData), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use llvm-cov to export detailed JSON
	cmd := exec.CommandContext(ctx, "llvm-cov", "export", data.BinaryPath,
		"-instr-profile", data.ProfdataPath, "-format=text")

	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("llvm-cov export to JSON timed out after 30 seconds")
		}
		// Fallback to basic JSON if detailed export fails
		ce.log.WithError(err).Warn("Failed to export detailed coverage, falling back to basic data")
		jsonData, jsonErr := json.MarshalIndent(data, "", "  ")
		if jsonErr != nil {
			return "", fmt.Errorf("failed to marshal fallback coverage data to JSON: %w", jsonErr)
		}
		return string(jsonData), nil
	}

	ce.log.Debug("Successfully converted coverage to JSON format")
	return string(output), nil
}

// GenerateCoverageReport is the main entry point with graceful fallback
func (ce *CoverageExtractor) GenerateCoverageReport(workDir, binaryPath, format string) (*CoverageData, error) {
	data := &CoverageData{
		Timestamp:    time.Now(),
		CollectedAt:  time.Now().Format(time.RFC3339),
		Format:       format,
		WorkDir:      workDir,
		BinaryPath:   binaryPath,
		HasProfdata:  false,
		ToolVersions: make(map[string]string),
	}

	// Look for profraw/profdata files in work directory
	profdataPath, err := ce.findProfdataFile(workDir)
	if err != nil {
		ce.log.WithError(err).Debug("No profdata file found, falling back to basic stats")
		return ce.generateBasicCoverageReport(data, workDir)
	}

	// Try to extract profdata coverage
	profdataData, err := ce.ExtractProfdataCoverage(profdataPath, binaryPath)
	if err != nil {
		ce.log.WithError(err).Warn("Failed to extract profdata coverage, falling back to basic stats")
		return ce.generateBasicCoverageReport(data, workDir)
	}

	// Merge the profdata results
	data.ProfdataPath = profdataData.ProfdataPath
	data.HasProfdata = profdataData.HasProfdata
	data.FunctionStats = profdataData.FunctionStats
	data.LineStats = profdataData.LineStats
	data.RegionStats = profdataData.RegionStats
	data.CoverageReport = profdataData.CoverageReport
	data.ToolVersions = profdataData.ToolVersions

	// Generate output based on requested format
	switch format {
	case "lcov":
		if data.HasProfdata {
			lcovContent, err := ce.ConvertToLCOV(data, binaryPath)
			if err != nil {
				ce.log.WithError(err).Warn("Failed to generate LCOV, including raw data")
			} else {
				outputPath := filepath.Join(workDir, "coverage.info")
				if writeErr := os.WriteFile(outputPath, []byte(lcovContent), 0644); writeErr == nil {
					data.OutputPath = outputPath
				}
			}
		}

	case "json":
		jsonContent, err := ce.ConvertToJSON(data)
		if err != nil {
			ce.log.WithError(err).Warn("Failed to generate JSON, including basic data")
		} else {
			outputPath := filepath.Join(workDir, "coverage.json")
			if writeErr := os.WriteFile(outputPath, []byte(jsonContent), 0644); writeErr == nil {
				data.OutputPath = outputPath
			}
		}

	case "html":
		if data.HasProfdata {
			htmlDir := filepath.Join(workDir, "html")
			if err := ce.generateHTMLReport(data.ProfdataPath, binaryPath, htmlDir); err != nil {
				ce.log.WithError(err).Warn("Failed to generate HTML report")
			} else {
				data.OutputPath = filepath.Join(htmlDir, "index.html")
			}
		}
	}

	// Always try to get basic stats as well
	basicData, _ := ce.generateBasicCoverageReport(&CoverageData{}, workDir)
	data.BasicStats = basicData.BasicStats

	return data, nil
}

// GetBasicStats provides fallback to basic coverage stats when profdata isn't available
func (ce *CoverageExtractor) GetBasicStats(workDir string) (*CoverageData, error) {
	data := &CoverageData{
		Timestamp:   time.Now(),
		CollectedAt: time.Now().Format(time.RFC3339),
		Format:      "basic",
		WorkDir:     workDir,
		HasProfdata: false,
	}

	return ce.generateBasicCoverageReport(data, workDir)
}

// checkCoverageInstrumentation checks if binary has coverage instrumentation
func (ce *CoverageExtractor) checkCoverageInstrumentation(binaryPath string) (bool, error) {
	// Use objdump to check for coverage-related symbols or sections
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "objdump", "-t", binaryPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try with readelf as fallback
		cmd = exec.CommandContext(ctx, "readelf", "-s", binaryPath)
		output, err = cmd.CombinedOutput()
		if err != nil {
			return false, fmt.Errorf("failed to analyze binary symbols: %w", err)
		}
	}

	outputStr := string(output)

	// Look for LLVM coverage instrumentation symbols
	coverageIndicators := []string{
		"__llvm_profile_",
		"__llvm_prf_",
		"__profc_",
		"__profn_",
		"__profd_",
	}

	for _, indicator := range coverageIndicators {
		if strings.Contains(outputStr, indicator) {
			return true, nil
		}
	}

	return false, nil
}

// findProfdataFile looks for profraw or profdata files in the work directory
func (ce *CoverageExtractor) findProfdataFile(workDir string) (string, error) {
	// Look for existing .profdata files first
	profdataPattern := filepath.Join(workDir, "*.profdata")
	matches, err := filepath.Glob(profdataPattern)
	if err == nil && len(matches) > 0 {
		return matches[0], nil
	}

	// Look for .profraw files and merge them
	profrawPattern := filepath.Join(workDir, "*.profraw")
	matches, err = filepath.Glob(profrawPattern)
	if err != nil || len(matches) == 0 {
		return "", errors.New("no profraw or profdata files found")
	}

	// Merge profraw files into profdata
	profdataPath := filepath.Join(workDir, "merged.profdata")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := append([]string{"merge", "-sparse"}, matches...)
	args = append(args, "-o", profdataPath)

	cmd := exec.CommandContext(ctx, "llvm-profdata", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("llvm-profdata merge timed out after 30 seconds")
		}
		return "", fmt.Errorf("llvm-profdata merge failed: %w, output: %s", err, string(output))
	}

	ce.log.WithField("profdata_path", profdataPath).Debug("Successfully merged profraw files")
	return profdataPath, nil
}

// extractCoverageStats extracts detailed coverage statistics
func (ce *CoverageExtractor) extractCoverageStats(ctx context.Context, profdataPath, binaryPath string, data *CoverageData) error {
	// Get summary statistics using llvm-cov report
	cmd := exec.CommandContext(ctx, "llvm-cov", "report", binaryPath,
		"-instr-profile", profdataPath, "-use-color=false")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("llvm-cov report failed: %w, output: %s", err, string(output))
	}

	// Parse the coverage report output
	if err := ce.parseCoverageReport(string(output), data); err != nil {
		ce.log.WithError(err).Warn("Failed to parse coverage report")
	}

	return nil
}

// parseCoverageReport parses llvm-cov report output
func (ce *CoverageExtractor) parseCoverageReport(reportOutput string, data *CoverageData) error {
	lines := strings.Split(reportOutput, "\n")

	// Look for the TOTAL line which contains summary statistics
	totalRegex := regexp.MustCompile(`TOTAL\s+(\d+)\s+(\d+)\s+([\d.]+)%\s+(\d+)\s+(\d+)\s+([\d.]+)%\s+(\d+)\s+(\d+)\s+([\d.]+)%`)

	for _, line := range lines {
		if matches := totalRegex.FindStringSubmatch(line); len(matches) > 9 {
			// Parse function coverage
			totalFuncs, _ := strconv.ParseUint(matches[1], 10, 64)
			coveredFuncs, _ := strconv.ParseUint(matches[2], 10, 64)
			funcPercent, _ := strconv.ParseFloat(matches[3], 64)

			data.FunctionStats = &FunctionCoverageStats{
				TotalFunctions:   totalFuncs,
				CoveredFunctions: coveredFuncs,
				CoveragePercent:  funcPercent,
			}

			// Parse line coverage
			totalLines, _ := strconv.ParseUint(matches[4], 10, 64)
			coveredLines, _ := strconv.ParseUint(matches[5], 10, 64)
			linePercent, _ := strconv.ParseFloat(matches[6], 64)

			data.LineStats = &LineCoverageStats{
				TotalLines:      totalLines,
				CoveredLines:    coveredLines,
				CoveragePercent: linePercent,
			}

			// Parse region coverage
			totalRegions, _ := strconv.ParseUint(matches[7], 10, 64)
			coveredRegions, _ := strconv.ParseUint(matches[8], 10, 64)
			regionPercent, _ := strconv.ParseFloat(matches[9], 64)

			data.RegionStats = &RegionCoverageStats{
				TotalRegions:    totalRegions,
				CoveredRegions:  coveredRegions,
				CoveragePercent: regionPercent,
			}

			break
		}
	}

	return nil
}

// generateHTMLReport generates HTML coverage report
func (ce *CoverageExtractor) generateHTMLReport(profdataPath, binaryPath, htmlDir string) error {
	if err := os.MkdirAll(htmlDir, 0755); err != nil {
		return fmt.Errorf("failed to create HTML directory: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "llvm-cov", "show", binaryPath,
		"-instr-profile", profdataPath, "-format=html", "-output-dir", htmlDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("llvm-cov show timed out after 60 seconds")
		}
		return fmt.Errorf("llvm-cov show failed: %w, output: %s", err, string(output))
	}

	return nil
}

// generateBasicCoverageReport generates basic coverage stats from fuzzer output
func (ce *CoverageExtractor) generateBasicCoverageReport(data *CoverageData, workDir string) (*CoverageData, error) {
	// Look for libfuzzer output files or logs
	logFiles := []string{
		filepath.Join(workDir, "fuzzer.log"),
		filepath.Join(workDir, "output.log"),
		filepath.Join(workDir, "libfuzzer.log"),
	}

	basicStats := &BasicCoverageStats{}
	found := false

	for _, logFile := range logFiles {
		if content, err := os.ReadFile(logFile); err == nil {
			if ce.parseBasicStats(string(content), basicStats) {
				found = true
				break
			}
		}
	}

	if !found {
		// Provide minimal default stats
		basicStats.CoverageScore = 0.0
	}

	data.BasicStats = basicStats
	data.Format = "basic"

	return data, nil
}

// parseBasicStats extracts basic coverage information from fuzzer logs
func (ce *CoverageExtractor) parseBasicStats(logContent string, stats *BasicCoverageStats) bool {
	found := false

	// Look for LibFuzzer coverage output patterns
	covRegex := regexp.MustCompile(`cov:\s*(\d+)`)
	pcRegex := regexp.MustCompile(`#(\d+).*cov:\s*(\d+)`)
	featRegex := regexp.MustCompile(`ft:\s*(\d+)`)

	lines := strings.Split(logContent, "\n")
	for _, line := range lines {
		if matches := covRegex.FindStringSubmatch(line); len(matches) > 1 {
			if coverage, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				stats.ExecutedBlocks = coverage
				found = true
			}
		}

		if matches := pcRegex.FindStringSubmatch(line); len(matches) > 2 {
			if executions, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				if coverage, err := strconv.ParseUint(matches[2], 10, 64); err == nil {
					stats.TotalPCs = executions
					stats.ExecutedBlocks = coverage
					found = true
				}
			}
		}

		if matches := featRegex.FindStringSubmatch(line); len(matches) > 1 {
			if features, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				stats.CoveredFeatures = features
				found = true
			}
		}
	}

	// Calculate basic coverage score if we have data
	if stats.ExecutedBlocks > 0 && stats.TotalPCs > 0 {
		stats.CoverageScore = float64(stats.ExecutedBlocks) / float64(stats.TotalPCs) * 100.0
	}

	return found
}

// getToolVersions retrieves versions of coverage tools for diagnostics
func (ce *CoverageExtractor) getToolVersions(versions map[string]string) {
	tools := map[string]string{
		"llvm-cov":      "llvm-cov",
		"llvm-profdata": "llvm-profdata",
		"objdump":       "objdump",
		"readelf":       "readelf",
	}

	for name, cmd := range tools {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		execCmd := exec.CommandContext(ctx, cmd, "--version")
		if output, err := execCmd.CombinedOutput(); err == nil {
			// Extract first line as version
			lines := strings.Split(string(output), "\n")
			if len(lines) > 0 {
				versions[name] = strings.TrimSpace(lines[0])
			}
		} else {
			versions[name] = "not available"
		}
		cancel()
	}
}
