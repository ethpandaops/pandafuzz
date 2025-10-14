# pandafuzz.JobsApi

All URIs are relative to *http://localhost:8080/api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**create_job**](JobsApi.md#create_job) | **POST** /jobs | Create a new job
[**delete_job**](JobsApi.md#delete_job) | **DELETE** /jobs/{jobId} | Cancel job
[**download_coverage_report**](JobsApi.md#download_coverage_report) | **GET** /jobs/{jobId}/coverage/{reportId} | Download coverage report
[**get_job**](JobsApi.md#get_job) | **GET** /jobs/{jobId} | Get job details
[**get_job_artifacts**](JobsApi.md#get_job_artifacts) | **GET** /jobs/{jobId}/artifacts | List job artifacts
[**get_job_coverage**](JobsApi.md#get_job_coverage) | **GET** /jobs/{jobId}/coverage | Get job coverage reports
[**get_job_logs**](JobsApi.md#get_job_logs) | **GET** /jobs/{jobId}/logs | Get job logs
[**list_jobs**](JobsApi.md#list_jobs) | **GET** /jobs | List jobs
[**update_job**](JobsApi.md#update_job) | **PUT** /jobs/{jobId} | Update job


# **create_job**
> Job create_job(job_create_request)

Create a new job

Submit a new fuzzing job to the system

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.job import Job
from pandafuzz.models.job_create_request import JobCreateRequest
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
    api_instance = pandafuzz.JobsApi(api_client)
    job_create_request = {"name":"libfuzzer-target-test","fuzzer":"libfuzzer","target_binary":"/path/to/target","campaign_id":"01234567-89ab-cdef-0123-456789abcdef","timeout_seconds":3600,"config":{"max_total_time":3600,"max_len":1024,"use_value_profile":true},"enable_coverage":true} # JobCreateRequest | 

    try:
        # Create a new job
        api_response = api_instance.create_job(job_create_request)
        print("The response of JobsApi->create_job:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling JobsApi->create_job: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **job_create_request** | [**JobCreateRequest**](JobCreateRequest.md)|  | 

### Return type

[**Job**](Job.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**201** | Job successfully created |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **delete_job**
> delete_job(job_id)

Cancel job

Cancel a pending or running job

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
    api_instance = pandafuzz.JobsApi(api_client)
    job_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a job

    try:
        # Cancel job
        api_instance.delete_job(job_id)
    except Exception as e:
        print("Exception when calling JobsApi->delete_job: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **job_id** | **str**| Unique identifier for a job | 

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
**204** | Job successfully cancelled |  -  |
**404** | Not Found - Resource does not exist |  -  |
**409** | Cannot cancel completed job |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **download_coverage_report**
> object download_coverage_report(job_id, report_id)

Download coverage report

Download a specific coverage report file

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
    api_instance = pandafuzz.JobsApi(api_client)
    job_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a job
    report_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a coverage report

    try:
        # Download coverage report
        api_response = api_instance.download_coverage_report(job_id, report_id)
        print("The response of JobsApi->download_coverage_report:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling JobsApi->download_coverage_report: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **job_id** | **str**| Unique identifier for a job | 
 **report_id** | **str**| Unique identifier for a coverage report | 

### Return type

**object**

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, text/html, text/plain, application/octet-stream

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Coverage report file |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_job**
> Job get_job(job_id, fields=fields)

Get job details

Retrieve detailed information about a specific job

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.job import Job
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
    api_instance = pandafuzz.JobsApi(api_client)
    job_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a job
    fields = 'id,name,status,created_at' # str | Comma-separated list of fields to include in response (sparse fieldsets) (optional)

    try:
        # Get job details
        api_response = api_instance.get_job(job_id, fields=fields)
        print("The response of JobsApi->get_job:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling JobsApi->get_job: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **job_id** | **str**| Unique identifier for a job | 
 **fields** | **str**| Comma-separated list of fields to include in response (sparse fieldsets) | [optional] 

### Return type

[**Job**](Job.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved job details |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_job_artifacts**
> ArtifactListResponse get_job_artifacts(job_id, limit=limit, offset=offset, type=type)

List job artifacts

Retrieve artifacts generated by a job (crashes, corpus files, etc.)

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.artifact_list_response import ArtifactListResponse
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
    api_instance = pandafuzz.JobsApi(api_client)
    job_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a job
    limit = 50 # int | Maximum number of items to return (optional) (default to 50)
    offset = 0 # int | Number of items to skip (for offset-based pagination) (optional) (default to 0)
    type = 'type_example' # str | Filter by artifact type (optional)

    try:
        # List job artifacts
        api_response = api_instance.get_job_artifacts(job_id, limit=limit, offset=offset, type=type)
        print("The response of JobsApi->get_job_artifacts:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling JobsApi->get_job_artifacts: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **job_id** | **str**| Unique identifier for a job | 
 **limit** | **int**| Maximum number of items to return | [optional] [default to 50]
 **offset** | **int**| Number of items to skip (for offset-based pagination) | [optional] [default to 0]
 **type** | **str**| Filter by artifact type | [optional] 

### Return type

[**ArtifactListResponse**](ArtifactListResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved job artifacts |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_job_coverage**
> CoverageReportListResponse get_job_coverage(job_id, limit=limit, offset=offset, format=format)

Get job coverage reports

Retrieve coverage reports generated by a job

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.coverage_format import CoverageFormat
from pandafuzz.models.coverage_report_list_response import CoverageReportListResponse
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
    api_instance = pandafuzz.JobsApi(api_client)
    job_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a job
    limit = 50 # int | Maximum number of items to return (optional) (default to 50)
    offset = 0 # int | Number of items to skip (for offset-based pagination) (optional) (default to 0)
    format = pandafuzz.CoverageFormat() # CoverageFormat | Filter by coverage report format (optional)

    try:
        # Get job coverage reports
        api_response = api_instance.get_job_coverage(job_id, limit=limit, offset=offset, format=format)
        print("The response of JobsApi->get_job_coverage:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling JobsApi->get_job_coverage: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **job_id** | **str**| Unique identifier for a job | 
 **limit** | **int**| Maximum number of items to return | [optional] [default to 50]
 **offset** | **int**| Number of items to skip (for offset-based pagination) | [optional] [default to 0]
 **format** | [**CoverageFormat**](.md)| Filter by coverage report format | [optional] 

### Return type

[**CoverageReportListResponse**](CoverageReportListResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved coverage reports |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_job_logs**
> JobLogsResponse get_job_logs(job_id, follow=follow, tail=tail, since=since)

Get job logs

Retrieve execution logs for a job

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.job_logs_response import JobLogsResponse
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
    api_instance = pandafuzz.JobsApi(api_client)
    job_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a job
    follow = False # bool | Stream logs in real-time (Server-Sent Events) (optional) (default to False)
    tail = 1000 # int | Number of lines to return from the end (optional) (default to 1000)
    since = '2013-10-20T19:20:30+01:00' # datetime | Only return logs after this timestamp (optional)

    try:
        # Get job logs
        api_response = api_instance.get_job_logs(job_id, follow=follow, tail=tail, since=since)
        print("The response of JobsApi->get_job_logs:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling JobsApi->get_job_logs: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **job_id** | **str**| Unique identifier for a job | 
 **follow** | **bool**| Stream logs in real-time (Server-Sent Events) | [optional] [default to False]
 **tail** | **int**| Number of lines to return from the end | [optional] [default to 1000]
 **since** | **datetime**| Only return logs after this timestamp | [optional] 

### Return type

[**JobLogsResponse**](JobLogsResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, text/event-stream

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved job logs |  -  |
**404** | Not Found - Resource does not exist |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **list_jobs**
> JobListResponse list_jobs(limit=limit, offset=offset, cursor=cursor, fields=fields, status=status, campaign_id=campaign_id, bot_id=bot_id, fuzzer=fuzzer, sort=sort)

List jobs

Retrieve a paginated list of jobs with optional filtering

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.fuzzer_type import FuzzerType
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
    api_instance = pandafuzz.JobsApi(api_client)
    limit = 50 # int | Maximum number of items to return (optional) (default to 50)
    offset = 0 # int | Number of items to skip (for offset-based pagination) (optional) (default to 0)
    cursor = 'eyJpZCI6IjEyMzQ1IiwidGltZXN0YW1wIjoiMjAyNC0wMS0xNVQxMDozMDowMFoifQ==' # str | Cursor for cursor-based pagination (optional)
    fields = 'id,name,status,created_at' # str | Comma-separated list of fields to include in response (sparse fieldsets) (optional)
    status = pandafuzz.JobStatus() # JobStatus | Filter by job status (optional)
    campaign_id = 'campaign_id_example' # str | Filter by campaign ID (optional)
    bot_id = 'bot_id_example' # str | Filter by assigned bot ID (optional)
    fuzzer = pandafuzz.FuzzerType() # FuzzerType | Filter by fuzzer type (optional)
    sort = 'created_at:desc,name:asc' # str | Sort specification (field:direction,field:direction) (optional)

    try:
        # List jobs
        api_response = api_instance.list_jobs(limit=limit, offset=offset, cursor=cursor, fields=fields, status=status, campaign_id=campaign_id, bot_id=bot_id, fuzzer=fuzzer, sort=sort)
        print("The response of JobsApi->list_jobs:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling JobsApi->list_jobs: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **limit** | **int**| Maximum number of items to return | [optional] [default to 50]
 **offset** | **int**| Number of items to skip (for offset-based pagination) | [optional] [default to 0]
 **cursor** | **str**| Cursor for cursor-based pagination | [optional] 
 **fields** | **str**| Comma-separated list of fields to include in response (sparse fieldsets) | [optional] 
 **status** | [**JobStatus**](.md)| Filter by job status | [optional] 
 **campaign_id** | **str**| Filter by campaign ID | [optional] 
 **bot_id** | **str**| Filter by assigned bot ID | [optional] 
 **fuzzer** | [**FuzzerType**](.md)| Filter by fuzzer type | [optional] 
 **sort** | **str**| Sort specification (field:direction,field:direction) | [optional] 

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
**200** | Successfully retrieved jobs |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **update_job**
> Job update_job(job_id, job_update_request)

Update job

Update job configuration and parameters

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.job import Job
from pandafuzz.models.job_update_request import JobUpdateRequest
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
    api_instance = pandafuzz.JobsApi(api_client)
    job_id = '01234567-89ab-cdef-0123-456789abcdef' # str | Unique identifier for a job
    job_update_request = pandafuzz.JobUpdateRequest() # JobUpdateRequest | 

    try:
        # Update job
        api_response = api_instance.update_job(job_id, job_update_request)
        print("The response of JobsApi->update_job:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling JobsApi->update_job: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **job_id** | **str**| Unique identifier for a job | 
 **job_update_request** | [**JobUpdateRequest**](JobUpdateRequest.md)|  | 

### Return type

[**Job**](Job.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Job successfully updated |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**404** | Not Found - Resource does not exist |  -  |
**409** | Cannot update running job |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

