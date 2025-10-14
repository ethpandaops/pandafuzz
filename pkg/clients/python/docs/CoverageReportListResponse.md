# CoverageReportListResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**data** | [**List[CoverageReport]**](CoverageReport.md) |  | 
**pagination** | [**Pagination**](Pagination.md) |  | 

## Example

```python
from pandafuzz.models.coverage_report_list_response import CoverageReportListResponse

# TODO update the JSON string below
json = "{}"
# create an instance of CoverageReportListResponse from a JSON string
coverage_report_list_response_instance = CoverageReportListResponse.from_json(json)
# print the JSON string representation of the object
print(CoverageReportListResponse.to_json())

# convert the object into a dict
coverage_report_list_response_dict = coverage_report_list_response_instance.to_dict()
# create an instance of CoverageReportListResponse from a dict
coverage_report_list_response_from_dict = CoverageReportListResponse.from_dict(coverage_report_list_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


