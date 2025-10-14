# PerformanceStatsResponseBottlenecksInner


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**component** | **str** |  | [optional] 
**issue** | **str** |  | [optional] 
**severity** | **str** |  | [optional] 
**impact** | **str** |  | [optional] 
**recommendation** | **str** |  | [optional] 

## Example

```python
from pandafuzz.models.performance_stats_response_bottlenecks_inner import PerformanceStatsResponseBottlenecksInner

# TODO update the JSON string below
json = "{}"
# create an instance of PerformanceStatsResponseBottlenecksInner from a JSON string
performance_stats_response_bottlenecks_inner_instance = PerformanceStatsResponseBottlenecksInner.from_json(json)
# print the JSON string representation of the object
print(PerformanceStatsResponseBottlenecksInner.to_json())

# convert the object into a dict
performance_stats_response_bottlenecks_inner_dict = performance_stats_response_bottlenecks_inner_instance.to_dict()
# create an instance of PerformanceStatsResponseBottlenecksInner from a dict
performance_stats_response_bottlenecks_inner_from_dict = PerformanceStatsResponseBottlenecksInner.from_dict(performance_stats_response_bottlenecks_inner_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


