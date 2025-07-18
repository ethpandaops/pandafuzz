package api_v3

import (
	"context"

	"github.com/ethpandaops/pandafuzz/pkg/common"
)

// CorpusServiceAdapter adapts the common.CorpusService interface to match API v3 expectations
type CorpusServiceAdapter struct {
	service common.CorpusService
}

// NewCorpusServiceAdapter creates a new adapter
func NewCorpusServiceAdapter(service common.CorpusService) *CorpusServiceAdapter {
	return &CorpusServiceAdapter{service: service}
}

// ListCorpusFiles lists corpus files for a campaign
func (a *CorpusServiceAdapter) ListCorpusFiles(ctx context.Context, campaignID string) ([]*common.CorpusFile, error) {
	// Use SyncCorpus with empty botID to get all files
	return a.service.SyncCorpus(ctx, campaignID, "")
}

// UploadCorpusFile uploads a new corpus file
func (a *CorpusServiceAdapter) UploadCorpusFile(ctx context.Context, file *common.CorpusFile) (*common.CorpusFile, error) {
	err := a.service.AddFile(ctx, file)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// GetCorpusFile retrieves a corpus file by ID
func (a *CorpusServiceAdapter) GetCorpusFile(ctx context.Context, fileID string) (*common.CorpusFile, error) {
	// TODO: This operation is not directly supported by the interface
	// Would need to be implemented through the storage layer
	return nil, common.ErrNotImplemented
}

// DeleteCorpusFile deletes a corpus file
func (a *CorpusServiceAdapter) DeleteCorpusFile(ctx context.Context, fileID string) error {
	// TODO: This operation is not directly supported by the interface
	// Would need to be implemented through the storage layer
	return common.ErrNotImplemented
}
