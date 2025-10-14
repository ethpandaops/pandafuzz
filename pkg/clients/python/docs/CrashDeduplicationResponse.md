# CrashDeduplicationResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**crash_id** | **str** | ID of the analyzed crash | 
**is_unique** | **bool** | Whether the crash is unique | 
**duplicate_of** | **str** | ID of the crash this is a duplicate of | [optional] 
**similarity_score** | **float** | Similarity score to the duplicate (0-1) | [optional] 
**algorithm_used** | **str** | Deduplication algorithm used | 
**group_id** | **str** | Group identifier assigned | [optional] 
**similar_crashes** | [**List[CrashDeduplicationResponseSimilarCrashesInner]**](CrashDeduplicationResponseSimilarCrashesInner.md) | List of similar crashes found | [optional] 
**processing_time_seconds** | **float** | Time taken for deduplication analysis | 

## Example

```python
from pandafuzz.models.crash_deduplication_response import CrashDeduplicationResponse

# TODO update the JSON string below
json = "{}"
# create an instance of CrashDeduplicationResponse from a JSON string
crash_deduplication_response_instance = CrashDeduplicationResponse.from_json(json)
# print the JSON string representation of the object
print(CrashDeduplicationResponse.to_json())

# convert the object into a dict
crash_deduplication_response_dict = crash_deduplication_response_instance.to_dict()
# create an instance of CrashDeduplicationResponse from a dict
crash_deduplication_response_from_dict = CrashDeduplicationResponse.from_dict(crash_deduplication_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


