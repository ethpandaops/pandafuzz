package abstraction

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
)

// CompositeDriverConfig contains configuration for composite driver
type CompositeDriverConfig struct {
	Primary     Driver
	Secondaries []Driver
	WriteMode   CompositeWriteMode
	ReadMode    CompositeReadMode
	Logger      logrus.FieldLogger
}

// CompositeDriver implements storage across multiple backends
type CompositeDriver struct {
	primary     Driver
	secondaries []Driver
	writeMode   CompositeWriteMode
	readMode    CompositeReadMode
	logger      logrus.FieldLogger
}

// NewCompositeDriver creates a new composite driver
func NewCompositeDriver(config CompositeDriverConfig) (Driver, error) {
	if config.Primary == nil {
		return nil, fmt.Errorf("primary driver is required")
	}

	if config.WriteMode == "" {
		config.WriteMode = WritePrimaryFirst
	}

	if config.ReadMode == "" {
		config.ReadMode = ReadPrimary
	}

	return &CompositeDriver{
		primary:     config.Primary,
		secondaries: config.Secondaries,
		writeMode:   config.WriteMode,
		readMode:    config.ReadMode,
		logger:      config.Logger.WithField("driver", "composite"),
	}, nil
}

// Put implements Driver.Put across multiple backends
func (d *CompositeDriver) Put(ctx context.Context, key string, data []byte) error {
	switch d.writeMode {
	case WriteAll:
		return d.putAll(ctx, key, data)
	case WritePrimaryFirst:
		return d.putPrimaryFirst(ctx, key, data)
	case WriteBestEffort:
		return d.putBestEffort(ctx, key, data)
	default:
		return fmt.Errorf("unsupported write mode: %s", d.writeMode)
	}
}

// putAll writes to all backends and fails if any fail
func (d *CompositeDriver) putAll(ctx context.Context, key string, data []byte) error {
	// Write to primary first
	if err := d.primary.Put(ctx, key, data); err != nil {
		return fmt.Errorf("primary write failed: %w", err)
	}

	// Write to all secondaries in parallel
	if len(d.secondaries) > 0 {
		var wg sync.WaitGroup
		errCh := make(chan error, len(d.secondaries))

		for i, secondary := range d.secondaries {
			wg.Add(1)
			go func(idx int, driver Driver) {
				defer wg.Done()
				if err := driver.Put(ctx, key, data); err != nil {
					errCh <- fmt.Errorf("secondary %d write failed: %w", idx, err)
				}
			}(i, secondary)
		}

		wg.Wait()
		close(errCh)

		// Collect any errors
		var errs []error
		for err := range errCh {
			errs = append(errs, err)
		}

		if len(errs) > 0 {
			// Attempt to rollback primary
			if delErr := d.primary.Delete(ctx, key); delErr != nil {
				d.logger.WithError(delErr).Error("Failed to rollback primary after secondary failures")
			}
			return errors.Join(errs...)
		}
	}

	return nil
}

// putPrimaryFirst writes to primary, then secondaries asynchronously
func (d *CompositeDriver) putPrimaryFirst(ctx context.Context, key string, data []byte) error {
	// Write to primary first
	if err := d.primary.Put(ctx, key, data); err != nil {
		return fmt.Errorf("primary write failed: %w", err)
	}

	// Write to secondaries asynchronously
	for i, secondary := range d.secondaries {
		go func(idx int, driver Driver) {
			if err := driver.Put(context.Background(), key, data); err != nil {
				d.logger.WithError(err).WithField("secondary", idx).Warn("Secondary write failed")
			}
		}(i, secondary)
	}

	return nil
}

// putBestEffort writes to all backends but only requires primary to succeed
func (d *CompositeDriver) putBestEffort(ctx context.Context, key string, data []byte) error {
	// Write to primary first
	if err := d.primary.Put(ctx, key, data); err != nil {
		return fmt.Errorf("primary write failed: %w", err)
	}

	// Write to secondaries in parallel, log failures
	var wg sync.WaitGroup
	for i, secondary := range d.secondaries {
		wg.Add(1)
		go func(idx int, driver Driver) {
			defer wg.Done()
			if err := driver.Put(ctx, key, data); err != nil {
				d.logger.WithError(err).WithField("secondary", idx).Warn("Secondary write failed")
			}
		}(i, secondary)
	}

	// Wait for all secondary writes to complete
	wg.Wait()
	return nil
}

// Get implements Driver.Get with configurable read modes
func (d *CompositeDriver) Get(ctx context.Context, key string) ([]byte, error) {
	switch d.readMode {
	case ReadPrimary:
		return d.primary.Get(ctx, key)
	case ReadFallback:
		return d.getFallback(ctx, key)
	case ReadFastest:
		return d.getFastest(ctx, key)
	default:
		return nil, fmt.Errorf("unsupported read mode: %s", d.readMode)
	}
}

