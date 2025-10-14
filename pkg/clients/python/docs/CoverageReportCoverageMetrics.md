# CoverageReportCoverageMetrics

Coverage metrics summary

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**line_coverage_percent** | **float** | Percentage of lines covered | [optional] 
**function_coverage_percent** | **float** | Percentage of functions covered | [optional] 
**branch_coverage_percent** | **float** | Percentage of branches covered | [optional] 
**total_lines** | **int** |  | [optional] 
**covered_lines** | **int** |  | [optional] 
**total_functions** | **int** |  | [optional] 
**covered_functions** | **int** |  | [optional] 
**total_branches** | **int** |  | [optional] 
**covered_branches** | **int** |  | [optional] 

## Example

```python
from pandafuzz.models.coverage_report_coverage_metrics import CoverageReportCoverageMetrics

# TODO update the JSON string below
json = "{}"
# create an instance of CoverageReportCoverageMetrics from a JSON string
coverage_report_coverage_metrics_instance = CoverageReportCoverageMetrics.from_json(json)
# print the JSON string representation of the object
print(CoverageReportCoverageMetrics.to_json())

# convert the object into a dict
coverage_report_coverage_metrics_dict = coverage_report_coverage_metrics_instance.to_dict()
# create an instance of CoverageReportCoverageMetrics from a dict
coverage_report_coverage_metrics_from_dict = CoverageReportCoverageMetrics.from_dict(coverage_report_coverage_metrics_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


