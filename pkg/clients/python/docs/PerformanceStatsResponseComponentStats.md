# PerformanceStatsResponseComponentStats


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**bots** | [**PerformanceStatsResponseComponentStatsBots**](PerformanceStatsResponseComponentStatsBots.md) |  | [optional] 
**jobs** | [**PerformanceStatsResponseComponentStatsJobs**](PerformanceStatsResponseComponentStatsJobs.md) |  | [optional] 
**storage** | [**PerformanceStatsResponseComponentStatsStorage**](PerformanceStatsResponseComponentStatsStorage.md) |  | [optional] 
**database** | [**PerformanceStatsResponseComponentStatsDatabase**](PerformanceStatsResponseComponentStatsDatabase.md) |  | [optional] 

## Example

```python
from pandafuzz.models.performance_stats_response_component_stats import PerformanceStatsResponseComponentStats

# TODO update the JSON string below
json = "{}"
# create an instance of PerformanceStatsResponseComponentStats from a JSON string
performance_stats_response_component_stats_instance = PerformanceStatsResponseComponentStats.from_json(json)
# print the JSON string representation of the object
print(PerformanceStatsResponseComponentStats.to_json())

# convert the object into a dict
performance_stats_response_component_stats_dict = performance_stats_response_component_stats_instance.to_dict()
# create an instance of PerformanceStatsResponseComponentStats from a dict
performance_stats_response_component_stats_from_dict = PerformanceStatsResponseComponentStats.from_dict(performance_stats_response_component_stats_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


