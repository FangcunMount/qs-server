package options

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// AIWorkflowManagementOptions is independent of the retiring in-process model runtime.
type AIWorkflowManagementOptions struct {
	Enabled  bool   `json:"enabled" mapstructure:"enabled"`
	Address  string `json:"address" mapstructure:"address"`
	CAFile   string `json:"ca_file" mapstructure:"ca_file"`
	CertFile string `json:"cert_file" mapstructure:"cert_file"`
	KeyFile  string `json:"key_file" mapstructure:"key_file"`
}

func (o AIWorkflowManagementOptions) Validate() error {
	if !o.Enabled {
		return nil
	}
	host, port, err := net.SplitHostPort(o.Address)
	number, portErr := strconv.Atoi(port)
	if err != nil || host == "" || portErr != nil || number < 1 || number > 65535 || strings.TrimSpace(o.CAFile) == "" || strings.TrimSpace(o.CertFile) == "" || strings.TrimSpace(o.KeyFile) == "" {
		return fmt.Errorf("ai_explanation.workflow_management requires address and mTLS CA/certificate/key files")
	}
	return nil
}
