# PerformanceStatsResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**time_range** | [**CoverageTrendsResponseTimeRange**](CoverageTrendsResponseTimeRange.md) |  | 
**component_stats** | [**PerformanceStatsResponseComponentStats**](PerformanceStatsResponseComponentStats.md) |  | 
**bottlenecks** | [**List[PerformanceStatsResponseBottlenecksInner]**](PerformanceStatsResponseBottlenecksInner.md) |  | [optional] 
**optimization_suggestions** | [**List[PerformanceStatsResponseOptimizationSuggestionsInner]**](PerformanceStatsResponseOptimizationSuggestionsInner.md) |  | [optional] 

## Example

```python
from pandafuzz.models.performance_stats_response import PerformanceStatsResponse

# TODO update the JSON string below
json = "{}"
# create an instance of PerformanceStatsResponse from a JSON string
performance_stats_response_instance = PerformanceStatsResponse.from_json(json)
# print the JSON string representation of the object
print(PerformanceStatsResponse.to_json())

# convert the object into a dict
performance_stats_response_dict = performance_stats_response_instance.to_dict()
# create an instance of PerformanceStatsResponse from a dict
performance_stats_response_from_dict = PerformanceStatsResponse.from_dict(performance_stats_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


