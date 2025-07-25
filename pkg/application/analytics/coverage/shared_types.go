package coverage

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

// ReportFormat represents the format for generated reports
type ReportFormat string

const (
	ReportFormatJSON     ReportFormat = "json"
	ReportFormatHTML     ReportFormat = "html"
	ReportFormatMarkdown ReportFormat = "markdown"
	ReportFormatPDF      ReportFormat = "pdf"
	ReportFormatCSV      ReportFormat = "csv"
)

// DetailLevel represents the level of detail in reports
type DetailLevel string

const (
	DetailLevelSummary  DetailLevel = "summary"
	DetailLevelStandard DetailLevel = "standard"
	DetailLevelDetailed DetailLevel = "detailed"
	DetailLevelFull     DetailLevel = "full"
)
