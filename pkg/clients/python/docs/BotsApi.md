# pandafuzz.BotsApi

All URIs are relative to *http://localhost:8080/api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**create_bot**](BotsApi.md#create_bot) | **POST** /bots | Register a new bot
[**delete_bot**](BotsApi.md#delete_bot) | **DELETE** /bots/{botId} | Unregister bot
[**get_bot**](BotsApi.md#get_bot) | **GET** /bots/{botId} | Get bot details
[**get_bot_jobs**](BotsApi.md#get_bot_jobs) | **GET** /bots/{botId}/jobs | Get bot jobs
[**list_bots**](BotsApi.md#list_bots) | **GET** /bots | List bots
[**send_bot_heartbeat**](BotsApi.md#send_bot_heartbeat) | **POST** /bots/{botId}/heartbeat | Send bot heartbeat
[**update_bot**](BotsApi.md#update_bot) | **PUT** /bots/{botId} | Update bot


# **create_bot**
> Bot create_bot(bot_create_request)

Register a new bot

Register a new bot agent with the system

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.bot import Bot
from pandafuzz.models.bot_create_request import BotCreateRequest
from pandafuzz.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080/api/v1
# See configuration.py for a list of all supported configuration parameters.
configuration = pandafuzz.Configuration(
    host = "http://localhost:8080/api/v1"
)

# The client must configure the authentication and authorization parameters
# in accordance with the API server security policy.
# Examples for each auth method are provided below, use the example that
# satisfies your auth use case.

# Configure API key authorization: apiKeyAuth
configuration.api_key['apiKeyAuth'] = os.environ["API_KEY"]

# Uncomment below to setup prefix (e.g. Bearer) for API key, if needed
# configuration.api_key_prefix['apiKeyAuth'] = 'Bearer'

# Configure Bearer authorization (JWT): bearerAuth
configuration = pandafuzz.Configuration(
    access_token = os.environ["BEARER_TOKEN"]
)

# Enter a context with an instance of the API client
with pandafuzz.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = pandafuzz.BotsApi(api_client)
    bot_create_request = {"name":"fuzzer-bot-02","hostname":"worker-002.internal","capabilities":["fuzzing","analysis"],"api_endpoint":"http://worker-002.internal:8081","metadata":{"region":"us-west-2","instance_type":"c5.xlarge"}} # BotCreateRequest | 

    try:
        # Register a new bot
        api_response = api_instance.create_bot(bot_create_request)
        print("The response of BotsApi->create_bot:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling BotsApi->create_bot: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **bot_create_request** | [**BotCreateRequest**](BotCreateRequest.md)|  | 

### Return type

[**Bot**](Bot.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**201** | Bot successfully registered |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**409** | Conflict - Resource already exists or state conflict |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **delete_bot**
> delete_bot(bot_id)

Unregister bot

Remove a bot from the system

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080/api/v1
# See configuration.py for a list of all supported configuration parameters.
configuration = pandafuzz.Configuration(
    host = "http://localhost:8080/api/v1"
)

# The client must configure the authentication and authorization parameters
# in accordance with the API server security policy.
# Examples for each auth method are provided below, use the example that
# satisfies your auth use case.

# Configure API key authorization: apiKeyAuth
configuration.api_key['apiKeyAuth'] = os.environ["API_KEY"]

# Uncomment below to setup prefix (e.g. Bearer) for API key, if needed
# configuration.api_key_prefix['apiKeyAuth'] = 'Bearer'

# Configure Bearer authorization (JWT): bearerAuth
configuration = pandafuzz.Configuration(
    access_token = os.environ["BEARER_TOKEN"]
)

# Enter a context with an instance of the API client
with pandafuzz.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = pandafuzz.BotsApi(api_client)
    bot_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a bot

    try:
        # Unregister bot
        api_instance.delete_bot(bot_id)
    except Exception as e:
        print("Exception when calling BotsApi->delete_bot: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **bot_id** | **str**| Unique identifier for a bot | 

### Return type

void (empty response body)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**204** | Bot successfully unregistered |  -  |
**404** | Not Found - Resource does not exist |  -  |
**409** | Cannot delete bot with active jobs |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_bot**
> Bot get_bot(bot_id, fields=fields)

Get bot details

Retrieve detailed information about a specific bot

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.bot import Bot
from pandafuzz.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080/api/v1
# See configuration.py for a list of all supported configuration parameters.
configuration = pandafuzz.Configuration(
    host = "http://localhost:8080/api/v1"
)

# The client must configure the authentication and authorization parameters
# in accordance with the API server security policy.
# Examples for each auth method are provided below, use the example that
# satisfies your auth use case.

# Configure API key authorization: apiKeyAuth
configuration.api_key['apiKeyAuth'] = os.environ["API_KEY"]

# Uncomment below to setup prefix (e.g. Bearer) for API key, if needed
# configuration.api_key_prefix['apiKeyAuth'] = 'Bearer'

# Configure Bearer authorization (JWT): bearerAuth
configuration = pandafuzz.Configuration(
    access_token = os.environ["BEARER_TOKEN"]
)

# Enter a context with an instance of the API client
with pandafuzz.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = pandafuzz.BotsApi(api_client)
    bot_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a bot
    fields = 'id,name,status,created_at' # str | Comma-separated list of fields to include in response (sparse fieldsets) (optional)

    try:
        # Get bot details
        api_response = api_instance.get_bot(bot_id, fields=fields)
        print("The response of BotsApi->get_bot:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling BotsApi->get_bot: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **bot_id** | **str**| Unique identifier for a bot | 
 **fields** | **str**| Comma-separated list of fields to include in response (sparse fieldsets) | [optional] 

### Return type

[**Bot**](Bot.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved bot details |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_bot_jobs**
> JobListResponse get_bot_jobs(bot_id, limit=limit, offset=offset, status=status)

Get bot jobs

Retrieve jobs assigned to or executed by this bot

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.job_list_response import JobListResponse
from pandafuzz.models.job_status import JobStatus
from pandafuzz.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080/api/v1
# See configuration.py for a list of all supported configuration parameters.
configuration = pandafuzz.Configuration(
    host = "http://localhost:8080/api/v1"
)

# The client must configure the authentication and authorization parameters
# in accordance with the API server security policy.
# Examples for each auth method are provided below, use the example that
# satisfies your auth use case.

# Configure API key authorization: apiKeyAuth
configuration.api_key['apiKeyAuth'] = os.environ["API_KEY"]

# Uncomment below to setup prefix (e.g. Bearer) for API key, if needed
# configuration.api_key_prefix['apiKeyAuth'] = 'Bearer'

# Configure Bearer authorization (JWT): bearerAuth
configuration = pandafuzz.Configuration(
    access_token = os.environ["BEARER_TOKEN"]
)

# Enter a context with an instance of the API client
with pandafuzz.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = pandafuzz.BotsApi(api_client)
    bot_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a bot
    limit = 50 # int | Maximum number of items to return (optional) (default to 50)
    offset = 0 # int | Number of items to skip (for offset-based pagination) (optional) (default to 0)
    status = pandafuzz.JobStatus() # JobStatus | Filter by job status (optional)

    try:
        # Get bot jobs
        api_response = api_instance.get_bot_jobs(bot_id, limit=limit, offset=offset, status=status)
        print("The response of BotsApi->get_bot_jobs:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling BotsApi->get_bot_jobs: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **bot_id** | **str**| Unique identifier for a bot | 
 **limit** | **int**| Maximum number of items to return | [optional] [default to 50]
 **offset** | **int**| Number of items to skip (for offset-based pagination) | [optional] [default to 0]
 **status** | [**JobStatus**](.md)| Filter by job status | [optional] 

### Return type

[**JobListResponse**](JobListResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved bot jobs |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **list_bots**
> BotListResponse list_bots(limit=limit, offset=offset, cursor=cursor, fields=fields, status=status, capabilities=capabilities, online_only=online_only, sort=sort)

List bots

Retrieve a paginated list of registered bots with optional filtering

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.bot_list_response import BotListResponse
from pandafuzz.models.bot_status import BotStatus
from pandafuzz.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080/api/v1
# See configuration.py for a list of all supported configuration parameters.
configuration = pandafuzz.Configuration(
    host = "http://localhost:8080/api/v1"
)

# The client must configure the authentication and authorization parameters
# in accordance with the API server security policy.
# Examples for each auth method are provided below, use the example that
# satisfies your auth use case.

# Configure API key authorization: apiKeyAuth
configuration.api_key['apiKeyAuth'] = os.environ["API_KEY"]

# Uncomment below to setup prefix (e.g. Bearer) for API key, if needed
# configuration.api_key_prefix['apiKeyAuth'] = 'Bearer'

# Configure Bearer authorization (JWT): bearerAuth
configuration = pandafuzz.Configuration(
    access_token = os.environ["BEARER_TOKEN"]
)

# Enter a context with an instance of the API client
with pandafuzz.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = pandafuzz.BotsApi(api_client)
    limit = 50 # int | Maximum number of items to return (optional) (default to 50)
    offset = 0 # int | Number of items to skip (for offset-based pagination) (optional) (default to 0)
    cursor = 'eyJpZCI6IjEyMzQ1IiwidGltZXN0YW1wIjoiMjAyNC0wMS0xNVQxMDozMDowMFoifQ==' # str | Cursor for cursor-based pagination (optional)
    fields = 'id,name,status,created_at' # str | Comma-separated list of fields to include in response (sparse fieldsets) (optional)
    status = pandafuzz.BotStatus() # BotStatus | Filter by bot status (optional)
    capabilities = 'fuzzing,analysis' # str | Filter by bot capabilities (comma-separated) (optional)
    online_only = False # bool | Only return online bots (optional) (default to False)
    sort = 'created_at:desc,name:asc' # str | Sort specification (field:direction,field:direction) (optional)

    try:
        # List bots
        api_response = api_instance.list_bots(limit=limit, offset=offset, cursor=cursor, fields=fields, status=status, capabilities=capabilities, online_only=online_only, sort=sort)
        print("The response of BotsApi->list_bots:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling BotsApi->list_bots: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **limit** | **int**| Maximum number of items to return | [optional] [default to 50]
 **offset** | **int**| Number of items to skip (for offset-based pagination) | [optional] [default to 0]
 **cursor** | **str**| Cursor for cursor-based pagination | [optional] 
 **fields** | **str**| Comma-separated list of fields to include in response (sparse fieldsets) | [optional] 
 **status** | [**BotStatus**](.md)| Filter by bot status | [optional] 
 **capabilities** | **str**| Filter by bot capabilities (comma-separated) | [optional] 
 **online_only** | **bool**| Only return online bots | [optional] [default to False]
 **sort** | **str**| Sort specification (field:direction,field:direction) | [optional] 

### Return type

[**BotListResponse**](BotListResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved bots |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **send_bot_heartbeat**
> BotHeartbeatResponse send_bot_heartbeat(bot_id, bot_heartbeat_request)

Send bot heartbeat

Update bot status and last seen timestamp

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.bot_heartbeat_request import BotHeartbeatRequest
from pandafuzz.models.bot_heartbeat_response import BotHeartbeatResponse
from pandafuzz.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080/api/v1
# See configuration.py for a list of all supported configuration parameters.
configuration = pandafuzz.Configuration(
    host = "http://localhost:8080/api/v1"
)

# The client must configure the authentication and authorization parameters
# in accordance with the API server security policy.
# Examples for each auth method are provided below, use the example that
# satisfies your auth use case.

# Configure API key authorization: apiKeyAuth
configuration.api_key['apiKeyAuth'] = os.environ["API_KEY"]

# Uncomment below to setup prefix (e.g. Bearer) for API key, if needed
# configuration.api_key_prefix['apiKeyAuth'] = 'Bearer'

# Configure Bearer authorization (JWT): bearerAuth
configuration = pandafuzz.Configuration(
    access_token = os.environ["BEARER_TOKEN"]
)

# Enter a context with an instance of the API client
with pandafuzz.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = pandafuzz.BotsApi(api_client)
    bot_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a bot
    bot_heartbeat_request = {"status":"busy","current_job_id":"01234567-89ab-cdef-0123-456789abcdef","resource_usage":{"cpu_percent":75.5,"memory_bytes":2147483648,"disk_usage_bytes":10737418240}} # BotHeartbeatRequest | 

    try:
        # Send bot heartbeat
        api_response = api_instance.send_bot_heartbeat(bot_id, bot_heartbeat_request)
        print("The response of BotsApi->send_bot_heartbeat:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling BotsApi->send_bot_heartbeat: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **bot_id** | **str**| Unique identifier for a bot | 
 **bot_heartbeat_request** | [**BotHeartbeatRequest**](BotHeartbeatRequest.md)|  | 

### Return type

[**BotHeartbeatResponse**](BotHeartbeatResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Heartbeat acknowledged |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **update_bot**
> Bot update_bot(bot_id, bot_update_request)

Update bot

Update bot information and configuration

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.bot import Bot
from pandafuzz.models.bot_update_request import BotUpdateRequest
from pandafuzz.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080/api/v1
# See configuration.py for a list of all supported configuration parameters.
configuration = pandafuzz.Configuration(
    host = "http://localhost:8080/api/v1"
)

# The client must configure the authentication and authorization parameters
# in accordance with the API server security policy.
# Examples for each auth method are provided below, use the example that
# satisfies your auth use case.

# Configure API key authorization: apiKeyAuth
configuration.api_key['apiKeyAuth'] = os.environ["API_KEY"]

# Uncomment below to setup prefix (e.g. Bearer) for API key, if needed
# configuration.api_key_prefix['apiKeyAuth'] = 'Bearer'

# Configure Bearer authorization (JWT): bearerAuth
configuration = pandafuzz.Configuration(
    access_token = os.environ["BEARER_TOKEN"]
)

# Enter a context with an instance of the API client
with pandafuzz.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = pandafuzz.BotsApi(api_client)
    bot_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a bot
    bot_update_request = pandafuzz.BotUpdateRequest() # BotUpdateRequest | 

    try:
        # Update bot
        api_response = api_instance.update_bot(bot_id, bot_update_request)
        print("The response of BotsApi->update_bot:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling BotsApi->update_bot: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **bot_id** | **str**| Unique identifier for a bot | 
 **bot_update_request** | [**BotUpdateRequest**](BotUpdateRequest.md)|  | 

### Return type

[**Bot**](Bot.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Bot successfully updated |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

