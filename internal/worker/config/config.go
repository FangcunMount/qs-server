package config

import (
	"github.com/FangcunMount/component-base/pkg/log"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/worker/options"
)

// Config is the worker runtime configuration wrapper.
//
// Worker previously mirrored selected Options fields into a second struct. The
// wrapper now matches apiserver and collection-server: yaml is decoded into
// Options, then process stages consume this Config without a second copy.
type Config struct {
	*options.Options
}

// Compatibility type aliases keep package-level function signatures stable
// while the concrete configuration truth stays in worker/options.
type LogConfig = log.Options
type MetricsConfig = options.MetricsOptions
type MySQLConfig = genericoptions.MySQLOptions
type MongoDBConfig = genericoptions.MongoDBOptions
type MessagingConfig = options.MessagingOptions
type GRPCConfig = options.GRPCOptions
type WorkerConfig = options.WorkerOptions
type NotificationConfig = options.NotificationOptions

// CreateConfigFromOptions creates a worker runtime config from decoded options.
func CreateConfigFromOptions(opts *options.Options) (*Config, error) {
	return &Config{Options: opts}, nil
}
