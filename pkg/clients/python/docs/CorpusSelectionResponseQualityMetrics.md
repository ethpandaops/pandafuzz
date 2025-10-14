# CorpusSelectionResponseQualityMetrics

Quality metrics of the selection

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**coverage_density** | **float** | Coverage per byte ratio | [optional] 
**diversity_score** | **float** | Diversity score of selected corpus | [optional] 
**redundancy_score** | **float** | Redundancy score (lower is better) | [optional] 

## Example

```python
from pandafuzz.models.corpus_selection_response_quality_metrics import CorpusSelectionResponseQualityMetrics

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusSelectionResponseQualityMetrics from a JSON string
corpus_selection_response_quality_metrics_instance = CorpusSelectionResponseQualityMetrics.from_json(json)
# print the JSON string representation of the object
print(CorpusSelectionResponseQualityMetrics.to_json())

# convert the object into a dict
corpus_selection_response_quality_metrics_dict = corpus_selection_response_quality_metrics_instance.to_dict()
# create an instance of CorpusSelectionResponseQualityMetrics from a dict
corpus_selection_response_quality_metrics_from_dict = CorpusSelectionResponseQualityMetrics.from_dict(corpus_selection_response_quality_metrics_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


