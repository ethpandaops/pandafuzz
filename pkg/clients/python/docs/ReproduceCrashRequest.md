# ReproduceCrashRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**attempts** | **int** |  | [optional] [default to 3]
**timeout_seconds** | **int** |  | [optional] [default to 300]
**environment** | **Dict[str, str]** | Environment variables for reproduction | [optional] 

## Example

```python
from pandafuzz.models.reproduce_crash_request import ReproduceCrashRequest

# TODO update the JSON string below
json = "{}"
# create an instance of ReproduceCrashRequest from a JSON string
reproduce_crash_request_instance = ReproduceCrashRequest.from_json(json)
# print the JSON string representation of the object
print(ReproduceCrashRequest.to_json())

# convert the object into a dict
reproduce_crash_request_dict = reproduce_crash_request_instance.to_dict()
# create an instance of ReproduceCrashRequest from a dict
reproduce_crash_request_from_dict = ReproduceCrashRequest.from_dict(reproduce_crash_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


