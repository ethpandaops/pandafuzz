# CrashCrashInfo

Detailed crash analysis information

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**address** | **str** | Memory address where crash occurred | [optional] 
**instruction** | **str** | Instruction that caused the crash | [optional] 
**registers** | **Dict[str, str]** | CPU register values at crash time | [optional] 
**memory_maps** | **List[str]** | Memory mappings at crash time | [optional] 

## Example

```python
from pandafuzz.models.crash_crash_info import CrashCrashInfo

# TODO update the JSON string below
json = "{}"
# create an instance of CrashCrashInfo from a JSON string
crash_crash_info_instance = CrashCrashInfo.from_json(json)
# print the JSON string representation of the object
print(CrashCrashInfo.to_json())

# convert the object into a dict
crash_crash_info_dict = crash_crash_info_instance.to_dict()
# create an instance of CrashCrashInfo from a dict
crash_crash_info_from_dict = CrashCrashInfo.from_dict(crash_crash_info_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


