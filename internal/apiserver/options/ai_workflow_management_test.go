package options

import "testing"

func TestWorkflowManagementDisabledByDefaultAndIndependentOfLocalModel(t *testing.T) {
	o := NewAIExplanationOptions()
	if o.WorkflowManagement.Enabled {
		t.Fatal("management enabled by default")
	}
	o.Enabled = false
	o.WorkflowManagement = AIWorkflowManagementOptions{Enabled: true, Address: "qs-ai-grpc:50061", CAFile: "ca.pem", CertFile: "qs.pem", KeyFile: "qs.key"}
	if errs := o.Validate(); len(errs) != 0 {
		t.Fatal(errs)
	}
	for _, change := range []func(*AIWorkflowManagementOptions){func(v *AIWorkflowManagementOptions) { v.Address = "" }, func(v *AIWorkflowManagementOptions) { v.CAFile = "" }, func(v *AIWorkflowManagementOptions) { v.CertFile = "" }, func(v *AIWorkflowManagementOptions) { v.KeyFile = "" }} {
		broken := o.WorkflowManagement
		change(&broken)
		if broken.Validate() == nil {
			t.Fatal("incomplete TLS configuration accepted")
		}
	}
}
