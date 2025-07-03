package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockDedupStorage is a mock implementation for deduplication storage
type MockDedupStorage struct {
	mock.Mock
}

func (m *MockDedupStorage) CreateCrashGroup(ctx context.Context, cg *common.CrashGroup) error {
	args := m.Called(ctx, cg)
	return args.Error(0)
}

func (m *MockDedupStorage) GetCrashGroup(ctx context.Context, campaignID, stackHash string) (*common.CrashGroup, error) {
	args := m.Called(ctx, campaignID, stackHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.CrashGroup), args.Error(1)
}

func (m *MockDedupStorage) UpdateCrashGroupCount(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockDedupStorage) ListCrashGroups(ctx context.Context, campaignID string) ([]*common.CrashGroup, error) {
	args := m.Called(ctx, campaignID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CrashGroup), args.Error(1)
}

func (m *MockDedupStorage) CreateStackTrace(ctx context.Context, crashID string, st *common.StackTrace) error {
	args := m.Called(ctx, crashID, st)
	return args.Error(0)
}

func (m *MockDedupStorage) GetStackTrace(ctx context.Context, crashID string) (*common.StackTrace, error) {
	args := m.Called(ctx, crashID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.StackTrace), args.Error(1)
}

func (m *MockDedupStorage) LinkCrashToGroup(ctx context.Context, crashID, groupID string) error {
	args := m.Called(ctx, crashID, groupID)
	return args.Error(0)
}

func (m *MockDedupStorage) UpdateCrash(ctx context.Context, crash *common.CrashResult) error {
	args := m.Called(ctx, crash)
	return args.Error(0)
}

