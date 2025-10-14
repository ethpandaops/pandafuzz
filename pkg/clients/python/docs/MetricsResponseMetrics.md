# MetricsResponseMetrics

Current system metrics

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**bots** | [**MetricsResponseMetricsBots**](MetricsResponseMetricsBots.md) |  | [optional] 
**jobs** | [**MetricsResponseMetricsJobs**](MetricsResponseMetricsJobs.md) |  | [optional] 
**campaigns** | [**MetricsResponseMetricsCampaigns**](MetricsResponseMetricsCampaigns.md) |  | [optional] 
**crashes** | [**MetricsResponseMetricsCrashes**](MetricsResponseMetricsCrashes.md) |  | [optional] 
**coverage** | [**MetricsResponseMetricsCoverage**](MetricsResponseMetricsCoverage.md) |  | [optional] 
**system** | [**MetricsResponseMetricsSystem**](MetricsResponseMetricsSystem.md) |  | [optional] 

## Example

```python
from pandafuzz.models.metrics_response_metrics import MetricsResponseMetrics

# TODO update the JSON string below
json = "{}"
# create an instance of MetricsResponseMetrics from a JSON string
metrics_response_metrics_instance = MetricsResponseMetrics.from_json(json)
# print the JSON string representation of the object
print(MetricsResponseMetrics.to_json())

# convert the object into a dict
metrics_response_metrics_dict = metrics_response_metrics_instance.to_dict()
# create an instance of MetricsResponseMetrics from a dict
metrics_response_metrics_from_dict = MetricsResponseMetrics.from_dict(metrics_response_metrics_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


