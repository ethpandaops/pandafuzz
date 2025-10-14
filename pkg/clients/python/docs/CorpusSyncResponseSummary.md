# CorpusSyncResponseSummary

Summary statistics of the sync operation

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**source_total_files** | **int** |  | [optional] 
**target_files_before** | **int** |  | [optional] 
**target_files_after** | **int** |  | [optional] 
**coverage_improvement** | **float** | Percentage improvement in coverage | [optional] 

## Example

```python
from pandafuzz.models.corpus_sync_response_summary import CorpusSyncResponseSummary

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusSyncResponseSummary from a JSON string
corpus_sync_response_summary_instance = CorpusSyncResponseSummary.from_json(json)
# print the JSON string representation of the object
print(CorpusSyncResponseSummary.to_json())

# convert the object into a dict
corpus_sync_response_summary_dict = corpus_sync_response_summary_instance.to_dict()
# create an instance of CorpusSyncResponseSummary from a dict
corpus_sync_response_summary_from_dict = CorpusSyncResponseSummary.from_dict(corpus_sync_response_summary_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


