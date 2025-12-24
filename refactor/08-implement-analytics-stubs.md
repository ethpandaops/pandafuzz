# 08: Implement Analytics Stubs

## Priority: LOW
## Risk Level: LOW
## Estimated Effort: 8-12 hours

## Prerequisites

- Complete steps 01-07 first
- API and data access patterns must be stable

## Problem Statement

`pkg/service/analytics_service.go` has 15+ TODO comments for unimplemented methods:

```go
Line 425: // TODO: Implement actual data retrieval from store
Line 469: // TODO: Implement actual crash rate calculation
Line 498: // TODO: Implement actual performance metrics calculation
Line 636: // TODO: Implement real-time metrics aggregation and broadcasting
Line 645: // TODO: Fill with actual metrics
Line 727: // TODO: Implement campaign comparison logic
Line 744: // TODO: Implement bot utilization logic
Line 759: // TODO: Implement campaign progress logic
Line 767: // TODO: Implement campaign summary logic
Line 776: // TODO: Implement coverage comparison logic
Line 784: // TODO: Implement crash distribution logic
Line 792: // TODO: Implement top crash groups logic
Line 798: // TODO: Implement job throughput logic
Line 806: // TODO: Implement real-time metrics logic
Line 815: // TODO: Implement metrics subscription logic
Line 822: // TODO: Implement unsubscribe logic
```

These stub methods return empty/default values, making analytics features non-functional.

## Invariants (MUST NOT CHANGE)

1. Method signatures must remain unchanged (API compatibility)
2. Return types must remain unchanged
3. Existing callers must continue to work
4. Performance should be acceptable (query optimization)
5. No new external dependencies without justification

## Method Implementation Plan

### Group 1: Data Retrieval Methods

#### GetCoverageHistory (Line ~425)

**Current stub:**
```go
func (s *AnalyticsService) GetCoverageHistory(ctx context.Context, jobID string, from, to time.Time) ([]CoverageDataPoint, error) {
    // TODO: Implement actual data retrieval from store
    return []CoverageDataPoint{}, nil
}
```

**Implementation:**
```go
func (s *AnalyticsService) GetCoverageHistory(ctx context.Context, jobID string, from, to time.Time) ([]CoverageDataPoint, error) {
    // Query coverage table for job within time range
    query := `
        SELECT timestamp, edges, blocks, features, exec_count
        FROM coverage
        WHERE job_id = ? AND timestamp >= ? AND timestamp <= ?
        ORDER BY timestamp ASC
    `

    rows, err := s.db.Query(ctx, query, jobID, from, to)
    if err != nil {
        return nil, fmt.Errorf("failed to query coverage history: %w", err)
    }
    defer rows.Close()

    var points []CoverageDataPoint
    for rows.Next() {
        var point CoverageDataPoint
        if err := rows.Scan(&point.Timestamp, &point.Edges, &point.Blocks, &point.Features, &point.ExecCount); err != nil {
            s.logger.WithError(err).Warn("Failed to scan coverage row")
            continue
        }
        points = append(points, point)
    }

    return points, nil
}
```

### Group 2: Calculation Methods

#### GetCrashRateMetrics (Line ~469)

**Implementation:**
```go
func (s *AnalyticsService) GetCrashRateMetrics(ctx context.Context, jobID string, interval time.Duration) (*CrashRateMetrics, error) {
    // Get crash count in recent interval
    query := `
        SELECT COUNT(*) as crash_count,
               COUNT(DISTINCT hash) as unique_crashes
        FROM crashes
        WHERE job_id = ? AND timestamp >= ?
    `

    since := time.Now().Add(-interval)
    var crashCount, uniqueCrashes int64

    row := s.db.QueryRow(ctx, query, jobID, since)
    if err := row.Scan(&crashCount, &uniqueCrashes); err != nil {
        return nil, fmt.Errorf("failed to query crash metrics: %w", err)
    }

    // Get execution count for rate calculation
    execCount, err := s.getExecutionCount(ctx, jobID, since)
    if err != nil {
        return nil, err
    }

    rate := float64(0)
    if execCount > 0 {
        rate = float64(crashCount) / float64(execCount) * 1000000 // Crashes per million executions
    }

    return &CrashRateMetrics{
        TotalCrashes:  crashCount,
        UniqueCrashes: uniqueCrashes,
        CrashRate:     rate,
        Interval:      interval,
        Since:         since,
    }, nil
}
```

