# BatchRequestOperationsInner


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**operation** | **str** | Type of operation to perform | 
**data** | **object** | Operation-specific data payload | 
**operation_id** | **str** | Optional client-provided operation ID for tracking | [optional] 

## Example

```python
from pandafuzz.models.batch_request_operations_inner import BatchRequestOperationsInner

# TODO update the JSON string below
json = "{}"
# create an instance of BatchRequestOperationsInner from a JSON string
batch_request_operations_inner_instance = BatchRequestOperationsInner.from_json(json)
# print the JSON string representation of the object
print(BatchRequestOperationsInner.to_json())

# convert the object into a dict
batch_request_operations_inner_dict = batch_request_operations_inner_instance.to_dict()
# create an instance of BatchRequestOperationsInner from a dict
batch_request_operations_inner_from_dict = BatchRequestOperationsInner.from_dict(batch_request_operations_inner_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


