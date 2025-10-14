# MetricsResponseMetricsSystem


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**uptime_seconds** | **int** |  | [optional] 
**cpu_usage_percent** | **float** |  | [optional] 
**memory_usage_bytes** | **int** |  | [optional] 
**disk_usage_bytes** | **int** |  | [optional] 
**request_rate_per_second** | **float** |  | [optional] 
**error_rate_per_second** | **float** |  | [optional] 

## Example

```python
from pandafuzz.models.metrics_response_metrics_system import MetricsResponseMetricsSystem

# TODO update the JSON string below
json = "{}"
# create an instance of MetricsResponseMetricsSystem from a JSON string
metrics_response_metrics_system_instance = MetricsResponseMetricsSystem.from_json(json)
# print the JSON string representation of the object
print(MetricsResponseMetricsSystem.to_json())

# convert the object into a dict
metrics_response_metrics_system_dict = metrics_response_metrics_system_instance.to_dict()
# create an instance of MetricsResponseMetricsSystem from a dict
metrics_response_metrics_system_from_dict = MetricsResponseMetricsSystem.from_dict(metrics_response_metrics_system_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


