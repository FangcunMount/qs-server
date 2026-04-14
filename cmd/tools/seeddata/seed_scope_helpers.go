package main

import (
	"context"
	"fmt"
	"strings"
)

type seedClinicianScopeSpec struct {
	refs []string
	ids  []FlexibleID
}

func resolveSeedClinicianScope(
	ctx context.Context,
	deps *dependencies,
	spec seedClinicianScopeSpec,
) ([]*ClinicianResponse, error) {
	orgID := deps.Config.Global.OrgID
	if orgID == 0 {
		return nil, fmt.Errorf("global.orgId is required")
	}

	existingClinicians, err := listAllClinicians(ctx, deps.APIClient, orgID)
	if err != nil {
		return nil, err
	}

	refs := nonEmptyStrings(spec.refs)
	ids := nonZeroFlexibleIDs(spec.ids)
	if len(refs) == 0 && len(ids) == 0 {
		return existingClinicians, nil
	}

	staffIndex, err := indexStaffConfigs(deps.Config.Staffs)
	if err != nil {
		return nil, err
	}
	clinicianIndex, err := indexClinicianConfigs(deps.Config.Clinicians)
	if err != nil {
		return nil, err
	}
	existingStaff, err := listAllStaff(ctx, deps.APIClient, orgID)
	if err != nil {
		return nil, err
	}

	type targetSpec struct {
		ref string
		id  FlexibleID
	}

	specs := make([]targetSpec, 0, len(refs)+len(ids))
	for _, ref := range refs {
		specs = append(specs, targetSpec{ref: ref})
	}
	for _, id := range ids {
		specs = append(specs, targetSpec{id: id})
	}

	resolved := make([]*ClinicianResponse, 0, len(specs))
	seen := make(map[string]struct{}, len(specs))
	for _, item := range specs {
		target, err := resolveAssignmentClinicianTarget(
			ctx,
			deps,
			orgID,
			struct {
				ref string
				id  FlexibleID
			}{ref: item.ref, id: item.id},
			clinicianIndex,
			staffIndex,
			&existingClinicians,
			&existingStaff,
		)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[target.ID]; exists {
			continue
		}
		seen[target.ID] = struct{}{}
		for _, clinicianItem := range existingClinicians {
			if clinicianItem != nil && strings.TrimSpace(clinicianItem.ID) == target.ID {
				resolved = append(resolved, clinicianItem)
				break
			}
		}
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("no clinicians resolved from configured scope")
	}
	return resolved, nil
}
