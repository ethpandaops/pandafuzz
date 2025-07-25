package quarantine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/repository"
	"github.com/ethpandaops/pandafuzz/pkg/domain/corpus/types"
	crashtypes "github.com/ethpandaops/pandafuzz/pkg/domain/crash/types"
)

// QuarantineReason represents the reason why a corpus entry was quarantined
type QuarantineReason string

const (
	ReasonCrashCausing     QuarantineReason = "crash_causing"
	ReasonTimeout          QuarantineReason = "timeout"
	ReasonExcessiveMemory  QuarantineReason = "excessive_memory"
	ReasonMalformed        QuarantineReason = "malformed"
	ReasonSlowExecution    QuarantineReason = "slow_execution"
	ReasonManualQuarantine QuarantineReason = "manual_quarantine"
	ReasonRepeatedFailures QuarantineReason = "repeated_failures"
)

// QuarantineEntry represents a quarantined corpus entry with metadata
type QuarantineEntry struct {
	CorpusEntry   *types.CorpusEntry `json:"corpus_entry"`
	QuarantinedAt time.Time          `json:"quarantined_at"`
	Reason        QuarantineReason   `json:"reason"`
	Details       string             `json:"details"`
	ReviewCount   int                `json:"review_count"`
	LastReviewAt  *time.Time         `json:"last_review_at,omitempty"`
	ReleasedAt    *time.Time         `json:"released_at,omitempty"`
	PermanentBan  bool               `json:"permanent_ban"`
	Metadata      map[string]string  `json:"metadata,omitempty"`
}

// ExecutionResult represents the result of executing a corpus entry
type ExecutionResult struct {
	EntryID        string
	Success        bool
	Crashed        bool
	TimedOut       bool
	ExecutionTime  time.Duration
	MemoryUsage    int64
	Error          error
	CrashSignature string
	CrashType      crashtypes.CrashType
}

// QuarantineEventType represents types of quarantine events
type QuarantineEventType string

const (
	EventEntryQuarantined QuarantineEventType = "corpus.quarantined"
	EventEntryReleased    QuarantineEventType = "corpus.released"
	EventEntryReviewed    QuarantineEventType = "corpus.reviewed"
	EventQuarantineFailed QuarantineEventType = "corpus.quarantine_failed"
)

// QuarantineEvent represents a quarantine-related event
type QuarantineEvent struct {
	Type      QuarantineEventType `json:"type"`
	EntryID   string              `json:"entry_id"`
	Reason    QuarantineReason    `json:"reason"`
	Details   string              `json:"details"`
	Timestamp time.Time           `json:"timestamp"`
}

// EventPublisher interface for publishing quarantine events
type EventPublisher interface {
	PublishEvent(event QuarantineEvent) error
}

// Service provides quarantine management functionality
type Service struct {
	mu                sync.RWMutex
	corpusRepo        repository.CorpusEntryRepository
	quarantineRepo    repository.CorpusCollectionRepository
	eventPublisher    EventPublisher
	rules             *Rules
	quarantineEntries map[string]*QuarantineEntry // In-memory cache
}

// NewService creates a new quarantine service
func NewService(
	corpusRepo repository.CorpusEntryRepository,
	quarantineRepo repository.CorpusCollectionRepository,
	eventPublisher EventPublisher,
	rules *Rules,
) (*Service, error) {
	if corpusRepo == nil {
		return nil, errors.New("corpus repository cannot be nil")
	}
	if quarantineRepo == nil {
		return nil, errors.New("quarantine repository cannot be nil")
	}
	if rules == nil {
		return nil, errors.New("quarantine rules cannot be nil")
	}

	return &Service{
		corpusRepo:        corpusRepo,
		quarantineRepo:    quarantineRepo,
		eventPublisher:    eventPublisher,
		rules:             rules,
		quarantineEntries: make(map[string]*QuarantineEntry),
	}, nil
}

// QuarantineEntry quarantines a corpus entry based on the provided reason
func (s *Service) QuarantineEntry(ctx context.Context, entryID string, reason QuarantineReason, details string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already quarantined
	if _, exists := s.quarantineEntries[entryID]; exists {
		return errors.New("entry is already quarantined")
	}

	// Fetch the corpus entry
	entry, err := s.corpusRepo.FindByID(ctx, entryID)
	if err != nil {
		return fmt.Errorf("failed to find corpus entry: %w", err)
	}

	// Create quarantine entry
	qEntry := &QuarantineEntry{
		CorpusEntry:   entry,
		QuarantinedAt: time.Now().UTC(),
		Reason:        reason,
		Details:       details,
		ReviewCount:   0,
		PermanentBan:  s.rules.IsPermanentBanReason(reason),
		Metadata:      make(map[string]string),
	}

	// Add quarantine tag to the entry
	entry.AddTag("quarantined")
	entry.SetMetadata("quarantine_reason", string(reason))
	entry.SetMetadata("quarantine_details", details)

	// Update entry in corpus repository
	if err := s.corpusRepo.Update(ctx, entry); err != nil {
		return fmt.Errorf("failed to update corpus entry: %w", err)
	}

	// Add to quarantine collection
	if err := s.quarantineRepo.AddEntryToCollection(ctx, "quarantine", entryID); err != nil {
		// Rollback the tag update
		entry.RemoveTag("quarantined")
		s.corpusRepo.Update(ctx, entry)
		return fmt.Errorf("failed to add to quarantine collection: %w", err)
	}

	// Store in memory
	s.quarantineEntries[entryID] = qEntry

	// Publish event
	if s.eventPublisher != nil {
		event := QuarantineEvent{
			Type:      EventEntryQuarantined,
			EntryID:   entryID,
			Reason:    reason,
			Details:   details,
			Timestamp: time.Now().UTC(),
		}
		s.eventPublisher.PublishEvent(event)
	}

	return nil
}

