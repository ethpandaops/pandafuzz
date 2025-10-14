# CoverageTrendsResponseTimeRange


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**start** | **datetime** |  | [optional] 
**end** | **datetime** |  | [optional] 

## Example

```python
from pandafuzz.models.coverage_trends_response_time_range import CoverageTrendsResponseTimeRange

# TODO update the JSON string below
json = "{}"
# create an instance of CoverageTrendsResponseTimeRange from a JSON string
coverage_trends_response_time_range_instance = CoverageTrendsResponseTimeRange.from_json(json)
# print the JSON string representation of the object
print(CoverageTrendsResponseTimeRange.to_json())

# convert the object into a dict
coverage_trends_response_time_range_dict = coverage_trends_response_time_range_instance.to_dict()
# create an instance of CoverageTrendsResponseTimeRange from a dict
coverage_trends_response_time_range_from_dict = CoverageTrendsResponseTimeRange.from_dict(coverage_trends_response_time_range_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


