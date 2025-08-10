# Adding Coverage Report Generation and Collection to PandaFuzz

This document describes the implementation plan for adding code coverage report generation and collection functionality to PandaFuzz. The design allows users to enable coverage collection via a checkbox when creating fuzzing jobs.

## Prerequisites and Important Notes

### Build Requirements for Coverage

1. **LibFuzzer Coverage**:
   - Target binaries must be compiled with Clang and the following flags:
     - `-fprofile-instr-generate` - Enables profile instrumentation
     - `-fcoverage-mapping` - Generates coverage mapping information
   - LibFuzzer itself no longer uses `-print_coverage` or `-dump_coverage` flags (deprecated)
   - Coverage is collected using LLVM's profiling infrastructure

2. **AFL++ Coverage**:
   - For LLVM-based coverage: compile with `-fprofile-instr-generate -fcoverage-mapping`
   - AFL++ doesn't have built-in coverage report generation
   - Coverage is collected post-fuzzing using:
     - `afl-cov-fast` for efficient coverage analysis
     - `llvm-cov` and `llvm-profdata` for LLVM-instrumented binaries
   - The `afl-cov-fast` tool must be installed separately

3. **Required Tools**:
   - `llvm-profdata` - For merging raw profile data
   - `llvm-cov` - For generating coverage reports
   - `afl-cov-fast.py` (optional) - For AFL++ coverage analysis
   - `gcovr` (optional) - For alternative coverage report generation

### Coverage Collection Approaches

1. **During Fuzzing (LibFuzzer)**:
   - Set `LLVM_PROFILE_FILE` environment variable
   - Profile data is written continuously during fuzzing

2. **Post-Fuzzing (AFL++)**:
   - Run all test cases through the instrumented binary
   - Collect profile data from each execution
   - Merge and generate reports

## Overview

Code coverage provides critical insights into fuzzing effectiveness by showing which parts of the target code have been exercised. This feature will:

1. Allow users to enable coverage collection when creating jobs
2. Generate coverage reports during fuzzing execution
3. Collect and store coverage data in the master node
4. Provide API endpoints to retrieve coverage reports

## Architecture Changes

### 1. Job Configuration Changes

#### API Schema Updates

Update the OpenAPI spec (`pkg/master/api_v3/openapi.yaml`):

```yaml
JobConfig:
  type: object
  properties:
    # Existing properties...
    enable_coverage:
      type: boolean
      description: Enable code coverage collection during fuzzing
      default: false
    coverage_format:
      type: string
      enum: ["json", "html", "lcov", "cobertura"]
      description: Format for coverage reports
      default: "json"
```

#### Domain Model Updates

Update `pkg/domain/job/types/job.go`:

```go
type Job struct {
    // Existing fields...
    
    // Coverage fields
    EnableCoverage   bool              `json:"enable_coverage"`
    CoverageFormat   string            `json:"coverage_format,omitempty"`
    CoverageReportID string            `json:"coverage_report_id,omitempty"`
    CoverageStats    *CoverageStats    `json:"coverage_stats,omitempty"`
}

type CoverageStats struct {
    LineCoverage     float64   `json:"line_coverage"`
    FunctionCoverage float64   `json:"function_coverage"`
    BranchCoverage   float64   `json:"branch_coverage,omitempty"`
    CollectedAt      time.Time `json:"collected_at"`
    ReportPath       string    `json:"report_path"`
}
```

### 2. Fuzzer Configuration Updates

#### Update FuzzerConfig

In `pkg/domain/fuzzer/types/config.go`:

