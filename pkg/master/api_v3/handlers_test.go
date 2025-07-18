package api_v3

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/ethpandaops/pandafuzz/pkg/service"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Mock service manager for testing
type mockServiceManager struct {
	bot             *mockBotService
	job             *mockJobService
	campaign        *mockCampaignService
	corpus          *mockCorpusService
	crash           *mockCrashService
	reproducibility *mockReproducibilityService
	result          *mockResultService
	system          *mockSystemService
}

func newMockServiceManager() *mockServiceManager {
	return &mockServiceManager{
		bot:             &mockBotService{},
		job:             &mockJobService{},
		campaign:        &mockCampaignService{},
		corpus:          &mockCorpusService{},
		crash:           &mockCrashService{},
		reproducibility: &mockReproducibilityService{},
		result:          &mockResultService{},
		system:          &mockSystemService{},
	}
}

// Mock bot service
type mockBotService struct {
	mock.Mock
}

func (m *mockBotService) RegisterBot(ctx context.Context, hostname, name string, capabilities []string, apiEndpoint string) (*common.Bot, error) {
	args := m.Called(ctx, hostname, name, capabilities, apiEndpoint)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.Bot), args.Error(1)
}

func (m *mockBotService) GetBot(ctx context.Context, botID string) (*common.Bot, error) {
	args := m.Called(ctx, botID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.Bot), args.Error(1)
}

func (m *mockBotService) DeregisterBot(ctx context.Context, botID string) error {
	args := m.Called(ctx, botID)
	return args.Error(0)
}

func (m *mockBotService) Heartbeat(ctx context.Context, botID string, status common.BotStatus, currentJob *string) (time.Time, error) {
	args := m.Called(ctx, botID, status, currentJob)
	return args.Get(0).(time.Time), args.Error(1)
}

func (m *mockBotService) GetCurrentJob(ctx context.Context, botID string) (*common.Job, error) {
	args := m.Called(ctx, botID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.Job), args.Error(1)
}

func (m *mockBotService) GetMetrics(ctx context.Context, botID string) (*service.BotMetrics, error) {
	args := m.Called(ctx, botID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*service.BotMetrics), args.Error(1)
}

func (m *mockBotService) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockBotService) Stop() error {
	args := m.Called()
	return args.Error(0)
}

func (m *mockBotService) DeleteBot(ctx context.Context, botID string) error {
	args := m.Called(ctx, botID)
	return args.Error(0)
}

func (m *mockBotService) UpdateHeartbeat(ctx context.Context, botID string, status common.BotStatus, currentJob *string) error {
	args := m.Called(ctx, botID, status, currentJob)
	return args.Error(0)
}

func (m *mockBotService) ListBots(ctx context.Context, statusFilter *common.BotStatus) ([]*common.Bot, error) {
	args := m.Called(ctx, statusFilter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.Bot), args.Error(1)
}

func (m *mockBotService) GetAvailableBot(ctx context.Context, requiredCapabilities []string) (*common.Bot, error) {
	args := m.Called(ctx, requiredCapabilities)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.Bot), args.Error(1)
}

// Mock job service
type mockJobService struct {
	mock.Mock
}

func (m *mockJobService) Start(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockJobService) Stop() error {
	args := m.Called()
	return args.Error(0)
}

// Mock campaign service
type mockCampaignService struct {
	mock.Mock
}

// Mock corpus service
type mockCorpusService struct {
	mock.Mock
}

// Mock crash service
type mockCrashService struct {
	mock.Mock
}

// Mock reproducibility service
type mockReproducibilityService struct {
	mock.Mock
}

// Mock result service
type mockResultService struct {
	mock.Mock
}

// Mock system service
type mockSystemService struct {
	mock.Mock
}

// Test setup helper
func setupTestHandler(t *testing.T) (*HandlerV3, *mockServiceManager, *mux.Router) {
	mockSvc := newMockServiceManager()

	// Create a proper service manager that returns our mocks
	services := &service.Manager{
		Bot:      mockSvc.bot,
		Job:      mockSvc.job,
		Campaign: mockSvc.campaign,
		Corpus:   mockSvc.corpus,
	}

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	config := &Config{
		MaxRequestSize:  1024 * 1024, // 1MB
		RequestTimeout:  30 * time.Second,
		MaxBatchSize:    100,
		EnableSwaggerUI: false,
	}

	handler := NewHandlerV3(services, logger, config)

	router := mux.NewRouter()
	handler.RegisterRoutes(router)

	return handler, mockSvc, router
}

