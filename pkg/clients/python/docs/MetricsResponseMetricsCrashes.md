# MetricsResponseMetricsCrashes


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**total** | **int** |  | [optional] 
**unique** | **int** |  | [optional] 
**today** | **int** |  | [optional] 
**critical** | **int** |  | [optional] 

## Example

```python
from pandafuzz.models.metrics_response_metrics_crashes import MetricsResponseMetricsCrashes

# TODO update the JSON string below
json = "{}"
# create an instance of MetricsResponseMetricsCrashes from a JSON string
metrics_response_metrics_crashes_instance = MetricsResponseMetricsCrashes.from_json(json)
# print the JSON string representation of the object
print(MetricsResponseMetricsCrashes.to_json())

# convert the object into a dict
metrics_response_metrics_crashes_dict = metrics_response_metrics_crashes_instance.to_dict()
# create an instance of MetricsResponseMetricsCrashes from a dict
metrics_response_metrics_crashes_from_dict = MetricsResponseMetricsCrashes.from_dict(metrics_response_metrics_crashes_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