func TestDeduplicationService_ProcessCrash(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("process new unique crash with ASAN trace", func(t *testing.T) {
		mockStorage := new(MockDedupStorage)
		ds := NewDeduplicationService(mockStorage, logger)

		crash := &common.CrashResult{
			ID:         "crash1",
			CampaignID: "campaign1",
			JobID:      "job1",
			BotID:      "bot1",
			Type:       "heap_overflow",
			Signal:     11,
			StackTrace: `==12345==ERROR: AddressSanitizer: heap-buffer-overflow on address 0x602000000150
    #0 0x7f8a6b1234ab in vulnerable_function /src/vulnerable.c:42:5
    #1 0x7f8a6b1235bc in process_input /src/main.c:156:3
    #2 0x7f8a6b1236cd in main /src/main.c:200:5
    #3 0x7f8a6a123456 in __libc_start_main /build/glibc/src/csu/libc-start.c:308:16
    #4 0x7f8a6b123789 in _start (/path/to/binary+0x1789)`,
		}

		// Compute expected hash for top 5 frames
		expectedFrames := []common.StackFrame{
			{Function: "vulnerable_function", File: "/src/vulnerable.c", Line: 42},
			{Function: "process_input", File: "/src/main.c", Line: 156},
			{Function: "main", File: "/src/main.c", Line: 200},
			{Function: "__libc_start_main", File: "/build/glibc/src/csu/libc-start.c", Line: 308},
			{Function: "_start", File: "/path/to/binary+0x1789", Line: 0},
		}

		// Mock no existing crash group found
		mockStorage.On("GetCrashGroup", ctx, crash.CampaignID, mock.Anything).
			Return(nil, common.ErrKeyNotFound).Once()

		// Mock creating new crash group
		mockStorage.On("CreateCrashGroup", ctx, mock.MatchedBy(func(cg *common.CrashGroup) bool {
			return cg.CampaignID == crash.CampaignID &&
				cg.Count == 1 &&
				cg.Severity == "high" &&
				cg.ExampleCrash == crash.ID &&
				len(cg.StackFrames) == 5
		})).Return(nil).Once()

		// Mock creating stack trace
		mockStorage.On("CreateStackTrace", ctx, crash.ID, mock.MatchedBy(func(st *common.StackTrace) bool {
			return len(st.Frames) == 5 && st.RawTrace == crash.StackTrace
		})).Return(nil).Once()

		// Mock linking crash to group
		mockStorage.On("LinkCrashToGroup", ctx, crash.ID, mock.Anything).Return(nil).Once()

		// Mock updating crash
		mockStorage.On("UpdateCrash", ctx, mock.MatchedBy(func(c *common.CrashResult) bool {
			return c.ID == crash.ID && c.StackHash != "" && c.CrashGroupID != ""
		})).Return(nil).Once()

		group, isNew, err := ds.ProcessCrash(ctx, crash)
		assert.NoError(t, err)
		assert.True(t, isNew)
		assert.NotNil(t, group)
		assert.Equal(t, crash.CampaignID, group.CampaignID)
		assert.Equal(t, 1, group.Count)
		assert.Equal(t, "high", group.Severity)
		mockStorage.AssertExpectations(t)
	})

	t.Run("process duplicate crash", func(t *testing.T) {
		mockStorage := new(MockDedupStorage)
		ds := NewDeduplicationService(mockStorage, logger)

		crash := &common.CrashResult{
			ID:         "crash2",
			CampaignID: "campaign1",
			JobID:      "job2",
			BotID:      "bot1",
			Type:       "heap_overflow",
			StackTrace: `    #0 0x7f8a6b1234ab in vulnerable_function /src/vulnerable.c:42:5
    #1 0x7f8a6b1235bc in process_input /src/main.c:156:3
    #2 0x7f8a6b1236cd in main /src/main.c:200:5`,
		}

		existingGroup := &common.CrashGroup{
			ID:           "group1",
			CampaignID:   crash.CampaignID,
			StackHash:    "existing-hash",
			Count:        5,
			Severity:     "high",
			ExampleCrash: "crash0",
		}

		// Mock finding existing crash group
		mockStorage.On("GetCrashGroup", ctx, crash.CampaignID, mock.Anything).
			Return(existingGroup, nil).Once()

		// Mock updating crash group count
		mockStorage.On("UpdateCrashGroupCount", ctx, existingGroup.ID).Return(nil).Once()

		// Mock creating stack trace
		mockStorage.On("CreateStackTrace", ctx, crash.ID, mock.Anything).Return(nil).Once()

		// Mock linking crash to group
		mockStorage.On("LinkCrashToGroup", ctx, crash.ID, existingGroup.ID).Return(nil).Once()

		// Mock updating crash
		mockStorage.On("UpdateCrash", ctx, mock.MatchedBy(func(c *common.CrashResult) bool {
			return c.ID == crash.ID && c.CrashGroupID == existingGroup.ID
		})).Return(nil).Once()

		group, isNew, err := ds.ProcessCrash(ctx, crash)
		assert.NoError(t, err)
		assert.False(t, isNew)
		assert.NotNil(t, group)
		assert.Equal(t, existingGroup.ID, group.ID)
		mockStorage.AssertExpectations(t)
	})

	t.Run("process crash with GDB trace", func(t *testing.T) {
		mockStorage := new(MockDedupStorage)
		ds := NewDeduplicationService(mockStorage, logger)

		crash := &common.CrashResult{
			ID:         "crash3",
			CampaignID: "campaign1",
			StackTrace: `#0  0x0000555555554a1e in vulnerable_function () at vulnerable.c:42
#1  0x0000555555554b2f in process_input () at main.c:156
#2  0x0000555555554c40 in main (argc=2, argv=0x7fffffffe3d8) at main.c:200`,
		}

		// Mock no existing crash group
		mockStorage.On("GetCrashGroup", ctx, crash.CampaignID, mock.Anything).
			Return(nil, common.ErrKeyNotFound).Once()

		// Mock creating new crash group
		mockStorage.On("CreateCrashGroup", ctx, mock.Anything).Return(nil).Once()
		mockStorage.On("CreateStackTrace", ctx, crash.ID, mock.Anything).Return(nil).Once()
		mockStorage.On("LinkCrashToGroup", ctx, crash.ID, mock.Anything).Return(nil).Once()
		mockStorage.On("UpdateCrash", ctx, mock.Anything).Return(nil).Once()

		group, isNew, err := ds.ProcessCrash(ctx, crash)
		assert.NoError(t, err)
		assert.True(t, isNew)
		assert.NotNil(t, group)
		mockStorage.AssertExpectations(t)
	})

	t.Run("process crash with LibFuzzer trace", func(t *testing.T) {
		mockStorage := new(MockDedupStorage)
		ds := NewDeduplicationService(mockStorage, logger)

		crash := &common.CrashResult{
			ID:         "crash4",
			CampaignID: "campaign1",
			StackTrace: `==12345== ERROR: libFuzzer: deadly signal
    #0 0x51dce0 in __sanitizer_print_stack_trace
    #1 0x468fc8 in fuzzer::PrintStackTrace()
    #2 0x44f862 in fuzzer::Fuzzer::CrashCallback()
    #3 0x44f901 in fuzzer::Fuzzer::StaticCrashSignalCallback()
    #4 0x7f9c42d8b3bf  (/lib/x86_64-linux-gnu/libpthread.so.0+0x153bf)
    #5 0x506cee in vulnerable_function /src/fuzz_target.cc:10:5`,
		}

		// Mock no existing crash group
		mockStorage.On("GetCrashGroup", ctx, crash.CampaignID, mock.Anything).
			Return(nil, common.ErrKeyNotFound).Once()

		// Mock creating new crash group
		mockStorage.On("CreateCrashGroup", ctx, mock.Anything).Return(nil).Once()
		mockStorage.On("CreateStackTrace", ctx, crash.ID, mock.Anything).Return(nil).Once()
		mockStorage.On("LinkCrashToGroup", ctx, crash.ID, mock.Anything).Return(nil).Once()
		mockStorage.On("UpdateCrash", ctx, mock.Anything).Return(nil).Once()

		group, isNew, err := ds.ProcessCrash(ctx, crash)
		assert.NoError(t, err)
		assert.True(t, isNew)
		assert.NotNil(t, group)
		mockStorage.AssertExpectations(t)
	})

	t.Run("invalid stack trace", func(t *testing.T) {
		mockStorage := new(MockDedupStorage)
		ds := NewDeduplicationService(mockStorage, logger)

		crash := &common.CrashResult{
			ID:         "crash5",
			CampaignID: "campaign1",
			StackTrace: "Invalid stack trace format",
		}

		group, isNew, err := ds.ProcessCrash(ctx, crash)
		assert.Error(t, err)
		assert.Equal(t, common.ErrInvalidStackTrace, err)
		assert.Nil(t, group)
		assert.False(t, isNew)
	})
}

