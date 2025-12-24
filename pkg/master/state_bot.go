package master

import (
	"context"
	"fmt"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/common"
	"github.com/sirupsen/logrus"
)

// Bot operations with retry logic

// SaveBotWithRetry persists a bot to database with retry logic
func (ps *PersistentState) SaveBotWithRetry(ctx context.Context, bot *common.Bot) error {
	return ps.retryManager.Execute(func() error {
		// Acquire lock late
		ps.mu.Lock()
		// Update in-memory state
		ps.bots[bot.ID] = bot
		ps.mu.Unlock()

		// Don't hold lock during database operation
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Persist to database
			if err := tx.Store(ctx, "bot:"+bot.ID, bot); err != nil {
				return common.NewDatabaseError("save_bot", err)
			}

			ps.mu.Lock()
			ps.stats.TransactionCount++
			ps.mu.Unlock()

			ps.logger.WithFields(logrus.Fields{
				"bot_id":   bot.ID,
				"hostname": bot.Hostname,
				"status":   bot.Status,
			}).Debug("Bot saved successfully")

			return nil
		})
	})
}

// GetBot retrieves a bot by ID
func (ps *PersistentState) GetBot(ctx context.Context, botID string) (*common.Bot, error) {
	// Check in-memory cache first with RLock
	ps.mu.RLock()
	if bot, exists := ps.bots[botID]; exists {
		ps.mu.RUnlock()
		// Update access time for LRU
		ps.mu.Lock()
		ps.cacheAccessTime["bot:"+botID] = time.Now()
		ps.mu.Unlock()
		return bot, nil
	}
	ps.mu.RUnlock()

	// Load from database without holding lock
	var bot common.Bot
	err := ps.retryManager.Execute(func() error {
		return ps.db.Get(ctx, "bot:"+botID, &bot)
	})

	if err != nil {
		if common.IsNotFoundError(err) {
			return nil, common.NewValidationError("get_bot", fmt.Errorf("bot not found: %s", botID))
		}
		return nil, common.NewDatabaseError("get_bot", err)
	}

	// Update cache with proper synchronization to avoid race condition
	ps.mu.Lock()
	// Double-check if another goroutine already cached it
	if existingBot, exists := ps.bots[botID]; exists {
		ps.mu.Unlock()
		return existingBot, nil
	}

	// Check cache size and evict if necessary
	if len(ps.bots) >= ps.maxCacheSize {
		ps.evictOldestBotFromCache()
	}

	ps.bots[botID] = &bot
	ps.cacheAccessTime["bot:"+botID] = time.Now()
	ps.mu.Unlock()

	return &bot, nil
}

// DeleteBot removes a bot
func (ps *PersistentState) DeleteBot(ctx context.Context, botID string) error {
	return ps.retryManager.Execute(func() error {
		// Remove from in-memory state first
		ps.mu.Lock()
		delete(ps.bots, botID)
		delete(ps.cacheAccessTime, "bot:"+botID)
		ps.mu.Unlock()

		// Don't hold lock during database operation
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			// Remove from database
			if err := tx.Delete(ctx, "bot:"+botID); err != nil {
				return common.NewDatabaseError("delete_bot", err)
			}

			ps.mu.Lock()
			ps.stats.TransactionCount++
			ps.mu.Unlock()

			ps.logger.WithField("bot_id", botID).Debug("Bot deleted successfully")

			return nil
		})
	})
}

// ListBots returns all registered bots
func (ps *PersistentState) ListBots(ctx context.Context) ([]*common.Bot, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	bots := make([]*common.Bot, 0, len(ps.bots))
	for _, bot := range ps.bots {
		bots = append(bots, bot)
	}

	return bots, nil
}

// FindTimedOutBots returns IDs of bots that have timed out
func (ps *PersistentState) FindTimedOutBots(ctx context.Context) ([]string, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var timedOut []string
	now := time.Now()

	for _, bot := range ps.bots {
		if now.After(bot.TimeoutAt) && bot.Status != common.BotStatusTimedOut {
			timedOut = append(timedOut, bot.ID)
		}
	}

	return timedOut, nil
}

// ResetBot resets a bot's state after timeout
func (ps *PersistentState) ResetBot(ctx context.Context, botID string) error {
	return ps.retryManager.Execute(func() error {
		return ps.db.Transaction(ctx, func(tx common.Transaction) error {
			ps.mu.Lock()
			defer ps.mu.Unlock()

			bot, exists := ps.bots[botID]
			if !exists {
				return common.NewValidationError("reset_bot", fmt.Errorf("bot not found: %s", botID))
			}

			// Reset bot state
			bot.Status = common.BotStatusTimedOut
			bot.CurrentJob = nil
			bot.FailureCount++

			// Persist changes
			if err := tx.Store(ctx, "bot:"+botID, bot); err != nil {
				return common.NewDatabaseError("reset_bot", err)
			}

			// Update in-memory state
			ps.bots[botID] = bot

			ps.stats.TransactionCount++

			ps.logger.WithFields(logrus.Fields{
				"bot_id":        botID,
				"failure_count": bot.FailureCount,
			}).Warn("Bot reset due to timeout")

			return nil
		})
	})
}

// UpdateBotInCache updates bot information in the in-memory cache
func (ps *PersistentState) UpdateBotInCache(botID string, status common.BotStatus, currentJob *string, lastSeen, timeoutAt time.Time) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if bot, exists := ps.bots[botID]; exists {
		bot.Status = status
		bot.CurrentJob = currentJob
		bot.LastSeen = lastSeen
		bot.TimeoutAt = timeoutAt
		bot.IsOnline = true
	}
}

// UpdateBotInCacheForJob updates bot status related to job assignment
func (ps *PersistentState) UpdateBotInCacheForJob(botID string, jobID *string, status common.BotStatus) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if bot, exists := ps.bots[botID]; exists {
		bot.Status = status
		bot.CurrentJob = jobID
		bot.LastSeen = time.Now()
	}
}

// evictOldestBotFromCache removes the least recently accessed bot from cache
func (ps *PersistentState) evictOldestBotFromCache() {
	// Must be called with lock held
	var oldestKey string
	oldestTime := time.Now()

	for botID := range ps.bots {
		key := "bot:" + botID
		if accessTime, exists := ps.cacheAccessTime[key]; exists {
			if accessTime.Before(oldestTime) {
				oldestTime = accessTime
				oldestKey = botID
			}
		} else {
			// If no access time, it's the oldest
			oldestKey = botID
			break
		}
	}

	if oldestKey != "" {
		delete(ps.bots, oldestKey)
		delete(ps.cacheAccessTime, "bot:"+oldestKey)
		ps.logger.WithField("bot_id", oldestKey).Debug("Evicted bot from cache")
	}
}
