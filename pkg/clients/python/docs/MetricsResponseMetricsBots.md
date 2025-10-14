# MetricsResponseMetricsBots


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**total** | **int** |  | [optional] 
**online** | **int** |  | [optional] 
**busy** | **int** |  | [optional] 
**idle** | **int** |  | [optional] 
**error** | **int** |  | [optional] 

## Example

```python
from pandafuzz.models.metrics_response_metrics_bots import MetricsResponseMetricsBots

# TODO update the JSON string below
json = "{}"
# create an instance of MetricsResponseMetricsBots from a JSON string
metrics_response_metrics_bots_instance = MetricsResponseMetricsBots.from_json(json)
# print the JSON string representation of the object
print(MetricsResponseMetricsBots.to_json())

# convert the object into a dict
metrics_response_metrics_bots_dict = metrics_response_metrics_bots_instance.to_dict()
# create an instance of MetricsResponseMetricsBots from a dict
metrics_response_metrics_bots_from_dict = MetricsResponseMetricsBots.from_dict(metrics_response_metrics_bots_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


