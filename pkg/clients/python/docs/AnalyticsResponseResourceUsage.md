# AnalyticsResponseResourceUsage

Resource utilization statistics

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**cpu_utilization_percent** | **float** |  | [optional] 
**memory_usage_bytes** | **int** |  | [optional] 
**storage_usage_bytes** | **int** |  | [optional] 
**network_throughput_bps** | **int** |  | [optional] 

## Example

```python
from pandafuzz.models.analytics_response_resource_usage import AnalyticsResponseResourceUsage

# TODO update the JSON string below
json = "{}"
# create an instance of AnalyticsResponseResourceUsage from a JSON string
analytics_response_resource_usage_instance = AnalyticsResponseResourceUsage.from_json(json)
# print the JSON string representation of the object
print(AnalyticsResponseResourceUsage.to_json())

# convert the object into a dict
analytics_response_resource_usage_dict = analytics_response_resource_usage_instance.to_dict()
# create an instance of AnalyticsResponseResourceUsage from a dict
analytics_response_resource_usage_from_dict = AnalyticsResponseResourceUsage.from_dict(analytics_response_resource_usage_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


