# JobListResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**data** | [**List[Job]**](Job.md) |  | 
**pagination** | [**Pagination**](Pagination.md) |  | 

## Example

```python
from pandafuzz.models.job_list_response import JobListResponse

# TODO update the JSON string below
json = "{}"
# create an instance of JobListResponse from a JSON string
job_list_response_instance = JobListResponse.from_json(json)
# print the JSON string representation of the object
print(JobListResponse.to_json())

# convert the object into a dict
job_list_response_dict = job_list_response_instance.to_dict()
# create an instance of JobListResponse from a dict
job_list_response_from_dict = JobListResponse.from_dict(job_list_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