// ReleaseEntry releases a corpus entry from quarantine
func (s *Service) ReleaseEntry(ctx context.Context, entryID string, reviewNotes string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	qEntry, exists := s.quarantineEntries[entryID]
	if !exists {
		return errors.New("entry is not in quarantine")
	}

	// Check if permanently banned
	if qEntry.PermanentBan {
		return errors.New("entry is permanently banned and cannot be released")
	}

	// Check if entry meets release criteria
	if !s.rules.CanRelease(qEntry) {
		return errors.New("entry does not meet release criteria")
	}

	// Update corpus entry
	entry := qEntry.CorpusEntry
	entry.RemoveTag("quarantined")
	entry.SetMetadata("release_notes", reviewNotes)
	entry.SetMetadata("released_at", time.Now().UTC().Format(time.RFC3339))

	// Update in repository
	if err := s.corpusRepo.Update(ctx, entry); err != nil {
		return fmt.Errorf("failed to update corpus entry: %w", err)
	}

	// Remove from quarantine collection
	if err := s.quarantineRepo.RemoveEntryFromCollection(ctx, "quarantine", entryID); err != nil {
		// Rollback the tag removal
		entry.AddTag("quarantined")
		s.corpusRepo.Update(ctx, entry)
		return fmt.Errorf("failed to remove from quarantine collection: %w", err)
	}

	// Update quarantine entry
	now := time.Now().UTC()
	qEntry.ReleasedAt = &now

	// Remove from memory after updating
	delete(s.quarantineEntries, entryID)

	// Publish event
	if s.eventPublisher != nil {
		event := QuarantineEvent{
			Type:      EventEntryReleased,
			EntryID:   entryID,
			Reason:    qEntry.Reason,
			Details:   reviewNotes,
			Timestamp: now,
		}
		s.eventPublisher.PublishEvent(event)
	}

	return nil
}

// ReviewEntry reviews a quarantined entry and updates its status
func (s *Service) ReviewEntry(ctx context.Context, entryID string, reviewNotes string) (*QuarantineEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	qEntry, exists := s.quarantineEntries[entryID]
	if !exists {
		return nil, errors.New("entry is not in quarantine")
	}

	// Update review information
	now := time.Now().UTC()
	qEntry.ReviewCount++
	qEntry.LastReviewAt = &now
	qEntry.Metadata[fmt.Sprintf("review_%d", qEntry.ReviewCount)] = reviewNotes
	qEntry.Metadata[fmt.Sprintf("review_%d_time", qEntry.ReviewCount)] = now.Format(time.RFC3339)

	// Check if entry should be permanently banned based on review count
	if s.rules.ShouldPermanentlyBan(qEntry) {
		qEntry.PermanentBan = true
		qEntry.Metadata["permanent_ban_reason"] = "exceeded maximum review count"
	}

	// Publish event
	if s.eventPublisher != nil {
		event := QuarantineEvent{
			Type:      EventEntryReviewed,
			EntryID:   entryID,
			Reason:    qEntry.Reason,
			Details:   reviewNotes,
			Timestamp: now,
		}
		s.eventPublisher.PublishEvent(event)
	}

	return qEntry, nil
}

// ProcessExecutionResult processes an execution result and quarantines if necessary
func (s *Service) ProcessExecutionResult(ctx context.Context, result ExecutionResult) error {
	// Check if the entry should be quarantined based on execution result
	reason, details := s.rules.ShouldQuarantine(result)
	if reason == "" {
		return nil // No quarantine needed
	}

	// Check if entry is already quarantined
	s.mu.RLock()
	_, alreadyQuarantined := s.quarantineEntries[result.EntryID]
	s.mu.RUnlock()

	if alreadyQuarantined {
		return nil // Already in quarantine
	}

	// Quarantine the entry
	return s.QuarantineEntry(ctx, result.EntryID, reason, details)
}

