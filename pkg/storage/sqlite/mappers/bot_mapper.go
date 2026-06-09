// Package mappers provides conversion functions between domain types and database row types.
package mappers

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	bottypes "github.com/ethpandaops/pandafuzz/pkg/domain/bot/types"
	"github.com/ethpandaops/pandafuzz/pkg/storage/sqlite/models"
)

// DomainBotToRow converts a domain Agent to a database row
func DomainBotToRow(agent *bottypes.Agent) *models.DomainBotRow {
	if agent == nil {
		return nil
	}

	row := &models.DomainBotRow{
		ID:            agent.ID,
		Name:          agent.Name,
		Status:        string(agent.Status),
		LastHeartbeat: agent.LastHeartbeat,
		CreatedAt:     agent.CreatedAt,
		UpdatedAt:     agent.UpdatedAt,
	}

	// Serialize capabilities to JSON
	if len(agent.Capabilities) > 0 {
		caps := make([]string, len(agent.Capabilities))
		for i, cap := range agent.Capabilities {
			caps[i] = string(cap)
		}
		if data, err := json.Marshal(caps); err == nil {
			row.CapabilitiesJSON = sql.NullString{String: string(data), Valid: true}
		}
	}

	// Serialize metadata to JSON
	if len(agent.Metadata) > 0 {
		if data, err := json.Marshal(agent.Metadata); err == nil {
			row.MetadataJSON = sql.NullString{String: string(data), Valid: true}
		}
	}

	return row
}

// BotRowToDomain converts a database row to a domain Agent
func BotRowToDomain(row *models.DomainBotRow) *bottypes.Agent {
	if row == nil {
		return nil
	}

	agent := &bottypes.Agent{
		ID:            row.ID,
		Name:          row.Name,
		Status:        bottypes.Status(row.Status),
		LastHeartbeat: row.LastHeartbeat,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
		Metadata:      make(map[string]interface{}),
	}

	// Parse capabilities from JSON
	if row.CapabilitiesJSON.Valid && row.CapabilitiesJSON.String != "" {
		var caps []string
		if err := json.Unmarshal([]byte(row.CapabilitiesJSON.String), &caps); err == nil {
			agent.Capabilities = make([]bottypes.Capability, len(caps))
			for i, cap := range caps {
				agent.Capabilities[i] = bottypes.Capability(cap)
			}
		}
	}

	// Parse metadata from JSON
	if row.MetadataJSON.Valid && row.MetadataJSON.String != "" {
		_ = json.Unmarshal([]byte(row.MetadataJSON.String), &agent.Metadata)
	}

	return agent
}

// BotStatusStringToDomain converts a string to domain Status
func BotStatusStringToDomain(s string) bottypes.Status {
	status, err := bottypes.ParseStatus(s)
	if err != nil {
		return bottypes.StatusOffline
	}
	return status
}

// NowUTC returns the current time in UTC
func NowUTC() time.Time {
	return time.Now().UTC()
}

// CommonBotToDomainAgent converts a common.Bot to a domain Agent
func CommonBotToDomainAgent(bot *common.Bot) *bottypes.Agent {
	if bot == nil {
		return nil
	}

	agent := &bottypes.Agent{
		ID:            bot.ID,
		Name:          bot.Name,
		Status:        CommonBotStatusToDomain(bot.Status),
		LastHeartbeat: bot.LastSeen,
		CreatedAt:     bot.RegisteredAt,
		UpdatedAt:     bot.LastSeen,
		Metadata:      make(map[string]interface{}),
	}

	// Convert capabilities
	if len(bot.Capabilities) > 0 {
		agent.Capabilities = make([]bottypes.Capability, len(bot.Capabilities))
		for i, cap := range bot.Capabilities {
			agent.Capabilities[i] = bottypes.Capability(cap)
		}
	}

	return agent
}

// DomainAgentToCommonBot converts a domain Agent to a common.Bot
func DomainAgentToCommonBot(agent *bottypes.Agent) *common.Bot {
	if agent == nil {
		return nil
	}

	bot := &common.Bot{
		ID:           agent.ID,
		Name:         agent.Name,
		Status:       DomainBotStatusToCommon(agent.Status),
		LastSeen:     agent.LastHeartbeat,
		RegisteredAt: agent.CreatedAt,
		IsOnline:     agent.Status == bottypes.StatusIdle || agent.Status == bottypes.StatusWorking,
	}

	// Convert capabilities
	if len(agent.Capabilities) > 0 {
		bot.Capabilities = make([]string, len(agent.Capabilities))
		for i, cap := range agent.Capabilities {
			bot.Capabilities[i] = string(cap)
		}
	}

	return bot
}

// CommonBotStatusToDomain converts common.BotStatus to domain Status
func CommonBotStatusToDomain(status common.BotStatus) bottypes.Status {
	switch status {
	case common.BotStatusIdle:
		return bottypes.StatusIdle
	case common.BotStatusBusy:
		return bottypes.StatusWorking
	case common.BotStatusTimedOut, common.BotStatusFailed:
		return bottypes.StatusError
	case common.BotStatusRegistering:
		return bottypes.StatusMaintenance
	default:
		return bottypes.StatusOffline
	}
}

// DomainBotStatusToCommon converts domain Status to common.BotStatus
func DomainBotStatusToCommon(status bottypes.Status) common.BotStatus {
	switch status {
	case bottypes.StatusIdle:
		return common.BotStatusIdle
	case bottypes.StatusWorking:
		return common.BotStatusBusy
	case bottypes.StatusError:
		return common.BotStatusFailed
	case bottypes.StatusMaintenance:
		return common.BotStatusRegistering
	case bottypes.StatusOffline:
		return common.BotStatusTimedOut
	default:
		return common.BotStatusTimedOut
	}
}
