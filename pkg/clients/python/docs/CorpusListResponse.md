# CorpusListResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**data** | [**List[CorpusEntry]**](CorpusEntry.md) |  | 
**pagination** | [**Pagination**](Pagination.md) |  | 

## Example

```python
from pandafuzz.models.corpus_list_response import CorpusListResponse

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusListResponse from a JSON string
corpus_list_response_instance = CorpusListResponse.from_json(json)
# print the JSON string representation of the object
print(CorpusListResponse.to_json())

# convert the object into a dict
corpus_list_response_dict = corpus_list_response_instance.to_dict()
# create an instance of CorpusListResponse from a dict
corpus_list_response_from_dict = CorpusListResponse.from_dict(corpus_list_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


