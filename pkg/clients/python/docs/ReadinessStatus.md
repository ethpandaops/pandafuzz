# ReadinessStatus


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**ready** | **bool** | Whether the system is ready to accept requests | 
**timestamp** | **datetime** | When the readiness check was performed | 
**message** | **str** | Additional information about readiness status | [optional] 

## Example

```python
from pandafuzz.models.readiness_status import ReadinessStatus

# TODO update the JSON string below
json = "{}"
# create an instance of ReadinessStatus from a JSON string
readiness_status_instance = ReadinessStatus.from_json(json)
# print the JSON string representation of the object
print(ReadinessStatus.to_json())

# convert the object into a dict
readiness_status_dict = readiness_status_instance.to_dict()
# create an instance of ReadinessStatus from a dict
readiness_status_from_dict = ReadinessStatus.from_dict(readiness_status_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


