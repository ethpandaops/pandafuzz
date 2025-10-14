# pandafuzz.AnalyticsApi

All URIs are relative to *http://localhost:8080/api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**get_analytics**](AnalyticsApi.md#get_analytics) | **GET** /analytics | Get system analytics
[**get_coverage_trends**](AnalyticsApi.md#get_coverage_trends) | **GET** /analytics/coverage | Get coverage trends
[**get_metrics**](AnalyticsApi.md#get_metrics) | **GET** /analytics/metrics | Get real-time metrics
[**get_performance_stats**](AnalyticsApi.md#get_performance_stats) | **GET** /analytics/performance | Get performance statistics


# **get_analytics**
> AnalyticsResponse get_analytics(time_range=time_range, campaign_id=campaign_id, metrics=metrics)

Get system analytics

Retrieve comprehensive analytics and metrics for the system

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.analytics_response import AnalyticsResponse
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
    api_instance = pandafuzz.AnalyticsApi(api_client)
    time_range = 24h # str | Time range for analytics (optional) (default to 24h)
    campaign_id = 'campaign_id_example' # str | Filter analytics by campaign (optional)
    metrics = 'coverage,crashes,performance' # str | Specific metrics to include (comma-separated) (optional)

    try:
        # Get system analytics
        api_response = api_instance.get_analytics(time_range=time_range, campaign_id=campaign_id, metrics=metrics)
        print("The response of AnalyticsApi->get_analytics:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling AnalyticsApi->get_analytics: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **time_range** | **str**| Time range for analytics | [optional] [default to 24h]
 **campaign_id** | **str**| Filter analytics by campaign | [optional] 
 **metrics** | **str**| Specific metrics to include (comma-separated) | [optional] 

### Return type

[**AnalyticsResponse**](AnalyticsResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved analytics |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_coverage_trends**
> CoverageTrendsResponse get_coverage_trends(campaign_id=campaign_id, time_range=time_range, granularity=granularity)

Get coverage trends

Retrieve coverage analysis and trend data

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.coverage_trends_response import CoverageTrendsResponse
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
    api_instance = pandafuzz.AnalyticsApi(api_client)
    campaign_id = 'campaign_id_example' # str | Filter by campaign ID (optional)
    time_range = 24h # str | Time range for trends (optional) (default to 24h)
    granularity = hour # str | Data point granularity (optional) (default to hour)

    try:
        # Get coverage trends
        api_response = api_instance.get_coverage_trends(campaign_id=campaign_id, time_range=time_range, granularity=granularity)
        print("The response of AnalyticsApi->get_coverage_trends:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling AnalyticsApi->get_coverage_trends: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **campaign_id** | **str**| Filter by campaign ID | [optional] 
 **time_range** | **str**| Time range for trends | [optional] [default to 24h]
 **granularity** | **str**| Data point granularity | [optional] [default to hour]

### Return type

[**CoverageTrendsResponse**](CoverageTrendsResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved coverage trends |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_metrics**
> MetricsResponse get_metrics()

Get real-time metrics

Retrieve current system metrics and performance indicators

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.metrics_response import MetricsResponse
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
    api_instance = pandafuzz.AnalyticsApi(api_client)

    try:
        # Get real-time metrics
        api_response = api_instance.get_metrics()
        print("The response of AnalyticsApi->get_metrics:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling AnalyticsApi->get_metrics: %s\n" % e)
```



### Parameters

This endpoint does not need any parameter.

### Return type

[**MetricsResponse**](MetricsResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/prometheus

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved metrics |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_performance_stats**
> PerformanceStatsResponse get_performance_stats(time_range=time_range, component=component)

Get performance statistics

Retrieve performance metrics and bottleneck analysis

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.performance_stats_response import PerformanceStatsResponse
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
    api_instance = pandafuzz.AnalyticsApi(api_client)
    time_range = 24h # str | Time range for analysis (optional) (default to 24h)
    component = 'component_example' # str | Filter by system component (optional)

    try:
        # Get performance statistics
        api_response = api_instance.get_performance_stats(time_range=time_range, component=component)
        print("The response of AnalyticsApi->get_performance_stats:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling AnalyticsApi->get_performance_stats: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **time_range** | **str**| Time range for analysis | [optional] [default to 24h]
 **component** | **str**| Filter by system component | [optional] 

### Return type

[**PerformanceStatsResponse**](PerformanceStatsResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successfully retrieved performance statistics |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