```go
type FuzzerConfig struct {
    // Existing fields...
    
    // Coverage configuration
    EnableCoverage   bool   `json:"enable_coverage,omitempty"`
    CoverageFormat   string `json:"coverage_format,omitempty"`
    CoverageDir      string `json:"coverage_dir,omitempty"`
}

// Add to LibFuzzerOptions
type LibFuzzerOptions struct {
    // Existing fields...
    
    // Coverage options (updated for modern LibFuzzer)
    UseCounters          int    `json:"use_counters,omitempty"`      // Enable counter tracking
    PrintCoveragePCs     int    `json:"print_pcs,omitempty"`         // Print newly covered PCs
    // Note: print_coverage and dump_coverage are deprecated in modern LibFuzzer
    // Coverage is now collected via Clang Coverage instrumentation
}

// Add to AFLPlusPlusOptions  
type AFLPlusPlusOptions struct {
    // Existing fields...
    
    // Coverage options
    UseAFLCov       bool   `json:"use_afl_cov,omitempty"`      // Use afl-cov-fast for coverage
    LLVMMode        bool   `json:"llvm_mode,omitempty"`         // Whether target is LLVM-instrumented
    SourceDir       string `json:"source_dir,omitempty"`        // Source code directory for afl-cov
    LLVMCovBinary   string `json:"llvm_cov_binary,omitempty"`   // Path to llvm-cov binary
    LLVMProfData    string `json:"llvm_profdata_binary,omitempty"` // Path to llvm-profdata binary
}

// Add to HonggfuzzOptions
type HonggfuzzOptions struct {
    // Existing fields...
    
    // Coverage options
    Sancov          bool   `json:"sancov,omitempty"`
    SancovDir       string `json:"sancov_dir,omitempty"`
    CoverageReport  bool   `json:"coverage_report,omitempty"`
}
```

### 3. Fuzzer Engine Implementation

#### LibFuzzer Coverage Support

Update `pkg/domain/fuzzer/engines/libfuzzer/engine.go`:

```go
func (e *Engine) buildCommandArgs() []string {
    args := []string{}
    
    // Existing args...
    
    if e.config.EnableCoverage {
        if e.config.LibFuzzerOptions != nil {
            opts := e.config.LibFuzzerOptions
            
            // Enable counter tracking for better coverage information
            args = append(args, "-use_counters=1")
            
            // Enable PC printing for debugging (optional)
            if opts.PrintCoveragePCs == 1 {
                args = append(args, "-print_pcs=1")
            }
            
            // Note: Modern LibFuzzer uses Clang Coverage for detailed reports
            // The deprecated -print_coverage and -dump_coverage flags are no longer used
        }
    }
    
    return args
}

// Configure environment for Clang coverage collection
func (e *Engine) configureCoverageEnvironment() error {
    if !e.config.EnableCoverage {
        return nil
    }
    
    // Set LLVM profile file for raw coverage data
    profilePath := filepath.Join(e.outputDir, "coverage", "%p-%m.profraw")
    os.Setenv("LLVM_PROFILE_FILE", profilePath)
    
    // Ensure coverage directory exists
    coverageDir := filepath.Join(e.outputDir, "coverage")
    return os.MkdirAll(coverageDir, 0755)
}

// Add coverage collection method using LLVM tools
func (e *Engine) collectCoverageData(ctx context.Context) (*types.CoverageData, error) {
    if !e.config.EnableCoverage {
        return nil, nil
    }
    
    coverageDir := filepath.Join(e.outputDir, "coverage")
    
    // First, merge all profraw files into a single profdata file
    profDataFile := filepath.Join(coverageDir, "merged.profdata")
    profrawPattern := filepath.Join(coverageDir, "*.profraw")
    
    // Find all profraw files
    profrawFiles, err := filepath.Glob(profrawPattern)
    if err != nil || len(profrawFiles) == 0 {
        return nil, fmt.Errorf("no profraw files found")
    }
    
    // Merge profraw files using llvm-profdata
    mergeArgs := append([]string{"merge", "-sparse", "-o", profDataFile}, profrawFiles...)
    mergeCmd := exec.CommandContext(ctx, "llvm-profdata", mergeArgs...)
    if err := mergeCmd.Run(); err != nil {
        return nil, fmt.Errorf("failed to merge profile data: %w", err)
    }
    
    // Generate coverage report in requested format
    data := &types.CoverageData{
        Format:      e.config.CoverageFormat,
        CollectedAt: time.Now(),
    }
    
    // Use llvm-cov to generate coverage report
    reportFile := filepath.Join(coverageDir, fmt.Sprintf("coverage.%s", e.config.CoverageFormat))
    
    covArgs := []string{"export", e.target, "-instr-profile=" + profDataFile}
    switch e.config.CoverageFormat {
    case "json":
        covArgs = append(covArgs, "-format=json")
    case "lcov":
        covArgs = append(covArgs, "-format=lcov")
    case "html":
        // For HTML, we use show instead of export
        covArgs = []string{"show", e.target, "-instr-profile=" + profDataFile, 
                          "-format=html", "-output-dir=" + coverageDir}
    default:
        return nil, fmt.Errorf("unsupported coverage format: %s", e.config.CoverageFormat)
    }
    
    if e.config.CoverageFormat != "html" {
        covArgs = append(covArgs, "-o", reportFile)
    }
    
    covCmd := exec.CommandContext(ctx, "llvm-cov", covArgs...)
    if err := covCmd.Run(); err != nil {
        return nil, fmt.Errorf("failed to generate coverage report: %w", err)
    }
    
    // Read and parse the coverage data
    if e.config.CoverageFormat != "html" {
        content, err := os.ReadFile(reportFile)
        if err != nil {
            return nil, fmt.Errorf("failed to read coverage report: %w", err)
        }
        data.Data = content
    }
    
    return data, nil
}
```

