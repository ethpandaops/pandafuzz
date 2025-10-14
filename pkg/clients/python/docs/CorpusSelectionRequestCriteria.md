# CorpusSelectionRequestCriteria

Selection criteria and preferences

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**min_coverage** | **int** |  | [optional] 
**max_size_bytes** | **int** |  | [optional] 
**prefer_small_files** | **bool** |  | [optional] 
**include_edge_cases** | **bool** |  | [optional] 
**min_generation** | **int** |  | [optional] 

## Example

```python
from pandafuzz.models.corpus_selection_request_criteria import CorpusSelectionRequestCriteria

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusSelectionRequestCriteria from a JSON string
corpus_selection_request_criteria_instance = CorpusSelectionRequestCriteria.from_json(json)
# print the JSON string representation of the object
print(CorpusSelectionRequestCriteria.to_json())

# convert the object into a dict
corpus_selection_request_criteria_dict = corpus_selection_request_criteria_instance.to_dict()
# create an instance of CorpusSelectionRequestCriteria from a dict
corpus_selection_request_criteria_from_dict = CorpusSelectionRequestCriteria.from_dict(corpus_selection_request_criteria_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


