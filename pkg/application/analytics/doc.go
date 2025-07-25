// Package analytics provides application-level analytics services for fuzzing data analysis.
//
// This package contains services that analyze fuzzing campaign data to provide insights on:
// - Coverage analysis and reporting
// - Performance metrics and optimization
// - Trend analysis over time
// - Anomaly detection
//
// The analytics services orchestrate domain logic and repositories to provide comprehensive
// analysis capabilities. They support multiple output formats including JSON, HTML, and Markdown.
//
// Architecture:
//
// The package is organized into sub-packages:
// - coverage: Analyzes and reports on fuzzing coverage data
// - performance: Tracks and analyzes performance metrics
// - trends: Identifies patterns and trends over time
//
// Usage:
//
//	cfg := analytics.NewConfig()
//	coverageAnalyzer := coverage.NewAnalyzer(campaignRepo, crashRepo, logger)
//	report, err := coverageAnalyzer.AnalyzeCampaign(ctx, campaignID)
//	if err != nil {
//	    log.WithError(err).Error("Failed to analyze campaign")
//	    return err
//	}
//
// The analytics services are designed to be used by API handlers, scheduled jobs,
// or other application services that need to generate insights from fuzzing data.
package analytics
