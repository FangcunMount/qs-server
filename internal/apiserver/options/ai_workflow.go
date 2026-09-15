package options

import "github.com/spf13/pflag"

// AIWorkflowOptions configures intake and management communication with qs-ai only.
type AIWorkflowOptions struct {
	Enabled    bool                        `json:"enabled" mapstructure:"enabled"`
	Management AIWorkflowManagementOptions `json:"management" mapstructure:"management"`
}

func NewAIWorkflowOptions() *AIWorkflowOptions { return &AIWorkflowOptions{} }
func (o *AIWorkflowOptions) Validate() []error {
	if o == nil {
		return nil
	}
	transport := o.Management
	transport.Enabled = transport.Enabled || o.Enabled
	if err := transport.Validate(); err != nil {
		return []error{err}
	}
	return nil
}
func (o *AIWorkflowOptions) AddFlags(fs *pflag.FlagSet) {
	if o == nil {
		return
	}
	fs.BoolVar(&o.Enabled, "ai_workflow.enabled", o.Enabled, "Enable qs-ai participant intake.")
	fs.BoolVar(&o.Management.Enabled, "ai_workflow.management.enabled", o.Management.Enabled, "Enable qs-ai management proxy.")
	fs.StringVar(&o.Management.Address, "ai_workflow.management.address", o.Management.Address, "qs-ai gRPC address.")
	fs.StringVar(&o.Management.CAFile, "ai_workflow.management.ca_file", o.Management.CAFile, "Trusted CA chain.")
	fs.StringVar(&o.Management.CertFile, "ai_workflow.management.cert_file", o.Management.CertFile, "QS client certificate chain.")
	fs.StringVar(&o.Management.KeyFile, "ai_workflow.management.key_file", o.Management.KeyFile, "QS client private key.")
}