#### AFL++ Coverage Support

Update `pkg/domain/fuzzer/engines/aflplusplus/engine.go`:

```go
func (e *Engine) buildCommandArgs() []string {
    args := []string{}
    
    // Existing args...
    
    // Note: AFL++ doesn't have built-in coverage report generation
    // Coverage is collected post-fuzzing using external tools like afl-cov
    
    return args
}

// Configure environment for coverage collection
func (e *Engine) configureCoverageEnvironment() error {
    if !e.config.EnableCoverage {
        return nil
    }
    
    // For LLVM-based coverage, set profile environment variable
    if e.isLLVMMode() {
        profilePath := filepath.Join(e.outputDir, "coverage", "%p-%m.profraw")
        os.Setenv("LLVM_PROFILE_FILE", profilePath)
        
        // Ensure coverage directory exists
        coverageDir := filepath.Join(e.outputDir, "coverage")
        return os.MkdirAll(coverageDir, 0755)
    }
    
    return nil
}

// Post-processing for coverage using afl-cov or llvm-cov
func (e *Engine) collectCoverageData(ctx context.Context) (*types.CoverageData, error) {
    if !e.config.EnableCoverage {
        return nil, nil
    }
    
    opts := e.config.AFLPlusPlusOptions
    
    // Check if we should use afl-cov-fast for coverage analysis
    if e.shouldUseAFLCov() {
        return e.collectAFLCovData(ctx)
    }
    
    // For LLVM-instrumented binaries, use llvm-cov
    if e.isLLVMMode() {
        return e.collectLLVMCoverageData(ctx)
    }
    
    return nil, fmt.Errorf("coverage collection not supported for this AFL++ configuration")
}

// Collect coverage using afl-cov-fast
func (e *Engine) collectAFLCovData(ctx context.Context) (*types.CoverageData, error) {
    coverageCmd := exec.CommandContext(ctx, "afl-cov-fast.py",
        "-m", "llvm",
        "--code-dir", e.sourceDir,
        "--afl-fuzzing-dir", e.outputDir,
        "--coverage-cmd", fmt.Sprintf("%s @@", e.target),
        "--binary-path", e.target,
        "-j", "8")
    
    output, err := coverageCmd.Output()
    if err != nil {
        return nil, fmt.Errorf("afl-cov-fast failed: %w", err)
    }
    
    data := &types.CoverageData{
        Format:      "afl-cov",
        CollectedAt: time.Now(),
        Data:        output,
    }
    
    return data, nil
}

// Collect coverage using LLVM tools for LLVM-instrumented binaries
func (e *Engine) collectLLVMCoverageData(ctx context.Context) (*types.CoverageData, error) {
    opts := e.config.AFLPlusPlusOptions
    if opts.LLVMCovBinary == "" {
        opts.LLVMCovBinary = "llvm-cov"
    }
    if opts.LLVMProfData == "" {
        opts.LLVMProfData = "llvm-profdata"
    }
    
    coverageDir := filepath.Join(e.outputDir, "coverage")
    
    // First, run the target binary on all test cases to generate profraw files
    testCasesDir := filepath.Join(e.outputDir, "default", "queue")
    testCases, err := filepath.Glob(filepath.Join(testCasesDir, "id:*"))
    if err != nil {
        return nil, fmt.Errorf("failed to find test cases: %w", err)
    }
    
    // Process test cases to generate coverage
    for _, testCase := range testCases {
        cmd := exec.CommandContext(ctx, e.target, testCase)
        cmd.Env = append(os.Environ(), 
            fmt.Sprintf("LLVM_PROFILE_FILE=%s/testcase-%s.profraw", coverageDir, filepath.Base(testCase)))
        cmd.Run() // Ignore errors from crashes
    }
    
    // Merge all profraw files
    profDataFile := filepath.Join(coverageDir, "merged.profdata")
    profrawPattern := filepath.Join(coverageDir, "*.profraw")
    
    profrawFiles, err := filepath.Glob(profrawPattern)
    if err != nil || len(profrawFiles) == 0 {
        return nil, fmt.Errorf("no profraw files found")
    }
    
    mergeArgs := append([]string{"merge", "-sparse", "-o", profDataFile}, profrawFiles...)
    mergeCmd := exec.CommandContext(ctx, opts.LLVMProfData, mergeArgs...)
    if err := mergeCmd.Run(); err != nil {
        return nil, fmt.Errorf("failed to merge profile data: %w", err)
    }
    
    // Generate coverage report in requested format
    reportFile := filepath.Join(coverageDir, fmt.Sprintf("coverage.%s", e.config.CoverageFormat))
    
    covArgs := []string{"export", e.target, "-instr-profile=" + profDataFile}
    switch e.config.CoverageFormat {
    case "json":
        covArgs = append(covArgs, "-format=json")
    case "lcov":
        covArgs = append(covArgs, "-format=lcov")
    case "html":
        covArgs = []string{"show", e.target, "-instr-profile=" + profDataFile,
                          "-format=html", "-output-dir=" + coverageDir}
    default:
        return nil, fmt.Errorf("unsupported coverage format: %s", e.config.CoverageFormat)
    }
    
    if e.config.CoverageFormat != "html" {
        covArgs = append(covArgs, "-o", reportFile)
    }
    
    covCmd := exec.CommandContext(ctx, opts.LLVMCovBinary, covArgs...)
    if err := covCmd.Run(); err != nil {
        return nil, fmt.Errorf("failed to generate coverage report: %w", err)
    }
    
    // Read and return the coverage data
    data := &types.CoverageData{
        Format:      e.config.CoverageFormat,
        CollectedAt: time.Now(),
    }
    
    if e.config.CoverageFormat != "html" {
        content, err := os.ReadFile(reportFile)
        if err != nil {
            return nil, fmt.Errorf("failed to read coverage report: %w", err)
        }
        data.Data = content
    }
    
    return data, nil
}
```

