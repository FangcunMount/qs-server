package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const (
	testeeAssignmentStrategyExplicit   = "explicit"
	testeeAssignmentStrategyRoundRobin = "round_robin"
	defaultAssignmentPageSize          = 100
)

type clinicianAssignmentTarget struct {
	ID           string
	Name         string
	EmployeeCode string
}

func seedAssignTestees(ctx context.Context, deps *dependencies) error {
	orgID := deps.Config.Global.OrgID
	if orgID == 0 {
		return fmt.Errorf("global.orgId is required for testee assignment seeding")
	}
	if len(deps.Config.TesteeAssignments) == 0 {
		deps.Logger.Infow("No testee assignment configs found, skipping assignment seeding")
		return nil
	}

	staffConfigs, err := effectiveStaffConfigs(deps.Config)
	if err != nil {
		return err
	}
	staffIndex, err := indexStaffConfigs(staffConfigs)
	if err != nil {
		return err
	}
	clinicianConfigs, err := effectiveClinicianConfigs(deps.Config)
	if err != nil {
		return err
	}
	clinicianIndex, err := indexClinicianConfigs(clinicianConfigs)
	if err != nil {
		return err
	}
	existingStaff, err := listAllStaff(ctx, deps.APIClient, orgID)
	if err != nil {
		return err
	}
	existingClinicians, err := listAllClinicians(ctx, deps.APIClient, orgID)
	if err != nil {
		return err
	}

	for idx, cfg := range deps.Config.TesteeAssignments {
		if err := validateTesteeAssignmentConfig(cfg); err != nil {
			return fmt.Errorf("invalid testee assignment config at index %d: %w", idx, err)
		}

		targets, err := resolveAssignmentClinicianTargets(ctx, deps, orgID, cfg, clinicianIndex, staffIndex, &existingClinicians, &existingStaff)
		if err != nil {
			return fmt.Errorf("resolve clinicians for assignment %q failed: %w", testeeAssignmentLabel(cfg, idx), err)
		}
		testees, err := resolveAssignmentTestees(ctx, deps.APIClient, orgID, cfg)
		if err != nil {
			return fmt.Errorf("resolve testees for assignment %q failed: %w", testeeAssignmentLabel(cfg, idx), err)
		}
		if len(testees) == 0 {
			deps.Logger.Warnw("No testees resolved for assignment, skipping",
				"assignment", testeeAssignmentLabel(cfg, idx),
			)
			continue
		}

		assignedCount, skippedCount, err := applyTesteeAssignment(ctx, deps, orgID, cfg, targets, testees)
		if err != nil {
			return fmt.Errorf("apply assignment %q failed: %w", testeeAssignmentLabel(cfg, idx), err)
		}
		deps.Logger.Infow("Testee assignment completed",
			"assignment", testeeAssignmentLabel(cfg, idx),
			"strategy", normalizedAssignmentStrategy(cfg.Strategy),
			"relation_type", normalizedAssignmentRelationType(cfg.RelationType),
			"testee_count", len(testees),
			"target_count", len(targets),
			"assigned", assignedCount,
			"skipped", skippedCount,
		)
	}
	return nil
}

func validateTesteeAssignmentConfig(cfg TesteeAssignmentConfig) error {
	strategy := normalizedAssignmentStrategy(cfg.Strategy)
	switch strategy {
	case testeeAssignmentStrategyExplicit:
		if strings.TrimSpace(cfg.ClinicianRef) == "" && cfg.ClinicianID.IsZero() {
			return fmt.Errorf("clinicianRef or clinicianId is required for explicit assignment")
		}
	case testeeAssignmentStrategyRoundRobin:
		targetCount := len(nonEmptyStrings(cfg.ClinicianRefs)) + len(nonEmptyStrings(cfg.ClinicianKeyPrefixes)) + len(nonZeroFlexibleIDs(cfg.ClinicianIDs))
		if strings.TrimSpace(cfg.ClinicianRef) != "" || !cfg.ClinicianID.IsZero() {
			targetCount++
		}
		if targetCount == 0 {
			return fmt.Errorf("at least one clinicianRef, clinicianKeyPrefixes, or clinicianId is required for round_robin assignment")
		}
	default:
		return fmt.Errorf("unsupported strategy %q", cfg.Strategy)
	}

	if len(cfg.TesteeIDs) == 0 && cfg.TesteeLimit < 0 {
		return fmt.Errorf("testeeLimit cannot be negative")
	}
	if cfg.TesteeOffset < 0 {
		return fmt.Errorf("testeeOffset cannot be negative")
	}
	return nil
}

