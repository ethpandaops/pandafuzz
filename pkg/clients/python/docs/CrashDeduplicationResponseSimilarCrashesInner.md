# CrashDeduplicationResponseSimilarCrashesInner


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**crash_id** | **str** |  | [optional] 
**similarity_score** | **float** |  | [optional] 

## Example

```python
from pandafuzz.models.crash_deduplication_response_similar_crashes_inner import CrashDeduplicationResponseSimilarCrashesInner

# TODO update the JSON string below
json = "{}"
# create an instance of CrashDeduplicationResponseSimilarCrashesInner from a JSON string
crash_deduplication_response_similar_crashes_inner_instance = CrashDeduplicationResponseSimilarCrashesInner.from_json(json)
# print the JSON string representation of the object
print(CrashDeduplicationResponseSimilarCrashesInner.to_json())

# convert the object into a dict
crash_deduplication_response_similar_crashes_inner_dict = crash_deduplication_response_similar_crashes_inner_instance.to_dict()
# create an instance of CrashDeduplicationResponseSimilarCrashesInner from a dict
crash_deduplication_response_similar_crashes_inner_from_dict = CrashDeduplicationResponseSimilarCrashesInner.from_dict(crash_deduplication_response_similar_crashes_inner_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


