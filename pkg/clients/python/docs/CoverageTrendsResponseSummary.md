# CoverageTrendsResponseSummary


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**total_growth** | **int** | Total new edges discovered in time range | [optional] 
**growth_rate** | **float** | Average edges per hour | [optional] 
**peak_discovery_time** | **datetime** | Time of highest edge discovery rate | [optional] 
**efficiency_score** | **float** | Coverage efficiency score | [optional] 

## Example

```python
from pandafuzz.models.coverage_trends_response_summary import CoverageTrendsResponseSummary

# TODO update the JSON string below
json = "{}"
# create an instance of CoverageTrendsResponseSummary from a JSON string
coverage_trends_response_summary_instance = CoverageTrendsResponseSummary.from_json(json)
# print the JSON string representation of the object
print(CoverageTrendsResponseSummary.to_json())

# convert the object into a dict
coverage_trends_response_summary_dict = coverage_trends_response_summary_instance.to_dict()
# create an instance of CoverageTrendsResponseSummary from a dict
coverage_trends_response_summary_from_dict = CoverageTrendsResponseSummary.from_dict(coverage_trends_response_summary_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


