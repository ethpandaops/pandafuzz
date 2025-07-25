/*
Package coverage provides coverage analysis services for fuzzing campaigns.

The coverage package is responsible for analyzing and reporting on code coverage
achieved during fuzzing operations. It provides comprehensive insights into
coverage metrics, identifies gaps, and tracks coverage growth over time.

# Core Components

The package consists of several key components:

1. **Analyzer**: Processes raw coverage data and generates structured reports
2. **Reporter**: Generates human-readable reports in multiple formats
3. **Types**: Defines data structures for coverage information

# Coverage Metrics

The package tracks several types of coverage:
  - Line coverage: Percentage of code lines executed
  - Function coverage: Percentage of functions called
  - Branch coverage: Percentage of conditional branches taken
  - Edge coverage: Execution paths through the program

# Analysis Features

  - Real-time coverage tracking during fuzzing campaigns
  - Historical trend analysis to identify coverage plateaus
  - Module-level and function-level breakdowns
  - Hot spot identification for frequently covered code
  - Cold spot detection for uncovered areas
  - Coverage quality scoring

# Report Generation

The reporter supports multiple output formats:
  - JSON for programmatic consumption
  - HTML for interactive visualization
  - Markdown for documentation

# Usage Example

	analyzer := coverage.NewAnalyzer(db, logger)

	// Analyze campaign coverage
	report, err := analyzer.AnalyzeCampaign(ctx, campaignID, timeRange)
	if err != nil {
		return err
	}

	// Generate HTML report
	reporter := coverage.NewReporter()
	html, err := reporter.GenerateHTML(report)
	if err != nil {
		return err
	}

# Integration

The coverage analyzer integrates with:
  - Campaign services for fuzzing context
  - Bot services for execution data
  - Storage services for coverage file access
  - Trend analyzer for historical analysis

# Performance Considerations

Coverage analysis can be resource-intensive for large campaigns. The analyzer
implements several optimizations:
  - Incremental processing of new coverage data
  - Caching of computed metrics
  - Parallel processing of independent modules
  - Configurable analysis depth

For more information, see the parent analytics package documentation.
*/
package coverage
