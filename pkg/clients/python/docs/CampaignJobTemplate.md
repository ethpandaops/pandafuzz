# CampaignJobTemplate

Template for jobs created in this campaign

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**fuzzer** | [**FuzzerType**](FuzzerType.md) |  | [optional] 
**timeout_seconds** | **int** |  | [optional] 
**config** | **Dict[str, object]** |  | [optional] 
**enable_coverage** | **bool** |  | [optional] 
**coverage_format** | [**CoverageFormat**](CoverageFormat.md) |  | [optional] 

## Example

```python
from pandafuzz.models.campaign_job_template import CampaignJobTemplate

# TODO update the JSON string below
json = "{}"
# create an instance of CampaignJobTemplate from a JSON string
campaign_job_template_instance = CampaignJobTemplate.from_json(json)
# print the JSON string representation of the object
print(CampaignJobTemplate.to_json())

# convert the object into a dict
campaign_job_template_dict = campaign_job_template_instance.to_dict()
# create an instance of CampaignJobTemplate from a dict
campaign_job_template_from_dict = CampaignJobTemplate.from_dict(campaign_job_template_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