#### GetPerformanceMetrics (Line ~498)

**Implementation:**
```go
func (s *AnalyticsService) GetPerformanceMetrics(ctx context.Context, jobID string) (*PerformanceMetrics, error) {
    // Get latest stats for job
    query := `
        SELECT
            AVG(exec_per_second) as avg_exec_speed,
            MAX(exec_per_second) as peak_exec_speed,
            AVG(cpu_usage) as avg_cpu,
            MAX(memory_usage) as peak_memory
        FROM job_stats
        WHERE job_id = ?
        AND timestamp >= datetime('now', '-1 hour')
    `

    var metrics PerformanceMetrics
    row := s.db.QueryRow(ctx, query, jobID)
    if err := row.Scan(&metrics.AverageExecSpeed, &metrics.PeakExecSpeed, &metrics.AverageCPU, &metrics.PeakMemory); err != nil {
        if err == sql.ErrNoRows {
            return &PerformanceMetrics{}, nil
        }
        return nil, fmt.Errorf("failed to query performance metrics: %w", err)
    }

    return &metrics, nil
}
```

### Group 3: Comparison Methods

#### CompareCampaigns (Line ~727)

**Implementation:**
```go
func (s *AnalyticsService) CompareCampaigns(ctx context.Context, campaignIDs []string) (*CampaignComparison, error) {
    if len(campaignIDs) < 2 {
        return nil, fmt.Errorf("at least 2 campaigns required for comparison")
    }

    comparison := &CampaignComparison{
        CampaignIDs: campaignIDs,
        Metrics:     make(map[string]*CampaignMetrics),
    }

    for _, campaignID := range campaignIDs {
        metrics, err := s.getCampaignMetrics(ctx, campaignID)
        if err != nil {
            s.logger.WithError(err).WithField("campaign_id", campaignID).Warn("Failed to get campaign metrics")
            continue
        }
        comparison.Metrics[campaignID] = metrics
    }

    return comparison, nil
}

func (s *AnalyticsService) getCampaignMetrics(ctx context.Context, campaignID string) (*CampaignMetrics, error) {
    query := `
        SELECT
            COUNT(DISTINCT j.id) as job_count,
            SUM(CASE WHEN j.status = 'completed' THEN 1 ELSE 0 END) as completed_jobs,
            COUNT(DISTINCT c.id) as crash_count,
            MAX(cov.edges) as max_coverage
        FROM campaigns camp
        LEFT JOIN jobs j ON j.campaign_id = camp.id
        LEFT JOIN crashes c ON c.job_id = j.id
        LEFT JOIN coverage cov ON cov.job_id = j.id
        WHERE camp.id = ?
        GROUP BY camp.id
    `

    var metrics CampaignMetrics
    metrics.CampaignID = campaignID

    row := s.db.QueryRow(ctx, query, campaignID)
    if err := row.Scan(&metrics.JobCount, &metrics.CompletedJobs, &metrics.CrashCount, &metrics.MaxCoverage); err != nil {
        if err == sql.ErrNoRows {
            return &CampaignMetrics{CampaignID: campaignID}, nil
        }
        return nil, err
    }

    return &metrics, nil
}
```

### Group 4: Utilization and Progress

#### GetBotUtilization (Line ~744)