### 4. Bot Executor Updates

Update `pkg/domain/bot/executor/fuzzer_executor.go`:

```go
type FuzzingResult struct {
    // Existing fields...
    
    // Coverage data
    CoverageCollected bool              `json:"coverage_collected"`
    CoverageStats     *CoverageStats    `json:"coverage_stats,omitempty"`
    CoverageReportID  string            `json:"coverage_report_id,omitempty"`
}

func (fe *FuzzerExecutor) runFuzzingJob(ctx context.Context, execCtx *ExecutionContext) *ExecutionResult {
    job := execCtx.Job
    
    // Configure fuzzer with coverage if enabled
    fuzzerConfig := fe.buildFuzzerConfig(job)
    if job.EnableCoverage {
        fuzzerConfig.EnableCoverage = true
        fuzzerConfig.CoverageFormat = job.CoverageFormat
        fuzzerConfig.CoverageDir = filepath.Join(fe.config.WorkDir, job.ID, "coverage")
    }
    
    // Create and run fuzzer
    fuzzer, err := fe.fuzzerFactory.CreateFuzzer(job.FuzzerType, job.TargetBinary, job.TargetArgs)
    if err != nil {
        return &ExecutionResult{Error: err}
    }
    
    if err := fuzzer.Configure(fuzzerConfig); err != nil {
        return &ExecutionResult{Error: err}
    }
    
    // Store active fuzzer
    fe.mu.Lock()
    fe.activeFuzzers[job.ID] = fuzzer
    fe.mu.Unlock()
    
    defer func() {
        fe.mu.Lock()
        delete(fe.activeFuzzers, job.ID)
        fe.mu.Unlock()
    }()
    
    // Start fuzzing
    if err := fuzzer.Start(ctx); err != nil {
        return &ExecutionResult{Error: err}
    }
    
    // Monitor fuzzing progress
    result := fe.monitorFuzzing(ctx, fuzzer, job)
    
    // Collect coverage if enabled
    if job.EnableCoverage {
        if err := fe.collectAndUploadCoverage(ctx, fuzzer, job, result); err != nil {
            fe.log.WithError(err).Error("Failed to collect coverage")
        }
    }
    
    return result
}

func (fe *FuzzerExecutor) collectAndUploadCoverage(ctx context.Context, fuzzer types.Fuzzer, job *jobtypes.Job, result *ExecutionResult) error {
    // Ensure fuzzer has stopped before collecting coverage
    fuzzer.Stop()
    
    // Get coverage data from fuzzer using type assertion based on fuzzer type
    var coverageData *types.CoverageData
    var err error
    
    switch job.FuzzerType {
    case "libfuzzer":
        if libfuzzerEngine, ok := fuzzer.(*libfuzzer.Engine); ok {
            coverageData, err = libfuzzerEngine.collectCoverageData(ctx)
        } else {
            return fmt.Errorf("invalid fuzzer type assertion for libfuzzer")
        }
    case "afl++":
        if aflEngine, ok := fuzzer.(*aflplusplus.Engine); ok {
            coverageData, err = aflEngine.collectCoverageData(ctx)
        } else {
            return fmt.Errorf("invalid fuzzer type assertion for afl++")
        }
    case "honggfuzz":
        if honggfuzzEngine, ok := fuzzer.(*honggfuzz.Engine); ok {
            coverageData, err = honggfuzzEngine.collectCoverageData(ctx)
        } else {
            return fmt.Errorf("invalid fuzzer type assertion for honggfuzz")
        }
    default:
        return fmt.Errorf("unsupported fuzzer type for coverage: %s", job.FuzzerType)
    }
    
    if err != nil {
        return fmt.Errorf("failed to collect coverage: %w", err)
    }
    
    if coverageData == nil {
        return nil
    }
    
    // Generate unique coverage report ID
    reportID := fmt.Sprintf("coverage_%s_%d", job.ID, time.Now().Unix())
    
    // Upload to storage
    storagePath := fmt.Sprintf("coverage/%s/%s", job.ID, reportID)
    if err := fe.uploadCoverageToStorage(ctx, storagePath, coverageData); err != nil {
        return fmt.Errorf("failed to upload coverage: %w", err)
    }
    
    // Parse coverage stats from the data
    stats, err := fe.parseCoverageStats(coverageData)
    if err != nil {
        fe.log.WithError(err).Warn("Failed to parse coverage stats")
    }
    
    // Update result
    result.CoverageCollected = true
    result.CoverageReportID = reportID
    result.CoverageStats = &CoverageStats{
        LineCoverage:     stats.LineCoverage,
        FunctionCoverage: stats.FunctionCoverage,
        BranchCoverage:   stats.BranchCoverage,
        CollectedAt:      coverageData.CollectedAt,
        ReportPath:       storagePath,
    }
    
    return nil
}
```

