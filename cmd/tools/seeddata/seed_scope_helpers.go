package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type seedClinicianScopeSpec struct {
	refs        []string
	keyPrefixes []string
	ids         []FlexibleID
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
	keyPrefixes := nonEmptyStrings(spec.keyPrefixes)
	ids := nonZeroFlexibleIDs(spec.ids)
	if len(refs) == 0 && len(keyPrefixes) == 0 && len(ids) == 0 {
		return existingClinicians, nil
	}

	staffConfigs, err := effectiveStaffConfigs(deps.Config)
	if err != nil {
		return nil, err
	}
	staffIndex, err := indexStaffConfigs(staffConfigs)
	if err != nil {
		return nil, err
	}
	clinicianConfigs, err := effectiveClinicianConfigs(deps.Config)
	if err != nil {
		return nil, err
	}
	clinicianIndex, err := indexClinicianConfigs(clinicianConfigs)
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
	for _, prefix := range keyPrefixes {
		matchedRefs := clinicianRefsByPrefix(clinicianIndex, prefix)
		for _, ref := range matchedRefs {
			specs = append(specs, targetSpec{ref: ref})
		}
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

func clinicianRefsByPrefix(clinicianIndex map[string]ClinicianConfig, prefix string) []string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || len(clinicianIndex) == 0 {
		return nil
	}
	refs := make([]string, 0, len(clinicianIndex))
	for key := range clinicianIndex {
		if strings.HasPrefix(strings.TrimSpace(key), prefix) {
			refs = append(refs, key)
		}
	}
	sort.Strings(refs)
	return refs
}
