package messaging

import (
	"fmt"
	"time"
)

// Config 事件总线配置
type Config struct {
	// Provider 消息中间件提供者类型（nsq, rabbitmq）
	Provider Provider `json:"provider" yaml:"provider"`

	// NSQ 配置
	NSQ NSQConfig `json:"nsq" yaml:"nsq"`

	// RabbitMQ 配置
	RabbitMQ RabbitMQConfig `json:"rabbitmq" yaml:"rabbitmq"`
}

// NSQConfig NSQ 配置
type NSQConfig struct {
	// NSQLookupd 地址列表
	LookupdAddrs []string `json:"lookupd_addrs" yaml:"lookupd_addrs"`

	// NSQd 地址（用于发布）
	NSQdAddr string `json:"nsqd_addr" yaml:"nsqd_addr"`

	// 最大消息重试次数
	MaxAttempts uint16 `json:"max_attempts" yaml:"max_attempts"`

	// 最大消息处理时间
	MaxInFlight int `json:"max_in_flight" yaml:"max_in_flight"`

	// 消息超时时间
	MsgTimeout time.Duration `json:"msg_timeout" yaml:"msg_timeout"`

	// 重新入队延迟
	RequeueDelay time.Duration `json:"requeue_delay" yaml:"requeue_delay"`

	// 拨号超时时间
	DialTimeout time.Duration `json:"dial_timeout" yaml:"dial_timeout"`

	// 读超时时间
	ReadTimeout time.Duration `json:"read_timeout" yaml:"read_timeout"`

	// 写超时时间
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout"`
}

