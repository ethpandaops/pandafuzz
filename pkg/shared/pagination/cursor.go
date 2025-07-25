package pagination

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

// Cursor represents a cursor-based pagination request
type Cursor struct {
	After  string
	Before string
	Limit  int
}

// CursorResponse represents a cursor-based pagination response
type CursorResponse struct {
	Edges      []Edge
	PageInfo   PageInfo
	TotalCount int
}

// Edge represents an edge in cursor pagination
type Edge struct {
	Cursor string
	Node   interface{}
}

// PageInfo contains pagination metadata
type PageInfo struct {
	HasNextPage     bool
	HasPreviousPage bool
	StartCursor     string
	EndCursor       string
}

// DefaultLimit is the default page size
const DefaultLimit = 20

// MaxLimit is the maximum allowed page size
const MaxLimit = 100

// NewCursor creates a new cursor with defaults
func NewCursor() *Cursor {
	return &Cursor{
		Limit: DefaultLimit,
	}
}

// WithAfter sets the after cursor
func (c *Cursor) WithAfter(after string) *Cursor {
	c.After = after
	return c
}

// WithBefore sets the before cursor
func (c *Cursor) WithBefore(before string) *Cursor {
	c.Before = before
	return c
}

// WithLimit sets the limit
func (c *Cursor) WithLimit(limit int) *Cursor {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	c.Limit = limit
	return c
}

// Validate validates the cursor parameters
func (c *Cursor) Validate() error {
	if c.After != "" && c.Before != "" {
		return fmt.Errorf("cannot specify both 'after' and 'before' cursors")
	}

	if c.Limit <= 0 {
		c.Limit = DefaultLimit
	}

	if c.Limit > MaxLimit {
		return fmt.Errorf("limit cannot exceed %d", MaxLimit)
	}

	return nil
}

// EncodeCursor encodes a cursor value
func EncodeCursor(prefix string, id string) string {
	cursor := fmt.Sprintf("%s:%s", prefix, id)
	return base64.URLEncoding.EncodeToString([]byte(cursor))
}

// DecodeCursor decodes a cursor value
func DecodeCursor(cursor string) (prefix, id string, err error) {
	if cursor == "" {
		return "", "", fmt.Errorf("cursor is empty")
	}

	decoded, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", fmt.Errorf("invalid cursor format: %w", err)
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid cursor structure")
	}

	return parts[0], parts[1], nil
}

// EncodeCursorWithTimestamp encodes a cursor with timestamp
func EncodeCursorWithTimestamp(prefix string, id string, timestamp int64) string {
	cursor := fmt.Sprintf("%s:%s:%d", prefix, id, timestamp)
	return base64.URLEncoding.EncodeToString([]byte(cursor))
}

// DecodeCursorWithTimestamp decodes a cursor with timestamp
func DecodeCursorWithTimestamp(cursor string) (prefix, id string, timestamp int64, err error) {
	if cursor == "" {
		return "", "", 0, fmt.Errorf("cursor is empty")
	}

	decoded, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid cursor format: %w", err)
	}

	parts := strings.Split(string(decoded), ":")
	if len(parts) != 3 {
		return "", "", 0, fmt.Errorf("invalid cursor structure")
	}

	timestamp, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid timestamp in cursor: %w", err)
	}

	return parts[0], parts[1], timestamp, nil
}

// BuildCursorResponse builds a cursor response from a slice of items
func BuildCursorResponse(items []interface{}, cursor *Cursor, totalCount int, getCursor func(interface{}) string) *CursorResponse {
	edges := make([]Edge, len(items))
	for i, item := range items {
		edges[i] = Edge{
			Cursor: getCursor(item),
			Node:   item,
		}
	}

	pageInfo := PageInfo{
		HasNextPage:     len(items) == cursor.Limit,
		HasPreviousPage: cursor.After != "" || cursor.Before != "",
	}

	if len(edges) > 0 {
		pageInfo.StartCursor = edges[0].Cursor
		pageInfo.EndCursor = edges[len(edges)-1].Cursor
	}

	return &CursorResponse{
		Edges:      edges,
		PageInfo:   pageInfo,
		TotalCount: totalCount,
	}
}
