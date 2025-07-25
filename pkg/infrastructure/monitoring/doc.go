// Package monitoring provides infrastructure components for system monitoring.
//
// This package includes:
//   - Prometheus metrics collection and registration
//   - Health checking infrastructure for services
//   - Support for readiness and liveness probes
//   - System metrics collection (CPU, memory, goroutines)
//
// The monitoring infrastructure is designed to be pluggable and extensible,
// allowing services to register custom metrics and health checks as needed.
//
// # Metrics
//
// The metrics sub-package provides a clean interface for collecting and
// exporting metrics to Prometheus:
//
//	collector := metrics.NewCollector(logger)
//	collector.RecordMetric("request_count", 1, map[string]string{"method": "GET"})
//	collector.RegisterCollector(customCollector)
//
// # Health Checks
//
// The health sub-package provides infrastructure for implementing health checks:
//
//	checker := health.NewChecker()
//	checker.Register("database", databaseHealthCheck)
//	status := checker.CheckHealth(ctx)
//
// Health checks support both liveness and readiness probes, making them
// suitable for Kubernetes deployments.
package monitoring
