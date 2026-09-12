package actor

import (
	clinician "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	"testing"
)

func TestRelationshipExportsKeepBackstageAndInternalServicesSeparate(t *testing.T) {
	internal := clinician.NewRelationshipService(nil, nil, nil, nil)
	backstage := clinician.NewOperatorRelationshipService(nil, nil, nil, nil, nil, nil, nil)
	module := &Module{ClinicianRelationshipService: internal, OperatorClinicianRelationshipService: backstage}
	if module.ExportRESTDeps(nil).ClinicianRelationshipService != backstage {
		t.Fatal("REST must use backstage relationship boundary")
	}
	if module.ExportGRPCDeps().ClinicianRelationshipService != internal {
		t.Fatal("internal relationship service was replaced")
	}
	module.OperatorClinicianRelationshipService = nil
	if module.ExportRESTDeps(nil).ClinicianRelationshipService != nil {
		t.Fatal("missing backstage service fell back to internal authority")
	}
}