func TestDeduplicationService_parseStackTrace(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	mockStorage := new(MockDedupStorage)
	ds := &dedupService{
		storage:    mockStorage,
		logger:     logger,
		topNFrames: 5,
	}

	t.Run("parse ASAN trace", func(t *testing.T) {
		trace := `    #0 0x7f8a6b1234ab in vulnerable_function /src/vulnerable.c:42:5
    #1 0x7f8a6b1235bc in process_input /src/main.c:156:3
    #2 0x7f8a6b1236cd in main /src/main.c:200:5`

		st, err := ds.parseStackTrace(trace)
		assert.NoError(t, err)
		assert.Len(t, st.Frames, 3)
		assert.Equal(t, "vulnerable_function", st.Frames[0].Function)
		assert.Equal(t, "/src/vulnerable.c", st.Frames[0].File)
		assert.Equal(t, 42, st.Frames[0].Line)
		assert.Equal(t, uint64(0x7f8a6b1234ab), st.Frames[0].Offset)
	})

	t.Run("parse GDB trace", func(t *testing.T) {
		trace := `#0  0x0000555555554a1e in vulnerable_function () at vulnerable.c:42
#1  0x0000555555554b2f in process_input () at main.c:156`

		st, err := ds.parseStackTrace(trace)
		assert.NoError(t, err)
		assert.Len(t, st.Frames, 2)
		assert.Equal(t, "vulnerable_function", st.Frames[0].Function)
		assert.Equal(t, "vulnerable.c", st.Frames[0].File)
		assert.Equal(t, 42, st.Frames[0].Line)
	})

	t.Run("parse simple trace", func(t *testing.T) {
		trace := `vulnerable_function
process_input
main`

		st, err := ds.parseStackTrace(trace)
		assert.NoError(t, err)
		assert.Len(t, st.Frames, 3)
		assert.Equal(t, "vulnerable_function", st.Frames[0].Function)
		assert.Empty(t, st.Frames[0].File)
		assert.Equal(t, 0, st.Frames[0].Line)
	})
}

