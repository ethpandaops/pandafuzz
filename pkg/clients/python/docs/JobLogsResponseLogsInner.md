# JobLogsResponseLogsInner


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**timestamp** | **datetime** | When the log entry was created | 
**level** | **str** | Log level | 
**message** | **str** | Log message content | 
**source** | **str** | Source component that generated the log | [optional] 
**metadata** | **Dict[str, object]** | Additional log metadata | [optional] 

## Example

```python
from pandafuzz.models.job_logs_response_logs_inner import JobLogsResponseLogsInner

# TODO update the JSON string below
json = "{}"
# create an instance of JobLogsResponseLogsInner from a JSON string
job_logs_response_logs_inner_instance = JobLogsResponseLogsInner.from_json(json)
# print the JSON string representation of the object
print(JobLogsResponseLogsInner.to_json())

# convert the object into a dict
job_logs_response_logs_inner_dict = job_logs_response_logs_inner_instance.to_dict()
# create an instance of JobLogsResponseLogsInner from a dict
job_logs_response_logs_inner_from_dict = JobLogsResponseLogsInner.from_dict(job_logs_response_logs_inner_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