// Test cases

func TestBotRegistration(t *testing.T) {
	_, mocks, router := setupTestHandler(t)

	// Setup mock expectations
	expectedBot := &common.Bot{
		ID:           "test-bot-id",
		Hostname:     "test-host",
		Name:         "Test Bot",
		Status:       common.BotStatusIdle,
		RegisteredAt: time.Now(),
		TimeoutAt:    time.Now().Add(5 * time.Minute),
		Capabilities: []string{"aflplusplus", "libfuzzer"},
		APIEndpoint:  "http://test-host:8081",
	}

	mocks.bot.On("RegisterBot", mock.Anything, "test-host", "Test Bot", []string{"afl++", "libfuzzer"}, "http://test-host:8081").
		Return(expectedBot, nil)

	// Create request
	reqBody := BotRegisterRequest{
		Hostname:     "test-host",
		Name:         "Test Bot",
		Capabilities: []string{"aflplusplus", "libfuzzer"},
		APIEndpoint:  "http://test-host:8081",
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/v3/bots", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// Check response
	if rr.Code != http.StatusCreated {
		t.Logf("Response body: %s", rr.Body.String())
	}
	assert.Equal(t, http.StatusCreated, rr.Code)

	var resp BotRegisterResponse
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, expectedBot.ID, resp.BotID)
	assert.Equal(t, "registered", resp.Status)
	assert.NotZero(t, resp.Timestamp)
	assert.NotZero(t, resp.Timeout)

	mocks.bot.AssertExpectations(t)
}