func indexClinicianConfigs(configs []ClinicianConfig) (map[string]ClinicianConfig, error) {
	index := make(map[string]ClinicianConfig, len(configs))
	for idx, cfg := range configs {
		key := strings.TrimSpace(cfg.Key)
		if key == "" {
			continue
		}
		if _, exists := index[key]; exists {
			return nil, fmt.Errorf("duplicate clinician key %q at index %d", key, idx)
		}
		index[key] = cfg
	}
	return index, nil
}

func resolveAssignmentClinicianTargets(
	ctx context.Context,
	deps *dependencies,
	orgID int64,
	cfg TesteeAssignmentConfig,
	clinicianIndex map[string]ClinicianConfig,
	staffIndex map[string]StaffConfig,
	existingClinicians *[]*ClinicianResponse,
	existingStaff *[]*StaffResponse,
) ([]clinicianAssignmentTarget, error) {
	type targetSpec struct {
		ref string
		id  FlexibleID
	}

	specs := make([]targetSpec, 0, 8)
	if ref := strings.TrimSpace(cfg.ClinicianRef); ref != "" {
		specs = append(specs, targetSpec{ref: ref})
	}
	if !cfg.ClinicianID.IsZero() {
		specs = append(specs, targetSpec{id: cfg.ClinicianID})
	}
	for _, ref := range nonEmptyStrings(cfg.ClinicianRefs) {
		specs = append(specs, targetSpec{ref: ref})
	}
	for _, prefix := range nonEmptyStrings(cfg.ClinicianKeyPrefixes) {
		matchedRefs := clinicianRefsByPrefix(clinicianIndex, prefix)
		for _, ref := range matchedRefs {
			specs = append(specs, targetSpec{ref: ref})
		}
	}
	for _, id := range nonZeroFlexibleIDs(cfg.ClinicianIDs) {
		specs = append(specs, targetSpec{id: id})
	}

	targets := make([]clinicianAssignmentTarget, 0, len(specs))
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		item, err := resolveAssignmentClinicianTarget(ctx, deps, orgID, spec, clinicianIndex, staffIndex, existingClinicians, existingStaff)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[item.ID]; exists {
			continue
		}
		seen[item.ID] = struct{}{}
		targets = append(targets, item)
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("no clinician targets resolved")
	}
	return targets, nil
}

func resolveAssignmentClinicianTarget(
	ctx context.Context,
	deps *dependencies,
	orgID int64,
	spec struct {
		ref string
		id  FlexibleID
	},
	clinicianIndex map[string]ClinicianConfig,
	staffIndex map[string]StaffConfig,
	existingClinicians *[]*ClinicianResponse,
	existingStaff *[]*StaffResponse,
) (clinicianAssignmentTarget, error) {
	if !spec.id.IsZero() {
		targetID := spec.id.String()
		for _, item := range *existingClinicians {
			if strings.TrimSpace(item.ID) == targetID {
				return toClinicianAssignmentTarget(item), nil
			}
		}
		return clinicianAssignmentTarget{}, fmt.Errorf("clinicianId %q not found in organization %d", targetID, orgID)
	}

	cfg, ok := clinicianIndex[strings.TrimSpace(spec.ref)]
	if !ok {
		return clinicianAssignmentTarget{}, fmt.Errorf("clinicianRef %q not found in clinicians config", spec.ref)
	}
	if err := validateClinicianConfig(cfg); err != nil {
		return clinicianAssignmentTarget{}, fmt.Errorf("referenced clinician %q invalid: %w", spec.ref, err)
	}

	operatorID, err := resolveClinicianOperatorID(ctx, deps, orgID, cfg, staffIndex, existingStaff)
	if err != nil {
		return clinicianAssignmentTarget{}, err
	}
	item, _, err := ensureClinician(ctx, deps, orgID, cfg, operatorID, existingClinicians)
	if err != nil {
		return clinicianAssignmentTarget{}, err
	}
	return toClinicianAssignmentTarget(item), nil
}

