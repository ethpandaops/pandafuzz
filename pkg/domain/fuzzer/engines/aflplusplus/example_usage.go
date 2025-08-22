package aflplusplus

import (
	"context"
	"fmt"
	"log"

	"github.com/sirupsen/logrus"
)

// ExampleUsage demonstrates how to use the AFL++ coverage extractor
func ExampleUsage() {
	// Create a logger
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// Create coverage extractor
	extractor := NewCoverageExtractor(logger)

	// Example 1: Extract bitmap coverage from AFL++ output directory
	ctx := context.Background()
	outputDir := "/path/to/afl/output"

	fmt.Println("=== Extracting Bitmap Coverage ===")
	coverageData, err := extractor.ExtractBitmapCoverage(ctx, outputDir)
	if err != nil {
		log.Printf("Failed to extract bitmap coverage: %v", err)
		return
	}

	fmt.Printf("Extracted Coverage Data:\n")
	fmt.Printf("  Edges Covered: %d\n", coverageData.Edges)
	fmt.Printf("  Total Edges: %d\n", coverageData.TotalEdges)
	fmt.Printf("  Coverage Percentage: %.2f%%\n", coverageData.CoveragePercent)
	fmt.Printf("  Queue Size: %d\n", coverageData.QueueSize)
	fmt.Printf("  AFL++ Version: %s\n", coverageData.FuzzerVersion)

	// Example 2: Convert to LCOV format
	targetBinary := "/path/to/target/binary"

	fmt.Println("\n=== Converting to LCOV Format ===")
	lcovContent, err := extractor.ConvertToLCOV(ctx, coverageData, targetBinary)
	if err != nil {
		log.Printf("Failed to convert to LCOV: %v", err)
		return
	}

	fmt.Printf("LCOV file generated: %s\n", coverageData.LCOVFile)
	fmt.Printf("LCOV content preview:\n%s...\n", lcovContent[:200])

	// Example 3: Convert to JSON format
	fmt.Println("\n=== Converting to JSON Format ===")
	jsonContent, err := extractor.ConvertToJSON(ctx, coverageData)
	if err != nil {
		log.Printf("Failed to convert to JSON: %v", err)
		return
	}

	fmt.Printf("JSON file generated: %s\n", coverageData.JSONFile)
	fmt.Printf("JSON content preview:\n%s...\n", jsonContent[:200])

	// Example 4: Generate complete coverage report (recommended approach)
	fmt.Println("\n=== Generating Complete Coverage Report ===")
	finalData, err := extractor.GenerateCoverageReport(ctx, outputDir, targetBinary, "both")
	if err != nil {
		log.Printf("Failed to generate coverage report: %v", err)
		return
	}

	fmt.Printf("Coverage Report Generated:\n")
	fmt.Printf("  LCOV File: %s\n", finalData.LCOVFile)
	fmt.Printf("  JSON File: %s\n", finalData.JSONFile)
	fmt.Printf("  Coverage: %.2f%% (%d/%d edges)\n",
		finalData.CoveragePercent, finalData.Edges, finalData.TotalEdges)

	// Example 5: Using afl-cov if available
	fmt.Println("\n=== Using afl-cov for Detailed Analysis ===")
	aflCovData, err := extractor.RunAFLCov(ctx, outputDir, targetBinary, "")
	if err != nil {
		fmt.Printf("afl-cov not available or failed: %v\n", err)
		fmt.Println("Falling back to basic bitmap analysis (this is normal)")
	} else {
		fmt.Printf("afl-cov analysis completed:\n")
		fmt.Printf("  Analysis Directory: %s\n", aflCovData.AFLCovDir)
		fmt.Printf("  Generated LCOV: %s\n", aflCovData.LCOVFile)
	}
}

// ExampleIntegrationWithEngine demonstrates integration with the AFL++ engine
func ExampleIntegrationWithEngine() {
	// This would typically be called from within the AFL++ engine
	// after a fuzzing session completes or periodically during fuzzing

	logger := logrus.New()
	extractor := NewCoverageExtractor(logger)
	ctx := context.Background()

	// Simulate AFL++ engine parameters
	outputDir := "/tmp/afl-output"
	targetBinary := "/usr/bin/target"

	fmt.Println("=== Integration Example ===")

	// Step 1: Check if coverage should be collected
	// (this would be based on engine configuration)
	enableCoverage := true
	coverageFormat := "auto" // "lcov", "json", "both", or "auto"

	if !enableCoverage {
		fmt.Println("Coverage collection disabled")
		return
	}

	// Step 2: Generate coverage report
	coverageData, err := extractor.GenerateCoverageReport(ctx, outputDir, targetBinary, coverageFormat)
	if err != nil {
		log.Printf("Coverage collection failed: %v", err)
		return
	}

	// Step 3: Log results for monitoring/debugging
	logger.WithFields(logrus.Fields{
		"edges":            coverageData.Edges,
		"coverage_percent": coverageData.CoveragePercent,
		"queue_size":       coverageData.QueueSize,
		"lcov_file":        coverageData.LCOVFile,
		"json_file":        coverageData.JSONFile,
	}).Info("Coverage analysis completed")

	// Step 4: Optional - store coverage data for later analysis
	// This could be saved to database, sent to monitoring system, etc.
	storeCoverageData(coverageData)
}

// storeCoverageData would integrate with your storage/monitoring system
func storeCoverageData(data *CoverageData) {
	// Example: Store to database, send to metrics collector, etc.
	fmt.Printf("Storing coverage data: %.2f%% coverage with %d edges\n",
		data.CoveragePercent, data.Edges)
}
