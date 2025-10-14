# PerformanceStatsResponseComponentStatsDatabase


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**avg_query_time_ms** | **float** |  | [optional] 
**connection_pool_utilization_percent** | **float** |  | [optional] 
**deadlock_count** | **int** |  | [optional] 
**slow_query_count** | **int** |  | [optional] 

## Example

```python
from pandafuzz.models.performance_stats_response_component_stats_database import PerformanceStatsResponseComponentStatsDatabase

# TODO update the JSON string below
json = "{}"
# create an instance of PerformanceStatsResponseComponentStatsDatabase from a JSON string
performance_stats_response_component_stats_database_instance = PerformanceStatsResponseComponentStatsDatabase.from_json(json)
# print the JSON string representation of the object
print(PerformanceStatsResponseComponentStatsDatabase.to_json())

# convert the object into a dict
performance_stats_response_component_stats_database_dict = performance_stats_response_component_stats_database_instance.to_dict()
# create an instance of PerformanceStatsResponseComponentStatsDatabase from a dict
performance_stats_response_component_stats_database_from_dict = PerformanceStatsResponseComponentStatsDatabase.from_dict(performance_stats_response_component_stats_database_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


