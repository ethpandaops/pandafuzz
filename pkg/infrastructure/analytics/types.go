package analytics

import "time"

// CoverageHistoryEntry represents a point in coverage history
type CoverageHistoryEntry struct {
	Timestamp        time.Time
	TotalCoverage    float64
	LineCoverage     float64
	FunctionCoverage float64
	BranchCoverage   float64
	CoveredEdges     int64
	TotalEdges       int64
}

// CrashTimelineEntry represents a point in crash timeline
type CrashTimelineEntry struct {
	Timestamp     time.Time
	CrashCount    int
	UniqueCrashes int
	Severity      string
}

// FuzzerComparison represents comparison data between fuzzers
type FuzzerComparison struct {
	Campaigns      []FuzzerCampaignData
	ComparisonDate time.Time
}

// FuzzerCampaignData represents campaign data for comparison
type FuzzerCampaignData struct {
	CampaignID   string
	CampaignName string
	FuzzerType   string
	Metrics      FuzzerMetrics
}

// FuzzerMetrics represents fuzzer performance metrics
type FuzzerMetrics struct {
	Coverage         float64
	Crashes          int
	ExecutionsPerSec int64
	CorpusSize       int
}

// CampaignInsights represents insights for a campaign
type CampaignInsights struct {
	CampaignID  string
	Insights    []Insight
	GeneratedAt time.Time
}

// Insight represents a single insight
type Insight struct {
	Type            string
	Severity        string
	Title           string
	Description     string
	Impact          string
	Recommendations []string
}