**Implementation:**
```go
func (s *AnalyticsService) GetBotUtilization(ctx context.Context, botID string, duration time.Duration) (*BotUtilization, error) {
    since := time.Now().Add(-duration)

    // Get time spent in each status
    query := `
        SELECT
            status,
            SUM(julianday(COALESCE(ended_at, datetime('now'))) - julianday(started_at)) * 24 * 60 as minutes
        FROM bot_status_history
        WHERE bot_id = ? AND started_at >= ?
        GROUP BY status
    `

    rows, err := s.db.Query(ctx, query, botID, since)
    if err != nil {
        return nil, fmt.Errorf("failed to query bot utilization: %w", err)
    }
    defer rows.Close()

    util := &BotUtilization{
        BotID:    botID,
        Duration: duration,
        StatusBreakdown: make(map[string]float64),
    }

    totalMinutes := float64(0)
    busyMinutes := float64(0)

    for rows.Next() {
        var status string
        var minutes float64
        if err := rows.Scan(&status, &minutes); err != nil {
            continue
        }
        util.StatusBreakdown[status] = minutes
        totalMinutes += minutes
        if status == "busy" || status == "running" {
            busyMinutes += minutes
        }
    }

    if totalMinutes > 0 {
        util.UtilizationPercent = (busyMinutes / totalMinutes) * 100
    }

    return util, nil
}
```

#### GetCampaignProgress (Line ~759)

**Implementation:**
```go
func (s *AnalyticsService) GetCampaignProgress(ctx context.Context, campaignID string) (*CampaignProgress, error) {
    query := `
        SELECT
            COUNT(*) as total_jobs,
            SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed,
            SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END) as running,
            SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending,
            SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed
        FROM jobs
        WHERE campaign_id = ?
    `

    var progress CampaignProgress
    progress.CampaignID = campaignID

    row := s.db.QueryRow(ctx, query, campaignID)
    if err := row.Scan(&progress.TotalJobs, &progress.Completed, &progress.Running, &progress.Pending, &progress.Failed); err != nil {
        if err == sql.ErrNoRows {
            return &CampaignProgress{CampaignID: campaignID}, nil
        }
        return nil, err
    }

    if progress.TotalJobs > 0 {
        progress.ProgressPercent = float64(progress.Completed) / float64(progress.TotalJobs) * 100
    }

    return &progress, nil
}
```

### Group 5: Real-Time Metrics

#### GetRealTimeMetrics (Line ~806)

**Implementation:**
```go
func (s *AnalyticsService) GetRealTimeMetrics(ctx context.Context) (*RealTimeMetrics, error) {
    metrics := &RealTimeMetrics{
        Timestamp: time.Now(),
    }

    // Get active jobs count
    var activeJobs int64
    row := s.db.QueryRow(ctx, "SELECT COUNT(*) FROM jobs WHERE status IN ('running', 'assigned')")
    row.Scan(&activeJobs)
    metrics.ActiveJobs = activeJobs

    // Get online bots count
    var onlineBots int64
    row = s.db.QueryRow(ctx, "SELECT COUNT(*) FROM bots WHERE is_online = true")
    row.Scan(&onlineBots)
    metrics.OnlineBots = onlineBots

    // Get recent crash count (last hour)
    var recentCrashes int64
    row = s.db.QueryRow(ctx, "SELECT COUNT(*) FROM crashes WHERE timestamp >= datetime('now', '-1 hour')")
    row.Scan(&recentCrashes)
    metrics.RecentCrashes = recentCrashes

    // Get aggregate exec speed
    var totalExecSpeed float64
    row = s.db.QueryRow(ctx, `
        SELECT COALESCE(SUM(exec_per_second), 0)
        FROM job_stats
        WHERE timestamp >= datetime('now', '-5 minutes')
    `)
    row.Scan(&totalExecSpeed)
    metrics.TotalExecSpeed = totalExecSpeed

    return metrics, nil
}
```

### Group 6: Subscription Methods

#### SubscribeToMetrics / UnsubscribeFromMetrics (Lines ~815, ~822)

