# Campaign


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **str** | Unique identifier for the campaign | 
**name** | **str** | Human-readable name for the campaign | 
**description** | **str** | Detailed description of the campaign | [optional] 
**status** | [**CampaignStatus**](CampaignStatus.md) |  | 
**target_binary** | **str** | Path to the target binary | 
**target_hash** | **str** | SHA256 hash of the target binary | [optional] 
**created_at** | **datetime** | When the campaign was created | 
**started_at** | **datetime** | When the campaign was started | [optional] 
**completed_at** | **datetime** | When the campaign completed | [optional] 
**max_duration_seconds** | **int** | Maximum campaign duration in seconds (1 hour to 30 days) | [optional] 
**max_jobs** | **int** | Maximum number of concurrent jobs | [optional] 
**job_template** | [**CampaignJobTemplate**](CampaignJobTemplate.md) |  | [optional] 
**auto_restart** | **bool** | Whether to automatically restart failed jobs | [optional] 
**shared_corpus** | **bool** | Whether to share corpus between jobs in this campaign | [optional] 
**tags** | **List[str]** | Tags for categorizing and filtering campaigns | [optional] 
**metadata** | **Dict[str, object]** | Key-value metadata for extensibility | [optional] 

## Example

```python
from pandafuzz.models.campaign import Campaign

# TODO update the JSON string below
json = "{}"
# create an instance of Campaign from a JSON string
campaign_instance = Campaign.from_json(json)
# print the JSON string representation of the object
print(Campaign.to_json())

# convert the object into a dict
campaign_dict = campaign_instance.to_dict()
# create an instance of Campaign from a dict
campaign_from_dict = Campaign.from_dict(campaign_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


