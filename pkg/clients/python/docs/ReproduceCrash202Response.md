# ReproduceCrash202Response


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**job_id** | **str** |  | [optional] 
**status** | **str** |  | [optional] 
**attempts_scheduled** | **int** |  | [optional] 

## Example

```python
from pandafuzz.models.reproduce_crash202_response import ReproduceCrash202Response

# TODO update the JSON string below
json = "{}"
# create an instance of ReproduceCrash202Response from a JSON string
reproduce_crash202_response_instance = ReproduceCrash202Response.from_json(json)
# print the JSON string representation of the object
print(ReproduceCrash202Response.to_json())

# convert the object into a dict
reproduce_crash202_response_dict = reproduce_crash202_response_instance.to_dict()
# create an instance of ReproduceCrash202Response from a dict
reproduce_crash202_response_from_dict = ReproduceCrash202Response.from_dict(reproduce_crash202_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