func TestDeduplicationService_computeStackHash(t *testing.T) {
	logger := logrus.New()
	mockStorage := new(MockDedupStorage)
	ds := &dedupService{
		storage:    mockStorage,
		logger:     logger,
		topNFrames: 5,
	}

	frames := []common.StackFrame{
		{Function: "func1", File: "file1.c", Line: 10},
		{Function: "func2", File: "file2.c", Line: 20},
		{Function: "func3", File: "file3.c", Line: 30},
		{Function: "func4", File: "file4.c", Line: 40},
		{Function: "func5", File: "file5.c", Line: 50},
		{Function: "func6", File: "file6.c", Line: 60},
	}

	t.Run("compute hash for top 5 frames", func(t *testing.T) {
		hash := ds.computeStackHash(frames, 5)
		assert.NotEmpty(t, hash)

		// Verify same frames produce same hash
		hash2 := ds.computeStackHash(frames, 5)
		assert.Equal(t, hash, hash2)
	})

	t.Run("compute hash for all frames", func(t *testing.T) {
		hashAll := ds.computeStackHash(frames, -1)
		assert.NotEmpty(t, hashAll)

		// Should be different from top 5
		hash5 := ds.computeStackHash(frames, 5)
		assert.NotEqual(t, hashAll, hash5)
	})

	t.Run("compute hash for fewer frames than requested", func(t *testing.T) {
		shortFrames := frames[:3]
		hash := ds.computeStackHash(shortFrames, 5)
		assert.NotEmpty(t, hash)
	})
}

func TestDeduplicationService_GetCrashGroups(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	mockStorage := new(MockDedupStorage)
	ds := NewDeduplicationService(mockStorage, logger)

	t.Run("get crash groups for campaign", func(t *testing.T) {
		campaignID := "campaign1"
		expectedGroups := []*common.CrashGroup{
			{
				ID:         "group1",
				CampaignID: campaignID,
				StackHash:  "hash1",
				Count:      10,
				Severity:   "high",
			},
			{
				ID:         "group2",
				CampaignID: campaignID,
				StackHash:  "hash2",
				Count:      5,
				Severity:   "medium",
			},
		}

		mockStorage.On("ListCrashGroups", ctx, campaignID).Return(expectedGroups, nil).Once()

		groups, err := ds.GetCrashGroups(ctx, campaignID)
		assert.NoError(t, err)
		assert.Equal(t, expectedGroups, groups)
		mockStorage.AssertExpectations(t)
	})
}

func TestDeduplicationService_computeSeverity(t *testing.T) {
	logger := logrus.New()
	mockStorage := new(MockDedupStorage)
	ds := &dedupService{
		storage:    mockStorage,
		logger:     logger,
		topNFrames: 5,
	}

	testCases := []struct {
		name     string
		crash    *common.CrashResult
		expected string
	}{
		{
			name:     "heap overflow",
			crash:    &common.CrashResult{Type: "heap_overflow"},
			expected: "critical",
		},
		{
			name:     "stack overflow",
			crash:    &common.CrashResult{Type: "stack_overflow"},
			expected: "critical",
		},
		{
			name:     "use after free",
			crash:    &common.CrashResult{Type: "use_after_free"},
			expected: "critical",
		},
		{
			name:     "double free",
			crash:    &common.CrashResult{Type: "double_free"},
			expected: "critical",
		},
		{
			name:     "null dereference",
			crash:    &common.CrashResult{Type: "null_deref"},
			expected: "medium",
		},
		{
			name:     "segmentation fault",
			crash:    &common.CrashResult{Type: "segfault"},
			expected: "high",
		},
		{
			name:     "assertion failure",
			crash:    &common.CrashResult{Type: "assertion"},
			expected: "low",
		},
		{
			name:     "timeout",
			crash:    &common.CrashResult{Type: "timeout"},
			expected: "low",
		},
		{
			name:     "unknown type",
			crash:    &common.CrashResult{Type: "unknown"},
			expected: "medium",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			severity := ds.computeSeverity(tc.crash)
			assert.Equal(t, tc.expected, severity)
		})
	}
}
