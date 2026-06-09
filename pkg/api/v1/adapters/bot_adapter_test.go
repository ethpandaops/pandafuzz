package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/ethpandaops/pandafuzz/pkg/api/v1/generated"
	"github.com/ethpandaops/pandafuzz/pkg/common"
)

func TestBotAdapter_ListBots_OnlineOnlyPagination(t *testing.T) {
	t.Parallel()

	now := time.Now()
	bots := []*common.Bot{
		{
			ID:           "bot-1",
			Name:         "Bot One",
			Hostname:     "host-1",
			Status:       common.BotStatusIdle,
			LastSeen:     now,
			RegisteredAt: now.Add(-time.Hour),
			Capabilities: []string{"fuzzing"},
			IsOnline:     true,
		},
		{
			ID:           "bot-2",
			Name:         "Bot Two",
			Hostname:     "host-2",
			Status:       common.BotStatusBusy,
			LastSeen:     now.Add(-time.Minute),
			RegisteredAt: now.Add(-2 * time.Hour),
			Capabilities: []string{"analysis"},
			IsOnline:     false,
		},
		{
			ID:           "bot-3",
			Name:         "Bot Three",
			Hostname:     "host-3",
			Status:       common.BotStatusIdle,
			LastSeen:     now.Add(-2 * time.Minute),
			RegisteredAt: now.Add(-3 * time.Hour),
			Capabilities: []string{"fuzzing"},
			IsOnline:     true,
		},
	}

	botService := &stubBotService{
		listFn: func(_ context.Context, statusFilter *common.BotStatus) ([]*common.Bot, error) {
			return bots, nil
		},
	}

	adapter := NewBotAdapter(nil, nil, nil, botService, nil, nil, logrus.New())

	limit := 1
	offset := 0
	onlineOnly := true
	params := generated.ListBotsParams{
		Limit:      &limit,
		Offset:     &offset,
		OnlineOnly: &onlineOnly,
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/bots", nil)

	adapter.ListBots(recorder, request, params)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Nil(t, botService.lastStatus)

	var response generated.BotListResponse
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	require.Len(t, response.Data, 1)
	require.Equal(t, 1, response.Pagination.Limit)
	require.Equal(t, 0, response.Pagination.Offset)
	require.Equal(t, 2, response.Pagination.Total)
	require.True(t, response.Pagination.HasMore)
}
