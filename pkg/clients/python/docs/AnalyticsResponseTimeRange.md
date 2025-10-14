# AnalyticsResponseTimeRange

Time range for the analytics data

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**start** | **datetime** |  | [optional] 
**end** | **datetime** |  | [optional] 
**duration** | **str** |  | [optional] 

## Example

```python
from pandafuzz.models.analytics_response_time_range import AnalyticsResponseTimeRange

# TODO update the JSON string below
json = "{}"
# create an instance of AnalyticsResponseTimeRange from a JSON string
analytics_response_time_range_instance = AnalyticsResponseTimeRange.from_json(json)
# print the JSON string representation of the object
print(AnalyticsResponseTimeRange.to_json())

# convert the object into a dict
analytics_response_time_range_dict = analytics_response_time_range_instance.to_dict()
# create an instance of AnalyticsResponseTimeRange from a dict
analytics_response_time_range_from_dict = AnalyticsResponseTimeRange.from_dict(analytics_response_time_range_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


