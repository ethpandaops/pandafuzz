# pandafuzz.CampaignsApi

All URIs are relative to *http://localhost:8080/api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**create_campaign**](CampaignsApi.md#create_campaign) | **POST** /campaigns | Create a new campaign
[**delete_campaign**](CampaignsApi.md#delete_campaign) | **DELETE** /campaigns/{campaignId} | Delete campaign
[**get_campaign**](CampaignsApi.md#get_campaign) | **GET** /campaigns/{campaignId} | Get campaign details
[**get_campaign_stats**](CampaignsApi.md#get_campaign_stats) | **GET** /campaigns/{campaignId}/stats | Get campaign statistics
[**list_campaigns**](CampaignsApi.md#list_campaigns) | **GET** /campaigns | List campaigns
[**start_campaign**](CampaignsApi.md#start_campaign) | **POST** /campaigns/{campaignId}/start | Start campaign
[**stop_campaign**](CampaignsApi.md#stop_campaign) | **POST** /campaigns/{campaignId}/stop | Stop campaign
[**update_campaign**](CampaignsApi.md#update_campaign) | **PUT** /campaigns/{campaignId} | Update campaign


# **create_campaign**
> Campaign create_campaign(campaign_create_request)

Create a new campaign

Create a new fuzzing campaign

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.campaign import Campaign
from pandafuzz.models.campaign_create_request import CampaignCreateRequest
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
    api_instance = pandafuzz.CampaignsApi(api_client)
    campaign_create_request = {"name":"HTTP Parser Security Test","description":"Comprehensive security testing of HTTP parser implementation","target_binary":"/usr/bin/http-parser","tags":["security","parser","http"],"max_duration_seconds":86400,"job_template":{"fuzzer":"libfuzzer","timeout_seconds":3600,"config":{"max_total_time":3600,"max_len":4096}}} # CampaignCreateRequest | 

    try:
        # Create a new campaign
        api_response = api_instance.create_campaign(campaign_create_request)
        print("The response of CampaignsApi->create_campaign:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CampaignsApi->create_campaign: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **campaign_create_request** | [**CampaignCreateRequest**](CampaignCreateRequest.md)|  | 

### Return type

[**Campaign**](Campaign.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**201** | Campaign successfully created |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **delete_campaign**
> delete_campaign(campaign_id)

Delete campaign

Delete a campaign and all associated data

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
    api_instance = pandafuzz.CampaignsApi(api_client)
    campaign_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a campaign

    try:
        # Delete campaign
        api_instance.delete_campaign(campaign_id)
    except Exception as e:
        print("Exception when calling CampaignsApi->delete_campaign: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **campaign_id** | **str**| Unique identifier for a campaign | 

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
**204** | Campaign successfully deleted |  -  |
**404** | Not Found - Resource does not exist |  -  |
**409** | Cannot delete campaign with active jobs |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_campaign**
> Campaign get_campaign(campaign_id, fields=fields)

Get campaign details

Retrieve detailed information about a specific campaign

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.campaign import Campaign
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
    api_instance = pandafuzz.CampaignsApi(api_client)
    campaign_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a campaign
    fields = 'id,name,status,created_at' # str | Comma-separated list of fields to include in response (sparse fieldsets) (optional)

    try:
        # Get campaign details
        api_response = api_instance.get_campaign(campaign_id, fields=fields)
        print("The response of CampaignsApi->get_campaign:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CampaignsApi->get_campaign: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **campaign_id** | **str**| Unique identifier for a campaign | 
 **fields** | **str**| Comma-separated list of fields to include in response (sparse fieldsets) | [optional] 

### Return type

[**Campaign**](Campaign.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved campaign details |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_campaign_stats**
> CampaignStats get_campaign_stats(campaign_id)

Get campaign statistics

Retrieve comprehensive statistics for a campaign

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.campaign_stats import CampaignStats
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
    api_instance = pandafuzz.CampaignsApi(api_client)
    campaign_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a campaign

    try:
        # Get campaign statistics
        api_response = api_instance.get_campaign_stats(campaign_id)
        print("The response of CampaignsApi->get_campaign_stats:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CampaignsApi->get_campaign_stats: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **campaign_id** | **str**| Unique identifier for a campaign | 

### Return type

[**CampaignStats**](CampaignStats.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved campaign statistics |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **list_campaigns**
> CampaignListResponse list_campaigns(limit=limit, offset=offset, cursor=cursor, fields=fields, status=status, tags=tags, sort=sort)

List campaigns

Retrieve a paginated list of campaigns with optional filtering

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.campaign_list_response import CampaignListResponse
from pandafuzz.models.campaign_status import CampaignStatus
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
    api_instance = pandafuzz.CampaignsApi(api_client)
    limit = 50 # int | Maximum number of items to return (optional) (default to 50)
    offset = 0 # int | Number of items to skip (for offset-based pagination) (optional) (default to 0)
    cursor = 'eyJpZCI6IjEyMzQ1IiwidGltZXN0YW1wIjoiMjAyNC0wMS0xNVQxMDozMDowMFoifQ==' # str | Cursor for cursor-based pagination (optional)
    fields = 'id,name,status,created_at' # str | Comma-separated list of fields to include in response (sparse fieldsets) (optional)
    status = pandafuzz.CampaignStatus() # CampaignStatus | Filter by campaign status (optional)
    tags = 'security,performance' # str | Filter by tags (comma-separated) (optional)
    sort = 'created_at:desc,name:asc' # str | Sort specification (field:direction,field:direction) (optional)

    try:
        # List campaigns
        api_response = api_instance.list_campaigns(limit=limit, offset=offset, cursor=cursor, fields=fields, status=status, tags=tags, sort=sort)
        print("The response of CampaignsApi->list_campaigns:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CampaignsApi->list_campaigns: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **limit** | **int**| Maximum number of items to return | [optional] [default to 50]
 **offset** | **int**| Number of items to skip (for offset-based pagination) | [optional] [default to 0]
 **cursor** | **str**| Cursor for cursor-based pagination | [optional] 
 **fields** | **str**| Comma-separated list of fields to include in response (sparse fieldsets) | [optional] 
 **status** | [**CampaignStatus**](.md)| Filter by campaign status | [optional] 
 **tags** | **str**| Filter by tags (comma-separated) | [optional] 
 **sort** | **str**| Sort specification (field:direction,field:direction) | [optional] 

### Return type

[**CampaignListResponse**](CampaignListResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved campaigns |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **start_campaign**
> Campaign start_campaign(campaign_id)

Start campaign

Start or resume a campaign

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.campaign import Campaign
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
    api_instance = pandafuzz.CampaignsApi(api_client)
    campaign_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a campaign

    try:
        # Start campaign
        api_response = api_instance.start_campaign(campaign_id)
        print("The response of CampaignsApi->start_campaign:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CampaignsApi->start_campaign: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **campaign_id** | **str**| Unique identifier for a campaign | 

### Return type

[**Campaign**](Campaign.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Campaign successfully started |  -  |
**404** | Not Found - Resource does not exist |  -  |
**409** | Campaign already running |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **stop_campaign**
> Campaign stop_campaign(campaign_id, stop_campaign_request=stop_campaign_request)

Stop campaign

Stop or pause a running campaign

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.campaign import Campaign
from pandafuzz.models.stop_campaign_request import StopCampaignRequest
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
    api_instance = pandafuzz.CampaignsApi(api_client)
    campaign_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a campaign
    stop_campaign_request = pandafuzz.StopCampaignRequest() # StopCampaignRequest |  (optional)

    try:
        # Stop campaign
        api_response = api_instance.stop_campaign(campaign_id, stop_campaign_request=stop_campaign_request)
        print("The response of CampaignsApi->stop_campaign:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CampaignsApi->stop_campaign: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **campaign_id** | **str**| Unique identifier for a campaign | 
 **stop_campaign_request** | [**StopCampaignRequest**](StopCampaignRequest.md)|  | [optional] 

### Return type

[**Campaign**](Campaign.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Campaign successfully stopped |  -  |
**404** | Not Found - Resource does not exist |  -  |
**409** | Campaign not running |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **update_campaign**
> Campaign update_campaign(campaign_id, campaign_update_request)

Update campaign

Update campaign configuration and settings

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.campaign import Campaign
from pandafuzz.models.campaign_update_request import CampaignUpdateRequest
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
    api_instance = pandafuzz.CampaignsApi(api_client)
    campaign_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a campaign
    campaign_update_request = pandafuzz.CampaignUpdateRequest() # CampaignUpdateRequest | 

    try:
        # Update campaign
        api_response = api_instance.update_campaign(campaign_id, campaign_update_request)
        print("The response of CampaignsApi->update_campaign:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CampaignsApi->update_campaign: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **campaign_id** | **str**| Unique identifier for a campaign | 
 **campaign_update_request** | [**CampaignUpdateRequest**](CampaignUpdateRequest.md)|  | 

### Return type

[**Campaign**](Campaign.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Campaign successfully updated |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

