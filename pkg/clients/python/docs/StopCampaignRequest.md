# StopCampaignRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**reason** | **str** | Reason for stopping the campaign | [optional] 

## Example

```python
from pandafuzz.models.stop_campaign_request import StopCampaignRequest

# TODO update the JSON string below
json = "{}"
# create an instance of StopCampaignRequest from a JSON string
stop_campaign_request_instance = StopCampaignRequest.from_json(json)
# print the JSON string representation of the object
print(StopCampaignRequest.to_json())

# convert the object into a dict
stop_campaign_request_dict = stop_campaign_request_instance.to_dict()
# create an instance of StopCampaignRequest from a dict
stop_campaign_request_from_dict = StopCampaignRequest.from_dict(stop_campaign_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


