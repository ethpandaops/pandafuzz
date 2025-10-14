# MetricsResponseMetricsJobs


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**total** | **int** |  | [optional] 
**pending** | **int** |  | [optional] 
**running** | **int** |  | [optional] 
**completed** | **int** |  | [optional] 
**failed** | **int** |  | [optional] 

## Example

```python
from pandafuzz.models.metrics_response_metrics_jobs import MetricsResponseMetricsJobs

# TODO update the JSON string below
json = "{}"
# create an instance of MetricsResponseMetricsJobs from a JSON string
metrics_response_metrics_jobs_instance = MetricsResponseMetricsJobs.from_json(json)
# print the JSON string representation of the object
print(MetricsResponseMetricsJobs.to_json())

# convert the object into a dict
metrics_response_metrics_jobs_dict = metrics_response_metrics_jobs_instance.to_dict()
# create an instance of MetricsResponseMetricsJobs from a dict
metrics_response_metrics_jobs_from_dict = MetricsResponseMetricsJobs.from_dict(metrics_response_metrics_jobs_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


