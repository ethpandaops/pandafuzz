# CorpusEntry


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **str** | Unique identifier for the corpus entry | 
**filename** | **str** | Original filename of the corpus file | 
**size_bytes** | **int** | Size of the corpus file in bytes | 
**hash** | **str** | SHA256 hash of the corpus file content | 
**campaign_id** | **str** | ID of the campaign this corpus belongs to | 
**job_id** | **str** | ID of the job that generated this corpus | 
**bot_id** | **str** | ID of the bot that generated this corpus | [optional] 
**created_at** | **datetime** | When the corpus entry was created | 
**coverage_info** | [**CorpusEntryCoverageInfo**](CorpusEntryCoverageInfo.md) |  | [optional] 
**generation_info** | [**CorpusEntryGenerationInfo**](CorpusEntryGenerationInfo.md) |  | [optional] 
**is_seed** | **bool** | Whether this is an initial seed corpus entry | [optional] 
**is_minimized** | **bool** | Whether this corpus has been minimized | [optional] 
**tags** | **List[str]** | Tags associated with this corpus entry | [optional] 
**metadata** | **Dict[str, object]** | Key-value metadata for extensibility | [optional] 

## Example

```python
from pandafuzz.models.corpus_entry import CorpusEntry

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusEntry from a JSON string
corpus_entry_instance = CorpusEntry.from_json(json)
# print the JSON string representation of the object
print(CorpusEntry.to_json())

# convert the object into a dict
corpus_entry_dict = corpus_entry_instance.to_dict()
# create an instance of CorpusEntry from a dict
corpus_entry_from_dict = CorpusEntry.from_dict(corpus_entry_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


