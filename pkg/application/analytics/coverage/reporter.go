package coverage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Reporter implements coverage report generation
type Reporter struct {
	logger logrus.FieldLogger
}

// NewReporter creates a new coverage reporter
func NewReporter(logger logrus.FieldLogger) *Reporter {
	return &Reporter{
		logger: logger.WithField("component", "coverage_reporter"),
	}
}

// GenerateHTMLReport generates an HTML coverage report
func (r *Reporter) GenerateHTMLReport(ctx context.Context, data *CoverageReport) (io.Reader, error) {
	r.logger.Debug("Generating HTML coverage report")

	tmpl := template.Must(template.New("coverage").Funcs(template.FuncMap{
		"formatTime":       formatTime,
		"formatPercentage": formatPercentage,
		"formatNumber":     formatNumber,
		"severityClass":    severityClass,
		"coverageClass":    coverageClass,
	}).Parse(htmlTemplate))

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute HTML template: %w", err)
	}

	return &buf, nil
}

// GenerateMarkdownReport generates a Markdown coverage report
func (r *Reporter) GenerateMarkdownReport(ctx context.Context, data *CoverageReport) (io.Reader, error) {
	r.logger.Debug("Generating Markdown coverage report")

	var buf bytes.Buffer

	// Title and metadata
	fmt.Fprintf(&buf, "# Coverage Report\n\n")
	fmt.Fprintf(&buf, "**Generated:** %s\n", data.GeneratedAt.Format(time.RFC3339))
	if data.CampaignID != "" {
		fmt.Fprintf(&buf, "**Campaign ID:** %s\n", data.CampaignID)
	}
	fmt.Fprintf(&buf, "**Time Range:** %s to %s\n\n",
		data.TimeRange.Start.Format(time.RFC3339),
		data.TimeRange.End.Format(time.RFC3339))

	// Summary section
	if data.Summary != nil {
		fmt.Fprintf(&buf, "## Summary\n\n")
		fmt.Fprintf(&buf, "| Metric | Value |\n")
		fmt.Fprintf(&buf, "|--------|-------|\n")
		fmt.Fprintf(&buf, "| Total Coverage | %.1f%% |\n", data.Summary.TotalCoverage)
		fmt.Fprintf(&buf, "| Line Coverage | %.1f%% |\n", data.Summary.LineCoverage)
		fmt.Fprintf(&buf, "| Function Coverage | %.1f%% |\n", data.Summary.FunctionCoverage)
		fmt.Fprintf(&buf, "| Branch Coverage | %.1f%% |\n", data.Summary.BranchCoverage)
		fmt.Fprintf(&buf, "| Covered Edges | %d / %d |\n", data.Summary.CoveredEdges, data.Summary.TotalEdges)
		fmt.Fprintf(&buf, "| Coverage Growth Rate | %.2f%%/hour |\n", data.Summary.CoverageGrowthRate)
		fmt.Fprintf(&buf, "| Quality Score | %.1f |\n", data.Summary.QualityScore)
		fmt.Fprintf(&buf, "\n")
	}

	// Insights section
	if len(data.Insights) > 0 {
		fmt.Fprintf(&buf, "## Key Insights\n\n")

		// Sort insights by severity
		sortedInsights := make([]CoverageInsight, len(data.Insights))
		copy(sortedInsights, data.Insights)
		sort.Slice(sortedInsights, func(i, j int) bool {
			return getSeverityWeight(sortedInsights[i].Severity) > getSeverityWeight(sortedInsights[j].Severity)
		})

		for _, insight := range sortedInsights {
			fmt.Fprintf(&buf, "### %s %s\n\n", getSeverityEmoji(insight.Severity), insight.Title)
			fmt.Fprintf(&buf, "**Type:** %s | **Severity:** %s\n\n", insight.Type, insight.Severity)
			fmt.Fprintf(&buf, "%s\n\n", insight.Description)
			fmt.Fprintf(&buf, "**Impact:** %s\n\n", insight.Impact)

			if len(insight.Actions) > 0 {
				fmt.Fprintf(&buf, "**Recommended Actions:**\n")
				for _, action := range insight.Actions {
					fmt.Fprintf(&buf, "- %s\n", action)
				}
				fmt.Fprintf(&buf, "\n")
			}
		}
	}

	// Coverage breakdown
	if data.Breakdown != nil {
		fmt.Fprintf(&buf, "## Coverage Breakdown\n\n")

		// By complexity
		if len(data.Breakdown.ByComplexity) > 0 {
			fmt.Fprintf(&buf, "### By Complexity\n\n")
			fmt.Fprintf(&buf, "| Complexity | Coverage |\n")
			fmt.Fprintf(&buf, "|------------|----------|\n")
			for complexity, coverage := range data.Breakdown.ByComplexity {
				fmt.Fprintf(&buf, "| %s | %.1f%% |\n", complexity, coverage)
			}
			fmt.Fprintf(&buf, "\n")
		}

		// By risk
		if len(data.Breakdown.ByRisk) > 0 {
			fmt.Fprintf(&buf, "### By Risk Level\n\n")
			fmt.Fprintf(&buf, "| Risk Level | Coverage |\n")
			fmt.Fprintf(&buf, "|------------|----------|\n")
			for risk, coverage := range data.Breakdown.ByRisk {
				fmt.Fprintf(&buf, "| %s | %.1f%% |\n", risk, coverage)
			}
			fmt.Fprintf(&buf, "\n")
		}
	}

	// Details section
	if data.Details != nil {
		// Hot spots
		if len(data.Details.HotSpots) > 0 {
			fmt.Fprintf(&buf, "## Coverage Hot Spots\n\n")
			fmt.Fprintf(&buf, "Areas with high coverage activity:\n\n")
			for i, hotspot := range data.Details.HotSpots {
				if i >= 5 {
					break // Limit to top 5
				}
				fmt.Fprintf(&buf, "%d. **%s** - %d hits (%.1f%% coverage)\n",
					i+1, hotspot.Location, hotspot.HitCount, hotspot.Coverage)
			}
			fmt.Fprintf(&buf, "\n")
		}

		// Cold spots
		if len(data.Details.ColdSpots) > 0 {
			fmt.Fprintf(&buf, "## Coverage Cold Spots\n\n")
			fmt.Fprintf(&buf, "Areas needing attention:\n\n")
			for i, coldspot := range data.Details.ColdSpots {
				if i >= 5 {
					break // Limit to top 5
				}
				fmt.Fprintf(&buf, "%d. **%s** - %.1f%% coverage (%s risk)\n",
					i+1, coldspot.Location, coldspot.Coverage, coldspot.Risk)
				if len(coldspot.Suggestions) > 0 {
					fmt.Fprintf(&buf, "   - %s\n", coldspot.Suggestions[0])
				}
			}
			fmt.Fprintf(&buf, "\n")
		}
	}

	// Trends section
	if data.Trends != nil && data.Trends.Growth != nil {
		fmt.Fprintf(&buf, "## Coverage Trends\n\n")
		fmt.Fprintf(&buf, "- **Growth Pattern:** %s\n", data.Trends.Growth.GrowthPattern)
		fmt.Fprintf(&buf, "- **Current Growth Rate:** %.2f%%/hour\n", data.Trends.Growth.CurrentGrowthRate)
		fmt.Fprintf(&buf, "- **Average Growth Rate:** %.2f%%/hour\n", data.Trends.Growth.AverageGrowthRate)

		if data.Trends.Growth.TimeToSaturation != nil {
			fmt.Fprintf(&buf, "- **Estimated Time to Saturation:** %s\n", data.Trends.Growth.TimeToSaturation)
		}
		fmt.Fprintf(&buf, "\n")

		// Projections
		if data.Trends.Projections != nil {
			fmt.Fprintf(&buf, "### Coverage Projections\n\n")
			fmt.Fprintf(&buf, "| Timeframe | Projected Coverage | Confidence |\n")
			fmt.Fprintf(&buf, "|-----------|-------------------|------------|\n")
			fmt.Fprintf(&buf, "| 1 Day | %.1f%% | %.0f%% |\n",
				data.Trends.Projections.OneDay, data.Trends.Projections.Confidence*100)
			fmt.Fprintf(&buf, "| 1 Week | %.1f%% | %.0f%% |\n",
				data.Trends.Projections.OneWeek, data.Trends.Projections.Confidence*100)
			fmt.Fprintf(&buf, "| 1 Month | %.1f%% | %.0f%% |\n",
				data.Trends.Projections.OneMonth, data.Trends.Projections.Confidence*100)
			fmt.Fprintf(&buf, "\n")
		}
	}

	return &buf, nil
}

