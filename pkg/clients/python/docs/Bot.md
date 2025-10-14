# Bot


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **str** | Unique identifier for the bot | 
**name** | **str** | Human-readable name for the bot | 
**hostname** | **str** | Hostname where the bot is running | 
**status** | [**BotStatus**](BotStatus.md) |  | 
**capabilities** | **List[str]** | Capabilities supported by this bot | 
**last_heartbeat** | **datetime** | Timestamp of last heartbeat received | 
**is_online** | **bool** | Whether the bot is currently online | 
**current_job_id** | **str** | ID of currently assigned job, if any | [optional] 
**registered_at** | **datetime** | When the bot was first registered | 
**api_endpoint** | **str** | API endpoint for communicating with the bot | [optional] 
**resource_usage** | [**BotResourceUsage**](BotResourceUsage.md) |  | [optional] 
**metadata** | **Dict[str, object]** | Key-value metadata for extensibility | [optional] 

## Example

```python
from pandafuzz.models.bot import Bot

# TODO update the JSON string below
json = "{}"
# create an instance of Bot from a JSON string
bot_instance = Bot.from_json(json)
# print the JSON string representation of the object
print(Bot.to_json())

# convert the object into a dict
bot_dict = bot_instance.to_dict()
# create an instance of Bot from a dict
bot_from_dict = Bot.from_dict(bot_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


