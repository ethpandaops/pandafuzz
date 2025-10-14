# MinimizeCrashRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**strategy** | **str** |  | [optional] [default to 'delta_debugging']
**timeout_seconds** | **int** |  | [optional] [default to 600]
**priority** | **int** |  | [optional] [default to 5]

## Example

```python
from pandafuzz.models.minimize_crash_request import MinimizeCrashRequest

# TODO update the JSON string below
json = "{}"
# create an instance of MinimizeCrashRequest from a JSON string
minimize_crash_request_instance = MinimizeCrashRequest.from_json(json)
# print the JSON string representation of the object
print(MinimizeCrashRequest.to_json())

# convert the object into a dict
minimize_crash_request_dict = minimize_crash_request_instance.to_dict()
# create an instance of MinimizeCrashRequest from a dict
minimize_crash_request_from_dict = MinimizeCrashRequest.from_dict(minimize_crash_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