**Implementation using channels:**
```go
type metricsSubscription struct {
    id     string
    filter MetricsFilter
    ch     chan *MetricsUpdate
}

func (s *AnalyticsService) SubscribeToMetrics(ctx context.Context, filter MetricsFilter) (<-chan *MetricsUpdate, string, error) {
    s.subMu.Lock()
    defer s.subMu.Unlock()

    id := uuid.New().String()
    ch := make(chan *MetricsUpdate, 100)

    sub := &metricsSubscription{
        id:     id,
        filter: filter,
        ch:     ch,
    }

    s.subscriptions[id] = sub

    // Start goroutine to push updates
    go s.pushMetricsUpdates(ctx, sub)

    return ch, id, nil
}

func (s *AnalyticsService) UnsubscribeFromMetrics(subscriptionID string) error {
    s.subMu.Lock()
    defer s.subMu.Unlock()

    sub, exists := s.subscriptions[subscriptionID]
    if !exists {
        return fmt.Errorf("subscription not found: %s", subscriptionID)
    }

    close(sub.ch)
    delete(s.subscriptions, subscriptionID)

    return nil
}

func (s *AnalyticsService) pushMetricsUpdates(ctx context.Context, sub *metricsSubscription) {
    ticker := time.NewTicker(sub.filter.Interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            metrics, err := s.GetRealTimeMetrics(ctx)
            if err != nil {
                s.logger.WithError(err).Warn("Failed to get real-time metrics")
                continue
            }

            select {
            case sub.ch <- &MetricsUpdate{Metrics: metrics, Timestamp: time.Now()}:
            default:
                // Channel full, skip update
            }
        }
    }
}
```

## Database Schema Requirements

Some implementations may require additional tables or indexes:

```sql
-- Job stats table for performance metrics
CREATE TABLE IF NOT EXISTS job_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    exec_per_second REAL,
    cpu_usage REAL,
    memory_usage INTEGER,
    corpus_size INTEGER,
    FOREIGN KEY (job_id) REFERENCES jobs(id)
);

CREATE INDEX idx_job_stats_job_timestamp ON job_stats(job_id, timestamp);

-- Bot status history for utilization tracking
CREATE TABLE IF NOT EXISTS bot_status_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bot_id TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at DATETIME NOT NULL,
    ended_at DATETIME,
    FOREIGN KEY (bot_id) REFERENCES bots(id)
);

CREATE INDEX idx_bot_status_history_bot ON bot_status_history(bot_id, started_at);
```

## Verification Steps

### 1. Unit Tests for Each Method
```go
func TestGetCoverageHistory(t *testing.T) {
    // Setup test data
    db := testutil.NewTestDB(t)
    svc := NewAnalyticsService(db, testLogger)

    // Insert test coverage data
    testutil.InsertCoverage(t, db, "job-1", time.Now().Add(-1*time.Hour), 100, 50)
    testutil.InsertCoverage(t, db, "job-1", time.Now(), 150, 75)

    // Query
    history, err := svc.GetCoverageHistory(context.Background(), "job-1", time.Now().Add(-2*time.Hour), time.Now())
    require.NoError(t, err)
    require.Len(t, history, 2)
    require.Equal(t, 150, history[1].Edges)
}
```

### 2. Integration Tests
```bash
make test-integration
```

### 3. API Tests
```bash
curl http://localhost:8080/api/v1/analytics/coverage/job-1 | jq .
curl http://localhost:8080/api/v1/analytics/performance/job-1 | jq .
```

## Notes for Future Runs

### Query Optimization

For large datasets, consider:
- Adding appropriate indexes
- Using query pagination
- Implementing caching for frequently accessed metrics
- Using materialized views for complex aggregations

### Subscription Cleanup

Ensure subscriptions are cleaned up:
- On context cancellation
- On client disconnect
- Periodically sweep for stale subscriptions

### Metrics Sampling

For high-frequency metrics:
- Consider sampling instead of every data point
- Use time-bucket aggregation (e.g., 1-minute buckets)

## Completion Checklist

- [ ] Implement GetCoverageHistory
- [ ] Implement GetCrashRateMetrics
- [ ] Implement GetPerformanceMetrics
- [ ] Implement CompareCampaigns
- [ ] Implement GetBotUtilization
- [ ] Implement GetCampaignProgress
- [ ] Implement GetCampaignSummary
- [ ] Implement CompareCoverage
- [ ] Implement GetCrashDistribution
- [ ] Implement GetTopCrashGroups
- [ ] Implement GetJobThroughput
- [ ] Implement GetRealTimeMetrics
- [ ] Implement SubscribeToMetrics
- [ ] Implement UnsubscribeFromMetrics
- [ ] Add required database schema
- [ ] Write unit tests for each method
- [ ] Write integration tests
- [ ] Verify API endpoints work
- [ ] Add query indexes for performance
