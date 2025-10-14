# JobCreateRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**name** | **str** | Human-readable name for the job | 
**fuzzer** | [**FuzzerType**](FuzzerType.md) |  | 
**target_binary** | **str** | Path to the target binary to fuzz | 
**campaign_id** | **str** | ID of the campaign this job belongs to | [optional] 
**timeout_seconds** | **int** | Job timeout in seconds | [optional] 
**config** | **Dict[str, object]** | Fuzzer-specific configuration | [optional] 
**enable_coverage** | **bool** | Whether to collect coverage information | [optional] [default to False]
**coverage_format** | [**CoverageFormat**](CoverageFormat.md) |  | [optional] 
**priority** | **int** | Job priority (1&#x3D;lowest, 10&#x3D;highest) | [optional] [default to 5]
**seed_corpus** | **List[str]** | Initial seed corpus file paths | [optional] 
**metadata** | **Dict[str, object]** | Key-value metadata for extensibility | [optional] 

## Example

```python
from pandafuzz.models.job_create_request import JobCreateRequest

# TODO update the JSON string below
json = "{}"
# create an instance of JobCreateRequest from a JSON string
job_create_request_instance = JobCreateRequest.from_json(json)
# print the JSON string representation of the object
print(JobCreateRequest.to_json())

# convert the object into a dict
job_create_request_dict = job_create_request_instance.to_dict()
# create an instance of JobCreateRequest from a dict
job_create_request_from_dict = JobCreateRequest.from_dict(job_create_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


