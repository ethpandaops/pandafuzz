# BotHeartbeatRequestResourceUsage

Current resource usage statistics

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**cpu_percent** | **float** | CPU usage percentage | [optional] 
**memory_bytes** | **int** | Memory usage in bytes | [optional] 
**disk_usage_bytes** | **int** | Disk usage in bytes | [optional] 

## Example

```python
from pandafuzz.models.bot_heartbeat_request_resource_usage import BotHeartbeatRequestResourceUsage

# TODO update the JSON string below
json = "{}"
# create an instance of BotHeartbeatRequestResourceUsage from a JSON string
bot_heartbeat_request_resource_usage_instance = BotHeartbeatRequestResourceUsage.from_json(json)
# print the JSON string representation of the object
print(BotHeartbeatRequestResourceUsage.to_json())

# convert the object into a dict
bot_heartbeat_request_resource_usage_dict = bot_heartbeat_request_resource_usage_instance.to_dict()
# create an instance of BotHeartbeatRequestResourceUsage from a dict
bot_heartbeat_request_resource_usage_from_dict = BotHeartbeatRequestResourceUsage.from_dict(bot_heartbeat_request_resource_usage_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