### 5. Storage Layer Updates

Create new coverage storage interface in `pkg/domain/coverage/repository/interface.go`:

```go
package repository

import (
    "context"
    "io"
    "time"
)

type CoverageRepository interface {
    // Store coverage report
    Store(ctx context.Context, jobID, reportID string, data io.Reader) error
    
    // Retrieve coverage report
    Get(ctx context.Context, jobID, reportID string) (io.ReadCloser, error)
    
    // List coverage reports for a job
    List(ctx context.Context, jobID string) ([]*CoverageReport, error)
    
    // Delete coverage report
    Delete(ctx context.Context, jobID, reportID string) error
    
    // Get metadata
    GetMetadata(ctx context.Context, jobID, reportID string) (*CoverageMetadata, error)
}

type CoverageReport struct {
    ID          string    `json:"id"`
    JobID       string    `json:"job_id"`
    Format      string    `json:"format"`
    Size        int64     `json:"size"`
    CreatedAt   time.Time `json:"created_at"`
    StoragePath string    `json:"storage_path"`
}

type CoverageMetadata struct {
    LineCoverage     float64   `json:"line_coverage"`
    FunctionCoverage float64   `json:"function_coverage"`
    BranchCoverage   float64   `json:"branch_coverage,omitempty"`
    TotalLines       int       `json:"total_lines"`
    CoveredLines     int       `json:"covered_lines"`
    TotalFunctions   int       `json:"total_functions"`
    CoveredFunctions int       `json:"covered_functions"`
    CollectedAt      time.Time `json:"collected_at"`
}
```

