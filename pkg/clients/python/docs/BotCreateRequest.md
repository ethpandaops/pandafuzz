# BotCreateRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**name** | **str** | Human-readable name for the bot | 
**hostname** | **str** | Hostname where the bot is running | 
**capabilities** | **List[str]** | Capabilities supported by this bot | 
**api_endpoint** | **str** | API endpoint for communicating with the bot | 
**metadata** | **Dict[str, object]** | Key-value metadata for extensibility | [optional] 

## Example

```python
from pandafuzz.models.bot_create_request import BotCreateRequest

# TODO update the JSON string below
json = "{}"
# create an instance of BotCreateRequest from a JSON string
bot_create_request_instance = BotCreateRequest.from_json(json)
# print the JSON string representation of the object
print(BotCreateRequest.to_json())

# convert the object into a dict
bot_create_request_dict = bot_create_request_instance.to_dict()
# create an instance of BotCreateRequest from a dict
bot_create_request_from_dict = BotCreateRequest.from_dict(bot_create_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


