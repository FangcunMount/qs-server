package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const seedActorPageSize = 100

func seedStaffs(ctx context.Context, deps *dependencies) error {
	orgID := deps.Config.Global.OrgID
	if orgID == 0 {
		return fmt.Errorf("global.orgId is required for staff seeding")
	}
	staffConfigs, err := effectiveStaffConfigs(deps.Config)
	if err != nil {
		return err
	}
	if len(staffConfigs) == 0 {
		deps.Logger.Infow("No staff configs found, skipping staff seeding")
		return nil
	}

	if _, err := indexStaffConfigs(staffConfigs); err != nil {
		return err
	}

	existing, err := listAllStaff(ctx, deps.APIClient, orgID)
	if err != nil {
		return err
	}

	createdCount := 0
	reusedCount := 0
	for idx, cfg := range staffConfigs {
		if err := validateStaffConfig(cfg); err != nil {
			return fmt.Errorf("invalid staff config at index %d: %w", idx, err)
		}
		item, created, err := ensureStaff(ctx, deps, orgID, cfg, &existing)
		if err != nil {
			return fmt.Errorf("seed staff %q failed: %w", staffConfigLabel(cfg, idx), err)
		}
		if created {
			createdCount++
		} else {
			reusedCount++
		}
		deps.Logger.Infow("Staff seed ensured",
			"key", cfg.Key,
			"name", item.Name,
			"staff_id", item.ID,
			"user_id", item.UserID,
			"created", created,
		)
	}

	deps.Logger.Infow("Staff seeding completed",
		"configured", len(staffConfigs),
		"created", createdCount,
		"reused", reusedCount,
	)
	return nil
}

func seedClinicians(ctx context.Context, deps *dependencies) error {
	orgID := deps.Config.Global.OrgID
	if orgID == 0 {
		return fmt.Errorf("global.orgId is required for clinician seeding")
	}
	clinicianConfigs, err := effectiveClinicianConfigs(deps.Config)
	if err != nil {
		return err
	}
	if len(clinicianConfigs) == 0 {
		deps.Logger.Infow("No clinician configs found, skipping clinician seeding")
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
	existingStaff, err := listAllStaff(ctx, deps.APIClient, orgID)
	if err != nil {
		return err
	}
	existingClinicians, err := listAllClinicians(ctx, deps.APIClient, orgID)
	if err != nil {
		return err
	}

	createdCount := 0
	reusedCount := 0
	for idx, cfg := range clinicianConfigs {
		if err := validateClinicianConfig(cfg); err != nil {
			return fmt.Errorf("invalid clinician config at index %d: %w", idx, err)
		}

		operatorID, err := resolveClinicianOperatorID(ctx, deps, orgID, cfg, staffIndex, &existingStaff)
		if err != nil {
			return fmt.Errorf("resolve operator for clinician %q failed: %w", clinicianConfigLabel(cfg, idx), err)
		}

		item, created, err := ensureClinician(ctx, deps, orgID, cfg, operatorID, &existingClinicians)
		if err != nil {
			return fmt.Errorf("seed clinician %q failed: %w", clinicianConfigLabel(cfg, idx), err)
		}
		if created {
			createdCount++
		} else {
			reusedCount++
		}
		deps.Logger.Infow("Clinician seed ensured",
			"key", cfg.Key,
			"name", item.Name,
			"clinician_id", item.ID,
			"operator_id", nullableString(item.OperatorID),
			"created", created,
		)
	}

	deps.Logger.Infow("Clinician seeding completed",
		"configured", len(clinicianConfigs),
		"created", createdCount,
		"reused", reusedCount,
	)
	return nil
}

func listAllStaff(ctx context.Context, client *APIClient, orgID int64) ([]*StaffResponse, error) {
	page := 1
	items := make([]*StaffResponse, 0, seedActorPageSize)
	for {
		resp, err := client.ListStaff(ctx, orgID, page, seedActorPageSize)
		if err != nil {
			return nil, err
		}
		if len(resp.Items) == 0 {
			break
		}
		items = append(items, resp.Items...)
		if resp.TotalPages > 0 && page >= resp.TotalPages {
			break
		}
		page++
	}
	return items, nil
}

func listAllClinicians(ctx context.Context, client *APIClient, orgID int64) ([]*ClinicianResponse, error) {
	page := 1
	items := make([]*ClinicianResponse, 0, seedActorPageSize)
	for {
		resp, err := client.ListClinicians(ctx, orgID, page, seedActorPageSize)
		if err != nil {
			return nil, err
		}
		if len(resp.Items) == 0 {
			break
		}
		items = append(items, resp.Items...)
		if resp.TotalPages > 0 && page >= resp.TotalPages {
			break
		}
		page++
	}
	return items, nil
}

func indexStaffConfigs(configs []StaffConfig) (map[string]StaffConfig, error) {
	index := make(map[string]StaffConfig, len(configs))
	for idx, cfg := range configs {
		key := strings.TrimSpace(cfg.Key)
		if key == "" {
			continue
		}
		if _, exists := index[key]; exists {
			return nil, fmt.Errorf("duplicate staff key %q at index %d", key, idx)
		}
		index[key] = cfg
	}
	return index, nil
}

func validateStaffConfig(cfg StaffConfig) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(cfg.Roles) == 0 {
		return fmt.Errorf("roles are required")
	}
	if cfg.UserID.IsZero() {
		if strings.TrimSpace(cfg.Phone) == "" {
			return fmt.Errorf("phone is required when userId is not provided")
		}
		if strings.TrimSpace(cfg.Password) == "" {
			return fmt.Errorf("password is required when userId is not provided")
		}
	}
	return nil
}

