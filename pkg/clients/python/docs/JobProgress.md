# JobProgress

Current job progress

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**percent_complete** | **float** | Percentage completion | [optional] 
**executions** | **int** | Number of executions performed | [optional] 
**crashes_found** | **int** | Number of crashes found | [optional] 
**coverage_edges** | **int** | Number of coverage edges discovered | [optional] 

## Example

```python
from pandafuzz.models.job_progress import JobProgress

# TODO update the JSON string below
json = "{}"
# create an instance of JobProgress from a JSON string
job_progress_instance = JobProgress.from_json(json)
# print the JSON string representation of the object
print(JobProgress.to_json())

# convert the object into a dict
job_progress_dict = job_progress_instance.to_dict()
# create an instance of JobProgress from a dict
job_progress_from_dict = JobProgress.from_dict(job_progress_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


