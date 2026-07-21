package attentionprojection

import (
	"context"
	"fmt"
	"log/slog"
)

// SyncClient performs the attention RPC side effect.
type SyncClient interface {
	SyncAssessmentAttention(ctx context.Context, testeeID uint64, riskLevel string, markKeyFocus bool) error
}

// Projector owns durable attention projection for interpretation.report.generated.
type Projector struct {
	store       Store
	client      SyncClient
	maxAttempts int
	logger      *slog.Logger
}

func NewProjector(store Store, client SyncClient, maxAttempts int, logger *slog.Logger) *Projector {
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	return &Projector{
		store:       store,
		client:      client,
		maxAttempts: maxAttempts,
		logger:      logger,
	}
}

// Project upserts pending state, syncs attention, and records durable outcome.
// Report status projection must already be committed; this method never rolls it back.
func (p *Projector) Project(ctx context.Context, input PendingInput) error {
	if p == nil || p.store == nil || p.client == nil {
		return fmt.Errorf("attention projector is not configured")
	}
	alreadySucceeded, err := p.store.EnsurePending(ctx, input)
	if err != nil {
		return fmt.Errorf("ensure attention projection pending: %w", err)
	}
	if alreadySucceeded {
		return nil
	}
	return p.syncOnce(ctx, input)
}

func (p *Projector) syncOnce(ctx context.Context, input PendingInput) error {
	if err := p.client.SyncAssessmentAttention(ctx, input.TesteeID, input.RiskLevel, input.MarkKeyFocus); err != nil {
		status, recordErr := p.store.RecordFailure(ctx, input.EventID, err.Error(), p.maxAttempts)
		if recordErr != nil {
			return fmt.Errorf("record attention projection failure: %w", recordErr)
		}
		if p.logger != nil {
			p.logger.Error("attention projection sync failed",
				slog.String("event_id", input.EventID),
				slog.String("report_id", input.ReportID),
				slog.Uint64("testee_id", input.TesteeID),
				slog.String("status", string(status)),
				slog.String("error", err.Error()),
			)
		}
		return nil
	}
	if err := p.store.MarkSucceeded(ctx, input.EventID); err != nil {
		return fmt.Errorf("mark attention projection succeeded: %w", err)
	}
	if p.logger != nil {
		p.logger.Info("attention projection synced successfully",
			slog.String("event_id", input.EventID),
			slog.Uint64("testee_id", input.TesteeID),
		)
	}
	return nil
}

func pendingInputFromRecord(rec Record) PendingInput {
	return PendingInput{
		EventID:      rec.EventID,
		ReportID:     rec.ReportID,
		AssessmentID: rec.AssessmentID,
		TesteeID:     rec.TesteeID,
		RiskLevel:    rec.RiskLevel,
		MarkKeyFocus: rec.MarkKeyFocus,
	}
}
