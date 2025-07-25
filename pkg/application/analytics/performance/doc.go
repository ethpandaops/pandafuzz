// Package performance provides performance analysis services for fuzzing campaigns.
//
// This package analyzes various performance metrics including:
// - Execution speed and throughput
// - Resource utilization (CPU, memory, disk)
// - Bot efficiency and productivity
// - Queue processing times
// - Bottleneck identification
//
// The performance analyzer helps identify optimization opportunities and provides
// recommendations for improving fuzzing efficiency.
//
// Usage:
//
//	analyzer := performance.NewAnalyzer(campaignRepo, botRepo, metricsCollector, logger)
//	report, err := analyzer.AnalyzeCampaignPerformance(ctx, campaignID)
//	if err != nil {
//	    log.WithError(err).Error("Failed to analyze performance")
//	    return err
//	}
//
// The analyzer can generate performance reports, identify bottlenecks, and provide
// optimization recommendations based on collected metrics.
package performance
