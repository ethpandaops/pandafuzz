# AnalyticsResponseSystemOverview

High-level system statistics

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**total_campaigns** | **int** |  | [optional] 
**active_campaigns** | **int** |  | [optional] 
**total_jobs** | **int** |  | [optional] 
**active_jobs** | **int** |  | [optional] 
**total_bots** | **int** |  | [optional] 
**online_bots** | **int** |  | [optional] 
**total_crashes** | **int** |  | [optional] 
**unique_crashes** | **int** |  | [optional] 
**total_corpus_entries** | **int** |  | [optional] 
**total_coverage_edges** | **int** |  | [optional] 

## Example

```python
from pandafuzz.models.analytics_response_system_overview import AnalyticsResponseSystemOverview

# TODO update the JSON string below
json = "{}"
# create an instance of AnalyticsResponseSystemOverview from a JSON string
analytics_response_system_overview_instance = AnalyticsResponseSystemOverview.from_json(json)
# print the JSON string representation of the object
print(AnalyticsResponseSystemOverview.to_json())

# convert the object into a dict
analytics_response_system_overview_dict = analytics_response_system_overview_instance.to_dict()
# create an instance of AnalyticsResponseSystemOverview from a dict
analytics_response_system_overview_from_dict = AnalyticsResponseSystemOverview.from_dict(analytics_response_system_overview_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


