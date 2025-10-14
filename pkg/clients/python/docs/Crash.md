# Crash


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **str** | Unique identifier for the crash | 
**hash** | **str** | SHA256 hash of the crash-inducing input | 
**type** | [**CrashType**](CrashType.md) |  | 
**severity** | [**CrashSeverity**](CrashSeverity.md) |  | 
**campaign_id** | **str** | ID of the campaign where crash was discovered | 
**job_id** | **str** | ID of the job that discovered the crash | 
**bot_id** | **str** | ID of the bot that discovered the crash | 
**discovered_at** | **datetime** | When the crash was discovered | 
**input_size_bytes** | **int** | Size of the crash-inducing input in bytes | 
**signal** | **int** | Signal number that caused the crash | [optional] 
**exit_code** | **int** | Exit code of the crashed process | [optional] 
**stack_trace** | **str** | Stack trace of the crash | [optional] 
**crash_info** | [**CrashCrashInfo**](CrashCrashInfo.md) |  | [optional] 
**reproduction_info** | [**CrashReproductionInfo**](CrashReproductionInfo.md) |  | [optional] 
**is_unique** | **bool** | Whether this crash is unique (not a duplicate) | [optional] 
**duplicate_of** | **str** | ID of the crash this is a duplicate of | [optional] 
**group_id** | **str** | Group identifier for similar crashes | [optional] 
**minimized_input_id** | **str** | ID of the minimized crash input, if available | [optional] 
**triaged** | **bool** | Whether this crash has been triaged | [optional] 
**priority** | **int** | Triage priority (1&#x3D;lowest, 10&#x3D;highest) | [optional] 
**tags** | **List[str]** | Tags for categorizing the crash | [optional] 
**metadata** | **Dict[str, object]** | Key-value metadata for extensibility | [optional] 

## Example

```python
from pandafuzz.models.crash import Crash

# TODO update the JSON string below
json = "{}"
# create an instance of Crash from a JSON string
crash_instance = Crash.from_json(json)
# print the JSON string representation of the object
print(Crash.to_json())

# convert the object into a dict
crash_dict = crash_instance.to_dict()
# create an instance of Crash from a dict
crash_from_dict = Crash.from_dict(crash_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


