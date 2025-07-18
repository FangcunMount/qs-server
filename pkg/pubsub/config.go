package pubsub

import "time"

// Config 发布订阅配置
type Config struct {
	// Redis 连接配置
	Addr     string `json:"addr"`
	Password string `json:"password"`
	DB       int    `json:"db"`

	// 消费者配置
	ConsumerGroup string `json:"consumer_group"`
	Consumer      string `json:"consumer"`

	// 性能配置
	MaxLen        int64         `json:"max_len"`
	ClaimInterval time.Duration `json:"claim_interval"`
	BlockTime     time.Duration `json:"block_time"`
	ReadBatchSize int64         `json:"read_batch_size"`

	// 重试配置
	MaxRetries      int           `json:"max_retries"`
	InitialInterval time.Duration `json:"initial_interval"`
	MaxInterval     time.Duration `json:"max_interval"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Addr:            "localhost:6379",
		Password:        "",
		DB:              0,
		ConsumerGroup:   "default-group",
		Consumer:        "consumer-1",
		MaxLen:          1000,
		ClaimInterval:   time.Second * 30,
		BlockTime:       time.Second * 5,
		ReadBatchSize:   10,
		MaxRetries:      3,
		InitialInterval: time.Millisecond * 100,
		MaxInterval:     time.Second * 10,
	}
}
