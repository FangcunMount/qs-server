package grpc

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	basegrpc "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	"gopkg.in/yaml.v3"
)

var exactFullMethodPattern = regexp.MustCompile(
	`^/[A-Za-z_][A-Za-z0-9_.]*\.[A-Za-z_][A-Za-z0-9_]*/[A-Za-z_][A-Za-z0-9_]*$`,
)

func parseACLConfigFile(configFile string) (*basegrpc.ACLConfig, error) {
	if strings.TrimSpace(configFile) == "" {
		return nil, fmt.Errorf("acl config file is required")
	}
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("read acl config: %w", err)
	}
	cfg := &basegrpc.ACLConfig{}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
		return nil, fmt.Errorf("parse acl config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("parse trailing acl config document: %w", err)
		}
		return nil, fmt.Errorf("acl config must contain exactly one YAML document")
	}
	if err := validateACLConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func validateACLConfig(cfg *basegrpc.ACLConfig) error {
	if cfg == nil {
		return fmt.Errorf("acl config is required")
	}
	if cfg.DefaultPolicy != "deny" {
		return fmt.Errorf("acl default_policy must be %q", "deny")
	}
	if len(cfg.Services) == 0 {
		return fmt.Errorf("acl must contain at least one service rule")
	}

	enabledRules := 0
	serviceNames := make(map[string]struct{}, len(cfg.Services))
	for index, service := range cfg.Services {
		if service == nil {
			return fmt.Errorf("acl service rule %d cannot be null", index)
		}
		if strings.TrimSpace(service.ServiceName) == "" || service.ServiceName != strings.TrimSpace(service.ServiceName) {
			return fmt.Errorf("acl service rule %d has invalid service_name", index)
		}
		if strings.ContainsAny(service.ServiceName, "*\t\r\n ") {
			return fmt.Errorf("acl service %q has invalid service_name", service.ServiceName)
		}
		if _, exists := serviceNames[service.ServiceName]; exists {
			return fmt.Errorf("acl service_name %q is duplicated", service.ServiceName)
		}
		serviceNames[service.ServiceName] = struct{}{}

		if len(service.MethodPermissions) > 0 {
			return fmt.Errorf("acl service %q uses unsupported method_permissions", service.ServiceName)
		}
		methods := make(map[string]struct{}, len(service.AllowedMethods)+len(service.DeniedMethods))
		if err := validateACLMethods(service.ServiceName, "allowed_methods", service.AllowedMethods, methods); err != nil {
			return err
		}
		if err := validateACLMethods(service.ServiceName, "denied_methods", service.DeniedMethods, methods); err != nil {
			return err
		}
		if !service.Enabled {
			continue
		}
		enabledRules++
		if len(service.AllowedMethods) == 0 {
			return fmt.Errorf("enabled acl service %q must allow at least one method", service.ServiceName)
		}
	}
	if enabledRules == 0 {
		return fmt.Errorf("acl must contain at least one enabled service rule")
	}
	return nil
}

func validateACLMethods(serviceName, field string, methods []string, seen map[string]struct{}) error {
	for _, method := range methods {
		if !isExactFullMethod(method) {
			return fmt.Errorf("acl service %q has invalid %s entry %q", serviceName, field, method)
		}
		if _, exists := seen[method]; exists {
			return fmt.Errorf("acl service %q has duplicate method entry %q in %s", serviceName, method, field)
		}
		seen[method] = struct{}{}
	}
	return nil
}

func isExactFullMethod(method string) bool {
	return exactFullMethodPattern.MatchString(method)
}
