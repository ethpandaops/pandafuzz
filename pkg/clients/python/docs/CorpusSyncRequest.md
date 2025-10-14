# CorpusSyncRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**source_campaign_id** | **str** | Campaign to sync corpus from | 
**target_campaign_id** | **str** | Campaign to sync corpus to | 
**sync_strategy** | **str** | Strategy for selecting corpus to sync | [optional] [default to 'coverage_based']
**filters** | [**CorpusSyncRequestFilters**](CorpusSyncRequestFilters.md) |  | [optional] 
**max_files** | **int** | Maximum number of files to sync | [optional] 
**overwrite_duplicates** | **bool** | Whether to overwrite existing files with same hash | [optional] [default to False]

## Example

```python
from pandafuzz.models.corpus_sync_request import CorpusSyncRequest

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusSyncRequest from a JSON string
corpus_sync_request_instance = CorpusSyncRequest.from_json(json)
# print the JSON string representation of the object
print(CorpusSyncRequest.to_json())

# convert the object into a dict
corpus_sync_request_dict = corpus_sync_request_instance.to_dict()
# create an instance of CorpusSyncRequest from a dict
corpus_sync_request_from_dict = CorpusSyncRequest.from_dict(corpus_sync_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


