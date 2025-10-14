# CoverageTrendsResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**time_range** | [**CoverageTrendsResponseTimeRange**](CoverageTrendsResponseTimeRange.md) |  | 
**granularity** | **str** |  | 
**campaign_id** | **str** | Campaign ID if filtered by campaign | [optional] 
**data_points** | [**List[CoverageTrendsResponseDataPointsInner]**](CoverageTrendsResponseDataPointsInner.md) |  | 
**summary** | [**CoverageTrendsResponseSummary**](CoverageTrendsResponseSummary.md) |  | [optional] 

## Example

```python
from pandafuzz.models.coverage_trends_response import CoverageTrendsResponse

# TODO update the JSON string below
json = "{}"
# create an instance of CoverageTrendsResponse from a JSON string
coverage_trends_response_instance = CoverageTrendsResponse.from_json(json)
# print the JSON string representation of the object
print(CoverageTrendsResponse.to_json())

# convert the object into a dict
coverage_trends_response_dict = coverage_trends_response_instance.to_dict()
# create an instance of CoverageTrendsResponse from a dict
coverage_trends_response_from_dict = CoverageTrendsResponse.from_dict(coverage_trends_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


