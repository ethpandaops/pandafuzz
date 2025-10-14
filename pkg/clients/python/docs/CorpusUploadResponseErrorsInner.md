# CorpusUploadResponseErrorsInner


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**filename** | **str** |  | [optional] 
**error** | **str** |  | [optional] 
**code** | **str** |  | [optional] 

## Example

```python
from pandafuzz.models.corpus_upload_response_errors_inner import CorpusUploadResponseErrorsInner

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusUploadResponseErrorsInner from a JSON string
corpus_upload_response_errors_inner_instance = CorpusUploadResponseErrorsInner.from_json(json)
# print the JSON string representation of the object
print(CorpusUploadResponseErrorsInner.to_json())

# convert the object into a dict
corpus_upload_response_errors_inner_dict = corpus_upload_response_errors_inner_instance.to_dict()
# create an instance of CorpusUploadResponseErrorsInner from a dict
corpus_upload_response_errors_inner_from_dict = CorpusUploadResponseErrorsInner.from_dict(corpus_upload_response_errors_inner_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


