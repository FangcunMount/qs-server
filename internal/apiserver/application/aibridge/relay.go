package aibridge

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
)

// ServeRelay continuously drains the durable outbox. Each batch retains the
// existing command IDs, acknowledgement and retry rules; it never calls a model.
func (s *Service) ServeRelay(ctx context.Context) {
	serveRelay(ctx, s.Relay, time.Second, 30*time.Second)
}

func serveRelay(ctx context.Context, relay func(context.Context) (int, error), idle, maximum time.Duration) {
	delay := idle
	for ctx.Err() == nil {
		batch, cancel := context.WithTimeout(ctx, 60*time.Second)
		_, err := relay(batch)
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			// Driver errors may contain connection details or command payloads.
			log.Warn("AI command relay batch failed; durable commands retained")
			delay = min(delay*2, maximum)
		} else {
			delay = idle
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