// getFallback tries primary first, then falls back to secondaries
func (d *CompositeDriver) getFallback(ctx context.Context, key string) ([]byte, error) {
	// Try primary first
	data, err := d.primary.Get(ctx, key)
	if err == nil {
		return data, nil
	}

	if err != ErrNotFound {
		d.logger.WithError(err).Warn("Primary read failed, trying secondaries")
	}

	// Try secondaries in order
	for i, secondary := range d.secondaries {
		data, err = secondary.Get(ctx, key)
		if err == nil {
			// Optionally sync back to primary
			go func() {
				if syncErr := d.primary.Put(context.Background(), key, data); syncErr != nil {
					d.logger.WithError(syncErr).Warn("Failed to sync data back to primary")
				}
			}()
			return data, nil
		}

		if err != ErrNotFound {
			d.logger.WithError(err).WithField("secondary", i).Warn("Secondary read failed")
		}
	}

	return nil, ErrNotFound
}

// getFastest reads from all backends and returns the first successful response
func (d *CompositeDriver) getFastest(ctx context.Context, key string) ([]byte, error) {
	type result struct {
		data []byte
		err  error
		from string
	}

	resultCh := make(chan result, 1+len(d.secondaries))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Start reads from all backends
	go func() {
		data, err := d.primary.Get(ctx, key)
		select {
		case resultCh <- result{data: data, err: err, from: "primary"}:
		case <-ctx.Done():
		}
	}()

	for i, secondary := range d.secondaries {
		go func(idx int, driver Driver) {
			data, err := driver.Get(ctx, key)
			select {
			case resultCh <- result{data: data, err: err, from: fmt.Sprintf("secondary_%d", idx)}:
			case <-ctx.Done():
			}
		}(i, secondary)
	}

	// Wait for first successful response
	var lastErr error
	for i := 0; i < 1+len(d.secondaries); i++ {
		select {
		case res := <-resultCh:
			if res.err == nil {
				d.logger.WithField("from", res.from).Debug("Got data from backend")
				return res.data, nil
			}
			lastErr = res.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if lastErr == nil {
		lastErr = ErrNotFound
	}
	return nil, lastErr
}

// Delete implements Driver.Delete across all backends
func (d *CompositeDriver) Delete(ctx context.Context, key string) error {
	// Delete from all backends in parallel
	var wg sync.WaitGroup
	errCh := make(chan error, 1+len(d.secondaries))

	// Delete from primary
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := d.primary.Delete(ctx, key); err != nil {
			errCh <- fmt.Errorf("primary delete failed: %w", err)
		}
	}()

	// Delete from secondaries
	for i, secondary := range d.secondaries {
		wg.Add(1)
		go func(idx int, driver Driver) {
			defer wg.Done()
			if err := driver.Delete(ctx, key); err != nil {
				errCh <- fmt.Errorf("secondary %d delete failed: %w", idx, err)
			}
		}(i, secondary)
	}

	wg.Wait()
	close(errCh)

	// Collect errors
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		// Log errors but don't fail the operation
		for _, err := range errs {
			d.logger.WithError(err).Warn("Delete operation failed on some backends")
		}
	}

	return nil
}

// List implements Driver.List from primary backend
func (d *CompositeDriver) List(ctx context.Context, prefix string) ([]string, error) {
	// For consistency, always list from primary
	return d.primary.List(ctx, prefix)
}

// Exists implements Driver.Exists with fallback
func (d *CompositeDriver) Exists(ctx context.Context, key string) (bool, error) {
	// Check primary first
	exists, err := d.primary.Exists(ctx, key)
	if err == nil && exists {
		return true, nil
	}

	if err != nil && err != ErrNotFound {
		d.logger.WithError(err).Warn("Primary exists check failed")
	}

	// Check secondaries if configured for fallback
	if d.readMode == ReadFallback || d.readMode == ReadFastest {
		for i, secondary := range d.secondaries {
			exists, err = secondary.Exists(ctx, key)
			if err == nil && exists {
				return true, nil
			}
			if err != nil && err != ErrNotFound {
				d.logger.WithError(err).WithField("secondary", i).Warn("Secondary exists check failed")
			}
		}
	}

	return false, nil
}

// GetURL implements Driver.GetURL from primary backend
func (d *CompositeDriver) GetURL(ctx context.Context, key string) (string, error) {
	// Always get URL from primary for consistency
	return d.primary.GetURL(ctx, key)
}
