# PerformanceStatsResponseComponentStatsBots


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**avg_utilization_percent** | **float** |  | [optional] 
**avg_job_completion_time_seconds** | **float** |  | [optional] 
**failure_rate_percent** | **float** |  | [optional] 
**throughput_jobs_per_hour** | **float** |  | [optional] 

## Example

```python
from pandafuzz.models.performance_stats_response_component_stats_bots import PerformanceStatsResponseComponentStatsBots

# TODO update the JSON string below
json = "{}"
# create an instance of PerformanceStatsResponseComponentStatsBots from a JSON string
performance_stats_response_component_stats_bots_instance = PerformanceStatsResponseComponentStatsBots.from_json(json)
# print the JSON string representation of the object
print(PerformanceStatsResponseComponentStatsBots.to_json())

# convert the object into a dict
performance_stats_response_component_stats_bots_dict = performance_stats_response_component_stats_bots_instance.to_dict()
# create an instance of PerformanceStatsResponseComponentStatsBots from a dict
performance_stats_response_component_stats_bots_from_dict = PerformanceStatsResponseComponentStatsBots.from_dict(performance_stats_response_component_stats_bots_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


