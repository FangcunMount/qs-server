package process

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
)

// The process resource stage owns this NORMAL-only client. Transaction owners
// are assembled separately; a legacy Outbox publisher never accepts their map.
type normalWireResource struct {
	*messagingruntime.NormalWirePublisher
}

func (p *normalWireResource) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return p.NormalWirePublisher.Close(ctx)
}
