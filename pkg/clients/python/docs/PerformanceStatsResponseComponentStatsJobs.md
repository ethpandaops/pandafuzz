# PerformanceStatsResponseComponentStatsJobs


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**avg_queue_time_seconds** | **float** |  | [optional] 
**avg_execution_time_seconds** | **float** |  | [optional] 
**success_rate_percent** | **float** |  | [optional] 
**timeout_rate_percent** | **float** |  | [optional] 

## Example

```python
from pandafuzz.models.performance_stats_response_component_stats_jobs import PerformanceStatsResponseComponentStatsJobs

# TODO update the JSON string below
json = "{}"
# create an instance of PerformanceStatsResponseComponentStatsJobs from a JSON string
performance_stats_response_component_stats_jobs_instance = PerformanceStatsResponseComponentStatsJobs.from_json(json)
# print the JSON string representation of the object
print(PerformanceStatsResponseComponentStatsJobs.to_json())

# convert the object into a dict
performance_stats_response_component_stats_jobs_dict = performance_stats_response_component_stats_jobs_instance.to_dict()
# create an instance of PerformanceStatsResponseComponentStatsJobs from a dict
performance_stats_response_component_stats_jobs_from_dict = PerformanceStatsResponseComponentStatsJobs.from_dict(performance_stats_response_component_stats_jobs_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


