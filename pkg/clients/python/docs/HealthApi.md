# pandafuzz.HealthApi

All URIs are relative to *http://localhost:8080/api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**get_health**](HealthApi.md#get_health) | **GET** /health | System health check
[**get_readiness**](HealthApi.md#get_readiness) | **GET** /ready | Readiness probe


# **get_health**
> HealthStatus get_health()

System health check

Check the overall health status of the PandaFuzz system

### Example


```python
import pandafuzz
from pandafuzz.models.health_status import HealthStatus
from pandafuzz.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080/api/v1
# See configuration.py for a list of all supported configuration parameters.
configuration = pandafuzz.Configuration(
    host = "http://localhost:8080/api/v1"
)


# Enter a context with an instance of the API client
with pandafuzz.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = pandafuzz.HealthApi(api_client)

    try:
        # System health check
        api_response = api_instance.get_health()
        print("The response of HealthApi->get_health:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling HealthApi->get_health: %s\n" % e)
```



### Parameters

This endpoint does not need any parameter.

### Return type

[**HealthStatus**](HealthStatus.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | System is healthy |  -  |
**503** | Service Unavailable - Service is temporarily unavailable |  * Retry-After - Number of seconds to wait before retrying <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **get_readiness**
> ReadinessStatus get_readiness()

Readiness probe

Check if the system is ready to accept requests

### Example


```python
import pandafuzz
from pandafuzz.models.readiness_status import ReadinessStatus
from pandafuzz.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080/api/v1
# See configuration.py for a list of all supported configuration parameters.
configuration = pandafuzz.Configuration(
    host = "http://localhost:8080/api/v1"
)


# Enter a context with an instance of the API client
with pandafuzz.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = pandafuzz.HealthApi(api_client)

    try:
        # Readiness probe
        api_response = api_instance.get_readiness()
        print("The response of HealthApi->get_readiness:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling HealthApi->get_readiness: %s\n" % e)
```



### Parameters

This endpoint does not need any parameter.

### Return type

[**ReadinessStatus**](ReadinessStatus.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | System is ready |  -  |
**503** | Service Unavailable - Service is temporarily unavailable |  * Retry-After - Number of seconds to wait before retrying <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

