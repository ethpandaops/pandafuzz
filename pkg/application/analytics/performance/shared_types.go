package performance

import "time"

// TimeRange represents a time range for analysis
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// TrendPeriod represents the period for trend analysis
type TrendPeriod string

const (
	TrendPeriodHourly  TrendPeriod = "hourly"
	TrendPeriodDaily   TrendPeriod = "daily"
	TrendPeriodWeekly  TrendPeriod = "weekly"
	TrendPeriodMonthly TrendPeriod = "monthly"
)

// AggregationType represents how metrics should be aggregated
type AggregationType string

const (
	AggregationTypeSum     AggregationType = "sum"
	AggregationTypeAverage AggregationType = "average"
	AggregationTypeMax     AggregationType = "max"
	AggregationTypeMin     AggregationType = "min"
	AggregationTypeMedian  AggregationType = "median"
	AggregationTypeP95     AggregationType = "p95"
	AggregationTypeP99     AggregationType = "p99"
)
