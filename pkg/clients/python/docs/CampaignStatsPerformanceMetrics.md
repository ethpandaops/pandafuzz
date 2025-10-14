# CampaignStatsPerformanceMetrics

Performance and efficiency metrics

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**avg_executions_per_second** | **float** | Average executions per second | [optional] 
**coverage_growth_rate** | **float** | Rate of coverage growth per hour | [optional] 
**crash_discovery_rate** | **float** | Rate of crash discovery per hour | [optional] 

## Example

```python
from pandafuzz.models.campaign_stats_performance_metrics import CampaignStatsPerformanceMetrics

# TODO update the JSON string below
json = "{}"
# create an instance of CampaignStatsPerformanceMetrics from a JSON string
campaign_stats_performance_metrics_instance = CampaignStatsPerformanceMetrics.from_json(json)
# print the JSON string representation of the object
print(CampaignStatsPerformanceMetrics.to_json())

# convert the object into a dict
campaign_stats_performance_metrics_dict = campaign_stats_performance_metrics_instance.to_dict()
# create an instance of CampaignStatsPerformanceMetrics from a dict
campaign_stats_performance_metrics_from_dict = CampaignStatsPerformanceMetrics.from_dict(campaign_stats_performance_metrics_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


