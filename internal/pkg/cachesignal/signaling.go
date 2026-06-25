package cachesignal

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/signaling"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	goredis "github.com/redis/go-redis/v9"
)

// SignalingOptions Redis signaling 配置。
type SignalingOptions struct {
	Enabled    bool
	Prefix     string
	Channel    string
	BufferSize int
}

func DefaultSignalingOptions() SignalingOptions {
	return SignalingOptions{
		Enabled:    false,
		Prefix:     "qs:signal",
		BufferSize: 100,
	}
}

func (o SignalingOptions) RedisOptions() signalredis.Options {
	opts := signalredis.DefaultOptions()
	if o.Prefix != "" {
		opts.Prefix = o.Prefix
	}
	if o.Channel != "" {
		opts.Channel = o.Channel
	}
	if o.BufferSize > 0 {
		opts.BufferSize = o.BufferSize
	}
	return opts
}

func NewQuestionnaireSignaler(client goredis.UniversalClient, opts SignalingOptions) (*signalredis.Signaler[QuestionnaireCacheChangedSignal], error) {
	standalone, err := AsStandaloneClient(client)
	if err != nil {
		return nil, err
	}
	return signalredis.NewSignaler[QuestionnaireCacheChangedSignal](standalone, opts.RedisOptions()), nil
}

func NewScaleSignaler(client goredis.UniversalClient, opts SignalingOptions) (*signalredis.Signaler[ScaleCacheChangedSignal], error) {
	standalone, err := AsStandaloneClient(client)
	if err != nil {
		return nil, err
	}
	return signalredis.NewSignaler[ScaleCacheChangedSignal](standalone, opts.RedisOptions()), nil
}

// AsStandaloneClient signaling/redis 当前仅支持 standalone *Client。
func AsStandaloneClient(client goredis.UniversalClient) (*goredis.Client, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}
	if c, ok := client.(*goredis.Client); ok {
		return c, nil
	}
	return nil, fmt.Errorf("signaling redis requires standalone *redis.Client")
}

var (
	_ signaling.Notifier[QuestionnaireCacheChangedSignal] = (*signalredis.Signaler[QuestionnaireCacheChangedSignal])(nil)
	_ signaling.Watcher[QuestionnaireCacheChangedSignal]  = (*signalredis.Signaler[QuestionnaireCacheChangedSignal])(nil)
	_ signaling.Notifier[ScaleCacheChangedSignal]         = (*signalredis.Signaler[ScaleCacheChangedSignal])(nil)
	_ signaling.Watcher[ScaleCacheChangedSignal]          = (*signalredis.Signaler[ScaleCacheChangedSignal])(nil)
)
