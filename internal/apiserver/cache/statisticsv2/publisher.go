package statisticsv2

import (
	"context"
	"fmt"
	"time"
)

type Warmer interface {
	Warm(context.Context, int64, time.Time) error
}

// Publisher closes the post-commit cache phase: first atomically switches the
// organization generation, then prewarms the common complete-day windows. A
// failure leaves SyncRun in data_committed so resume-cache can retry only this
// phase without recollecting or reprojecting data.
type Publisher struct {
	generation *GenerationPublisher
	warmer     Warmer
}

func NewPublisher(generation *GenerationPublisher, warmer Warmer) *Publisher {
	return &Publisher{generation: generation, warmer: warmer}
}

func (p *Publisher) Publish(ctx context.Context, orgID int64, asOfDate time.Time) (int64, error) {
	if p == nil || p.generation == nil || p.warmer == nil {
		return 0, fmt.Errorf("statistics v2 cache publisher is unavailable")
	}
	generation, err := p.generation.Publish(ctx, orgID, asOfDate)
	if err != nil {
		return 0, err
	}
	if err := p.warmer.Warm(ctx, orgID, asOfDate); err != nil {
		return generation, err
	}
	return generation, nil
}