func resolveAssignmentTestees(ctx context.Context, client *APIClient, orgID int64, cfg TesteeAssignmentConfig) ([]*ApiserverTesteeResponse, error) {
	if len(cfg.TesteeIDs) > 0 {
		items := make([]*ApiserverTesteeResponse, 0, len(cfg.TesteeIDs))
		for _, id := range cfg.TesteeIDs {
			if id.IsZero() {
				continue
			}
			items = append(items, &ApiserverTesteeResponse{ID: id.String()})
		}
		return items, nil
	}

	pageSize := cfg.TesteePageSize
	if pageSize <= 0 {
		pageSize = defaultAssignmentPageSize
	}
	remaining := cfg.TesteeLimit
	offset := cfg.TesteeOffset
	page := 1
	skipped := 0
	items := make([]*ApiserverTesteeResponse, 0, pageSize)
	for {
		resp, err := client.ListTesteesByOrg(ctx, orgID, page, pageSize)
		if err != nil {
			return nil, err
		}
		if len(resp.Items) == 0 {
			break
		}
		for _, item := range resp.Items {
			if skipped < offset {
				skipped++
				continue
			}
			if remaining == 0 && cfg.TesteeLimit > 0 {
				return items, nil
			}
			items = append(items, item)
			if cfg.TesteeLimit > 0 {
				remaining--
			}
		}
		if resp.TotalPages > 0 && page >= resp.TotalPages {
			break
		}
		page++
	}
	return items, nil
}

func applyTesteeAssignment(
	ctx context.Context,
	deps *dependencies,
	orgID int64,
	cfg TesteeAssignmentConfig,
	targets []clinicianAssignmentTarget,
	testees []*ApiserverTesteeResponse,
) (assignedCount int, skippedCount int, err error) {
	strategy := normalizedAssignmentStrategy(cfg.Strategy)
	relationType := normalizedAssignmentRelationType(cfg.RelationType)
	sourceType := strings.TrimSpace(cfg.SourceType)
	if sourceType == "" {
		sourceType = "manual"
	}

	switch strategy {
	case testeeAssignmentStrategyRoundRobin:
		targetIndex := 0
		for _, testee := range testees {
			relations, relationErr := deps.APIClient.GetTesteeClinicians(ctx, testee.ID)
			if relationErr != nil {
				return assignedCount, skippedCount, fmt.Errorf("get clinician relations for testee %s: %w", testee.ID, relationErr)
			}
			if !cfg.IncludeAlreadyAssigned && hasAnyActiveAccessRelation(relations.Items) {
				skippedCount++
				continue
			}

			target := targets[targetIndex%len(targets)]
			targetIndex++
			if hasMatchingActiveRelation(relations.Items, target.ID, relationType) {
				skippedCount++
				continue
			}
			if _, assignErr := assignSingleTestee(ctx, deps.APIClient, orgID, target, testee.ID, relationType, sourceType); assignErr != nil {
				return assignedCount, skippedCount, assignErr
			}
			assignedCount++
		}
		return assignedCount, skippedCount, nil

	default:
		target := targets[0]
		for _, testee := range testees {
			relations, relationErr := deps.APIClient.GetTesteeClinicians(ctx, testee.ID)
			if relationErr != nil {
				return assignedCount, skippedCount, fmt.Errorf("get clinician relations for testee %s: %w", testee.ID, relationErr)
			}
			if !cfg.IncludeAlreadyAssigned && hasAnyActiveAccessRelation(relations.Items) {
				skippedCount++
				continue
			}
			if hasMatchingActiveRelation(relations.Items, target.ID, relationType) {
				skippedCount++
				continue
			}
			if _, assignErr := assignSingleTestee(ctx, deps.APIClient, orgID, target, testee.ID, relationType, sourceType); assignErr != nil {
				return assignedCount, skippedCount, assignErr
			}
			assignedCount++
		}
		return assignedCount, skippedCount, nil
	}
}

