# CrashListResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**data** | [**List[Crash]**](Crash.md) |  | 
**pagination** | [**Pagination**](Pagination.md) |  | 

## Example

```python
from pandafuzz.models.crash_list_response import CrashListResponse

# TODO update the JSON string below
json = "{}"
# create an instance of CrashListResponse from a JSON string
crash_list_response_instance = CrashListResponse.from_json(json)
# print the JSON string representation of the object
print(CrashListResponse.to_json())

# convert the object into a dict
crash_list_response_dict = crash_list_response_instance.to_dict()
# create an instance of CrashListResponse from a dict
crash_list_response_from_dict = CrashListResponse.from_dict(crash_list_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


