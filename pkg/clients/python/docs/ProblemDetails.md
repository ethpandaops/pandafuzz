# ProblemDetails

RFC 7807 Problem Details for HTTP APIs

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**type** | **str** | A URI reference that identifies the problem type | 
**title** | **str** | A short, human-readable summary of the problem type | 
**status** | **int** | The HTTP status code | 
**detail** | **str** | A human-readable explanation specific to this occurrence | [optional] 
**instance** | **str** | A URI reference that identifies the specific occurrence | [optional] 
**timestamp** | **datetime** | When the error occurred | [optional] 
**trace_id** | **str** | Request trace identifier for debugging | [optional] 

## Example

```python
from pandafuzz.models.problem_details import ProblemDetails

# TODO update the JSON string below
json = "{}"
# create an instance of ProblemDetails from a JSON string
problem_details_instance = ProblemDetails.from_json(json)
# print the JSON string representation of the object
print(ProblemDetails.to_json())

# convert the object into a dict
problem_details_dict = problem_details_instance.to_dict()
# create an instance of ProblemDetails from a dict
problem_details_from_dict = ProblemDetails.from_dict(problem_details_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


