package sse

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// Filter represents an event filtering interface
type Filter interface {
	Matches(event Event) bool
	String() string
}

// TypeFilter filters events by their type
type TypeFilter struct {
	EventTypes map[string]bool
}

// NewTypeFilter creates a new type filter with the given event types
func NewTypeFilter(eventTypes []string) *TypeFilter {
	typeMap := make(map[string]bool, len(eventTypes))
	for _, eventType := range eventTypes {
		typeMap[eventType] = true
	}
	return &TypeFilter{EventTypes: typeMap}
}

// Matches checks if the event type is in the allowed types
func (f *TypeFilter) Matches(event Event) bool {
	if len(f.EventTypes) == 0 {
		return true // No filter means all events pass
	}
	return f.EventTypes[event.GetType()]
}

// String returns a string representation of the filter
func (f *TypeFilter) String() string {
	types := make([]string, 0, len(f.EventTypes))
	for eventType := range f.EventTypes {
		types = append(types, eventType)
	}
	return "type_filter(" + strings.Join(types, ",") + ")"
}

// ResourceFilter filters events by resource ID (bot, job, campaign, etc.)
type ResourceFilter struct {
	ResourceType string // "bot", "job", "campaign", "crash", "corpus"
	ResourceID   string
}

// NewResourceFilter creates a new resource filter
func NewResourceFilter(resourceType, resourceID string) *ResourceFilter {
	return &ResourceFilter{
		ResourceType: resourceType,
		ResourceID:   resourceID,
	}
}

// Matches checks if the event relates to the specified resource
func (f *ResourceFilter) Matches(event Event) bool {
	switch f.ResourceType {
	case "bot":
		if botEvent, ok := event.(*BotEvent); ok {
			return botEvent.BotID.String() == f.ResourceID
		}
	case "job":
		if jobEvent, ok := event.(*JobEvent); ok {
			return jobEvent.JobID.String() == f.ResourceID
		}
	case "campaign":
		if campaignEvent, ok := event.(*CampaignEvent); ok {
			return campaignEvent.CampaignID.String() == f.ResourceID
		}
		if jobEvent, ok := event.(*JobEvent); ok {
			return jobEvent.CampaignID.String() == f.ResourceID
		}
		if crashEvent, ok := event.(*CrashEvent); ok {
			return crashEvent.CampaignID.String() == f.ResourceID
		}
		if corpusEvent, ok := event.(*CorpusEvent); ok {
			return corpusEvent.CampaignID != nil && corpusEvent.CampaignID.String() == f.ResourceID
		}
	case "crash":
		if crashEvent, ok := event.(*CrashEvent); ok {
			return crashEvent.CrashID.String() == f.ResourceID
		}
	case "corpus":
		if corpusEvent, ok := event.(*CorpusEvent); ok {
			return corpusEvent.CorpusID != nil && corpusEvent.CorpusID.String() == f.ResourceID
		}
	}
	return false
}

// String returns a string representation of the filter
func (f *ResourceFilter) String() string {
	return "resource_filter(" + f.ResourceType + ":" + f.ResourceID + ")"
}

// PatternFilter filters events using regex patterns on event types
type PatternFilter struct {
	Pattern *regexp.Regexp
}

// NewPatternFilter creates a new pattern filter with the given regex
func NewPatternFilter(pattern string) (*PatternFilter, error) {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &PatternFilter{Pattern: regex}, nil
}

// Matches checks if the event type matches the pattern
func (f *PatternFilter) Matches(event Event) bool {
	return f.Pattern.MatchString(event.GetType())
}

// String returns a string representation of the filter
func (f *PatternFilter) String() string {
	return "pattern_filter(" + f.Pattern.String() + ")"
}

// SeverityFilter filters events by severity level
type SeverityFilter struct {
	MinSeverity string
	Levels      map[string]int
}

// NewSeverityFilter creates a new severity filter
func NewSeverityFilter(minSeverity string) *SeverityFilter {
	levels := map[string]int{
		"debug":    0,
		"info":     1,
		"warning":  2,
		"error":    3,
		"critical": 4,
	}
	return &SeverityFilter{
		MinSeverity: minSeverity,
		Levels:      levels,
	}
}

// Matches checks if the event severity meets the minimum level
func (f *SeverityFilter) Matches(event Event) bool {
	minLevel, exists := f.Levels[f.MinSeverity]
	if !exists {
		return true // Unknown severity level, allow all
	}

	// Extract severity from different event types
	var eventSeverity string
	switch e := event.(type) {
	case *SystemEvent:
		eventSeverity = e.Severity
	case *CrashEvent:
		eventSeverity = e.Severity
	default:
		// For other event types, check if severity is in the data
		data := event.GetData()
		if strings.Contains(data, `"severity"`) {
			// This is a simple check; in practice, you might want to unmarshal the JSON
			if strings.Contains(data, `"critical"`) {
				eventSeverity = "critical"
			} else if strings.Contains(data, `"error"`) {
				eventSeverity = "error"
			} else if strings.Contains(data, `"warning"`) {
				eventSeverity = "warning"
			} else if strings.Contains(data, `"info"`) {
				eventSeverity = "info"
			} else {
				eventSeverity = "debug"
			}
		} else {
			return true // No severity info, allow all
		}
	}

	if eventSeverity == "" {
		return true // No severity info, allow all
	}

	eventLevel, exists := f.Levels[eventSeverity]
	if !exists {
		return true // Unknown event severity, allow all
	}

	return eventLevel >= minLevel
}