### 6. API Endpoints

Add coverage endpoints to `pkg/master/api_v3/openapi.yaml`:

```yaml
/jobs/{jobId}/coverage:
  get:
    tags: [jobs]
    summary: List coverage reports for a job
    operationId: listJobCoverage
    parameters:
      - $ref: '#/components/parameters/jobIdParam'
    responses:
      '200':
        description: List of coverage reports
        content:
          application/json:
            schema:
              type: array
              items:
                $ref: '#/components/schemas/CoverageReport'
      '404':
        $ref: '#/components/responses/NotFound'

/jobs/{jobId}/coverage/{reportId}:
  get:
    tags: [jobs]
    summary: Download coverage report
    operationId: getJobCoverageReport
    parameters:
      - $ref: '#/components/parameters/jobIdParam'
      - name: reportId
        in: path
        required: true
        schema:
          type: string
    responses:
      '200':
        description: Coverage report data
        content:
          application/json:
            schema:
              type: object
          text/html:
            schema:
              type: string
          text/plain:
            schema:
              type: string
      '404':
        $ref: '#/components/responses/NotFound'

/jobs/{jobId}/coverage/{reportId}/metadata:
  get:
    tags: [jobs]
    summary: Get coverage report metadata
    operationId: getJobCoverageMetadata
    parameters:
      - $ref: '#/components/parameters/jobIdParam'
      - name: reportId
        in: path
        required: true
        schema:
          type: string
    responses:
      '200':
        description: Coverage metadata
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/CoverageMetadata'
      '404':
        $ref: '#/components/responses/NotFound'
```

### 7. Web UI Updates

Update the job creation form to include coverage options:

