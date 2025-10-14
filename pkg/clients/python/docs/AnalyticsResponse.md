# AnalyticsResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**time_range** | [**AnalyticsResponseTimeRange**](AnalyticsResponseTimeRange.md) |  | 
**generated_at** | **datetime** | When the analytics were generated | 
**system_overview** | [**AnalyticsResponseSystemOverview**](AnalyticsResponseSystemOverview.md) |  | 
**performance_metrics** | [**AnalyticsResponsePerformanceMetrics**](AnalyticsResponsePerformanceMetrics.md) |  | [optional] 
**resource_usage** | [**AnalyticsResponseResourceUsage**](AnalyticsResponseResourceUsage.md) |  | [optional] 
**trends** | [**AnalyticsResponseTrends**](AnalyticsResponseTrends.md) |  | [optional] 

## Example

```python
from pandafuzz.models.analytics_response import AnalyticsResponse

# TODO update the JSON string below
json = "{}"
# create an instance of AnalyticsResponse from a JSON string
analytics_response_instance = AnalyticsResponse.from_json(json)
# print the JSON string representation of the object
print(AnalyticsResponse.to_json())

# convert the object into a dict
analytics_response_dict = analytics_response_instance.to_dict()
# create an instance of AnalyticsResponse from a dict
analytics_response_from_dict = AnalyticsResponse.from_dict(analytics_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


