# MetricsResponseMetricsCoverage


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**total_edges** | **int** |  | [optional] 
**edges_per_second** | **float** |  | [optional] 
**growth_rate** | **float** |  | [optional] 

## Example

```python
from pandafuzz.models.metrics_response_metrics_coverage import MetricsResponseMetricsCoverage

# TODO update the JSON string below
json = "{}"
# create an instance of MetricsResponseMetricsCoverage from a JSON string
metrics_response_metrics_coverage_instance = MetricsResponseMetricsCoverage.from_json(json)
# print the JSON string representation of the object
print(MetricsResponseMetricsCoverage.to_json())

# convert the object into a dict
metrics_response_metrics_coverage_dict = metrics_response_metrics_coverage_instance.to_dict()
# create an instance of MetricsResponseMetricsCoverage from a dict
metrics_response_metrics_coverage_from_dict = MetricsResponseMetricsCoverage.from_dict(metrics_response_metrics_coverage_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


