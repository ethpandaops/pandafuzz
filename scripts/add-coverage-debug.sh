#!/bin/bash

# Quick script to add debug logging to trace coverage data flow

set -e

echo "=== Adding Debug Logging to Coverage Flow ==="

# Add debug logging to AFL++ CollectCoverageData
echo "Adding debug to pkg/fuzzer/aflplusplus.go..."

# Find the CollectCoverageData function and add logging
sed -i.bak '/func (a \*AFLPlusPlus) CollectCoverageData/,/^}$/{
    /defer a.mu.RUnlock()/a\
\	a.logger.Debug("DEBUG: CollectCoverageData started")
    /if statsData != nil {/a\
\	\	a.logger.WithField("stats_file_size", len(statsData)).Debug("DEBUG: Found stats file")
    /if val, ok := stats\["edges_found"\]/a\
\	\	\	a.logger.WithFields(logrus.Fields{"edges_found_raw": val, "type": fmt.Sprintf("%T", val)}).Debug("DEBUG: Found edges_found")
    /coverageData\["line_coverage"\] = float64(edges) \* 2.0/a\
\	\	\	\	a.logger.WithFields(logrus.Fields{"edges": edges, "line_coverage": float64(edges) * 2.0}).Debug("DEBUG: Calculated line coverage")
    /return coverageData, nil/i\
\	a.logger.WithFields(logrus.Fields{"coverage_data_keys": fmt.Sprintf("%v", reflect.ValueOf(coverageData).MapKeys()), "line_coverage": coverageData["line_coverage"]}).Debug("DEBUG: Returning coverage data")
}' pkg/fuzzer/aflplusplus.go

# Add debug logging to executor_fuzzer.go
echo "Adding debug to pkg/bot/executor_fuzzer.go..."

sed -i.bak '/coverageData, err = collector.CollectCoverageData()/a\
\	\	fje.logger.WithFields(logrus.Fields{"coverage_data": fmt.Sprintf("%+v", coverageData), "err": err}).Debug("DEBUG: Collected coverage from fuzzer")' pkg/bot/executor_fuzzer.go

sed -i.bak '/if val, ok := coverageData\["line_coverage"\]/,/}/{ 
    /lineCoverage = val/a\
\	\	\	fje.logger.WithFields(logrus.Fields{"line_coverage_float": val}).Debug("DEBUG: Extracted line coverage as float64")
}' pkg/bot/executor_fuzzer.go

sed -i.bak '/} else if val, ok := coverageData\["coverage_percent"\]/,/}/{
    /lineCoverage = val/a\
\	\	\	fje.logger.WithFields(logrus.Fields{"coverage_percent_float": val}).Debug("DEBUG: Extracted coverage_percent as float64")
}' pkg/bot/executor_fuzzer.go

sed -i.bak '/if err := reporter.ReportCoverageData(coverageReport)/i\
\	\	\	fje.logger.WithFields(logrus.Fields{"report": fmt.Sprintf("%+v", coverageReport)}).Debug("DEBUG: About to send coverage report")' pkg/bot/executor_fuzzer.go

# Add debug to client
echo "Adding debug to pkg/bot/client.go..."

sed -i.bak '/func (rc \*RetryClient) ReportCoverageData/,/^}/{
    /err := rc.retryManager.Execute/i\
\	rc.logger.WithFields(logrus.Fields{"coverage_data": fmt.Sprintf("%+v", coverageData)}).Debug("DEBUG: ReportCoverageData called")
}' pkg/bot/client.go

# Add debug to master API
echo "Adding debug to pkg/master/api_coverage_simple.go..."

sed -i.bak '/func (s \*Server) handleSubmitCoverageReport/,/^}/{
    /var req CoverageReportRequest/a\
\	s.logger.Debug("DEBUG: handleSubmitCoverageReport called")
    /if err := json.NewDecoder/a\
\	s.logger.WithFields(logrus.Fields{"request": fmt.Sprintf("%+v", req)}).Debug("DEBUG: Decoded coverage report request")
}' pkg/master/api_coverage_simple.go

echo "Debug logging added!"
echo
echo "Now rebuild and test:"
echo "1. make docker"
echo "2. docker-compose down && docker-compose up -d"
echo "3. Run a test job and check the logs"