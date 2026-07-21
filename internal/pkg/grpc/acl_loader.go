package grpc

import (
	"fmt"
	"os"

	basegrpc "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	"gopkg.in/yaml.v3"
)

func parseACLConfigFile(configFile string) (*basegrpc.ACLConfig, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("read acl config: %w", err)
	}
	cfg := &basegrpc.ACLConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse acl config: %w", err)
	}
	return cfg, nil
}
