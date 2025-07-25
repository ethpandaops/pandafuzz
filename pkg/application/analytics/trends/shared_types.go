package trends

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