```typescript
// web/src/components/JobCreationForm.tsx
interface JobFormData {
  // Existing fields...
  enableCoverage: boolean;
  coverageFormat: 'json' | 'html' | 'lcov' | 'cobertura';
}

const JobCreationForm: React.FC = () => {
  const [formData, setFormData] = useState<JobFormData>({
    // Existing defaults...
    enableCoverage: false,
    coverageFormat: 'json',
  });

  return (
    <Form onSubmit={handleSubmit}>
      {/* Existing form fields... */}
      
      <Form.Group>
        <Form.Check
          type="checkbox"
          id="enable-coverage"
          label="Enable code coverage collection"
          checked={formData.enableCoverage}
          onChange={(e) => setFormData({
            ...formData,
            enableCoverage: e.target.checked
          })}
        />
        <Form.Text className="text-muted">
          Collect code coverage data during fuzzing execution
        </Form.Text>
      </Form.Group>

      {formData.enableCoverage && (
        <Form.Group>
          <Form.Label>Coverage Report Format</Form.Label>
          <Form.Select
            value={formData.coverageFormat}
            onChange={(e) => setFormData({
              ...formData,
              coverageFormat: e.target.value as any
            })}
          >
            <option value="json">JSON</option>
            <option value="html">HTML</option>
            <option value="lcov">LCOV</option>
            <option value="cobertura">Cobertura XML</option>
          </Form.Select>
        </Form.Group>
      )}
    </Form>
  );
};
```

Add coverage report viewing:

```typescript
// web/src/components/JobCoverageView.tsx
const JobCoverageView: React.FC<{ jobId: string }> = ({ jobId }) => {
  const [reports, setReports] = useState<CoverageReport[]>([]);
  const [selectedReport, setSelectedReport] = useState<string | null>(null);
  const [metadata, setMetadata] = useState<CoverageMetadata | null>(null);

  useEffect(() => {
    fetchCoverageReports(jobId).then(setReports);
  }, [jobId]);

  const handleViewReport = async (reportId: string) => {
    const meta = await fetchCoverageMetadata(jobId, reportId);
    setMetadata(meta);
    setSelectedReport(reportId);
  };

  return (
    <div>
      <h3>Coverage Reports</h3>
      
      {metadata && (
        <Card className="mb-3">
          <Card.Body>
            <h5>Coverage Summary</h5>
            <ProgressBar>
              <ProgressBar
                variant="success"
                now={metadata.lineCoverage}
                label={`Lines: ${metadata.lineCoverage.toFixed(1)}%`}
              />
            </ProgressBar>
            <div className="mt-2">
              <Badge bg="info">
                {metadata.coveredLines}/{metadata.totalLines} lines
              </Badge>
              <Badge bg="info" className="ms-2">
                {metadata.coveredFunctions}/{metadata.totalFunctions} functions
              </Badge>
            </div>
          </Card.Body>
        </Card>
      )}

      <Table striped bordered hover>
        <thead>
          <tr>
            <th>Report ID</th>
            <th>Format</th>
            <th>Created</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {reports.map(report => (
            <tr key={report.id}>
              <td>{report.id}</td>
              <td>{report.format}</td>
              <td>{new Date(report.createdAt).toLocaleString()}</td>
              <td>
                <Button
                  size="sm"
                  onClick={() => handleViewReport(report.id)}
                >
                  View
                </Button>
                <Button
                  size="sm"
                  variant="secondary"
                  className="ms-2"
                  onClick={() => downloadReport(jobId, report.id)}
                >
                  Download
                </Button>
              </td>
            </tr>
          ))}
        </tbody>
      </Table>
    </div>
  );
};
```

### 8. Database Schema Updates

Add coverage tracking tables:

