package pagination

import "fmt"

// Offset represents offset-based pagination parameters
type Offset struct {
	Offset int
	Limit  int
}

// OffsetResponse represents an offset-based pagination response
type OffsetResponse struct {
	Items      []interface{}
	TotalCount int
	Offset     int
	Limit      int
}

// NewOffset creates a new offset pagination with defaults
func NewOffset() *Offset {
	return &Offset{
		Offset: 0,
		Limit:  DefaultLimit,
	}
}

// WithOffset sets the offset
func (o *Offset) WithOffset(offset int) *Offset {
	if offset < 0 {
		offset = 0
	}
	o.Offset = offset
	return o
}

// WithLimit sets the limit
func (o *Offset) WithLimit(limit int) *Offset {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	o.Limit = limit
	return o
}

// WithPage sets offset based on page number (1-indexed)
func (o *Offset) WithPage(page int) *Offset {
	if page <= 0 {
		page = 1
	}
	o.Offset = (page - 1) * o.Limit
	return o
}

// Validate validates the offset parameters
func (o *Offset) Validate() error {
	if o.Offset < 0 {
		return fmt.Errorf("offset cannot be negative")
	}

	if o.Limit <= 0 {
		o.Limit = DefaultLimit
	}

	if o.Limit > MaxLimit {
		return fmt.Errorf("limit cannot exceed %d", MaxLimit)
	}

	return nil
}

// Page returns the current page number (1-indexed)
func (o *Offset) Page() int {
	if o.Limit <= 0 {
		return 1
	}
	return (o.Offset / o.Limit) + 1
}

// TotalPages calculates the total number of pages
func (o *Offset) TotalPages(totalCount int) int {
	if o.Limit <= 0 || totalCount <= 0 {
		return 0
	}
	return (totalCount + o.Limit - 1) / o.Limit
}

// HasNextPage checks if there's a next page
func (o *Offset) HasNextPage(totalCount int) bool {
	return o.Offset+o.Limit < totalCount
}

// HasPreviousPage checks if there's a previous page
func (o *Offset) HasPreviousPage() bool {
	return o.Offset > 0
}

// NextOffset returns the offset for the next page
func (o *Offset) NextOffset() int {
	return o.Offset + o.Limit
}

// PreviousOffset returns the offset for the previous page
func (o *Offset) PreviousOffset() int {
	offset := o.Offset - o.Limit
	if offset < 0 {
		offset = 0
	}
	return offset
}

// BuildOffsetResponse builds an offset response
func BuildOffsetResponse(items []interface{}, offset *Offset, totalCount int) *OffsetResponse {
	return &OffsetResponse{
		Items:      items,
		TotalCount: totalCount,
		Offset:     offset.Offset,
		Limit:      offset.Limit,
	}
}

// PageMetadata contains metadata about the current page
type PageMetadata struct {
	Page        int  `json:"page"`
	PerPage     int  `json:"per_page"`
	PageCount   int  `json:"page_count"`
	TotalCount  int  `json:"total_count"`
	HasNext     bool `json:"has_next"`
	HasPrevious bool `json:"has_previous"`
}

// GetPageMetadata returns metadata about the current page
func (o *Offset) GetPageMetadata(totalCount int) PageMetadata {
	return PageMetadata{
		Page:        o.Page(),
		PerPage:     o.Limit,
		PageCount:   o.TotalPages(totalCount),
		TotalCount:  totalCount,
		HasNext:     o.HasNextPage(totalCount),
		HasPrevious: o.HasPreviousPage(),
	}
}

// ApplyToSlice applies pagination to a slice (for in-memory pagination)
func (o *Offset) ApplyToSlice(items []interface{}) []interface{} {
	if o.Offset >= len(items) {
		return []interface{}{}
	}

	end := o.Offset + o.Limit
	if end > len(items) {
		end = len(items)
	}

	return items[o.Offset:end]
}
