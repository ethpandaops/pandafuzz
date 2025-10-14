# BotHeartbeatResponseCommandsInner


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**type** | **str** |  | [optional] 
**parameters** | **Dict[str, object]** |  | [optional] 

## Example

```python
from pandafuzz.models.bot_heartbeat_response_commands_inner import BotHeartbeatResponseCommandsInner

# TODO update the JSON string below
json = "{}"
# create an instance of BotHeartbeatResponseCommandsInner from a JSON string
bot_heartbeat_response_commands_inner_instance = BotHeartbeatResponseCommandsInner.from_json(json)
# print the JSON string representation of the object
print(BotHeartbeatResponseCommandsInner.to_json())

# convert the object into a dict
bot_heartbeat_response_commands_inner_dict = bot_heartbeat_response_commands_inner_instance.to_dict()
# create an instance of BotHeartbeatResponseCommandsInner from a dict
bot_heartbeat_response_commands_inner_from_dict = BotHeartbeatResponseCommandsInner.from_dict(bot_heartbeat_response_commands_inner_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


