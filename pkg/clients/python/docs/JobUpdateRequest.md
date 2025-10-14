# JobUpdateRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**name** | **str** | Human-readable name for the job | [optional] 
**timeout_seconds** | **int** | Job timeout in seconds | [optional] 
**priority** | **int** | Job priority (1&#x3D;lowest, 10&#x3D;highest) | [optional] 
**config** | **Dict[str, object]** | Fuzzer-specific configuration | [optional] 
**metadata** | **Dict[str, object]** | Key-value metadata for extensibility | [optional] 

## Example

```python
from pandafuzz.models.job_update_request import JobUpdateRequest

# TODO update the JSON string below
json = "{}"
# create an instance of JobUpdateRequest from a JSON string
job_update_request_instance = JobUpdateRequest.from_json(json)
# print the JSON string representation of the object
print(JobUpdateRequest.to_json())

# convert the object into a dict
job_update_request_dict = job_update_request_instance.to_dict()
# create an instance of JobUpdateRequest from a dict
job_update_request_from_dict = JobUpdateRequest.from_dict(job_update_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


