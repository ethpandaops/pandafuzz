# CorpusEntryGenerationInfo

Information about how this corpus was generated

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**parent_hash** | **str** | Hash of the parent corpus entry if mutated | [optional] 
**mutation_type** | **str** | How this corpus entry was generated | [optional] 
**generation** | **int** | Generation number in the fuzzing process | [optional] 

## Example

```python
from pandafuzz.models.corpus_entry_generation_info import CorpusEntryGenerationInfo

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusEntryGenerationInfo from a JSON string
corpus_entry_generation_info_instance = CorpusEntryGenerationInfo.from_json(json)
# print the JSON string representation of the object
print(CorpusEntryGenerationInfo.to_json())

# convert the object into a dict
corpus_entry_generation_info_dict = corpus_entry_generation_info_instance.to_dict()
# create an instance of CorpusEntryGenerationInfo from a dict
corpus_entry_generation_info_from_dict = CorpusEntryGenerationInfo.from_dict(corpus_entry_generation_info_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


