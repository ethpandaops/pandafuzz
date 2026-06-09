package mappers

import (
	"time"

	"github.com/google/uuid"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// CommonBotToAPI converts a common.Bot to the generated API Bot type.
func CommonBotToAPI(bot *common.Bot) generated.Bot {
	if bot == nil {
		return generated.Bot{}
	}

	// Convert capabilities []string to []generated.BotCapabilities
	capabilities := make([]generated.BotCapabilities, len(bot.Capabilities))
	for i, cap := range bot.Capabilities {
		capabilities[i] = generated.BotCapabilities(cap)
	}

	apiBot := generated.Bot{
		Id:            uuid.MustParse(bot.ID),
		Name:          bot.Name,
		Hostname:      bot.Hostname,
		Status:        CommonBotStatusToAPI(bot.Status),
		Capabilities:  capabilities,
		RegisteredAt:  bot.RegisteredAt,
		LastHeartbeat: bot.LastSeen,
		IsOnline:      bot.IsOnline,
	}

	// Set API endpoint if available
	if bot.APIEndpoint != "" {
		apiBot.ApiEndpoint = &bot.APIEndpoint
	}

	// Set current job if assigned
	if bot.CurrentJob != nil && *bot.CurrentJob != "" {
		jobID, err := uuid.Parse(*bot.CurrentJob)
		if err != nil {
			jobID = uuid.NewSHA1(uuid.Nil, []byte(*bot.CurrentJob))
		}
		apiBot.CurrentJobId = &jobID
	}

	return apiBot
}

// CommonBotsToAPI converts a slice of common.Bots to API Bots.
func CommonBotsToAPI(bots []*common.Bot) []generated.Bot {
	result := make([]generated.Bot, len(bots))
	for i, bot := range bots {
		result[i] = CommonBotToAPI(bot)
	}
	return result
}

// CommonBotStatusToAPI converts common.BotStatus to API BotStatus.
func CommonBotStatusToAPI(status common.BotStatus) generated.BotStatus {
	switch status {
	case common.BotStatusIdle:
		return generated.BotStatusIdle
	case common.BotStatusBusy:
		return generated.BotStatusBusy
	case common.BotStatusTimedOut:
		return generated.BotStatusOffline // Map timed_out to offline
	case common.BotStatusFailed:
		return generated.BotStatusError // Map failed to error
	case common.BotStatusRegistering:
		return generated.BotStatusOffline // Map registering to offline
	default:
		return generated.BotStatusOffline
	}
}

// APIBotStatusToCommon converts API BotStatus to common.BotStatus.
func APIBotStatusToCommon(status generated.BotStatus) common.BotStatus {
	switch status {
	case generated.BotStatusIdle:
		return common.BotStatusIdle
	case generated.BotStatusBusy:
		return common.BotStatusBusy
	case generated.BotStatusOffline:
		return common.BotStatusTimedOut // Map offline to timed_out
	case generated.BotStatusError:
		return common.BotStatusFailed // Map error to failed
	case generated.BotStatusMaintenance:
		return common.BotStatusIdle // Map maintenance to idle
	default:
		return common.BotStatusIdle
	}
}

// APIBotCreateRequestToCommon converts an API bot creation request to a common.Bot.
func APIBotCreateRequestToCommon(req *generated.BotCreateRequest) *common.Bot {
	if req == nil {
		return nil
	}

	// Convert capabilities
	capabilities := make([]string, len(req.Capabilities))
	for i, cap := range req.Capabilities {
		capabilities[i] = string(cap)
	}

	bot := &common.Bot{
		ID:           uuid.New().String(),
		Name:         req.Name,
		Hostname:     req.Hostname,
		Status:       common.BotStatusRegistering,
		Capabilities: capabilities,
		RegisteredAt: time.Now(),
		LastSeen:     time.Now(),
		IsOnline:     false,
		APIEndpoint:  req.ApiEndpoint,
	}

	return bot
}