func validateClinicianConfig(cfg ClinicianConfig) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(cfg.ClinicianType) == "" {
		return fmt.Errorf("clinicianType is required")
	}
	if strings.TrimSpace(cfg.OperatorRef) == "" && cfg.OperatorID.IsZero() && strings.TrimSpace(cfg.EmployeeCode) == "" {
		return fmt.Errorf("one of operatorRef, operatorId, employeeCode is required for idempotent seeding")
	}
	return nil
}

func ensureStaff(
	ctx context.Context,
	deps *dependencies,
	orgID int64,
	cfg StaffConfig,
	existing *[]*StaffResponse,
) (*StaffResponse, bool, error) {
	if matched := findMatchingStaff(*existing, cfg); matched != nil {
		return matched, false, nil
	}

	req := CreateStaffRequest{
		OrgID:    orgID,
		Roles:    append([]string(nil), cfg.Roles...),
		Name:     strings.TrimSpace(cfg.Name),
		Email:    strings.TrimSpace(cfg.Email),
		Phone:    strings.TrimSpace(cfg.Phone),
		Password: cfg.Password,
		IsActive: cfg.IsActive,
	}
	if !cfg.UserID.IsZero() {
		userID, err := cfg.UserID.Uint64()
		if err != nil {
			return nil, false, fmt.Errorf("parse userId: %w", err)
		}
		req.UserID = &userID
	}

	created, err := deps.APIClient.CreateStaff(ctx, req)
	if err != nil {
		return nil, false, err
	}
	*existing = append(*existing, created)
	return created, true, nil
}

