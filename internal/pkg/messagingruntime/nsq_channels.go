package messagingruntime

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	rmnsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
)

type DurableChannel struct {
	Topic   string
	Channel string
}

// EnsureNSQChannels uses the host's explicit HTTP endpoints. In particular it
// does not infer an HTTP port from the publisher's TCP address. The caller
// must finish this step before it allows the first publish to a new topic.
func EnsureNSQChannels(ctx context.Context, endpoints []string, channels []DurableChannel) error {
	provisioner, err := rmnsq.NewProvisioner(&http.Client{Timeout: 6 * time.Second}, endpoints)
	if err != nil {
		return err
	}
	seen := make(map[DurableChannel]struct{}, len(channels))
	unique := make([]DurableChannel, 0, len(channels))
	for _, channel := range channels {
		if _, exists := seen[channel]; exists {
			continue
		}
		seen[channel] = struct{}{}
		unique = append(unique, channel)
	}
	sort.Slice(unique, func(i, j int) bool {
		if unique[i].Topic != unique[j].Topic {
			return unique[i].Topic < unique[j].Topic
		}
		return unique[i].Channel < unique[j].Channel
	})
	for _, channel := range unique {
		if err := provisioner.EnsureChannel(ctx, channel.Topic, channel.Channel); err != nil {
			return fmt.Errorf("prepare durable NSQ channel %s/%s: %w", channel.Topic, channel.Channel, err)
		}
	}
	return nil
}
