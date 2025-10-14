# BatchResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**batch_id** | **str** | Unique identifier for this batch execution | 
**total_operations** | **int** | Total number of operations in the batch | 
**successful_operations** | **int** | Number of operations that succeeded | 
**failed_operations** | **int** | Number of operations that failed | 
**execution_time_seconds** | **float** | Total time taken to execute the batch | 
**results** | [**List[BatchResponseResultsInner]**](BatchResponseResultsInner.md) | Results for each operation in the batch | 
**partial_success** | **bool** | Whether the batch had partial success (some ops succeeded, some failed) | [optional] 

## Example

```python
from pandafuzz.models.batch_response import BatchResponse

# TODO update the JSON string below
json = "{}"
# create an instance of BatchResponse from a JSON string
batch_response_instance = BatchResponse.from_json(json)
# print the JSON string representation of the object
print(BatchResponse.to_json())

# convert the object into a dict
batch_response_dict = batch_response_instance.to_dict()
# create an instance of BatchResponse from a dict
batch_response_from_dict = BatchResponse.from_dict(batch_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


