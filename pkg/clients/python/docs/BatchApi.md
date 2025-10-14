# pandafuzz.BatchApi

All URIs are relative to *http://localhost:8080/api/v1*

Method | HTTP request | Description
------------- | ------------- | -------------
[**execute_batch**](BatchApi.md#execute_batch) | **POST** /batch | Execute batch operations


# **execute_batch**
> BatchResponse execute_batch(batch_request)

Execute batch operations

Execute multiple operations in a single transactional request

### Example

* Api Key Authentication (apiKeyAuth):
* Bearer (JWT) Authentication (bearerAuth):

```python
import pandafuzz
from pandafuzz.models.batch_request import BatchRequest
from pandafuzz.models.batch_response import BatchResponse
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
    api_instance = pandafuzz.BatchApi(api_client)
    batch_request = {"operations":[{"operation":"create_job","data":{"name":"batch-job-1","fuzzer":"libfuzzer","target_binary":"/path/to/target1","campaign_id":"01234567-89ab-cdef-0123-456789abcdef"}},{"operation":"create_job","data":{"name":"batch-job-2","fuzzer":"aflplusplus","target_binary":"/path/to/target2","campaign_id":"01234567-89ab-cdef-0123-456789abcdef"}}],"options":{"atomic":true,"fail_fast":false}} # BatchRequest | 

    try:
        # Execute batch operations
        api_response = api_instance.execute_batch(batch_request)
        print("The response of BatchApi->execute_batch:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling BatchApi->execute_batch: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **batch_request** | [**BatchRequest**](BatchRequest.md)|  | 

### Return type

[**BatchResponse**](BatchResponse.md)

### Authorization

[apiKeyAuth](../README.md#apiKeyAuth), [bearerAuth](../README.md#bearerAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Batch operations completed successfully |  -  |
**207** | Partial success - some operations failed |  -  |
**400** | Bad Request - Invalid request parameters or payload |  -  |
**401** | Unauthorized - Authentication required or invalid |  -  |
**500** | Internal Server Error - Unexpected server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

