# Pagination


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**total** | **int** | Total number of items available | 
**limit** | **int** | Maximum number of items per page | 
**offset** | **int** | Number of items skipped | 
**has_more** | **bool** | Whether there are more items available | 
**next_cursor** | **str** | Cursor for next page (cursor-based pagination) | [optional] 
**prev_cursor** | **str** | Cursor for previous page (cursor-based pagination) | [optional] 

## Example

```python
from pandafuzz.models.pagination import Pagination

# TODO update the JSON string below
json = "{}"
# create an instance of Pagination from a JSON string
pagination_instance = Pagination.from_json(json)
# print the JSON string representation of the object
print(Pagination.to_json())

# convert the object into a dict
pagination_dict = pagination_instance.to_dict()
# create an instance of Pagination from a dict
pagination_from_dict = Pagination.from_dict(pagination_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