// RabbitMQConfig RabbitMQ 配置
type RabbitMQConfig struct {
	// RabbitMQ 连接 URL（推荐方式）
	// 格式：amqp://username:password@host:port/vhost
	// 例如：amqp://guest:guest@localhost:5672/
	// 注意：如果设置了 URL，下面的独立配置项会被忽略
	URL string `json:"url" yaml:"url"`

	// ========== 独立配置项（可选，URL 为空时使用） ==========

	// Host 主机地址
	Host string `json:"host" yaml:"host"`

	// Port 端口号（默认 5672）
	Port int `json:"port" yaml:"port"`

	// Username 用户名（默认 guest）
	Username string `json:"username" yaml:"username"`

	// Password 密码（默认 guest）
	Password string `json:"password" yaml:"password"`

	// VHost 虚拟主机（默认 /）
	// VHost 相当于命名空间，用于隔离不同应用的消息
	VHost string `json:"vhost" yaml:"vhost"`

	// ========== 连接池配置 ==========

	// MaxChannels 最大 channel 数量（默认 100）
	// Channel 是轻量级的连接，用于发送和接收消息
	MaxChannels int `json:"max_channels" yaml:"max_channels"`

	// ========== QoS（服务质量）配置 ==========

	// PrefetchCount 预取数量（默认 200）
	// 消费者一次最多预取多少条未确认的消息
	// 值越大，吞吐量越高，但内存占用也越大
	PrefetchCount int `json:"prefetch_count" yaml:"prefetch_count"`

	// PrefetchSize 预取大小（默认 0，不限制）
	// 消费者一次最多预取多少字节的未确认消息
	PrefetchSize int `json:"prefetch_size" yaml:"prefetch_size"`

	// ========== 消息持久化配置 ==========

	// Durable 是否持久化 exchange 和 queue（默认 true）
	// true: RabbitMQ 重启后，exchange 和 queue 不会丢失
	Durable bool `json:"durable" yaml:"durable"`

	// PersistentMessages 消息是否持久化（默认 true）
	// true: 消息会写入磁盘，RabbitMQ 重启后消息不会丢失
	PersistentMessages bool `json:"persistent_messages" yaml:"persistent_messages"`

	// ========== 超时配置 ==========

	// ConnectionTimeout 连接超时时间（默认 10s）
	ConnectionTimeout time.Duration `json:"connection_timeout" yaml:"connection_timeout"`

	// HeartbeatInterval 心跳间隔（默认 10s）
	// 用于检测连接是否存活
	HeartbeatInterval time.Duration `json:"heartbeat_interval" yaml:"heartbeat_interval"`

	// ========== 重连配置 ==========

	// AutoReconnect 是否自动重连（默认 true）
	AutoReconnect bool `json:"auto_reconnect" yaml:"auto_reconnect"`

	// ReconnectDelay 重连延迟（默认 5s）
	ReconnectDelay time.Duration `json:"reconnect_delay" yaml:"reconnect_delay"`

	// MaxReconnectAttempts 最大重连次数（默认 0，无限重试）
	MaxReconnectAttempts int `json:"max_reconnect_attempts" yaml:"max_reconnect_attempts"`

	// ========== 高级配置 ==========

	// ExchangeType Exchange 类型（默认 fanout）
	// fanout: 广播，发送给所有绑定的队列
	// direct: 直接路由，根据 routing key 精确匹配
	// topic: 主题路由，支持通配符（* 和 #）
	// headers: 根据消息头路由
	ExchangeType string `json:"exchange_type" yaml:"exchange_type"`

	// AutoDelete 是否自动删除（默认 false）
	// true: 当没有消费者时，自动删除 exchange 和 queue
	AutoDelete bool `json:"auto_delete" yaml:"auto_delete"`

	// Exclusive 是否独占（默认 false）
	// true: 队列只能被当前连接使用，连接断开后队列自动删除
	Exclusive bool `json:"exclusive" yaml:"exclusive"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Provider: ProviderNSQ, // 默认使用 NSQ
		NSQ: NSQConfig{
			LookupdAddrs: []string{"127.0.0.1:4161"},
			NSQdAddr:     "127.0.0.1:4150",
			MaxAttempts:  5,
			MaxInFlight:  200,
			MsgTimeout:   time.Minute,
			RequeueDelay: time.Second * 5,
			DialTimeout:  time.Second * 5,
			ReadTimeout:  time.Minute,
			WriteTimeout: time.Second * 5,
		},
		RabbitMQ: RabbitMQConfig{
			URL:                  "amqp://guest:guest@localhost:5672/",
			Host:                 "localhost",
			Port:                 5672,
			Username:             "guest",
			Password:             "guest",
			VHost:                "/",
			MaxChannels:          100,
			PrefetchCount:        200,
			PrefetchSize:         0,
			Durable:              true,
			PersistentMessages:   true,
			ConnectionTimeout:    time.Second * 10,
			HeartbeatInterval:    time.Second * 10,
			AutoReconnect:        true,
			ReconnectDelay:       time.Second * 5,
			MaxReconnectAttempts: 0, // 无限重试
			ExchangeType:         "fanout",
			AutoDelete:           false,
			Exclusive:            false,
		},
	}
}

// DefaultNSQConfig 返回默认 NSQ 配置
func DefaultNSQConfig() NSQConfig {
	return DefaultConfig().NSQ
}

// DefaultRabbitMQConfig 返回默认 RabbitMQ 配置
func DefaultRabbitMQConfig() RabbitMQConfig {
	return DefaultConfig().RabbitMQ
}

// BuildURL 构建 RabbitMQ 连接 URL
// 如果已经设置了 URL，直接返回；否则根据独立配置项构建
func (c *RabbitMQConfig) BuildURL() string {
	if c.URL != "" {
		return c.URL
	}

	// 默认值
	host := c.Host
	if host == "" {
		host = "localhost"
	}

	port := c.Port
	if port == 0 {
		port = 5672
	}

	username := c.Username
	if username == "" {
		username = "guest"
	}

	password := c.Password
	if password == "" {
		password = "guest"
	}

	vhost := c.VHost
	if vhost == "" {
		vhost = "/"
	}

	return fmt.Sprintf("amqp://%s:%s@%s:%d%s", username, password, host, port, vhost)
}
