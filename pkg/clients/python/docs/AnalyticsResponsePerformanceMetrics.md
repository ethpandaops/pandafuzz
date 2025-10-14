# AnalyticsResponsePerformanceMetrics

Performance and efficiency metrics

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**avg_job_completion_time_seconds** | **float** |  | [optional] 
**avg_executions_per_second** | **float** |  | [optional] 
**avg_coverage_per_hour** | **float** |  | [optional] 
**crash_discovery_rate_per_hour** | **float** |  | [optional] 
**system_efficiency_score** | **float** |  | [optional] 

## Example

```python
from pandafuzz.models.analytics_response_performance_metrics import AnalyticsResponsePerformanceMetrics

# TODO update the JSON string below
json = "{}"
# create an instance of AnalyticsResponsePerformanceMetrics from a JSON string
analytics_response_performance_metrics_instance = AnalyticsResponsePerformanceMetrics.from_json(json)
# print the JSON string representation of the object
print(AnalyticsResponsePerformanceMetrics.to_json())

# convert the object into a dict
analytics_response_performance_metrics_dict = analytics_response_performance_metrics_instance.to_dict()
# create an instance of AnalyticsResponsePerformanceMetrics from a dict
analytics_response_performance_metrics_from_dict = AnalyticsResponsePerformanceMetrics.from_dict(analytics_response_performance_metrics_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


