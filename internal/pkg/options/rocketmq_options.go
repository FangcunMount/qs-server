package options

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/spf13/pflag"

	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
)

// RocketMQOptions selects an explicit endpoint. Files belong to QS runtime
// configuration; the SDK never discovers production credentials or resources.
type RocketMQOptions struct {
	Transactions    map[string]TransactionOptions `json:"transactions,omitempty" mapstructure:"transactions"`
	Endpoint        string                        `json:"endpoint" mapstructure:"endpoint"`
	CredentialsFile string                        `json:"credentials_file" mapstructure:"credentials_file"`
	CAFile          string                        `json:"ca_file" mapstructure:"ca_file"`
	RoutesFile      string                        `json:"routes_file" mapstructure:"routes_file"`
	InstanceID      string                        `json:"instance_id" mapstructure:"instance_id"`
	PublishEnabled  *bool                         `json:"publish_enabled" mapstructure:"publish_enabled"`
}

func (o *RocketMQOptions) Validate() error {
	if o == nil || o.Endpoint == "" || o.CredentialsFile == "" || o.CAFile == "" || o.RoutesFile == "" {
		return errors.New("RocketMQ requires endpoint and explicit credential/CA/route files")
	}
	for owner, binding := range o.Transactions {
		if _, err := binding.ProtocolLimits(owner); err != nil {
			return err
		}
	}
	return nil
}
func (o *RocketMQOptions) PublishingEnabled() bool {
	return o == nil || o.PublishEnabled == nil || *o.PublishEnabled
}

func (o *RocketMQOptions) Load(topics []string, concurrency int, timeout time.Duration) (rocketmq.Config, error) {
	if err := o.Validate(); err != nil {
		return rocketmq.Config{}, err
	}
	var credentials struct{ Username, Password string }
	body, err := os.ReadFile(o.CredentialsFile)
	if err != nil {
		return rocketmq.Config{}, errors.New("read configured RocketMQ credentials")
	}
	if json.Unmarshal(body, &credentials) != nil || credentials.Username == "" || credentials.Password == "" {
		return rocketmq.Config{}, errors.New("invalid RocketMQ credentials")
	}
	body, err = os.ReadFile(o.RoutesFile)
	if err != nil {
		return rocketmq.Config{}, errors.New("read configured RocketMQ route manifest")
	}
	routes, err := decodeRouteManifest(body)
	if err != nil {
		return rocketmq.Config{}, err
	}
	selected := make([]transport.RouteBinding, 0)
	seen := map[string]bool{}
	for _, r := range routes {
		if len(topics) == 0 {
			selected = append(selected, r)
			continue
		}
		for _, topic := range topics {
			if r.Topic == topic {
				selected = append(selected, r)
				seen[topic] = true
				break
			}
		}
	}
	for _, topic := range topics {
		if !seen[topic] {
			return rocketmq.Config{}, errors.New("required logical topic absent from RocketMQ manifest")
		}
	}
	ca, err := os.ReadFile(o.CAFile)
	if err != nil {
		return rocketmq.Config{}, errors.New("read configured RocketMQ CA")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return rocketmq.Config{}, errors.New("invalid RocketMQ CA")
	}
	return rocketmq.Config{Endpoint: o.Endpoint, AccessKey: credentials.Username, SecretKey: credentials.Password, Routes: selected, MaxInFlight: concurrency, RequestTimeout: timeout, TLS: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}, nil
}

func (o *RocketMQOptions) AddFlags(fs *pflag.FlagSet, prefix string) {
	fs.StringVar(&o.Endpoint, prefix+".endpoint", o.Endpoint, "RocketMQ Proxy endpoint")
	fs.StringVar(&o.CredentialsFile, prefix+".credentials_file", o.CredentialsFile, "Private runtime credentials")
	fs.StringVar(&o.CAFile, prefix+".ca_file", o.CAFile, "RocketMQ CA file")
	fs.StringVar(&o.RoutesFile, prefix+".routes_file", o.RoutesFile, "Reviewed logical/physical manifest")
	fs.StringVar(&o.InstanceID, prefix+".instance_id", o.InstanceID, "Stable fanout instance identity")
	if o.PublishEnabled == nil {
		enabled := true
		o.PublishEnabled = &enabled
	}
	fs.BoolVar(o.PublishEnabled, prefix+".publish_enabled", *o.PublishEnabled, "Admit publishing and Outbox claims")
}
