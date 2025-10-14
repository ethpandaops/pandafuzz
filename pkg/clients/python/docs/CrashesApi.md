# pandafuzz.CrashesApi

All URIs are relative to *http://localhost:8080/api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**deduplicate_crash**](CrashesApi.md#deduplicate_crash) | **POST** /crashes/{crashId}/deduplicate | Deduplicate crash
[**get_crash**](CrashesApi.md#get_crash) | **GET** /crashes/{crashId} | Get crash details
[**list_crashes**](CrashesApi.md#list_crashes) | **GET** /crashes | List crashes
[**minimize_crash**](CrashesApi.md#minimize_crash) | **POST** /crashes/{crashId}/minimize | Minimize crash input
[**reproduce_crash**](CrashesApi.md#reproduce_crash) | **POST** /crashes/{crashId}/reproduce | Reproduce crash


# **deduplicate_crash**
> CrashDeduplicationResponse deduplicate_crash(crash_id, deduplicate_crash_request=deduplicate_crash_request)

Deduplicate crash

Find and group similar crashes using various deduplication algorithms

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.crash_deduplication_response import CrashDeduplicationResponse
from pandafuzz.models.deduplicate_crash_request import DeduplicateCrashRequest
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
    api_instance = pandafuzz.CrashesApi(api_client)
    crash_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a crash
    deduplicate_crash_request = pandafuzz.DeduplicateCrashRequest() # DeduplicateCrashRequest |  (optional)

    try:
        # Deduplicate crash
        api_response = api_instance.deduplicate_crash(crash_id, deduplicate_crash_request=deduplicate_crash_request)
        print("The response of CrashesApi->deduplicate_crash:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CrashesApi->deduplicate_crash: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **crash_id** | **str**| Unique identifier for a crash | 
 **deduplicate_crash_request** | [**DeduplicateCrashRequest**](DeduplicateCrashRequest.md)|  | [optional] 

### Return type

[**CrashDeduplicationResponse**](CrashDeduplicationResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Deduplication completed |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_crash**
> Crash get_crash(crash_id, fields=fields)

Get crash details

Retrieve detailed information about a specific crash

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.crash import Crash
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
    api_instance = pandafuzz.CrashesApi(api_client)
    crash_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a crash
    fields = 'id,name,status,created_at' # str | Comma-separated list of fields to include in response (sparse fieldsets) (optional)

    try:
        # Get crash details
        api_response = api_instance.get_crash(crash_id, fields=fields)
        print("The response of CrashesApi->get_crash:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CrashesApi->get_crash: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **crash_id** | **str**| Unique identifier for a crash | 
 **fields** | **str**| Comma-separated list of fields to include in response (sparse fieldsets) | [optional] 

### Return type

[**Crash**](Crash.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved crash details |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **list_crashes**
> CrashListResponse list_crashes(limit=limit, offset=offset, cursor=cursor, fields=fields, campaign_id=campaign_id, job_id=job_id, unique_only=unique_only, crash_type=crash_type, severity=severity, sort=sort)

List crashes

Retrieve a paginated list of crashes with optional filtering

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.crash_list_response import CrashListResponse
from pandafuzz.models.crash_severity import CrashSeverity
from pandafuzz.models.crash_type import CrashType
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
    api_instance = pandafuzz.CrashesApi(api_client)
    limit = 50 # int | Maximum number of items to return (optional) (default to 50)
    offset = 0 # int | Number of items to skip (for offset-based pagination) (optional) (default to 0)
    cursor = 'eyJpZCI6IjEyMzQ1IiwidGltZXN0YW1wIjoiMjAyNC0wMS0xNVQxMDozMDowMFoifQ==' # str | Cursor for cursor-based pagination (optional)
    fields = 'id,name,status,created_at' # str | Comma-separated list of fields to include in response (sparse fieldsets) (optional)
    campaign_id = 'campaign_id_example' # str | Filter by campaign ID (optional)
    job_id = 'job_id_example' # str | Filter by job ID (optional)
    unique_only = False # bool | Only return unique crashes (optional) (default to False)
    crash_type = pandafuzz.CrashType() # CrashType | Filter by crash type (optional)
    severity = pandafuzz.CrashSeverity() # CrashSeverity | Filter by crash severity (optional)
    sort = 'created_at:desc,name:asc' # str | Sort specification (field:direction,field:direction) (optional)

    try:
        # List crashes
        api_response = api_instance.list_crashes(limit=limit, offset=offset, cursor=cursor, fields=fields, campaign_id=campaign_id, job_id=job_id, unique_only=unique_only, crash_type=crash_type, severity=severity, sort=sort)
        print("The response of CrashesApi->list_crashes:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CrashesApi->list_crashes: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **limit** | **int**| Maximum number of items to return | [optional] [default to 50]
 **offset** | **int**| Number of items to skip (for offset-based pagination) | [optional] [default to 0]
 **cursor** | **str**| Cursor for cursor-based pagination | [optional] 
 **fields** | **str**| Comma-separated list of fields to include in response (sparse fieldsets) | [optional] 
 **campaign_id** | **str**| Filter by campaign ID | [optional] 
 **job_id** | **str**| Filter by job ID | [optional] 
 **unique_only** | **bool**| Only return unique crashes | [optional] [default to False]
 **crash_type** | [**CrashType**](.md)| Filter by crash type | [optional] 
 **severity** | [**CrashSeverity**](.md)| Filter by crash severity | [optional] 
 **sort** | **str**| Sort specification (field:direction,field:direction) | [optional] 

### Return type

[**CrashListResponse**](CrashListResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved crashes |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **minimize_crash**
> MinimizeCrash202Response minimize_crash(crash_id, minimize_crash_request=minimize_crash_request)

Minimize crash input

Create a job to minimize the crash-triggering input

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.minimize_crash202_response import MinimizeCrash202Response
from pandafuzz.models.minimize_crash_request import MinimizeCrashRequest
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
    api_instance = pandafuzz.CrashesApi(api_client)
    crash_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a crash
    minimize_crash_request = pandafuzz.MinimizeCrashRequest() # MinimizeCrashRequest |  (optional)

    try:
        # Minimize crash input
        api_response = api_instance.minimize_crash(crash_id, minimize_crash_request=minimize_crash_request)
        print("The response of CrashesApi->minimize_crash:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CrashesApi->minimize_crash: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **crash_id** | **str**| Unique identifier for a crash | 
 **minimize_crash_request** | [**MinimizeCrashRequest**](MinimizeCrashRequest.md)|  | [optional] 

### Return type

[**MinimizeCrash202Response**](MinimizeCrash202Response.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**202** | Minimization job created |  -  |
**404** | Not Found - Resource does not exist |  -  |
**409** | Crash already being minimized |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **reproduce_crash**
> ReproduceCrash202Response reproduce_crash(crash_id, reproduce_crash_request=reproduce_crash_request)

Reproduce crash

Create a job to reproduce and verify the crash

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.reproduce_crash202_response import ReproduceCrash202Response
from pandafuzz.models.reproduce_crash_request import ReproduceCrashRequest
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
    api_instance = pandafuzz.CrashesApi(api_client)
    crash_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a crash
    reproduce_crash_request = pandafuzz.ReproduceCrashRequest() # ReproduceCrashRequest |  (optional)

    try:
        # Reproduce crash
        api_response = api_instance.reproduce_crash(crash_id, reproduce_crash_request=reproduce_crash_request)
        print("The response of CrashesApi->reproduce_crash:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CrashesApi->reproduce_crash: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **crash_id** | **str**| Unique identifier for a crash | 
 **reproduce_crash_request** | [**ReproduceCrashRequest**](ReproduceCrashRequest.md)|  | [optional] 

### Return type

[**ReproduceCrash202Response**](ReproduceCrash202Response.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**202** | Reproduction job created |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

