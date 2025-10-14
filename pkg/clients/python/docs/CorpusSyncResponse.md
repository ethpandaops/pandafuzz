# CorpusSyncResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**sync_id** | **str** | Unique identifier for this sync operation | 
**synced_files** | **int** | Number of files successfully synced | 
**skipped_files** | **int** | Number of files skipped (duplicates, etc.) | [optional] 
**total_size_bytes** | **int** | Total size of synced files in bytes | 
**duration_seconds** | **float** | Time taken for the sync operation | 
**strategy_used** | **str** | Sync strategy that was applied | 
**summary** | [**CorpusSyncResponseSummary**](CorpusSyncResponseSummary.md) |  | [optional] 

## Example

```python
from pandafuzz.models.corpus_sync_response import CorpusSyncResponse

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusSyncResponse from a JSON string
corpus_sync_response_instance = CorpusSyncResponse.from_json(json)
# print the JSON string representation of the object
print(CorpusSyncResponse.to_json())

# convert the object into a dict
corpus_sync_response_dict = corpus_sync_response_instance.to_dict()
# create an instance of CorpusSyncResponse from a dict
corpus_sync_response_from_dict = CorpusSyncResponse.from_dict(corpus_sync_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