// GetQuarantinedEntries returns all quarantined entries
func (s *Service) GetQuarantinedEntries() []*QuarantineEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]*QuarantineEntry, 0, len(s.quarantineEntries))
	for _, entry := range s.quarantineEntries {
		entries = append(entries, entry)
	}
	return entries
}

// GetQuarantinedEntry returns a specific quarantined entry
func (s *Service) GetQuarantinedEntry(entryID string) (*QuarantineEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.quarantineEntries[entryID]
	return entry, exists
}

// GetQuarantineHistory returns the quarantine history for an entry
func (s *Service) GetQuarantineHistory(entryID string) ([]QuarantineEvent, error) {
	// In a real implementation, this would query an event store
	// For now, we'll return a simplified version
	history := []QuarantineEvent{}

	s.mu.RLock()
	qEntry, exists := s.quarantineEntries[entryID]
	s.mu.RUnlock()

	if exists {
		// Add quarantine event
		history = append(history, QuarantineEvent{
			Type:      EventEntryQuarantined,
			EntryID:   entryID,
			Reason:    qEntry.Reason,
			Details:   qEntry.Details,
			Timestamp: qEntry.QuarantinedAt,
		})

		// Add review events
		for i := 1; i <= qEntry.ReviewCount; i++ {
			if reviewNote, ok := qEntry.Metadata[fmt.Sprintf("review_%d", i)]; ok {
				if reviewTime, ok := qEntry.Metadata[fmt.Sprintf("review_%d_time", i)]; ok {
					if t, err := time.Parse(time.RFC3339, reviewTime); err == nil {
						history = append(history, QuarantineEvent{
							Type:      EventEntryReviewed,
							EntryID:   entryID,
							Reason:    qEntry.Reason,
							Details:   reviewNote,
							Timestamp: t,
						})
					}
				}
			}
		}

		// Add release event if released
		if qEntry.ReleasedAt != nil {
			history = append(history, QuarantineEvent{
				Type:      EventEntryReleased,
				EntryID:   entryID,
				Reason:    qEntry.Reason,
				Details:   "Entry released from quarantine",
				Timestamp: *qEntry.ReleasedAt,
			})
		}
	}

	return history, nil
}

// IsQuarantined checks if an entry is currently quarantined
func (s *Service) IsQuarantined(entryID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.quarantineEntries[entryID]
	return exists
}

// LoadQuarantinedEntries loads quarantined entries from the repository
func (s *Service) LoadQuarantinedEntries(ctx context.Context) error {
	// Get all entries from the quarantine collection
	entries, err := s.quarantineRepo.GetCollectionEntries(ctx, "quarantine")
	if err != nil {
		return fmt.Errorf("failed to load quarantine collection: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear existing entries
	s.quarantineEntries = make(map[string]*QuarantineEntry)

	// Load each entry
	for _, entry := range entries {
		// Extract quarantine metadata
		reason := QuarantineReason(entry.Metadata["quarantine_reason"])
		details := entry.Metadata["quarantine_details"]

		// Parse quarantine time
		quarantinedAt := entry.CreatedAt
		if qtStr, ok := entry.Metadata["quarantined_at"]; ok {
			if qt, err := time.Parse(time.RFC3339, qtStr); err == nil {
				quarantinedAt = qt
			}
		}

		qEntry := &QuarantineEntry{
			CorpusEntry:   entry,
			QuarantinedAt: quarantinedAt,
			Reason:        reason,
			Details:       details,
			ReviewCount:   0,
			PermanentBan:  s.rules.IsPermanentBanReason(reason),
			Metadata:      make(map[string]string),
		}

		// Extract review count and other metadata
		for k, v := range entry.Metadata {
			if k != "quarantine_reason" && k != "quarantine_details" && k != "quarantined_at" {
				qEntry.Metadata[k] = v
			}
		}

		s.quarantineEntries[entry.ID] = qEntry
	}

	return nil
}

// CleanupExpiredQuarantines removes entries that have been quarantined beyond the retention period
func (s *Service) CleanupExpiredQuarantines(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	expiredEntries := []string{}
	now := time.Now().UTC()

	for id, qEntry := range s.quarantineEntries {
		if s.rules.IsExpired(qEntry, now) && !qEntry.PermanentBan {
			expiredEntries = append(expiredEntries, id)
		}
	}

	// Release expired entries
	for _, entryID := range expiredEntries {
		// Remove quarantine tag
		if entry, err := s.corpusRepo.FindByID(ctx, entryID); err == nil {
			entry.RemoveTag("quarantined")
			entry.SetMetadata("auto_released", "true")
			entry.SetMetadata("auto_release_time", now.Format(time.RFC3339))
			s.corpusRepo.Update(ctx, entry)
		}

		// Remove from quarantine collection
		s.quarantineRepo.RemoveEntryFromCollection(ctx, "quarantine", entryID)

		// Remove from memory
		delete(s.quarantineEntries, entryID)
	}

	return nil
}
