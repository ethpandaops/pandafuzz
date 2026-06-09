package service

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockCorpusStorage is a mock implementation for corpus storage
type MockCorpusStorage struct {
	MockStorage
}

func (m *MockCorpusStorage) AddCorpusFile(ctx context.Context, cf *common.CorpusFile) error {
	args := m.Called(ctx, cf)
	return args.Error(0)
}

func (m *MockCorpusStorage) GetCorpusFiles(ctx context.Context, campaignID string) ([]*common.CorpusFile, error) {
	args := m.Called(ctx, campaignID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CorpusFile), args.Error(1)
}

func (m *MockCorpusStorage) GetCorpusFileByHash(ctx context.Context, hash string) (*common.CorpusFile, error) {
	args := m.Called(ctx, hash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*common.CorpusFile), args.Error(1)
}

func (m *MockCorpusStorage) UpdateCorpusCoverage(ctx context.Context, id string, coverage, newCoverage int64) error {
	args := m.Called(ctx, id, coverage, newCoverage)
	return args.Error(0)
}

func (m *MockCorpusStorage) RecordCorpusEvolution(ctx context.Context, ce *common.CorpusEvolution) error {
	args := m.Called(ctx, ce)
	return args.Error(0)
}

func (m *MockCorpusStorage) GetCorpusEvolution(ctx context.Context, campaignID string, limit int) ([]*common.CorpusEvolution, error) {
	args := m.Called(ctx, campaignID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CorpusEvolution), args.Error(1)
}

func (m *MockCorpusStorage) GetUnsyncedCorpusFiles(ctx context.Context, campaignID, botID string) ([]*common.CorpusFile, error) {
	args := m.Called(ctx, campaignID, botID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*common.CorpusFile), args.Error(1)
}

func (m *MockCorpusStorage) MarkCorpusFilesSynced(ctx context.Context, fileIDs []string, botID string) error {
	args := m.Called(ctx, fileIDs, botID)
	return args.Error(0)
}

// MockFileStorage is a mock implementation for file storage
type MockFileStorage struct {
	mock.Mock
}

func (m *MockFileStorage) Store(ctx context.Context, path string, data []byte) error {
	args := m.Called(ctx, path, data)
	return args.Error(0)
}

