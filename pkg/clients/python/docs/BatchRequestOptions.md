# BatchRequestOptions

Batch execution options

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**atomic** | **bool** | Whether all operations must succeed or all fail | [optional] [default to False]
**fail_fast** | **bool** | Whether to stop on first failure | [optional] [default to True]
**timeout_seconds** | **int** | Total timeout for batch execution | [optional] [default to 300]

## Example

```python
from pandafuzz.models.batch_request_options import BatchRequestOptions

# TODO update the JSON string below
json = "{}"
# create an instance of BatchRequestOptions from a JSON string
batch_request_options_instance = BatchRequestOptions.from_json(json)
# print the JSON string representation of the object
print(BatchRequestOptions.to_json())

# convert the object into a dict
batch_request_options_dict = batch_request_options_instance.to_dict()
# create an instance of BatchRequestOptions from a dict
batch_request_options_from_dict = BatchRequestOptions.from_dict(batch_request_options_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