func TestBotRegistrationValidation(t *testing.T) {
	_, _, router := setupTestHandler(t)

	tests := []struct {
		name       string
		request    BotRegisterRequest
		wantStatus int
		wantError  string
	}{
		{
			name: "missing hostname",
			request: BotRegisterRequest{
				Capabilities: []string{"aflplusplus"},
				APIEndpoint:  "http://test:8081",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "hostname",
		},
		{
			name: "missing capabilities",
			request: BotRegisterRequest{
				Hostname:    "test-host",
				APIEndpoint: "http://test:8081",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "capabilities",
		},
		{
			name: "invalid API endpoint",
			request: BotRegisterRequest{
				Hostname:     "test-host",
				Capabilities: []string{"aflplusplus"},
				APIEndpoint:  "not-a-url",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "url",
		},
		{
			name: "hostname too long",
			request: BotRegisterRequest{
				Hostname:     string(make([]byte, 300)),
				Capabilities: []string{"aflplusplus"},
				APIEndpoint:  "http://test:8081",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "exceeds maximum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest("POST", "/api/v3/bots", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)

			var resp ErrorResponse
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			require.NoError(t, err)

			assert.Contains(t, resp.Message, tt.wantError)
		})
	}
}

func TestPaginationParams(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		wantPage      int
		wantLimit     int
		wantSortBy    string
		wantSortOrder string
		wantOffset    int
	}{
		{
			name:          "default values",
			query:         "",
			wantPage:      1,
			wantLimit:     50,
			wantSortBy:    "created_at",
			wantSortOrder: "desc",
			wantOffset:    0,
		},
		{
			name:          "custom values",
			query:         "?page=3&limit=25&sortBy=name&sortOrder=asc",
			wantPage:      3,
			wantLimit:     25,
			wantSortBy:    "name",
			wantSortOrder: "asc",
			wantOffset:    50,
		},
		{
			name:          "invalid values use defaults",
			query:         "?page=-1&limit=200&sortBy=invalid&sortOrder=invalid",
			wantPage:      1,
			wantLimit:     50,
			wantSortBy:    "invalid",
			wantSortOrder: "desc",
			wantOffset:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test"+tt.query, nil)
			params := parsePaginationParams(req)

			assert.Equal(t, tt.wantPage, params.Page)
			assert.Equal(t, tt.wantLimit, params.Limit)
			assert.Equal(t, tt.wantSortBy, params.SortBy)
			assert.Equal(t, tt.wantSortOrder, params.SortOrder)
			assert.Equal(t, tt.wantOffset, params.Offset)
		})
	}
}

func TestValidation(t *testing.T) {
	v := NewValidator()

	t.Run("required validation", func(t *testing.T) {
		type TestStruct struct {
			Required string `validate:"required"`
			Optional string
		}

		// Missing required field
		err := v.Validate(&TestStruct{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "required")

		// Valid
		err = v.Validate(&TestStruct{Required: "value"})
		assert.NoError(t, err)
	})

	t.Run("max length validation", func(t *testing.T) {
		type TestStruct struct {
			Name string `validate:"max=10"`
		}

		// Too long
		err := v.Validate(&TestStruct{Name: "this is too long"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum")

		// Valid
		err = v.Validate(&TestStruct{Name: "short"})
		assert.NoError(t, err)
	})

	t.Run("UUID validation", func(t *testing.T) {
		type TestStruct struct {
			ID string `validate:"uuid"`
		}

		// Invalid UUID
		err := v.Validate(&TestStruct{ID: "not-a-uuid"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "UUID")

		// Valid UUID
		err = v.Validate(&TestStruct{ID: "123e4567-e89b-12d3-a456-426614174000"})
		assert.NoError(t, err)
	})

	t.Run("oneof validation", func(t *testing.T) {
		type TestStruct struct {
			Fuzzer string `validate:"oneof=afl++ libfuzzer honggfuzz"`
		}

		// Invalid value
		err := v.Validate(&TestStruct{Fuzzer: "invalid"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must be one of")

		// Valid value
		err = v.Validate(&TestStruct{Fuzzer: "afl++"})
		assert.NoError(t, err)
	})
}

func TestErrorHandling(t *testing.T) {
	handler, _, _ := setupTestHandler(t)

	t.Run("validation error", func(t *testing.T) {
		rr := httptest.NewRecorder()

		handler.writeError(rr, &ValidationError{
			Field:   "test_field",
			Message: "invalid value",
		})

		assert.Equal(t, http.StatusBadRequest, rr.Code)

		var resp ErrorResponse
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Equal(t, "validation_error", resp.Error)
		assert.Equal(t, "test_field", resp.Details["field"])
	})

	t.Run("not found error", func(t *testing.T) {
		rr := httptest.NewRecorder()

		handler.writeError(rr, &NotFoundError{
			Resource: "bot",
			ID:       "123",
		})

		assert.Equal(t, http.StatusNotFound, rr.Code)

		var resp ErrorResponse
		json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.Equal(t, "not_found", resp.Error)
	})
}

// Benchmark tests

func BenchmarkBotRegistration(b *testing.B) {
	_, mocks, router := setupTestHandler(&testing.T{})

	mocks.bot.On("RegisterBot", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&common.Bot{ID: "test"}, nil)

	reqBody := BotRegisterRequest{
		Hostname:     "test-host",
		Name:         "Test Bot",
		Capabilities: []string{"afl++"},
		APIEndpoint:  "http://test:8081",
	}
	body, _ := json.Marshal(reqBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/api/v3/bots", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
	}
}

func BenchmarkValidation(b *testing.B) {
	v := NewValidator()

	type ComplexStruct struct {
		ID           string   `validate:"required,uuid"`
		Name         string   `validate:"required,min=3,max=100"`
		Type         string   `validate:"required,oneof=type1 type2 type3"`
		Tags         []string `validate:"max=10"`
		Capabilities []string `validate:"required,min=1,max=5,dive,required,alphanum_dash"`
		URL          string   `validate:"required,url"`
		Count        int      `validate:"min=0,max=1000"`
	}

	obj := &ComplexStruct{
		ID:           "123e4567-e89b-12d3-a456-426614174000",
		Name:         "Test Object",
		Type:         "type1",
		Tags:         []string{"tag1", "tag2"},
		Capabilities: []string{"cap-1", "cap-2"},
		URL:          "http://example.com",
		Count:        42,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Validate(obj)
	}
}