```sql
-- migrations/XXX_add_coverage_support.sql

-- Coverage reports table
CREATE TABLE coverage_reports (
    id VARCHAR(255) PRIMARY KEY,
    job_id VARCHAR(255) NOT NULL,
    format VARCHAR(50) NOT NULL,
    storage_path TEXT NOT NULL,
    size BIGINT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE,
    INDEX idx_coverage_job_id (job_id),
    INDEX idx_coverage_created_at (created_at)
);

-- Coverage metadata table
CREATE TABLE coverage_metadata (
    report_id VARCHAR(255) PRIMARY KEY,
    line_coverage FLOAT,
    function_coverage FLOAT,
    branch_coverage FLOAT,
    total_lines INT,
    covered_lines INT,
    total_functions INT,
    covered_functions INT,
    collected_at TIMESTAMP,
    
    FOREIGN KEY (report_id) REFERENCES coverage_reports(id) ON DELETE CASCADE
);

-- Update jobs table
ALTER TABLE jobs ADD COLUMN enable_coverage BOOLEAN DEFAULT FALSE;
ALTER TABLE jobs ADD COLUMN coverage_format VARCHAR(50);
ALTER TABLE jobs ADD COLUMN coverage_report_id VARCHAR(255);
```

### 9. Implementation Checklist

1. **API Updates**
   - [ ] Update OpenAPI spec with coverage fields
   - [ ] Add coverage endpoints
   - [ ] Update job creation endpoint to accept coverage options

2. **Domain Model Updates**
   - [ ] Add coverage fields to Job struct
   - [ ] Update FuzzerConfig with coverage options
   - [ ] Add coverage-specific options to each fuzzer type

3. **Fuzzer Engine Updates**
   - [ ] Implement coverage collection in LibFuzzer engine
   - [ ] Implement coverage collection in AFL++ engine
   - [ ] Implement coverage collection in Honggfuzz engine
   - [ ] Add coverage data parsing methods

4. **Bot Executor Updates**
   - [ ] Add coverage configuration to fuzzer setup
   - [ ] Implement coverage collection after fuzzing
   - [ ] Upload coverage reports to storage

5. **Storage Layer**
   - [ ] Create CoverageRepository interface
   - [ ] Implement file system storage backend
   - [ ] Implement S3 storage backend
   - [ ] Add coverage metadata storage

6. **Database Updates**
   - [ ] Create migration for coverage tables
   - [ ] Update job repository to handle coverage fields
   - [ ] Implement coverage repository

7. **API Implementation**
   - [ ] Implement coverage listing endpoint
   - [ ] Implement coverage download endpoint
   - [ ] Implement coverage metadata endpoint

8. **Web UI Updates**
   - [ ] Add coverage checkbox to job creation form
   - [ ] Add coverage format selection
   - [ ] Create coverage report viewing component
   - [ ] Add coverage summary visualization

9. **Testing**
   - [ ] Unit tests for coverage collection
   - [ ] Integration tests for coverage flow
   - [ ] E2E tests for coverage UI

10. **Documentation**
    - [ ] Update API documentation
    - [ ] Add coverage usage guide
    - [ ] Document coverage formats

## Coverage Format Details

### JSON Format
The JSON format follows the LLVM coverage JSON schema, providing detailed line-by-line and function coverage information.

### LCOV Format
LCOV format is widely supported by coverage visualization tools and CI systems.

### HTML Format
HTML reports provide a visual representation of code coverage with syntax highlighting.

### Cobertura XML Format
Cobertura format is commonly used in Java ecosystems and supported by many CI tools.

## Security Considerations

1. **Input Validation**: Validate coverage format selection to prevent injection
2. **File Access**: Restrict coverage report access to authorized users
3. **Storage Limits**: Implement size limits for coverage reports
4. **Cleanup**: Implement retention policies for old coverage reports

## Performance Considerations

1. **Async Processing**: Process coverage reports asynchronously to avoid blocking fuzzing
2. **Compression**: Compress large coverage reports before storage
3. **Caching**: Cache coverage metadata for quick access
4. **Batch Operations**: Support batch coverage report retrieval

## Future Enhancements

1. **Differential Coverage**: Show coverage changes between fuzzing runs
2. **Coverage Trends**: Track coverage progress over time
3. **Coverage Targets**: Set coverage goals and alerts
4. **Coverage Merging**: Combine coverage from multiple fuzzing jobs
5. **Real-time Updates**: Stream coverage updates during fuzzing