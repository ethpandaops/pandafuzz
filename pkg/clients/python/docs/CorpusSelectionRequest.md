# CorpusSelectionRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**campaign_id** | **str** | Campaign to select corpus from | 
**selection_strategy** | **str** | Strategy for corpus selection | 
**max_entries** | **int** | Maximum number of entries to select | [optional] 
**criteria** | [**CorpusSelectionRequestCriteria**](CorpusSelectionRequestCriteria.md) |  | [optional] 
**seed_corpus_weight** | **float** | Weight to give to seed corpus entries (0-1) | [optional] 

## Example

```python
from pandafuzz.models.corpus_selection_request import CorpusSelectionRequest

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusSelectionRequest from a JSON string
corpus_selection_request_instance = CorpusSelectionRequest.from_json(json)
# print the JSON string representation of the object
print(CorpusSelectionRequest.to_json())

# convert the object into a dict
corpus_selection_request_dict = corpus_selection_request_instance.to_dict()
# create an instance of CorpusSelectionRequest from a dict
corpus_selection_request_from_dict = CorpusSelectionRequest.from_dict(corpus_selection_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