func assignSingleTestee(
	ctx context.Context,
	client *APIClient,
	orgID int64,
	target clinicianAssignmentTarget,
	testeeID string,
	relationType string,
	sourceType string,
) (*RelationResponse, error) {
	clinicianID, err := parseUint64String(target.ID, "clinician_id")
	if err != nil {
		return nil, err
	}
	parsedTesteeID, err := parseUint64String(testeeID, "testee_id")
	if err != nil {
		return nil, err
	}

	resp, err := client.AssignClinicianTesteeWithRelationType(ctx, relationType, AssignClinicianTesteeRequest{
		OrgID:        orgID,
		ClinicianID:  clinicianID,
		TesteeID:     parsedTesteeID,
		RelationType: relationType,
		SourceType:   sourceType,
	})
	if err != nil {
		return nil, fmt.Errorf("assign testee %s to clinician %s: %w", testeeID, target.ID, err)
	}
	return resp, nil
}

func hasAnyActiveAccessRelation(items []*TesteeClinicianRelationResponse) bool {
	for _, item := range items {
		if item == nil || item.Relation == nil {
			continue
		}
		if item.Relation.IsActive && isAccessGrantRelationType(item.Relation.RelationType) {
			return true
		}
	}
	return false
}

func hasMatchingActiveRelation(items []*TesteeClinicianRelationResponse, clinicianID string, relationType string) bool {
	for _, item := range items {
		if item == nil || item.Relation == nil {
			continue
		}
		if !item.Relation.IsActive {
			continue
		}
		if strings.TrimSpace(item.Relation.ClinicianID) != clinicianID {
			continue
		}
		if normalizedAssignmentRelationType(item.Relation.RelationType) == relationType {
			return true
		}
	}
	return false
}

func normalizedAssignmentStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", testeeAssignmentStrategyExplicit:
		return testeeAssignmentStrategyExplicit
	case "roundrobin":
		return testeeAssignmentStrategyRoundRobin
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizedAssignmentRelationType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "assigned", "attending":
		return "attending"
	case "primary":
		return "primary"
	case "collaborator":
		return "collaborator"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func nonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	return result
}

func nonZeroFlexibleIDs(values []FlexibleID) []FlexibleID {
	result := make([]FlexibleID, 0, len(values))
	for _, value := range values {
		if value.IsZero() {
			continue
		}
		result = append(result, value)
	}
	return result
}

func toClinicianAssignmentTarget(item *ClinicianResponse) clinicianAssignmentTarget {
	return clinicianAssignmentTarget{
		ID:           strings.TrimSpace(item.ID),
		Name:         item.Name,
		EmployeeCode: item.EmployeeCode,
	}
}

func parseUint64String(value string, field string) (uint64, error) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s %q: %w", field, value, err)
	}
	return parsed, nil
}

func testeeAssignmentLabel(cfg TesteeAssignmentConfig, idx int) string {
	switch {
	case strings.TrimSpace(cfg.Key) != "":
		return cfg.Key
	case strings.TrimSpace(cfg.ClinicianRef) != "":
		return cfg.ClinicianRef
	default:
		return fmt.Sprintf("testeeAssignment[%d]", idx)
	}
}
