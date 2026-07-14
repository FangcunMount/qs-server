package main

import (
	"encoding/json"
	"fmt"
)

type normalizationSummary struct {
	OutcomeAdapter string
	ReportAdapter  string
}

func (s normalizationSummary) Changed() bool {
	return s.OutcomeAdapter != "" || s.ReportAdapter != ""
}

func normalizeDefinition(input []byte, target migrationTarget) ([]byte, normalizationSummary, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(input, &root); err != nil {
		return nil, normalizationSummary{}, fmt.Errorf("decode DefinitionV2: %w", err)
	}
	if len(root) == 0 {
		return nil, normalizationSummary{}, fmt.Errorf("DefinitionV2 must be a non-empty object")
	}
	var conclusions []json.RawMessage
	if err := json.Unmarshal(root["Conclusions"], &conclusions); err != nil {
		return nil, normalizationSummary{}, fmt.Errorf("decode Conclusions: %w", err)
	}
	typeIndex := -1
	for index, raw := range conclusions {
		var header struct {
			Kind string `json:"Kind"`
		}
		if err := json.Unmarshal(raw, &header); err != nil {
			return nil, normalizationSummary{}, fmt.Errorf("decode Conclusions[%d]: %w", index, err)
		}
		if header.Kind != "type" {
			continue
		}
		if typeIndex >= 0 {
			return nil, normalizationSummary{}, fmt.Errorf("DefinitionV2 contains more than one type conclusion")
		}
		typeIndex = index
	}
	if typeIndex < 0 {
		return nil, normalizationSummary{}, fmt.Errorf("DefinitionV2 does not contain a type conclusion")
	}

	var typeConclusion map[string]json.RawMessage
	if err := json.Unmarshal(conclusions[typeIndex], &typeConclusion); err != nil {
		return nil, normalizationSummary{}, fmt.Errorf("decode type conclusion: %w", err)
	}
	changedOutcome, err := normalizeOutcomeAdapter(typeConclusion, target)
	if err != nil {
		return nil, normalizationSummary{}, err
	}
	if changedOutcome {
		conclusions[typeIndex] = mustJSON(typeConclusion)
		root["Conclusions"] = mustJSON(conclusions)
	}
	changedReport, err := normalizeReportAdapter(root, target)
	if err != nil {
		return nil, normalizationSummary{}, err
	}
	output, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, normalizationSummary{}, fmt.Errorf("encode normalized DefinitionV2: %w", err)
	}
	summary := normalizationSummary{}
	if changedOutcome {
		summary.OutcomeAdapter = target.GenericAdapter
	}
	if changedReport {
		summary.ReportAdapter = target.GenericAdapter
	}
	if err := verifyNormalizedDefinition(output, target); err != nil {
		return nil, normalizationSummary{}, err
	}
	return append(output, '\n'), summary, nil
}

func normalizeOutcomeAdapter(typeConclusion map[string]json.RawMessage, target migrationTarget) (bool, error) {
	var mapping map[string]json.RawMessage
	if err := json.Unmarshal(typeConclusion["OutcomeMapping"], &mapping); err != nil {
		return false, fmt.Errorf("decode type conclusion OutcomeMapping: %w", err)
	}
	detailKind, _, err := rawString(mapping, "DetailKind")
	if err != nil {
		return false, fmt.Errorf("decode OutcomeMapping.DetailKind: %w", err)
	}
	if detailKind != target.GenericAdapter {
		return false, fmt.Errorf("OutcomeMapping.DetailKind is %q, want %s", detailKind, target.GenericAdapter)
	}
	adapter, present, err := rawString(mapping, "DetailAdapterKey")
	if err != nil {
		return false, fmt.Errorf("decode OutcomeMapping.DetailAdapterKey: %w", err)
	}
	if adapter == target.GenericAdapter {
		return false, nil
	}
	if !present || adapter != target.LegacyAdapter {
		return false, fmt.Errorf("OutcomeMapping.DetailAdapterKey is %q; refusing to replace anything except legacy %q", adapter, target.LegacyAdapter)
	}
	mapping["DetailAdapterKey"] = mustJSON(target.GenericAdapter)
	typeConclusion["OutcomeMapping"] = mustJSON(mapping)
	return true, nil
}

