# BotHeartbeatResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**acknowledged** | **bool** | Whether the heartbeat was acknowledged | 
**next_heartbeat_interval_seconds** | **int** | Interval for next heartbeat in seconds | 
**assigned_job_id** | **str** | ID of newly assigned job, if any | [optional] 
**commands** | [**List[BotHeartbeatResponseCommandsInner]**](BotHeartbeatResponseCommandsInner.md) | Commands for the bot to execute | [optional] 
**message** | **str** | Optional message from the master | [optional] 

## Example

```python
from pandafuzz.models.bot_heartbeat_response import BotHeartbeatResponse

# TODO update the JSON string below
json = "{}"
# create an instance of BotHeartbeatResponse from a JSON string
bot_heartbeat_response_instance = BotHeartbeatResponse.from_json(json)
# print the JSON string representation of the object
print(BotHeartbeatResponse.to_json())

# convert the object into a dict
bot_heartbeat_response_dict = bot_heartbeat_response_instance.to_dict()
# create an instance of BotHeartbeatResponse from a dict
bot_heartbeat_response_from_dict = BotHeartbeatResponse.from_dict(bot_heartbeat_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


