package main

import (
	"encoding/json"
	"errors"
)

// These owner/freeze DTOs are shared literally with the IAM-owned coordinator.
// Hashes bind one original group/protection file; neither side reserializes it.
type ownerConfirmation struct {
	Format         int      `json:"format_version"`
	Kind           string   `json:"kind"`
	Operation      string   `json:"operation_id"`
	Owner          string   `json:"owner_user_id"`
	IAMSHA         string   `json:"iam_sha256"`
	QSSHA          string   `json:"qs_sha256"`
	ProtectedSHA   string   `json:"protected_manifest_sha256"`
	Dedicated      bool     `json:"dedicated_mock_cohort"`
	SharedExcluded bool     `json:"real_and_shared_excluded"`
	AssignmentOrgs []string `json:"iam_assignment_org_ids"`
}

type freezeReceipt struct {
	Format        int    `json:"format_version"`
	Kind          string `json:"kind"`
	Operation     string `json:"operation_id"`
	GroupSHA      string `json:"group_manifest_sha256"`
	IAMStopped    bool   `json:"all_iam_writers_stopped"`
	QSStopped     bool   `json:"all_qs_writers_stopped"`
	ReplaySettled bool   `json:"messages_and_manual_replay_settled"`
	Reload        bool   `json:"iam_projection_reload_before_reopen"`
}

type actionReceipt struct {
	Format      int    `json:"format_version"`
	Kind        string `json:"kind"`
	Action      string `json:"action"`
	Operation   string `json:"operation_id"`
	GroupSHA    string `json:"group_manifest_sha256"`
	Source      string `json:"compiled_source"`
	MaterialSHA string `json:"material_sha256"`
	Complete    bool   `json:"complete"`
	DryRun      bool   `json:"dry_run"`
	Whole       bool   `json:"whole_cohort_complete"`
	Pending     bool   `json:"semantic_graph_expansion_pending"`
	Exclusions  int    `json:"excluded_components"`
	Released    bool   `json:"resource_release_proven"`
}

func validateOwner(rawGroup, rawOwner []byte, g group) error {
	if digest(rawOwner) != g.OwnerSHA {
		return errors.New("owner original bytes changed")
	}
	var o ownerConfirmation
	if e := strictDecode(rawOwner, &o); e != nil {
		return e
	}
	var parts map[string]json.RawMessage
	if e := json.Unmarshal(rawGroup, &parts); e != nil {
		return e
	}
	if o.Format != 1 || o.Kind != "mock_cohort_owner_confirmation" || o.Operation != g.Operation || o.Owner != "10001" || o.IAMSHA != digest(parts["iam"]) || o.QSSHA != digest(parts["qs"]) || o.ProtectedSHA != g.ProtectedSHA || !o.Dedicated || !o.SharedExcluded || o.AssignmentOrgs == nil {
		return errors.New("owner confirmation does not bind the exact group subdocuments")
	}
	return nil
}

func validateFreeze(raw []byte, g group, groupSHA string) error {
	var f freezeReceipt
	if e := strictDecode(raw, &f); e != nil {
		return e
	}
	if f.Format != 1 || f.Kind != "mock_cohort_frozen" || f.Operation != g.Operation || f.GroupSHA != groupSHA || !f.IAMStopped || !f.QSStopped || !f.ReplaySettled || !f.Reload {
		return errors.New("independent freeze/replay evidence incomplete")
	}
	return nil
}
