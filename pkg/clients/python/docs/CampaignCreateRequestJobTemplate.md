# CampaignCreateRequestJobTemplate

Template for jobs created in this campaign

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**fuzzer** | [**FuzzerType**](FuzzerType.md) |  | [optional] 
**timeout_seconds** | **int** |  | [optional] [default to 3600]
**config** | **Dict[str, object]** |  | [optional] 
**enable_coverage** | **bool** |  | [optional] [default to True]
**coverage_format** | [**CoverageFormat**](CoverageFormat.md) |  | [optional] 

## Example

```python
from pandafuzz.models.campaign_create_request_job_template import CampaignCreateRequestJobTemplate

# TODO update the JSON string below
json = "{}"
# create an instance of CampaignCreateRequestJobTemplate from a JSON string
campaign_create_request_job_template_instance = CampaignCreateRequestJobTemplate.from_json(json)
# print the JSON string representation of the object
print(CampaignCreateRequestJobTemplate.to_json())

# convert the object into a dict
campaign_create_request_job_template_dict = campaign_create_request_job_template_instance.to_dict()
# create an instance of CampaignCreateRequestJobTemplate from a dict
campaign_create_request_job_template_from_dict = CampaignCreateRequestJobTemplate.from_dict(campaign_create_request_job_template_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


