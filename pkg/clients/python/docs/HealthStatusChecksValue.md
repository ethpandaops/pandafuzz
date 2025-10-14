# HealthStatusChecksValue


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**status** | **str** |  | [optional] 
**message** | **str** |  | [optional] 
**latency_ms** | **int** |  | [optional] 
**last_check** | **datetime** |  | [optional] 

## Example

```python
from pandafuzz.models.health_status_checks_value import HealthStatusChecksValue

# TODO update the JSON string below
json = "{}"
# create an instance of HealthStatusChecksValue from a JSON string
health_status_checks_value_instance = HealthStatusChecksValue.from_json(json)
# print the JSON string representation of the object
print(HealthStatusChecksValue.to_json())

# convert the object into a dict
health_status_checks_value_dict = health_status_checks_value_instance.to_dict()
# create an instance of HealthStatusChecksValue from a dict
health_status_checks_value_from_dict = HealthStatusChecksValue.from_dict(health_status_checks_value_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


