# ArtifactListResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**data** | [**List[Artifact]**](Artifact.md) |  | 
**pagination** | [**Pagination**](Pagination.md) |  | 

## Example

```python
from pandafuzz.models.artifact_list_response import ArtifactListResponse

# TODO update the JSON string below
json = "{}"
# create an instance of ArtifactListResponse from a JSON string
artifact_list_response_instance = ArtifactListResponse.from_json(json)
# print the JSON string representation of the object
print(ArtifactListResponse.to_json())

# convert the object into a dict
artifact_list_response_dict = artifact_list_response_instance.to_dict()
# create an instance of ArtifactListResponse from a dict
artifact_list_response_from_dict = ArtifactListResponse.from_dict(artifact_list_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


