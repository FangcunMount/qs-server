package transport

import (
	"fmt"
	"time"

	"github.com/nsqio/go-nsq"
)

const (
	DefaultRetryBaseDelay = 30 * time.Second
	DefaultRetryMaxDelay  = 5 * time.Minute
	DefaultRetryJitter    = 0.2
)

type SubscriberConfig struct {
	Provider           string
	NSQLookupdAddr     string
	NSQMessageTimeout  time.Duration
	FailedHandoffGroup string
	// NSQDHTTPEndpoints are the host's loaded topology observation addresses.
	// They are not NSQD connections or inferred lookupd producer addresses.
	NSQDHTTPEndpoints []string
}

func newNSQConfig(messageTimeout time.Duration) (*nsq.Config, error) {
	if messageTimeout < 0 {
		return nil, fmt.Errorf("NSQ message timeout must not be negative")
	}
	config := nsq.NewConfig()
	if messageTimeout > 0 {
		config.MsgTimeout = messageTimeout
	}
	return config, nil
}
