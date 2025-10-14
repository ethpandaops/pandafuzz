# CoverageTrendsResponseDataPointsInner


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**timestamp** | **datetime** |  | [optional] 
**total_edges** | **int** |  | [optional] 
**new_edges** | **int** |  | [optional] 
**cumulative_edges** | **int** |  | [optional] 
**coverage_density** | **float** | Coverage per execution | [optional] 
**execution_count** | **int** |  | [optional] 

## Example

```python
from pandafuzz.models.coverage_trends_response_data_points_inner import CoverageTrendsResponseDataPointsInner

# TODO update the JSON string below
json = "{}"
# create an instance of CoverageTrendsResponseDataPointsInner from a JSON string
coverage_trends_response_data_points_inner_instance = CoverageTrendsResponseDataPointsInner.from_json(json)
# print the JSON string representation of the object
print(CoverageTrendsResponseDataPointsInner.to_json())

# convert the object into a dict
coverage_trends_response_data_points_inner_dict = coverage_trends_response_data_points_inner_instance.to_dict()
# create an instance of CoverageTrendsResponseDataPointsInner from a dict
coverage_trends_response_data_points_inner_from_dict = CoverageTrendsResponseDataPointsInner.from_dict(coverage_trends_response_data_points_inner_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


