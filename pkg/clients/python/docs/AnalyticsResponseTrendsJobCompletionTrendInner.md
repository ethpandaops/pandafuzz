# AnalyticsResponseTrendsJobCompletionTrendInner


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**timestamp** | **datetime** |  | [optional] 
**completed_jobs** | **int** |  | [optional] 

## Example

```python
from pandafuzz.models.analytics_response_trends_job_completion_trend_inner import AnalyticsResponseTrendsJobCompletionTrendInner

# TODO update the JSON string below
json = "{}"
# create an instance of AnalyticsResponseTrendsJobCompletionTrendInner from a JSON string
analytics_response_trends_job_completion_trend_inner_instance = AnalyticsResponseTrendsJobCompletionTrendInner.from_json(json)
# print the JSON string representation of the object
print(AnalyticsResponseTrendsJobCompletionTrendInner.to_json())

# convert the object into a dict
analytics_response_trends_job_completion_trend_inner_dict = analytics_response_trends_job_completion_trend_inner_instance.to_dict()
# create an instance of AnalyticsResponseTrendsJobCompletionTrendInner from a dict
analytics_response_trends_job_completion_trend_inner_from_dict = AnalyticsResponseTrendsJobCompletionTrendInner.from_dict(analytics_response_trends_job_completion_trend_inner_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


