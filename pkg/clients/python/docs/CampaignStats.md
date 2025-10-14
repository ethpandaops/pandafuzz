# CampaignStats


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**campaign_id** | **str** | ID of the campaign | 
**total_jobs** | **int** | Total number of jobs in the campaign | 
**active_jobs** | **int** | Number of currently running jobs | 
**completed_jobs** | **int** | Number of completed jobs | 
**failed_jobs** | **int** | Number of failed jobs | 
**total_crashes** | **int** | Total number of crashes found | 
**unique_crashes** | **int** | Number of unique crashes | 
**corpus_size** | **int** | Total number of corpus entries | 
**corpus_size_bytes** | **int** | Total corpus size in bytes | [optional] 
**total_coverage_edges** | **int** | Total coverage edges discovered | 
**execution_time_seconds** | **int** | Total execution time across all jobs | 
**last_updated** | **datetime** | When the statistics were last updated | 
**performance_metrics** | [**CampaignStatsPerformanceMetrics**](CampaignStatsPerformanceMetrics.md) |  | [optional] 

## Example

```python
from pandafuzz.models.campaign_stats import CampaignStats

# TODO update the JSON string below
json = "{}"
# create an instance of CampaignStats from a JSON string
campaign_stats_instance = CampaignStats.from_json(json)
# print the JSON string representation of the object
print(CampaignStats.to_json())

# convert the object into a dict
campaign_stats_dict = campaign_stats_instance.to_dict()
# create an instance of CampaignStats from a dict
campaign_stats_from_dict = CampaignStats.from_dict(campaign_stats_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