func (m *MockFileStorage) Retrieve(ctx context.Context, path string) ([]byte, error) {
	args := m.Called(ctx, path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockFileStorage) Delete(ctx context.Context, path string) error {
	args := m.Called(ctx, path)
	return args.Error(0)
}

func (m *MockFileStorage) Exists(ctx context.Context, path string) (bool, error) {
	args := m.Called(ctx, path)
	return args.Bool(0), args.Error(1)
}

func (m *MockFileStorage) List(ctx context.Context, prefix string) ([]string, error) {
	args := m.Called(ctx, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Implement the FileStorage interface methods
func (m *MockFileStorage) SaveFile(ctx context.Context, path string, data []byte) error {
	args := m.Called(ctx, path, data)
	return args.Error(0)
}

func (m *MockFileStorage) SaveFileStream(ctx context.Context, path string, reader io.Reader, size int64) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	args := m.Called(ctx, path, data, size)
	return args.Error(0)
}

func (m *MockFileStorage) ReadFile(ctx context.Context, path string) ([]byte, error) {
	args := m.Called(ctx, path)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockFileStorage) DeleteFile(ctx context.Context, path string) error {
	args := m.Called(ctx, path)
	return args.Error(0)
}

func (m *MockFileStorage) ListFiles(ctx context.Context, prefix string) ([]string, error) {
	args := m.Called(ctx, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockFileStorage) FileExists(ctx context.Context, path string) (bool, error) {
	args := m.Called(ctx, path)
	return args.Bool(0), args.Error(1)
}

func newCorpusService(t *testing.T, storage common.Storage, fileStorage common.FileStorage, corpusDir string, logger logrus.FieldLogger) common.CorpusService {
	t.Helper()

	cs, err := NewCorpusService(storage, fileStorage, corpusDir, logger)
	require.NoError(t, err)
	return cs
}

func TestCorpusService_AddFile(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("add new corpus file", func(t *testing.T) {
		mockStorage := new(MockCorpusStorage)
		mockFileStorage := new(MockFileStorage)
		cs := newCorpusService(t, mockStorage, mockFileStorage, "/tmp/test-corpus", logger)
		service := cs.(*corpusService)
		service.quarantine = nil
		service.enableMetrics = false

		file := &common.CorpusFile{
			CampaignID:  "campaign1",
			JobID:       "job1",
			BotID:       "bot1",
			Filename:    "test_input_001",
			Hash:        "abc123def456",
			Size:        1024,
			Coverage:    5000,
			NewCoverage: 100,
			IsSeed:      false,
		}

		// Mock checking if file already exists
		mockStorage.On("GetCorpusFileByHash", ctx, file.Hash).
			Return(nil, common.ErrKeyNotFound).Once()

		// Mock adding corpus file to database
		mockStorage.On("AddCorpusFile", ctx, mock.MatchedBy(func(cf *common.CorpusFile) bool {
			return cf.Hash == file.Hash && cf.ID != "" && !cf.CreatedAt.IsZero()
		})).Return(nil).Once()

		// Mock tracking evolution
		mockStorage.On("GetCorpusFiles", ctx, file.CampaignID).Return([]*common.CorpusFile{file}, nil).Once()
		mockStorage.On("GetCorpusEvolution", ctx, file.CampaignID, 1).Return([]*common.CorpusEvolution{}, nil).Once()
		mockStorage.On("RecordCorpusEvolution", ctx, mock.MatchedBy(func(ce *common.CorpusEvolution) bool {
			return ce.CampaignID == file.CampaignID && ce.NewFiles == 1 && ce.NewCoverage == file.NewCoverage
		})).Return(nil).Once()

		err := cs.AddFile(ctx, file)
		assert.NoError(t, err)
		assert.NotEmpty(t, file.ID)
		assert.NotZero(t, file.CreatedAt)
		mockStorage.AssertExpectations(t)
	})

	t.Run("add duplicate corpus file", func(t *testing.T) {
		mockStorage := new(MockCorpusStorage)
		mockFileStorage := new(MockFileStorage)
		cs := newCorpusService(t, mockStorage, mockFileStorage, "/tmp/test-corpus", logger)

		file := &common.CorpusFile{
			CampaignID: "campaign1",
			Filename:   "existing_input",
			Hash:       "existing_hash",
		}

		existingFile := &common.CorpusFile{
			ID:   "existing_id",
			Hash: file.Hash,
		}

		// Mock finding existing file
		mockStorage.On("GetQuarantinedFile", ctx, mock.Anything).Return(nil, common.ErrQuarantinedFileNotFound).Once()
		mockStorage.On("GetCorpusFileByHash", ctx, file.Hash).
			Return(existingFile, nil).Once()

		err := cs.AddFile(ctx, file)
		assert.Error(t, err)
		assert.Equal(t, common.ErrDuplicateCorpusFile, err)
		mockStorage.AssertExpectations(t)
	})

	t.Run("add file missing filename", func(t *testing.T) {
		mockStorage := new(MockCorpusStorage)
		mockFileStorage := new(MockFileStorage)
		cs := newCorpusService(t, mockStorage, mockFileStorage, "/tmp/test-corpus", logger)

		file := &common.CorpusFile{
			CampaignID: "campaign1",
			Hash:       "large_file_hash",
			Size:       11 * 1024 * 1024, // 11MB, exceeds default 10MB limit
		}

		err := cs.AddFile(ctx, file)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "filename is required")
	})
}

func TestCorpusService_GetEvolution(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	mockStorage := new(MockCorpusStorage)
	mockFileStorage := new(MockFileStorage)
	cs := newCorpusService(t, mockStorage, mockFileStorage, "/tmp/test-corpus", logger)

	t.Run("get corpus evolution history", func(t *testing.T) {
		campaignID := "campaign1"
		now := time.Now()

		expectedEvolution := []*common.CorpusEvolution{
			{
				CampaignID:    campaignID,
				Timestamp:     now.Add(-2 * time.Hour),
				TotalFiles:    100,
				TotalSize:     1024 * 1024 * 50,
				TotalCoverage: 10000,
				NewFiles:      10,
				NewCoverage:   500,
			},
			{
				CampaignID:    campaignID,
				Timestamp:     now.Add(-1 * time.Hour),
				TotalFiles:    110,
				TotalSize:     1024 * 1024 * 55,
				TotalCoverage: 10500,
				NewFiles:      10,
				NewCoverage:   500,
			},
			{
				CampaignID:    campaignID,
				Timestamp:     now,
				TotalFiles:    120,
				TotalSize:     1024 * 1024 * 60,
				TotalCoverage: 11000,
				NewFiles:      10,
				NewCoverage:   500,
			},
		}

		mockStorage.On("GetCorpusEvolution", ctx, campaignID, 1000).
			Return(expectedEvolution, nil).Once()

		evolution, err := cs.GetEvolution(ctx, campaignID)
		assert.NoError(t, err)
		assert.Equal(t, expectedEvolution, evolution)
		mockStorage.AssertExpectations(t)
	})
}

func TestCorpusService_SyncCorpus(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("sync corpus files to bot", func(t *testing.T) {
		mockStorage := new(MockCorpusStorage)
		mockFileStorage := new(MockFileStorage)
		cs := newCorpusService(t, mockStorage, mockFileStorage, "/tmp/test-corpus", logger)

		campaignID := "campaign1"
		botID := "bot1"

		unsyncedFiles := []*common.CorpusFile{
			{
				ID:         "file1",
				CampaignID: campaignID,
				Hash:       "hash1",
				Filename:   "input1",
			},
			{
				ID:         "file2",
				CampaignID: campaignID,
				Hash:       "hash2",
				Filename:   "input2",
			},
		}

		// Mock getting unsynced files
		mockStorage.On("GetUnsyncedCorpusFiles", ctx, campaignID, botID).
			Return(unsyncedFiles, nil).Once()
		mockStorage.On("GetQuarantinedFile", ctx, "file1").Return(nil, common.ErrQuarantinedFileNotFound).Once()
		mockStorage.On("GetQuarantinedFile", ctx, "file2").Return(nil, common.ErrQuarantinedFileNotFound).Once()

		// Mock marking files as synced
		fileIDs := []string{"file1", "file2"}
		mockStorage.On("MarkCorpusFilesSynced", ctx, fileIDs, botID).
			Return(nil).Once()

		files, err := cs.SyncCorpus(ctx, campaignID, botID)
		assert.NoError(t, err)
		assert.Equal(t, unsyncedFiles, files)
		mockStorage.AssertExpectations(t)
	})

	t.Run("no files to sync", func(t *testing.T) {
		mockStorage := new(MockCorpusStorage)
		mockFileStorage := new(MockFileStorage)
		cs := newCorpusService(t, mockStorage, mockFileStorage, "/tmp/test-corpus", logger)

		campaignID := "campaign1"
		botID := "bot1"

		// Mock getting unsynced files - empty result
		mockStorage.On("GetUnsyncedCorpusFiles", ctx, campaignID, botID).
			Return([]*common.CorpusFile{}, nil).Once()

		files, err := cs.SyncCorpus(ctx, campaignID, botID)
		assert.NoError(t, err)
		assert.Empty(t, files)
		mockStorage.AssertExpectations(t)
	})
}

func TestCorpusService_ShareCorpus(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("share corpus between campaigns", func(t *testing.T) {
		mockStorage := new(MockCorpusStorage)
		mockFileStorage := new(MockFileStorage)
		cs := newCorpusService(t, mockStorage, mockFileStorage, "/tmp/test-corpus", logger)
		service := cs.(*corpusService)
		service.quarantine = nil
		service.enableMetrics = false

		fromCampaign := "campaign1"
		toCampaign := "campaign2"

		// Source campaign files
		sourceFiles := []*common.CorpusFile{
			{
				ID:          "file1",
				CampaignID:  fromCampaign,
				Hash:        "hash1",
				Filename:    "input1",
				Coverage:    5000,
				NewCoverage: 100,
			},
			{
				ID:          "file2",
				CampaignID:  fromCampaign,
				Hash:        "hash2",
				Filename:    "input2",
				Coverage:    6000,
				NewCoverage: 200,
			},
		}

		campaignHash := "binary-hash"
		mockStorage.On("GetCampaign", ctx, fromCampaign).Return(&common.Campaign{
			ID:         fromCampaign,
			BinaryHash: campaignHash,
		}, nil).Once()
		mockStorage.On("GetCampaign", ctx, toCampaign).Return(&common.Campaign{
			ID:         toCampaign,
			BinaryHash: campaignHash,
		}, nil).Once()

		// Mock getting source campaign files
		mockStorage.On("GetCorpusFiles", ctx, fromCampaign).
			Return(sourceFiles, nil).Once()
		mockStorage.On("GetCorpusFiles", ctx, toCampaign).
			Return([]*common.CorpusFile{}, nil).Once()

		// For each file, check if it exists in target campaign
		for _, file := range sourceFiles {
			mockStorage.On("GetCorpusFileByHash", ctx, file.Hash).
				Return(nil, common.ErrKeyNotFound).Once()

			// Mock adding file to target campaign
			mockStorage.On("AddCorpusFile", ctx, mock.MatchedBy(func(cf *common.CorpusFile) bool {
				return cf.CampaignID == toCampaign && cf.Hash == file.Hash
			})).Return(nil).Once()
		}

		err := cs.ShareCorpus(ctx, fromCampaign, toCampaign)
		assert.NoError(t, err)
		mockStorage.AssertExpectations(t)
	})

	t.Run("share corpus with some duplicates", func(t *testing.T) {
		mockStorage := new(MockCorpusStorage)
		mockFileStorage := new(MockFileStorage)
		cs := newCorpusService(t, mockStorage, mockFileStorage, "/tmp/test-corpus", logger)
		service := cs.(*corpusService)
		service.quarantine = nil
		service.enableMetrics = false

		fromCampaign := "campaign1"
		toCampaign := "campaign2"

		sourceFiles := []*common.CorpusFile{
			{
				ID:          "file1",
				CampaignID:  fromCampaign,
				Hash:        "hash1",
				Filename:    "input1",
				NewCoverage: 50,
			},
			{
				ID:          "file2",
				CampaignID:  fromCampaign,
				Hash:        "hash2",
				Filename:    "input2",
				NewCoverage: 75,
			},
		}

		campaignHash := "binary-hash"
		mockStorage.On("GetCampaign", ctx, fromCampaign).Return(&common.Campaign{
			ID:         fromCampaign,
			BinaryHash: campaignHash,
		}, nil).Once()
		mockStorage.On("GetCampaign", ctx, toCampaign).Return(&common.Campaign{
			ID:         toCampaign,
			BinaryHash: campaignHash,
		}, nil).Once()

		// Mock getting source campaign files
		mockStorage.On("GetCorpusFiles", ctx, fromCampaign).
			Return(sourceFiles, nil).Once()
		mockStorage.On("GetCorpusFiles", ctx, toCampaign).
			Return([]*common.CorpusFile{
				{
					ID:         "existing",
					CampaignID: toCampaign,
					Hash:       "hash1",
				},
			}, nil).Once()

		// Only share the non-duplicate file
		mockStorage.On("GetCorpusFileByHash", ctx, "hash2").
			Return(nil, common.ErrKeyNotFound).Once()

		// Only add the non-duplicate file
		mockStorage.On("AddCorpusFile", ctx, mock.MatchedBy(func(cf *common.CorpusFile) bool {
			return cf.CampaignID == toCampaign && cf.Hash == "hash2"
		})).Return(nil).Once()

		err := cs.ShareCorpus(ctx, fromCampaign, toCampaign)
		assert.NoError(t, err)
		mockStorage.AssertExpectations(t)
	})
}

func TestCorpusService_trackEvolution(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("track corpus evolution metrics", func(t *testing.T) {
		mockStorage := new(MockCorpusStorage)
		mockFileStorage := new(MockFileStorage)
		cs := &corpusService{
			storage:     mockStorage,
			fileStorage: mockFileStorage,
			corpusDir:   "/tmp/test-corpus",
			logger:      logger,
		}

		campaignID := "campaign1"

		mockStorage.On("GetCorpusFiles", ctx, campaignID).Return([]*common.CorpusFile{}, nil).Once()
		mockStorage.On("GetCorpusEvolution", ctx, campaignID, 1).Return([]*common.CorpusEvolution{}, nil).Once()

		// Mock recording evolution
		mockStorage.On("RecordCorpusEvolution", ctx, mock.MatchedBy(func(ce *common.CorpusEvolution) bool {
			return ce.CampaignID == campaignID &&
				ce.TotalFiles == 0 &&
				ce.TotalSize == 0 &&
				ce.TotalCoverage == 0 &&
				ce.NewFiles == 0 &&
				ce.NewCoverage == 0 &&
				!ce.Timestamp.IsZero()
		})).Return(nil).Once()

		err := cs.trackEvolution(ctx, campaignID)
		assert.NoError(t, err)
		mockStorage.AssertExpectations(t)
	})
}

func TestCorpusService_findCoverageIncreasingFiles(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	t.Run("find files with new coverage", func(t *testing.T) {
		mockStorage := new(MockCorpusStorage)
		mockFileStorage := new(MockFileStorage)
		cs := &corpusService{
			storage:     mockStorage,
			fileStorage: mockFileStorage,
			corpusDir:   "/tmp/test-corpus",
			logger:      logger,
		}

		campaignID := "campaign1"

		allFiles := []*common.CorpusFile{
			{
				ID:          "file1",
				NewCoverage: 100,
			},
			{
				ID:          "file2",
				NewCoverage: 0, // No new coverage
			},
			{
				ID:          "file3",
				NewCoverage: 50,
			},
		}

		// Mock getting all corpus files
		mockStorage.On("GetCorpusFiles", ctx, campaignID).
			Return(allFiles, nil).Once()

		files, err := cs.findCoverageIncreasingFiles(ctx, campaignID)
		assert.NoError(t, err)
		assert.Len(t, files, 2)
		assert.Equal(t, "file1", files[0].ID)
		assert.Equal(t, "file3", files[1].ID)
		mockStorage.AssertExpectations(t)
	})
}
