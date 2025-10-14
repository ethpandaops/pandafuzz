package test

import (
	"context"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/infrastructure/monitoring/health"
	"github.com/sirupsen/logrus"
)

// TestVersionInfoStructure tests that the version info structure works correctly
func TestVersionInfoStructure(t *testing.T) {
	version := &common.VersionInfo{
		Version:   "1.0.0",
		BuildTime: "2024-01-15T10:00:00Z",
		GitCommit: "abc123",
	}

	if version.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", version.Version)
	}

	if version.BuildTime != "2024-01-15T10:00:00Z" {
		t.Errorf("Expected build time 2024-01-15T10:00:00Z, got %s", version.BuildTime)
	}

	if version.GitCommit != "abc123" {
		t.Errorf("Expected git commit abc123, got %s", version.GitCommit)
	}
}

// TestHealthChecker tests the data consistency health checker
func TestHealthChecker(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create health checker without services (using placeholder interface)
	checker := health.NewDataConsistencyChecker(nil, logger)
	if checker == nil {
		t.Fatal("Failed to create health checker")
	}

	// Test basic data consistency check
	ctx := context.Background()
	err := checker.CheckDataConsistency(ctx)
	// Should error since services are not available in this test
	if err == nil {
		t.Error("CheckDataConsistency should fail when services are not available")
	}
}

// TestSystemHealthSummary tests the system health summary functionality
func TestSystemHealthSummary(t *testing.T) {
	logger := logrus.New()
	checker := health.NewDataConsistencyChecker(nil, logger)

	ctx := context.Background()
	summary, err := checker.GetSystemHealthSummary(ctx)
	if err != nil {
		t.Fatalf("GetSystemHealthSummary failed: %v", err)
	}

	if summary == nil {
		t.Fatal("Summary should not be nil")
	}

	if summary.OverallStatus != "unhealthy" {
		t.Errorf("Expected overall status to be unhealthy (services unavailable), got %s", summary.OverallStatus)
	}

	if summary.Checks == nil {
		t.Error("Checks should not be nil")
	}

	expectedChecks := []string{
		"phantom_jobs",
		"stuck_bots",
		"orphaned_crashes",
		"coverage_system",
		"timeout_consistency",
		"database_integrity",
	}

	for _, checkName := range expectedChecks {
		if _, exists := summary.Checks[checkName]; !exists {
			t.Errorf("Expected check %s to be present in results", checkName)
		}
	}

	// Verify timestamp is recent
	if time.Since(summary.Timestamp) > time.Minute {
		t.Error("Summary timestamp is too old")
	}
}

// TestHealthCheckResult tests individual health check result structure
func TestHealthCheckResult(t *testing.T) {
	result := &health.HealthCheckResult{
		Name:      "test_check",
		Status:    "healthy",
		Duration:  100 * time.Millisecond,
		Timestamp: time.Now(),
		Metadata:  map[string]interface{}{"test": "value"},
	}

	if result.Name != "test_check" {
		t.Errorf("Expected name test_check, got %s", result.Name)
	}

	if result.Status != "healthy" {
		t.Errorf("Expected status healthy, got %s", result.Status)
	}

	if result.Duration != 100*time.Millisecond {
		t.Errorf("Expected duration 100ms, got %v", result.Duration)
	}

	if result.Metadata["test"] != "value" {
		t.Errorf("Expected metadata test=value, got %v", result.Metadata["test"])
	}
}
