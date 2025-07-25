// Package trends provides trend analysis services for fuzzing campaigns.
//
// This package analyzes patterns and trends in fuzzing data over time including:
// - Coverage growth trends
// - Performance trends
// - Crash discovery patterns
// - Anomaly detection
// - Trend forecasting
//
// The trend analyzer helps identify patterns, detect anomalies, and forecast
// future behavior based on historical data.
//
// Usage:
//
//	analyzer := trends.NewAnalyzer(campaignRepo, crashRepo, logger)
//	trends, err := analyzer.AnalyzeCoverageTrends(ctx, campaignID, trends.TrendPeriodDaily)
//	if err != nil {
//	    log.WithError(err).Error("Failed to analyze trends")
//	    return err
//	}
//
// The analyzer can generate trend reports, detect anomalies, and provide
// forecasts based on historical patterns.
package trends
