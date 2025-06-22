package common

import "fmt"

// GetMetricsAddr returns the metrics server address from MonitoringConfig
func (c *MonitoringConfig) GetMetricsAddr() string {
	if !c.Enabled || !c.MetricsEnabled || c.MetricsPort == 0 {
		return ""
	}
	return fmt.Sprintf(":%d", c.MetricsPort)
}

// Extension to access MetricsAddr for compatibility
type MonitoringConfigExt struct {
	*MonitoringConfig
}

// MetricsAddr returns the metrics server address
func (c *MonitoringConfigExt) MetricsAddr() string {
	if c.MonitoringConfig == nil {
		return ""
	}
	return c.GetMetricsAddr()
}