// GenerateJSONReport generates a JSON coverage report
func (r *Reporter) GenerateJSONReport(ctx context.Context, data *CoverageReport) (io.Reader, error) {
	r.logger.Debug("Generating JSON coverage report")

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal coverage report: %w", err)
	}

	return bytes.NewReader(jsonData), nil
}

// GenerateSummary generates a brief summary of coverage
func (r *Reporter) GenerateSummary(ctx context.Context, data *CoverageReport) (*CoverageSummary, error) {
	if data.Summary == nil {
		return nil, fmt.Errorf("no summary data available")
	}
	return data.Summary, nil
}

// Template helper functions

func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func formatPercentage(value float64) string {
	return fmt.Sprintf("%.1f%%", value)
}

func formatNumber(value int64) string {
	// Add comma separators for large numbers
	str := fmt.Sprintf("%d", value)
	n := len(str)
	if n <= 3 {
		return str
	}

	var result strings.Builder
	for i, ch := range str {
		if i > 0 && (n-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(ch)
	}
	return result.String()
}

func severityClass(severity InsightSeverity) string {
	switch severity {
	case InsightSeverityCritical:
		return "severity-critical"
	case InsightSeverityHigh:
		return "severity-high"
	case InsightSeverityMedium:
		return "severity-medium"
	case InsightSeverityLow:
		return "severity-low"
	default:
		return "severity-info"
	}
}

func coverageClass(coverage float64) string {
	if coverage >= 80 {
		return "coverage-excellent"
	} else if coverage >= 60 {
		return "coverage-good"
	} else if coverage >= 40 {
		return "coverage-fair"
	} else if coverage >= 20 {
		return "coverage-poor"
	}
	return "coverage-critical"
}

func getSeverityWeight(severity InsightSeverity) int {
	switch severity {
	case InsightSeverityCritical:
		return 5
	case InsightSeverityHigh:
		return 4
	case InsightSeverityMedium:
		return 3
	case InsightSeverityLow:
		return 2
	default:
		return 1
	}
}

func getSeverityEmoji(severity InsightSeverity) string {
	switch severity {
	case InsightSeverityCritical:
		return "🚨"
	case InsightSeverityHigh:
		return "⚠️"
	case InsightSeverityMedium:
		return "⚡"
	case InsightSeverityLow:
		return "📌"
	default:
		return "ℹ️"
	}
}

// HTML template
const htmlTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Coverage Report</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            line-height: 1.6;
            color: #333;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .header {
            background-color: #fff;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }
        h1 {
            color: #2c3e50;
            margin-bottom: 10px;
        }
        .metadata {
            color: #666;
            font-size: 14px;
        }
        .summary-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .metric-card {
            background-color: #fff;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .metric-label {
            font-size: 14px;
            color: #666;
            margin-bottom: 5px;
        }
        .metric-value {
            font-size: 28px;
            font-weight: bold;
            color: #2c3e50;
        }
        .metric-subvalue {
            font-size: 14px;
            color: #999;
        }
        .section {
            background-color: #fff;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }
        .insight {
            padding: 15px;
            margin-bottom: 15px;
            border-radius: 6px;
            border-left: 4px solid;
        }
        .severity-critical {
            background-color: #fee;
            border-color: #dc3545;
        }
        .severity-high {
            background-color: #fff3cd;
            border-color: #ffc107;
        }
        .severity-medium {
            background-color: #e7f3ff;
            border-color: #0066cc;
        }
        .severity-low {
            background-color: #d4edda;
            border-color: #28a745;
        }
        .severity-info {
            background-color: #f8f9fa;
            border-color: #6c757d;
        }
        .coverage-bar {
            background-color: #e0e0e0;
            border-radius: 4px;
            height: 20px;
            overflow: hidden;
            position: relative;
        }
        .coverage-fill {
            height: 100%;
            transition: width 0.3s ease;
        }
        .coverage-excellent { background-color: #28a745; }
        .coverage-good { background-color: #17a2b8; }
        .coverage-fair { background-color: #ffc107; }
        .coverage-poor { background-color: #fd7e14; }
        .coverage-critical { background-color: #dc3545; }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 10px;
        }
        th, td {
            padding: 10px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th {
            background-color: #f8f9fa;
            font-weight: 600;
        }
        .trend-chart {
            height: 200px;
            background-color: #f8f9fa;
            border-radius: 4px;
            display: flex;
            align-items: center;
            justify-content: center;
            color: #999;
            margin-top: 20px;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>Coverage Report</h1>
        <div class="metadata">
            <div>Generated: {{formatTime .GeneratedAt}}</div>
            {{if .CampaignID}}<div>Campaign ID: {{.CampaignID}}</div>{{end}}
            <div>Time Range: {{formatTime .TimeRange.Start}} to {{formatTime .TimeRange.End}}</div>
        </div>
    </div>

    {{if .Summary}}
    <div class="summary-grid">
        <div class="metric-card">
            <div class="metric-label">Total Coverage</div>
            <div class="metric-value">{{formatPercentage .Summary.TotalCoverage}}</div>
            <div class="coverage-bar">
                <div class="coverage-fill {{coverageClass .Summary.TotalCoverage}}" style="width: {{.Summary.TotalCoverage}}%"></div>
            </div>
        </div>
        <div class="metric-card">
            <div class="metric-label">Function Coverage</div>
            <div class="metric-value">{{formatPercentage .Summary.FunctionCoverage}}</div>
            <div class="metric-subvalue">{{.Summary.CoveredFunctions}} / {{.Summary.TotalFunctions}} functions</div>
        </div>
        <div class="metric-card">
            <div class="metric-label">Branch Coverage</div>
            <div class="metric-value">{{formatPercentage .Summary.BranchCoverage}}</div>
            <div class="metric-subvalue">{{.Summary.CoveredBranches}} / {{.Summary.TotalBranches}} branches</div>
        </div>
        <div class="metric-card">
            <div class="metric-label">Quality Score</div>
            <div class="metric-value">{{.Summary.QualityScore}}</div>
            <div class="metric-subvalue">Out of 100</div>
        </div>
    </div>
    {{end}}

    {{if .Insights}}
    <div class="section">
        <h2>Key Insights</h2>
        {{range .Insights}}
        <div class="insight {{severityClass .Severity}}">
            <h3>{{.Title}}</h3>
            <p><strong>Type:</strong> {{.Type}} | <strong>Severity:</strong> {{.Severity}}</p>
            <p>{{.Description}}</p>
            <p><strong>Impact:</strong> {{.Impact}}</p>
            {{if .Actions}}
            <p><strong>Recommended Actions:</strong></p>
            <ul>
                {{range .Actions}}
                <li>{{.}}</li>
                {{end}}
            </ul>
            {{end}}
        </div>
        {{end}}
    </div>
    {{end}}

    {{if .Breakdown}}
    <div class="section">
        <h2>Coverage Breakdown</h2>
        
        {{if .Breakdown.ByRisk}}
        <h3>By Risk Level</h3>
        <table>
            <thead>
                <tr>
                    <th>Risk Level</th>
                    <th>Coverage</th>
                    <th>Visual</th>
                </tr>
            </thead>
            <tbody>
                {{range $risk, $coverage := .Breakdown.ByRisk}}
                <tr>
                    <td>{{$risk}}</td>
                    <td>{{formatPercentage $coverage}}</td>
                    <td>
                        <div class="coverage-bar" style="width: 200px;">
                            <div class="coverage-fill {{coverageClass $coverage}}" style="width: {{$coverage}}%"></div>
                        </div>
                    </td>
                </tr>
                {{end}}
            </tbody>
        </table>
        {{end}}
    </div>
    {{end}}

    {{if .Trends}}
    <div class="section">
        <h2>Coverage Trends</h2>
        {{if .Trends.Growth}}
        <p><strong>Growth Pattern:</strong> {{.Trends.Growth.GrowthPattern}}</p>
        <p><strong>Current Growth Rate:</strong> {{.Trends.Growth.CurrentGrowthRate}}%/hour</p>
        {{if .Trends.Growth.TimeToSaturation}}
        <p><strong>Estimated Time to Saturation:</strong> {{.Trends.Growth.TimeToSaturation}}</p>
        {{end}}
        {{end}}
        
        <div class="trend-chart">
            [Trend visualization would go here]
        </div>
    </div>
    {{end}}
</body>
</html>
`
