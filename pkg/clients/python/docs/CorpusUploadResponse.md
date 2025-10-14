# CorpusUploadResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**uploaded_count** | **int** | Number of files successfully uploaded | 
**duplicate_count** | **int** | Number of duplicate files skipped | 
**error_count** | **int** | Number of files that failed to upload | [optional] 
**total_size_bytes** | **int** | Total size of uploaded files in bytes | 
**upload_id** | **str** | Unique identifier for this upload batch | 
**errors** | [**List[CorpusUploadResponseErrorsInner]**](CorpusUploadResponseErrorsInner.md) | Details of any upload errors | [optional] 
**processing_time_seconds** | **float** | Time taken to process the upload | [optional] 

## Example

```python
from pandafuzz.models.corpus_upload_response import CorpusUploadResponse

# TODO update the JSON string below
json = "{}"
# create an instance of CorpusUploadResponse from a JSON string
corpus_upload_response_instance = CorpusUploadResponse.from_json(json)
# print the JSON string representation of the object
print(CorpusUploadResponse.to_json())

# convert the object into a dict
corpus_upload_response_dict = corpus_upload_response_instance.to_dict()
# create an instance of CorpusUploadResponse from a dict
corpus_upload_response_from_dict = CorpusUploadResponse.from_dict(corpus_upload_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