func ensureClinician(
	ctx context.Context,
	deps *dependencies,
	orgID int64,
	cfg ClinicianConfig,
	operatorID string,
	existing *[]*ClinicianResponse,
) (*ClinicianResponse, bool, error) {
	if matched := findMatchingClinician(*existing, cfg, operatorID); matched != nil {
		return matched, false, nil
	}

	isActive := true
	if cfg.IsActive != nil {
		isActive = *cfg.IsActive
	}

	req := CreateClinicianRequest{
		OrgID:         orgID,
		Name:          strings.TrimSpace(cfg.Name),
		Department:    strings.TrimSpace(cfg.Department),
		Title:         strings.TrimSpace(cfg.Title),
		ClinicianType: strings.TrimSpace(cfg.ClinicianType),
		EmployeeCode:  strings.TrimSpace(cfg.EmployeeCode),
		IsActive:      isActive,
	}
	if operatorID != "" {
		value, err := strconv.ParseUint(operatorID, 10, 64)
		if err != nil {
			return nil, false, fmt.Errorf("parse operator_id %q: %w", operatorID, err)
		}
		req.OperatorID = &value
	}

	created, err := deps.APIClient.CreateClinician(ctx, req)
	if err != nil {
		return nil, false, err
	}
	*existing = append(*existing, created)
	return created, true, nil
}

func resolveClinicianOperatorID(
	ctx context.Context,
	deps *dependencies,
	orgID int64,
	cfg ClinicianConfig,
	staffIndex map[string]StaffConfig,
	existingStaff *[]*StaffResponse,
) (string, error) {
	if !cfg.OperatorID.IsZero() {
		return cfg.OperatorID.String(), nil
	}
	if strings.TrimSpace(cfg.OperatorRef) == "" {
		return "", nil
	}

	staffCfg, ok := staffIndex[strings.TrimSpace(cfg.OperatorRef)]
	if !ok {
		return "", fmt.Errorf("operatorRef %q not found in staffs config", cfg.OperatorRef)
	}
	if err := validateStaffConfig(staffCfg); err != nil {
		return "", fmt.Errorf("referenced staff %q invalid: %w", cfg.OperatorRef, err)
	}

	staffItem, _, err := ensureStaff(ctx, deps, orgID, staffCfg, existingStaff)
	if err != nil {
		return "", err
	}
	return staffItem.ID, nil
}

func findMatchingStaff(existing []*StaffResponse, cfg StaffConfig) *StaffResponse {
	if !cfg.UserID.IsZero() {
		target := cfg.UserID.String()
		for _, item := range existing {
			if strings.TrimSpace(item.UserID) == target {
				return item
			}
		}
		return nil
	}

	phone := normalizePhone(cfg.Phone)
	if phone != "" {
		for _, item := range existing {
			if normalizePhone(item.Phone) == phone {
				return item
			}
		}
		return nil
	}

	email := normalizeEmail(cfg.Email)
	if email != "" {
		for _, item := range existing {
			if normalizeEmail(item.Email) == email {
				return item
			}
		}
	}
	return nil
}

func findMatchingClinician(existing []*ClinicianResponse, cfg ClinicianConfig, operatorID string) *ClinicianResponse {
	if operatorID != "" {
		for _, item := range existing {
			if item.OperatorID != nil && strings.TrimSpace(*item.OperatorID) == operatorID {
				return item
			}
		}
		return nil
	}

	employeeCode := strings.TrimSpace(cfg.EmployeeCode)
	if employeeCode != "" {
		for _, item := range existing {
			if strings.TrimSpace(item.EmployeeCode) == employeeCode {
				return item
			}
		}
	}
	return nil
}

func staffConfigLabel(cfg StaffConfig, idx int) string {
	switch {
	case strings.TrimSpace(cfg.Key) != "":
		return cfg.Key
	case strings.TrimSpace(cfg.Phone) != "":
		return cfg.Phone
	case !cfg.UserID.IsZero():
		return cfg.UserID.String()
	default:
		return fmt.Sprintf("staff[%d]", idx)
	}
}

func clinicianConfigLabel(cfg ClinicianConfig, idx int) string {
	switch {
	case strings.TrimSpace(cfg.Key) != "":
		return cfg.Key
	case strings.TrimSpace(cfg.EmployeeCode) != "":
		return cfg.EmployeeCode
	default:
		return fmt.Sprintf("clinician[%d]", idx)
	}
}

func normalizePhone(value string) string {
	return strings.TrimSpace(value)
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func nullableString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
