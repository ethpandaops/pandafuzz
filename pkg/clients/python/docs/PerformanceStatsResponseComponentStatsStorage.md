# PerformanceStatsResponseComponentStatsStorage


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**avg_read_latency_ms** | **float** |  | [optional] 
**avg_write_latency_ms** | **float** |  | [optional] 
**throughput_mbps** | **float** |  | [optional] 
**error_rate_percent** | **float** |  | [optional] 

## Example

```python
from pandafuzz.models.performance_stats_response_component_stats_storage import PerformanceStatsResponseComponentStatsStorage

# TODO update the JSON string below
json = "{}"
# create an instance of PerformanceStatsResponseComponentStatsStorage from a JSON string
performance_stats_response_component_stats_storage_instance = PerformanceStatsResponseComponentStatsStorage.from_json(json)
# print the JSON string representation of the object
print(PerformanceStatsResponseComponentStatsStorage.to_json())

# convert the object into a dict
performance_stats_response_component_stats_storage_dict = performance_stats_response_component_stats_storage_instance.to_dict()
# create an instance of PerformanceStatsResponseComponentStatsStorage from a dict
performance_stats_response_component_stats_storage_from_dict = PerformanceStatsResponseComponentStatsStorage.from_dict(performance_stats_response_component_stats_storage_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


