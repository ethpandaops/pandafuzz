// Package campaign implements the application layer for campaign management using CQRS pattern.
//
// The package provides a clean separation between read and write operations through
// commands and queries, following the Command Query Responsibility Segregation (CQRS) pattern.
//
// # Architecture
//
// The package is organized into:
//   - Commands: Write operations that change campaign state
//   - Queries: Read operations that retrieve campaign data
//   - DTOs: Data Transfer Objects for API communication
//   - Handlers: Business logic orchestration
//
// # Commands
//
// Commands encapsulate write operations:
//   - CreateCampaignCommand: Creates a new campaign
//   - UpdateCampaignCommand: Updates campaign properties
//   - DeleteCampaignCommand: Removes a campaign
//   - StartCampaignCommand: Transitions campaign to active state
//   - PauseCampaignCommand: Pauses an active campaign
//   - CompleteCampaignCommand: Marks campaign as completed
//
// # Queries
//
// Queries handle read operations:
//   - GetCampaignQuery: Retrieves a single campaign by ID
//   - ListCampaignsQuery: Lists campaigns with pagination and filtering
//   - GetCampaignStatsQuery: Retrieves campaign statistics and metrics
//
// # Usage
//
//	// Create command handler
//	handler := NewCreateCampaignHandler(repo, validator, eventBus)
//
//	// Execute command
//	command := &CreateCampaignCommand{
//	    Name:        "Test Campaign",
//	    Description: "A test fuzzing campaign",
//	}
//	campaign, err := handler.Handle(ctx, command)
//
//	// Execute query
//	query := &GetCampaignQuery{ID: "campaign-123"}
//	result, err := queryHandler.Handle(ctx, query)
//
// # Error Handling
//
// The package uses typed errors for different failure scenarios:
//   - ValidationError: Input validation failures
//   - NotFoundError: Resource not found
//   - ConflictError: Business rule violations
//   - OperationError: Infrastructure failures
//
// # Event Publishing
//
// All state-changing operations publish domain events:
//   - CampaignCreated
//   - CampaignUpdated
//   - CampaignDeleted
//   - CampaignStarted
//   - CampaignCompleted
//
// These events enable:
//   - Audit logging
//   - Real-time notifications
//   - Event sourcing
//   - Integration with other systems
package campaign
