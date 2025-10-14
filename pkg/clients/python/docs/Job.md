# Job


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **str** | Unique identifier for the job | 
**name** | **str** | Human-readable name for the job | 
**status** | [**JobStatus**](JobStatus.md) |  | 
**fuzzer** | [**FuzzerType**](FuzzerType.md) |  | 
**target_binary** | **str** | Path to the target binary to fuzz | 
**campaign_id** | **str** | ID of the campaign this job belongs to | [optional] 
**assigned_bot_id** | **str** | ID of the bot assigned to this job | [optional] 
**created_at** | **datetime** | When the job was created | 
**started_at** | **datetime** | When the job started execution | [optional] 
**completed_at** | **datetime** | When the job completed | [optional] 
**timeout_at** | **datetime** | When the job will timeout | 
**timeout_seconds** | **int** | Job timeout in seconds | [optional] 
**config** | **Dict[str, object]** | Fuzzer-specific configuration | [optional] 
**enable_coverage** | **bool** | Whether to collect coverage information | [optional] 
**coverage_format** | [**CoverageFormat**](CoverageFormat.md) |  | [optional] 
**priority** | **int** | Job priority (1&#x3D;lowest, 10&#x3D;highest) | [optional] 
**progress** | [**JobProgress**](JobProgress.md) |  | [optional] 
**metadata** | **Dict[str, object]** | Key-value metadata for extensibility | [optional] 

## Example

```python
from pandafuzz.models.job import Job

# TODO update the JSON string below
json = "{}"
# create an instance of Job from a JSON string
job_instance = Job.from_json(json)
# print the JSON string representation of the object
print(Job.to_json())

# convert the object into a dict
job_dict = job_instance.to_dict()
# create an instance of Job from a dict
job_from_dict = Job.from_dict(job_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


