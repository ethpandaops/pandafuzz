# BotHeartbeatRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**status** | [**BotStatus**](BotStatus.md) |  | 
**current_job_id** | **str** | ID of currently assigned job, if any | [optional] 
**resource_usage** | [**BotHeartbeatRequestResourceUsage**](BotHeartbeatRequestResourceUsage.md) |  | [optional] 
**message** | **str** | Optional status message | [optional] 

## Example

```python
from pandafuzz.models.bot_heartbeat_request import BotHeartbeatRequest

# TODO update the JSON string below
json = "{}"
# create an instance of BotHeartbeatRequest from a JSON string
bot_heartbeat_request_instance = BotHeartbeatRequest.from_json(json)
# print the JSON string representation of the object
print(BotHeartbeatRequest.to_json())

# convert the object into a dict
bot_heartbeat_request_dict = bot_heartbeat_request_instance.to_dict()
# create an instance of BotHeartbeatRequest from a dict
bot_heartbeat_request_from_dict = BotHeartbeatRequest.from_dict(bot_heartbeat_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


