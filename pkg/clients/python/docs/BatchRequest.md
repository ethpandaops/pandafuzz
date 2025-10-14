# BatchRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**operations** | [**List[BatchRequestOperationsInner]**](BatchRequestOperationsInner.md) | List of operations to execute | 
**options** | [**BatchRequestOptions**](BatchRequestOptions.md) |  | [optional] 

## Example

```python
from pandafuzz.models.batch_request import BatchRequest

# TODO update the JSON string below
json = "{}"
# create an instance of BatchRequest from a JSON string
batch_request_instance = BatchRequest.from_json(json)
# print the JSON string representation of the object
print(BatchRequest.to_json())

# convert the object into a dict
batch_request_dict = batch_request_instance.to_dict()
# create an instance of BatchRequest from a dict
batch_request_from_dict = BatchRequest.from_dict(batch_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


