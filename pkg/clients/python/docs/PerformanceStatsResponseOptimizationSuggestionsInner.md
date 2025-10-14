# PerformanceStatsResponseOptimizationSuggestionsInner


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**category** | **str** |  | [optional] 
**suggestion** | **str** |  | [optional] 
**estimated_improvement_percent** | **float** |  | [optional] 
**effort_level** | **str** |  | [optional] 

## Example

```python
from pandafuzz.models.performance_stats_response_optimization_suggestions_inner import PerformanceStatsResponseOptimizationSuggestionsInner

# TODO update the JSON string below
json = "{}"
# create an instance of PerformanceStatsResponseOptimizationSuggestionsInner from a JSON string
performance_stats_response_optimization_suggestions_inner_instance = PerformanceStatsResponseOptimizationSuggestionsInner.from_json(json)
# print the JSON string representation of the object
print(PerformanceStatsResponseOptimizationSuggestionsInner.to_json())

# convert the object into a dict
performance_stats_response_optimization_suggestions_inner_dict = performance_stats_response_optimization_suggestions_inner_instance.to_dict()
# create an instance of PerformanceStatsResponseOptimizationSuggestionsInner from a dict
performance_stats_response_optimization_suggestions_inner_from_dict = PerformanceStatsResponseOptimizationSuggestionsInner.from_dict(performance_stats_response_optimization_suggestions_inner_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


