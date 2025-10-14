# CampaignCreateRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**name** | **str** | Human-readable name for the campaign | 
**description** | **str** | Detailed description of the campaign | [optional] 
**target_binary** | **str** | Path to the target binary | 
**max_duration_seconds** | **int** | Maximum campaign duration in seconds | [optional] [default to 86400]
**max_jobs** | **int** | Maximum number of concurrent jobs | [optional] [default to 5]
**job_template** | [**CampaignCreateRequestJobTemplate**](CampaignCreateRequestJobTemplate.md) |  | [optional] 
**auto_restart** | **bool** | Whether to automatically restart failed jobs | [optional] [default to True]
**shared_corpus** | **bool** | Whether to share corpus between jobs in this campaign | [optional] [default to True]
**tags** | **List[str]** | Tags for categorizing and filtering campaigns | [optional] 
**metadata** | **Dict[str, object]** | Key-value metadata for extensibility | [optional] 

## Example

```python
from pandafuzz.models.campaign_create_request import CampaignCreateRequest

# TODO update the JSON string below
json = "{}"
# create an instance of CampaignCreateRequest from a JSON string
campaign_create_request_instance = CampaignCreateRequest.from_json(json)
# print the JSON string representation of the object
print(CampaignCreateRequest.to_json())

# convert the object into a dict
campaign_create_request_dict = campaign_create_request_instance.to_dict()
# create an instance of CampaignCreateRequest from a dict
campaign_create_request_from_dict = CampaignCreateRequest.from_dict(campaign_create_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


