# CorpusSelectionResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**selection_id** | **str** | Unique identifier for this selection | 
**selected_entries** | **List[str]** | IDs of selected corpus entries | 
**total_coverage** | **int** | Total coverage edges of selected corpus | 
**total_size_bytes** | **int** | Total size of selected corpus in bytes | [optional] 
**selection_time_seconds** | **float** | Time taken for selection | 
**strategy_used** | **str** | Selection strategy that was applied | [optional] 
**quality_metrics** | [**CorpusSelectionResponseQualityMetrics**](CorpusSelectionResponseQualityMetrics.md) |  | [optional] 

## Example

```python
from pandafuzz.models.corpus_selection_response import CorpusSelectionResponse

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusSelectionResponse from a JSON string
corpus_selection_response_instance = CorpusSelectionResponse.from_json(json)
# print the JSON string representation of the object
print(CorpusSelectionResponse.to_json())

# convert the object into a dict
corpus_selection_response_dict = corpus_selection_response_instance.to_dict()
# create an instance of CorpusSelectionResponse from a dict
corpus_selection_response_from_dict = CorpusSelectionResponse.from_dict(corpus_selection_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