func normalizeReportAdapter(root map[string]json.RawMessage, target migrationTarget) (bool, error) {
	var reportMap map[string]json.RawMessage
	if err := json.Unmarshal(root["ReportMap"], &reportMap); err != nil {
		return false, fmt.Errorf("decode ReportMap: %w", err)
	}
	var sections []map[string]json.RawMessage
	if err := json.Unmarshal(reportMap["Sections"], &sections); err != nil {
		return false, fmt.Errorf("decode ReportMap.Sections: %w", err)
	}
	sectionIndex := -1
	for index, section := range sections {
		kind, _, err := rawString(section, "Kind")
		if err != nil {
			return false, fmt.Errorf("decode ReportMap.Sections[%d].Kind: %w", index, err)
		}
		if kind != target.GenericAdapter {
			continue
		}
		if sectionIndex >= 0 {
			return false, fmt.Errorf("ReportMap contains more than one %s section", target.GenericAdapter)
		}
		sectionIndex = index
	}
	if sectionIndex < 0 {
		return false, fmt.Errorf("ReportMap does not contain a %s section", target.GenericAdapter)
	}
	adapter, present, err := rawString(sections[sectionIndex], "AdapterKey")
	if err != nil {
		return false, fmt.Errorf("decode ReportMap.Sections[%d].AdapterKey: %w", sectionIndex, err)
	}
	if adapter == target.GenericAdapter {
		return false, nil
	}
	if !present || adapter != target.LegacyAdapter {
		return false, fmt.Errorf("ReportMap.Sections[%d].AdapterKey is %q; refusing to replace anything except legacy %q", sectionIndex, adapter, target.LegacyAdapter)
	}
	sections[sectionIndex]["AdapterKey"] = mustJSON(target.GenericAdapter)
	reportMap["Sections"] = mustJSON(sections)
	root["ReportMap"] = mustJSON(reportMap)
	return true, nil
}

func verifyNormalizedDefinition(input []byte, target migrationTarget) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(input, &root); err != nil {
		return fmt.Errorf("decode normalized DefinitionV2: %w", err)
	}
	var conclusions []map[string]json.RawMessage
	if err := json.Unmarshal(root["Conclusions"], &conclusions); err != nil {
		return fmt.Errorf("decode normalized Conclusions: %w", err)
	}
	foundType := false
	for _, item := range conclusions {
		kind, _, err := rawString(item, "Kind")
		if err != nil {
			return err
		}
		if kind != "type" {
			continue
		}
		if foundType {
			return fmt.Errorf("DefinitionV2 contains more than one type conclusion")
		}
		foundType = true
		var mapping map[string]json.RawMessage
		if err := json.Unmarshal(item["OutcomeMapping"], &mapping); err != nil {
			return fmt.Errorf("decode normalized OutcomeMapping: %w", err)
		}
		detailKind, _, err := rawString(mapping, "DetailKind")
		if err != nil {
			return err
		}
		adapter, _, err := rawString(mapping, "DetailAdapterKey")
		if err != nil {
			return err
		}
		if detailKind != target.GenericAdapter || adapter != target.GenericAdapter {
			return fmt.Errorf("OutcomeMapping is kind=%q adapter=%q, want %q", detailKind, adapter, target.GenericAdapter)
		}
	}
	if !foundType {
		return fmt.Errorf("DefinitionV2 does not contain a type conclusion")
	}
	var reportMap map[string]json.RawMessage
	if err := json.Unmarshal(root["ReportMap"], &reportMap); err != nil {
		return fmt.Errorf("decode normalized ReportMap: %w", err)
	}
	var sections []map[string]json.RawMessage
	if err := json.Unmarshal(reportMap["Sections"], &sections); err != nil {
		return fmt.Errorf("decode normalized ReportMap.Sections: %w", err)
	}
	foundReport := false
	for _, section := range sections {
		kind, _, err := rawString(section, "Kind")
		if err != nil {
			return err
		}
		if kind != target.GenericAdapter {
			continue
		}
		if foundReport {
			return fmt.Errorf("ReportMap contains more than one %s section", target.GenericAdapter)
		}
		foundReport = true
		adapter, _, err := rawString(section, "AdapterKey")
		if err != nil {
			return err
		}
		if adapter != target.GenericAdapter {
			return fmt.Errorf("ReportMap %s AdapterKey is %q, want %q", target.GenericAdapter, adapter, target.GenericAdapter)
		}
	}
	if !foundReport {
		return fmt.Errorf("ReportMap does not contain a %s section", target.GenericAdapter)
	}
	return nil
}

func rawString(object map[string]json.RawMessage, key string) (string, bool, error) {
	raw, ok := object[key]
	if !ok || string(raw) == "null" {
		return "", false, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", true, err
	}
	return value, true, nil
}

func mustJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
