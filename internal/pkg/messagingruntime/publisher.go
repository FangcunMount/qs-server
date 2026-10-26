package messagingruntime

import (
	"errors"

	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

// NewSDKWirePublisher cannot reconstruct the retired publication-only runtime.
// Current hosts explicitly construct NORMAL clients and transaction owners.
func NewSDKWirePublisher(_ *options.MessagingOptions) (*sdkNSQWirePublisher, error) {
	return nil, errors.New("legacy Outbox wire publisher retired; use final transaction owners or NORMAL client")
}