// String returns a string representation of the filter
func (f *SeverityFilter) String() string {
	return "severity_filter(min:" + f.MinSeverity + ")"
}

// CompoundFilter combines multiple filters with AND logic
type CompoundFilter struct {
	Filters []Filter
	Logic   string // "AND" or "OR"
}

// NewCompoundFilter creates a new compound filter
func NewCompoundFilter(logic string, filters ...Filter) *CompoundFilter {
	return &CompoundFilter{
		Filters: filters,
		Logic:   strings.ToUpper(logic),
	}
}

// Matches checks if the event passes all filters (AND) or any filter (OR)
func (f *CompoundFilter) Matches(event Event) bool {
	if len(f.Filters) == 0 {
		return true
	}

	switch f.Logic {
	case "OR":
		for _, filter := range f.Filters {
			if filter.Matches(event) {
				return true
			}
		}
		return false
	default: // "AND" is default
		for _, filter := range f.Filters {
			if !filter.Matches(event) {
				return false
			}
		}
		return true
	}
}

// String returns a string representation of the filter
func (f *CompoundFilter) String() string {
	filterStrs := make([]string, len(f.Filters))
	for i, filter := range f.Filters {
		filterStrs[i] = filter.String()
	}
	return "compound_filter(" + f.Logic + ":[" + strings.Join(filterStrs, ",") + "])"
}

// FilterBuilder helps build complex filters from query parameters
type FilterBuilder struct {
	filters []Filter
}

// NewFilterBuilder creates a new filter builder
func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{
		filters: make([]Filter, 0),
	}
}

// AddTypeFilter adds a type filter from comma-separated event types
func (fb *FilterBuilder) AddTypeFilter(types string) *FilterBuilder {
	if types != "" {
		eventTypes := strings.Split(types, ",")
		for i, t := range eventTypes {
			eventTypes[i] = strings.TrimSpace(t)
		}
		fb.filters = append(fb.filters, NewTypeFilter(eventTypes))
	}
	return fb
}

// AddResourceFilter adds a resource filter
func (fb *FilterBuilder) AddResourceFilter(resourceType, resourceID string) *FilterBuilder {
	if resourceType != "" && resourceID != "" {
		fb.filters = append(fb.filters, NewResourceFilter(resourceType, resourceID))
	}
	return fb
}

// AddPatternFilter adds a pattern filter
func (fb *FilterBuilder) AddPatternFilter(pattern string) *FilterBuilder {
	if pattern != "" {
		if filter, err := NewPatternFilter(pattern); err == nil {
			fb.filters = append(fb.filters, filter)
		}
	}
	return fb
}

// AddSeverityFilter adds a severity filter
func (fb *FilterBuilder) AddSeverityFilter(minSeverity string) *FilterBuilder {
	if minSeverity != "" {
		fb.filters = append(fb.filters, NewSeverityFilter(minSeverity))
	}
	return fb
}

// Build creates a compound filter with AND logic
func (fb *FilterBuilder) Build() Filter {
	if len(fb.filters) == 0 {
		return &AllowAllFilter{}
	}
	if len(fb.filters) == 1 {
		return fb.filters[0]
	}
	return NewCompoundFilter("AND", fb.filters...)
}

// BuildOr creates a compound filter with OR logic
func (fb *FilterBuilder) BuildOr() Filter {
	if len(fb.filters) == 0 {
		return &AllowAllFilter{}
	}
	if len(fb.filters) == 1 {
		return fb.filters[0]
	}
	return NewCompoundFilter("OR", fb.filters...)
}

// AllowAllFilter is a filter that allows all events
type AllowAllFilter struct{}

// Matches always returns true
func (f *AllowAllFilter) Matches(event Event) bool {
	return true
}

// String returns a string representation of the filter
func (f *AllowAllFilter) String() string {
	return "allow_all_filter"
}

// ParseFiltersFromParams parses filters from HTTP query parameters
func ParseFiltersFromParams(params map[string]string) Filter {
	builder := NewFilterBuilder()

	// Parse event types filter
	if types, exists := params["types"]; exists {
		builder.AddTypeFilter(types)
	}

	// Parse resource filters
	if botID, exists := params["bot_id"]; exists {
		builder.AddResourceFilter("bot", botID)
	}
	if jobID, exists := params["job_id"]; exists {
		builder.AddResourceFilter("job", jobID)
	}
	if campaignID, exists := params["campaign_id"]; exists {
		builder.AddResourceFilter("campaign", campaignID)
	}
	if crashID, exists := params["crash_id"]; exists {
		builder.AddResourceFilter("crash", crashID)
	}
	if corpusID, exists := params["corpus_id"]; exists {
		builder.AddResourceFilter("corpus", corpusID)
	}

	// Parse pattern filter
	if pattern, exists := params["pattern"]; exists {
		builder.AddPatternFilter(pattern)
	}

	// Parse severity filter
	if severity, exists := params["min_severity"]; exists {
		builder.AddSeverityFilter(severity)
	}

	return builder.Build()
}

// ParseUUIDFromString safely parses a UUID string
func ParseUUIDFromString(uuidStr string) (*openapi_types.UUID, error) {
	parsed, err := uuid.Parse(uuidStr)
	if err != nil {
		return nil, err
	}
	result := openapi_types.UUID(parsed)
	return &result, nil
}
