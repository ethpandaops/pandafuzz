# CorpusSyncRequestFilters

Filters for corpus selection

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**min_coverage** | **int** | Minimum coverage edges required | [optional] 
**max_size_bytes** | **int** | Maximum file size to sync | [optional] 
**created_after** | **datetime** | Only sync files created after this time | [optional] 
**tags** | **List[str]** | Only sync files with these tags | [optional] 

## Example

```python
from pandafuzz.models.corpus_sync_request_filters import CorpusSyncRequestFilters

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusSyncRequestFilters from a JSON string
corpus_sync_request_filters_instance = CorpusSyncRequestFilters.from_json(json)
# print the JSON string representation of the object
print(CorpusSyncRequestFilters.to_json())

# convert the object into a dict
corpus_sync_request_filters_dict = corpus_sync_request_filters_instance.to_dict()
# create an instance of CorpusSyncRequestFilters from a dict
corpus_sync_request_filters_from_dict = CorpusSyncRequestFilters.from_dict(corpus_sync_request_filters_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


