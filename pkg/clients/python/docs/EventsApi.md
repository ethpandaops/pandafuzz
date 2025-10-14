# pandafuzz.EventsApi

All URIs are relative to *http://localhost:8080/api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**get_event_stream**](EventsApi.md#get_event_stream) | **GET** /events | Real-time event stream


# **get_event_stream**
> str get_event_stream(types=types, campaign_id=campaign_id, bot_id=bot_id)

Real-time event stream

Subscribe to real-time system events via Server-Sent Events

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
    api_instance = pandafuzz.EventsApi(api_client)
    types = 'job.started,job.completed,crash.detected' # str | Event types to subscribe to (comma-separated) (optional)
    campaign_id = 'campaign_id_example' # str | Filter events by campaign ID (optional)
    bot_id = 'bot_id_example' # str | Filter events by bot ID (optional)

    try:
        # Real-time event stream
        api_response = api_instance.get_event_stream(types=types, campaign_id=campaign_id, bot_id=bot_id)
        print("The response of EventsApi->get_event_stream:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling EventsApi->get_event_stream: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **types** | **str**| Event types to subscribe to (comma-separated) | [optional] 
 **campaign_id** | **str**| Filter events by campaign ID | [optional] 
 **bot_id** | **str**| Filter events by bot ID | [optional] 

### Return type

**str**

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: text/event-stream, application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Event stream established |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

