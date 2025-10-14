# Artifact


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **str** | Unique identifier for the artifact | 
**type** | **str** | Type of artifact | 
**filename** | **str** | Original filename of the artifact | 
**size_bytes** | **int** | Size of the artifact in bytes | 
**hash** | **str** | SHA256 hash of the artifact content | 
**job_id** | **str** | ID of the job that generated this artifact | 
**created_at** | **datetime** | When the artifact was created | 
**description** | **str** | Human-readable description of the artifact | [optional] 
**content_type** | **str** | MIME type of the artifact | [optional] 
**download_url** | **str** | URL to download the artifact | [optional] 
**metadata** | **Dict[str, object]** | Key-value metadata for extensibility | [optional] 

## Example

```python
from pandafuzz.models.artifact import Artifact

# TODO update the JSON string below
json = "{}"
# create an instance of Artifact from a JSON string
artifact_instance = Artifact.from_json(json)
# print the JSON string representation of the object
print(Artifact.to_json())

# convert the object into a dict
artifact_dict = artifact_instance.to_dict()
# create an instance of Artifact from a dict
artifact_from_dict = Artifact.from_dict(artifact_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


