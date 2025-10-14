# CrashReproductionInfo

Information about crash reproduction

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**reproducible** | **bool** | Whether the crash is reproducible | [optional] 
**reproduction_rate** | **float** | Rate of successful reproductions (0-1) | [optional] 
**last_reproduction_attempt** | **datetime** | When reproduction was last attempted | [optional] 
**environment** | **Dict[str, str]** | Environment conditions for reproduction | [optional] 

## Example

```python
from pandafuzz.models.crash_reproduction_info import CrashReproductionInfo

# TODO update the JSON string below
json = "{}"
# create an instance of CrashReproductionInfo from a JSON string
crash_reproduction_info_instance = CrashReproductionInfo.from_json(json)
# print the JSON string representation of the object
print(CrashReproductionInfo.to_json())

# convert the object into a dict
crash_reproduction_info_dict = crash_reproduction_info_instance.to_dict()
# create an instance of CrashReproductionInfo from a dict
crash_reproduction_info_from_dict = CrashReproductionInfo.from_dict(crash_reproduction_info_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


