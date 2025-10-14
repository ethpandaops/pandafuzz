# BotUpdateRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**name** | **str** | Human-readable name for the bot | [optional] 
**capabilities** | **List[str]** | Capabilities supported by this bot | [optional] 
**api_endpoint** | **str** | API endpoint for communicating with the bot | [optional] 
**metadata** | **Dict[str, object]** | Key-value metadata for extensibility | [optional] 

## Example

```python
from pandafuzz.models.bot_update_request import BotUpdateRequest

# TODO update the JSON string below
json = "{}"
# create an instance of BotUpdateRequest from a JSON string
bot_update_request_instance = BotUpdateRequest.from_json(json)
# print the JSON string representation of the object
print(BotUpdateRequest.to_json())

# convert the object into a dict
bot_update_request_dict = bot_update_request_instance.to_dict()
# create an instance of BotUpdateRequest from a dict
bot_update_request_from_dict = BotUpdateRequest.from_dict(bot_update_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


