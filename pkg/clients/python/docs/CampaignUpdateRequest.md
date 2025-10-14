# CampaignUpdateRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**name** | **str** | Human-readable name for the campaign | [optional] 
**description** | **str** | Detailed description of the campaign | [optional] 
**max_duration_seconds** | **int** | Maximum campaign duration in seconds | [optional] 
**max_jobs** | **int** | Maximum number of concurrent jobs | [optional] 
**auto_restart** | **bool** | Whether to automatically restart failed jobs | [optional] 
**shared_corpus** | **bool** | Whether to share corpus between jobs in this campaign | [optional] 
**tags** | **List[str]** | Tags for categorizing and filtering campaigns | [optional] 
**metadata** | **Dict[str, object]** | Key-value metadata for extensibility | [optional] 

## Example

```python
from pandafuzz.models.campaign_update_request import CampaignUpdateRequest

# TODO update the JSON string below
json = "{}"
# create an instance of CampaignUpdateRequest from a JSON string
campaign_update_request_instance = CampaignUpdateRequest.from_json(json)
# print the JSON string representation of the object
print(CampaignUpdateRequest.to_json())

# convert the object into a dict
campaign_update_request_dict = campaign_update_request_instance.to_dict()
# create an instance of CampaignUpdateRequest from a dict
campaign_update_request_from_dict = CampaignUpdateRequest.from_dict(campaign_update_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


