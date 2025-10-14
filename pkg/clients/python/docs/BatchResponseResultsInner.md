# BatchResponseResultsInner


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**operation_index** | **int** | Index of the operation in the original request | 
**operation** | **str** | Type of operation that was executed | 
**operation_id** | **str** | Client-provided operation ID if specified | [optional] 
**status** | **str** | Status of the operation | 
**result** | **object** | Operation result data (for successful operations) | [optional] 
**error** | [**ProblemDetails**](ProblemDetails.md) |  | [optional] 
**execution_time_seconds** | **float** | Time taken for this operation | [optional] 

## Example

```python
from pandafuzz.models.batch_response_results_inner import BatchResponseResultsInner

# TODO update the JSON string below
json = "{}"
# create an instance of BatchResponseResultsInner from a JSON string
batch_response_results_inner_instance = BatchResponseResultsInner.from_json(json)
# print the JSON string representation of the object
print(BatchResponseResultsInner.to_json())

# convert the object into a dict
batch_response_results_inner_dict = batch_response_results_inner_instance.to_dict()
# create an instance of BatchResponseResultsInner from a dict
batch_response_results_inner_from_dict = BatchResponseResultsInner.from_dict(batch_response_results_inner_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


