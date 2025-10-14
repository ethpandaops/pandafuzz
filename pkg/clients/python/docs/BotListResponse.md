# BotListResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**data** | [**List[Bot]**](Bot.md) |  | 
**pagination** | [**Pagination**](Pagination.md) |  | 

## Example

```python
from pandafuzz.models.bot_list_response import BotListResponse

# TODO update the JSON string below
json = "{}"
# create an instance of BotListResponse from a JSON string
bot_list_response_instance = BotListResponse.from_json(json)
# print the JSON string representation of the object
print(BotListResponse.to_json())

# convert the object into a dict
bot_list_response_dict = bot_list_response_instance.to_dict()
# create an instance of BotListResponse from a dict
bot_list_response_from_dict = BotListResponse.from_dict(bot_list_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


