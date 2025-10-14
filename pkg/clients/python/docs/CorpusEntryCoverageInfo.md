# CorpusEntryCoverageInfo

Coverage information for this corpus entry

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**edges_covered** | **int** | Number of coverage edges this input covers | [optional] 
**new_edges** | **int** | Number of new edges discovered by this input | [optional] 
**blocks_covered** | **int** | Number of basic blocks covered | [optional] 

## Example

```python
from pandafuzz.models.corpus_entry_coverage_info import CorpusEntryCoverageInfo

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusEntryCoverageInfo from a JSON string
corpus_entry_coverage_info_instance = CorpusEntryCoverageInfo.from_json(json)
# print the JSON string representation of the object
print(CorpusEntryCoverageInfo.to_json())

# convert the object into a dict
corpus_entry_coverage_info_dict = corpus_entry_coverage_info_instance.to_dict()
# create an instance of CorpusEntryCoverageInfo from a dict
corpus_entry_coverage_info_from_dict = CorpusEntryCoverageInfo.from_dict(corpus_entry_coverage_info_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


