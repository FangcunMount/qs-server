package systemgovernance

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	actionAuditPrimaryRetryWindow = 3 * time.Second
	actionAuditFallbackTimeout    = 2 * time.Second
)

// RecoverableActionAuditStore keeps MySQL as the claim authority while using a
// durable fallback only for terminal outcomes that MySQL could not complete.
type RecoverableActionAuditStore struct {
	primary            ActionAuditStore
	fallback           ActionAuditFallbackStore
	primaryRetryWindow time.Duration
	fallbackTimeout    time.Duration
}

func NewRecoverableActionAuditStore(primary ActionAuditStore, fallback ActionAuditFallbackStore) *RecoverableActionAuditStore {
	return &RecoverableActionAuditStore{
		primary: primary, fallback: fallback,
		primaryRetryWindow: actionAuditPrimaryRetryWindow,
		fallbackTimeout:    actionAuditFallbackTimeout,
	}
}

func (s *RecoverableActionAuditStore) Claim(ctx context.Context, record ActionAuditRecord) (*ActionAuditReplay, bool, error) {
	if s == nil || s.primary == nil {
		return nil, false, errors.New("primary action audit store is unavailable")
	}
	if s.fallback != nil {
		terminal, exists, err := s.fallback.Load(ctx, record.OrgID, record.RequestID)
		if err != nil {
			return nil, false, fmt.Errorf("load action audit fallback: %w", err)
		}
		if exists {
			return replayFromAuditRecord(terminal), false, nil
		}
	}
	return s.primary.Claim(ctx, record)
}

func (s *RecoverableActionAuditStore) Complete(ctx context.Context, record ActionAuditRecord) error {
	if s == nil || s.primary == nil {
		return errors.New("primary action audit store is unavailable")
	}
	primaryCtx, cancelPrimary := context.WithTimeout(context.WithoutCancel(ctx), s.primaryRetryWindow)
	err := retryActionAuditComplete(primaryCtx, s.primary, record)
	cancelPrimary()
	if err == nil {
		if s.fallback != nil {
			deleteCtx, cancelDelete := context.WithTimeout(context.WithoutCancel(ctx), s.fallbackTimeout)
			_ = s.fallback.Delete(deleteCtx, record.OrgID, record.RequestID)
			cancelDelete()
		}
		return nil
	}
	if s.fallback == nil {
		return err
	}
	fallbackCtx, cancelFallback := context.WithTimeout(context.WithoutCancel(ctx), s.fallbackTimeout)
	defer cancelFallback()
	if fallbackErr := s.fallback.Put(fallbackCtx, record); fallbackErr != nil {
		return fmt.Errorf("primary action audit complete failed: %v; fallback failed: %w", err, fallbackErr)
	}
	return nil
}

func (s *RecoverableActionAuditStore) Recover(ctx context.Context, limit int) (int, error) {
	if s == nil || s.primary == nil || s.fallback == nil {
		return 0, nil
	}
	if limit <= 0 {
		limit = 100
	}
	records, err := s.fallback.List(ctx, limit)
	if err != nil {
		return 0, err
	}
	recovered := 0
	failed := 0
	var lastErr error
	for _, record := range records {
		if err := s.primary.Complete(ctx, record); err != nil {
			failed++
			lastErr = err
			continue
		}
		if err := s.fallback.Delete(ctx, record.OrgID, record.RequestID); err != nil {
			failed++
			lastErr = err
			continue
		}
		recovered++
	}
	if failed > 0 {
		return recovered, fmt.Errorf("recover %d governance audit fallbacks failed: %w", failed, lastErr)
	}
	return recovered, nil
}

func retryActionAuditComplete(ctx context.Context, store ActionAuditStore, record ActionAuditRecord) error {
	delay := 50 * time.Millisecond
	var lastErr error
	for {
		if err := store.Complete(ctx, record); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %v", ctx.Err(), lastErr)
		case <-time.After(delay):
			if delay < 500*time.Millisecond {
				delay *= 2
			}
		}
	}
}

func replayFromAuditRecord(record ActionAuditRecord) *ActionAuditReplay {
	return &ActionAuditReplay{ActionID: record.ActionID, Result: record.Result, Error: record.Error}
}

type ActionAuditRecoveryRunner struct {
	recoverer ActionAuditRecoverer
	interval  time.Duration
	batchSize int
	warn      func(error)
}

func NewActionAuditRecoveryRunner(recoverer ActionAuditRecoverer, warn func(error)) *ActionAuditRecoveryRunner {
	return &ActionAuditRecoveryRunner{recoverer: recoverer, interval: 30 * time.Second, batchSize: 100, warn: warn}
}

func (r *ActionAuditRecoveryRunner) Start(ctx context.Context) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)
	if r == nil || r.recoverer == nil {
		return cancel
	}
	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			if _, err := r.recoverer.Recover(ctx, r.batchSize); err != nil && r.warn != nil {
				r.warn(err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return cancel
}

var _ ActionAuditStore = (*RecoverableActionAuditStore)(nil)
var _ ActionAuditRecoverer = (*RecoverableActionAuditStore)(nil)
