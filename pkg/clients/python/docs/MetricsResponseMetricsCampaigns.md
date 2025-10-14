# MetricsResponseMetricsCampaigns


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**total** | **int** |  | [optional] 
**active** | **int** |  | [optional] 
**paused** | **int** |  | [optional] 
**completed** | **int** |  | [optional] 

## Example

```python
from pandafuzz.models.metrics_response_metrics_campaigns import MetricsResponseMetricsCampaigns

# TODO update the JSON string below
json = "{}"
# create an instance of MetricsResponseMetricsCampaigns from a JSON string
metrics_response_metrics_campaigns_instance = MetricsResponseMetricsCampaigns.from_json(json)
# print the JSON string representation of the object
print(MetricsResponseMetricsCampaigns.to_json())

# convert the object into a dict
metrics_response_metrics_campaigns_dict = metrics_response_metrics_campaigns_instance.to_dict()
# create an instance of MetricsResponseMetricsCampaigns from a dict
metrics_response_metrics_campaigns_from_dict = MetricsResponseMetricsCampaigns.from_dict(metrics_response_metrics_campaigns_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


