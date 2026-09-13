package options

import "testing"

func TestWorkflowIntakeRequiresTransportWithoutLegacyAIOrManagement(t *testing.T) {
	o := NewAIExplanationOptions()
	o.Enabled, o.ParticipantEnabled, o.Evaluation.Enabled = false, false, false
	o.WorkflowEnabled = true
	if len(o.Validate()) == 0 {
		t.Fatal("intake accepted without relay configuration")
	}
	o.WorkflowManagement = AIWorkflowManagementOptions{Address: "qs-ai-grpc:50061", CAFile: "ca.pem", CertFile: "qs.pem", KeyFile: "qs.key"}
	if errs := o.Validate(); len(errs) != 0 {
		t.Fatal(errs)
	}
	if o.WorkflowManagement.Enabled {
		t.Fatal("validation changed management flag")
	}
}

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
