# DeduplicateCrashRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**algorithm** | **str** |  | [optional] [default to 'stack_trace']
**threshold** | **float** | Similarity threshold for fuzzy matching | [optional] [default to 0.8]

## Example

```python
from pandafuzz.models.deduplicate_crash_request import DeduplicateCrashRequest

# TODO update the JSON string below
json = "{}"
# create an instance of DeduplicateCrashRequest from a JSON string
deduplicate_crash_request_instance = DeduplicateCrashRequest.from_json(json)
# print the JSON string representation of the object
print(DeduplicateCrashRequest.to_json())

# convert the object into a dict
deduplicate_crash_request_dict = deduplicate_crash_request_instance.to_dict()
# create an instance of DeduplicateCrashRequest from a dict
deduplicate_crash_request_from_dict = DeduplicateCrashRequest.from_dict(deduplicate_crash_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


