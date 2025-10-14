# CoverageReport


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **str** | Unique identifier for the coverage report | 
**job_id** | **str** | ID of the job that generated this report | 
**format** | [**CoverageFormat**](CoverageFormat.md) |  | 
**size_bytes** | **int** | Size of the coverage report in bytes | 
**created_at** | **datetime** | When the coverage report was created | 
**checksum** | **str** | SHA256 checksum of the report file | [optional] 
**coverage_metrics** | [**CoverageReportCoverageMetrics**](CoverageReportCoverageMetrics.md) |  | [optional] 
**file_path** | **str** | Path to the coverage report file in storage | [optional] 
**download_url** | **str** | URL to download the coverage report | [optional] 

## Example

```python
from pandafuzz.models.coverage_report import CoverageReport

# TODO update the JSON string below
json = "{}"
# create an instance of CoverageReport from a JSON string
coverage_report_instance = CoverageReport.from_json(json)
# print the JSON string representation of the object
print(CoverageReport.to_json())

# convert the object into a dict
coverage_report_dict = coverage_report_instance.to_dict()
# create an instance of CoverageReport from a dict
coverage_report_from_dict = CoverageReport.from_dict(coverage_report_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


