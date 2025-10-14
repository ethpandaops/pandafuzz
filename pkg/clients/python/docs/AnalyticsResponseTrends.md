# AnalyticsResponseTrends

Historical trend data

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**job_completion_trend** | [**List[AnalyticsResponseTrendsJobCompletionTrendInner]**](AnalyticsResponseTrendsJobCompletionTrendInner.md) |  | [optional] 
**coverage_growth_trend** | [**List[AnalyticsResponseTrendsCoverageGrowthTrendInner]**](AnalyticsResponseTrendsCoverageGrowthTrendInner.md) |  | [optional] 
**crash_discovery_trend** | [**List[AnalyticsResponseTrendsCrashDiscoveryTrendInner]**](AnalyticsResponseTrendsCrashDiscoveryTrendInner.md) |  | [optional] 

## Example

```python
from pandafuzz.models.analytics_response_trends import AnalyticsResponseTrends

# TODO update the JSON string below
json = "{}"
# create an instance of AnalyticsResponseTrends from a JSON string
analytics_response_trends_instance = AnalyticsResponseTrends.from_json(json)
# print the JSON string representation of the object
print(AnalyticsResponseTrends.to_json())

# convert the object into a dict
analytics_response_trends_dict = analytics_response_trends_instance.to_dict()
# create an instance of AnalyticsResponseTrends from a dict
analytics_response_trends_from_dict = AnalyticsResponseTrends.from_dict(analytics_response_trends_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


