# pandafuzz.CorpusApi

All URIs are relative to *http://localhost:8080/api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**delete_corpus_entry**](CorpusApi.md#delete_corpus_entry) | **DELETE** /corpus/{entryId} | Delete corpus entry
[**download_corpus_file**](CorpusApi.md#download_corpus_file) | **GET** /corpus/{entryId}/download | Download corpus file
[**get_corpus_entry**](CorpusApi.md#get_corpus_entry) | **GET** /corpus/{entryId} | Get corpus entry details
[**list_corpus**](CorpusApi.md#list_corpus) | **GET** /corpus | List corpus entries
[**list_quarantined_corpus**](CorpusApi.md#list_quarantined_corpus) | **GET** /corpus/quarantine | List quarantined corpus entries
[**select_corpus**](CorpusApi.md#select_corpus) | **POST** /corpus/selection | Select corpus subset
[**sync_corpus**](CorpusApi.md#sync_corpus) | **POST** /corpus/sync | Synchronize corpus
[**upload_corpus**](CorpusApi.md#upload_corpus) | **POST** /corpus | Upload corpus files


# **delete_corpus_entry**
> delete_corpus_entry(entry_id)

Delete corpus entry

Remove a corpus entry from the system

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
    api_instance = pandafuzz.CorpusApi(api_client)
    entry_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a corpus entry

    try:
        # Delete corpus entry
        api_instance.delete_corpus_entry(entry_id)
    except Exception as e:
        print("Exception when calling CorpusApi->delete_corpus_entry: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **entry_id** | **str**| Unique identifier for a corpus entry | 

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
**204** | Corpus entry successfully deleted |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **download_corpus_file**
> bytearray download_corpus_file(entry_id)

Download corpus file

Download the binary content of a corpus file

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
    api_instance = pandafuzz.CorpusApi(api_client)
    entry_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a corpus entry

    try:
        # Download corpus file
        api_response = api_instance.download_corpus_file(entry_id)
        print("The response of CorpusApi->download_corpus_file:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CorpusApi->download_corpus_file: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **entry_id** | **str**| Unique identifier for a corpus entry | 

### Return type

**bytearray**

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/octet-stream, application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Corpus file content |  * Content-Disposition - Filename for download <br>  * Content-Length - File size in bytes <br>  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_corpus_entry**
> CorpusEntry get_corpus_entry(entry_id, fields=fields)

Get corpus entry details

Retrieve detailed information about a specific corpus entry

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.corpus_entry import CorpusEntry
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
    api_instance = pandafuzz.CorpusApi(api_client)
    entry_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a corpus entry
    fields = 'id,name,status,created_at' # str | Comma-separated list of fields to include in response (sparse fieldsets) (optional)

    try:
        # Get corpus entry details
        api_response = api_instance.get_corpus_entry(entry_id, fields=fields)
        print("The response of CorpusApi->get_corpus_entry:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CorpusApi->get_corpus_entry: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **entry_id** | **str**| Unique identifier for a corpus entry | 
 **fields** | **str**| Comma-separated list of fields to include in response (sparse fieldsets) | [optional] 

### Return type

[**CorpusEntry**](CorpusEntry.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved corpus entry details |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **list_corpus**
> CorpusListResponse list_corpus(limit=limit, offset=offset, cursor=cursor, fields=fields, campaign_id=campaign_id, job_id=job_id, min_coverage=min_coverage, sort=sort)

List corpus entries

Retrieve a paginated list of corpus entries with optional filtering

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.corpus_list_response import CorpusListResponse
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
    api_instance = pandafuzz.CorpusApi(api_client)
    limit = 50 # int | Maximum number of items to return (optional) (default to 50)
    offset = 0 # int | Number of items to skip (for offset-based pagination) (optional) (default to 0)
    cursor = 'eyJpZCI6IjEyMzQ1IiwidGltZXN0YW1wIjoiMjAyNC0wMS0xNVQxMDozMDowMFoifQ==' # str | Cursor for cursor-based pagination (optional)
    fields = 'id,name,status,created_at' # str | Comma-separated list of fields to include in response (sparse fieldsets) (optional)
    campaign_id = 'campaign_id_example' # str | Filter by campaign ID (optional)
    job_id = 'job_id_example' # str | Filter by job ID (optional)
    min_coverage = 56 # int | Minimum coverage threshold (optional)
    sort = 'created_at:desc,name:asc' # str | Sort specification (field:direction,field:direction) (optional)

    try:
        # List corpus entries
        api_response = api_instance.list_corpus(limit=limit, offset=offset, cursor=cursor, fields=fields, campaign_id=campaign_id, job_id=job_id, min_coverage=min_coverage, sort=sort)
        print("The response of CorpusApi->list_corpus:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CorpusApi->list_corpus: %s\n" % e)
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
 **min_coverage** | **int**| Minimum coverage threshold | [optional] 
 **sort** | **str**| Sort specification (field:direction,field:direction) | [optional] 

### Return type

[**CorpusListResponse**](CorpusListResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved corpus entries |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **list_quarantined_corpus**
> CorpusListResponse list_quarantined_corpus(limit=limit, offset=offset, reason=reason)

List quarantined corpus entries

Retrieve entries that have been quarantined due to analysis results

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.corpus_list_response import CorpusListResponse
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
    api_instance = pandafuzz.CorpusApi(api_client)
    limit = 50 # int | Maximum number of items to return (optional) (default to 50)
    offset = 0 # int | Number of items to skip (for offset-based pagination) (optional) (default to 0)
    reason = 'reason_example' # str | Filter by quarantine reason (optional)

    try:
        # List quarantined corpus entries
        api_response = api_instance.list_quarantined_corpus(limit=limit, offset=offset, reason=reason)
        print("The response of CorpusApi->list_quarantined_corpus:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CorpusApi->list_quarantined_corpus: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **limit** | **int**| Maximum number of items to return | [optional] [default to 50]
 **offset** | **int**| Number of items to skip (for offset-based pagination) | [optional] [default to 0]
 **reason** | **str**| Filter by quarantine reason | [optional] 

### Return type

[**CorpusListResponse**](CorpusListResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved quarantined corpus entries |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **select_corpus**
> CorpusSelectionResponse select_corpus(corpus_selection_request)

Select corpus subset

Select an optimal subset of corpus for a specific purpose

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.corpus_selection_request import CorpusSelectionRequest
from pandafuzz.models.corpus_selection_response import CorpusSelectionResponse
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
    api_instance = pandafuzz.CorpusApi(api_client)
    corpus_selection_request = {"campaign_id":"01234567-89ab-cdef-0123-456789abcdef","selection_strategy":"coverage_maximizing","max_entries":100,"min_coverage":500,"criteria":{"prefer_small_files":true,"include_edge_cases":true}} # CorpusSelectionRequest | 

    try:
        # Select corpus subset
        api_response = api_instance.select_corpus(corpus_selection_request)
        print("The response of CorpusApi->select_corpus:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CorpusApi->select_corpus: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **corpus_selection_request** | [**CorpusSelectionRequest**](CorpusSelectionRequest.md)|  | 

### Return type

[**CorpusSelectionResponse**](CorpusSelectionResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Corpus selection completed |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **sync_corpus**
> CorpusSyncResponse sync_corpus(corpus_sync_request)

Synchronize corpus

Synchronize corpus between campaigns or jobs

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.corpus_sync_request import CorpusSyncRequest
from pandafuzz.models.corpus_sync_response import CorpusSyncResponse
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
    api_instance = pandafuzz.CorpusApi(api_client)
    corpus_sync_request = {"source_campaign_id":"01234567-89ab-cdef-0123-456789abcdef","target_campaign_id":"fedcba98-7654-3210-fedc-ba9876543210","sync_strategy":"coverage_based","min_coverage":1000,"max_files":500} # CorpusSyncRequest | 

    try:
        # Synchronize corpus
        api_response = api_instance.sync_corpus(corpus_sync_request)
        print("The response of CorpusApi->sync_corpus:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CorpusApi->sync_corpus: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **corpus_sync_request** | [**CorpusSyncRequest**](CorpusSyncRequest.md)|  | 

### Return type

[**CorpusSyncResponse**](CorpusSyncResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Corpus synchronization completed |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **upload_corpus**
> CorpusUploadResponse upload_corpus(campaign_id, files, job_id=job_id, tags=tags)

Upload corpus files

Upload multiple corpus files to a campaign

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.corpus_upload_response import CorpusUploadResponse
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
    api_instance = pandafuzz.CorpusApi(api_client)
    campaign_id = 'campaign_id_example' # str | Target campaign ID
    files = None # List[bytearray] | Corpus files to upload
    job_id = 'job_id_example' # str | Optional job ID for context (optional)
    tags = 'tags_example' # str | Comma-separated tags for the corpus files (optional)

    try:
        # Upload corpus files
        api_response = api_instance.upload_corpus(campaign_id, files, job_id=job_id, tags=tags)
        print("The response of CorpusApi->upload_corpus:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling CorpusApi->upload_corpus: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **campaign_id** | **str**| Target campaign ID | 
 **files** | **List[bytearray]**| Corpus files to upload | 
 **job_id** | **str**| Optional job ID for context | [optional] 
 **tags** | **str**| Comma-separated tags for the corpus files | [optional] 

### Return type

[**CorpusUploadResponse**](CorpusUploadResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: multipart/form-data
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**201** | Corpus files successfully uploaded |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**413** | Payload Too Large - Request payload exceeds size limits |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

