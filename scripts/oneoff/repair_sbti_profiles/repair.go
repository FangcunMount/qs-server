package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
)

type profileSeed struct {
	Pattern   string
	Trigger   string
	IsSpecial bool
}

type repairCatalog struct {
	DimensionOrder []string
	Profiles       map[string]profileSeed
	NormalCount    int
	SpecialCount   int
	Source         string
	Revision       string
	License        string
	Attribution    string
}

type fieldChange struct {
	OutcomeCode string
	Field       string
	Before      string
	After       string
}

type repairSummary struct {
	ProfileCount       int
	NormalCount        int
	SpecialCount       int
	PatternChanges     int
	SpecialFlagChanges int
	TriggerChanges     int
	Changes            []fieldChange
}

func (s repairSummary) Changed() bool { return len(s.Changes) > 0 }

func addSeedProfile(profiles map[string]profileSeed, code, pattern, trigger string, special bool, dimensions int) error {
	if code == "" {
		return fmt.Errorf("SBTI catalog contains an outcome without code")
	}
	if _, exists := profiles[code]; exists {
		return fmt.Errorf("SBTI catalog outcome code %s is duplicated", code)
	}
	if !special {
		if err := validatePattern(code, pattern, dimensions); err != nil {
			return err
		}
	}
	profiles[code] = profileSeed{Pattern: pattern, Trigger: trigger, IsSpecial: special}
	return nil
}

func validatePattern(code, pattern string, dimensions int) error {
	if pattern == "" || pattern != strings.TrimSpace(pattern) {
		return fmt.Errorf("normal SBTI outcome %s has an empty or padded pattern", code)
	}
	compact := strings.ReplaceAll(pattern, "-", "")
	if len(compact) != dimensions {
		return fmt.Errorf("normal SBTI outcome %s pattern has %d levels, want %d", code, len(compact), dimensions)
	}
	for _, level := range compact {
		if level != 'L' && level != 'M' && level != 'H' {
			return fmt.Errorf("normal SBTI outcome %s pattern contains invalid level %q", code, string(level))
		}
	}
	return nil
}

func repairDefinition(input []byte, catalog repairCatalog) ([]byte, repairSummary, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(input, &root); err != nil {
		return nil, repairSummary{}, fmt.Errorf("decode DefinitionV2: %w", err)
	}
	if len(root) == 0 {
		return nil, repairSummary{}, fmt.Errorf("DefinitionV2 must be a non-empty object")
	}
	if err := validateFactorOrder(root["Measure"], catalog.DimensionOrder); err != nil {
		return nil, repairSummary{}, err
	}
	if err := validateOutcomeRegistry(root["Outcomes"], catalog.Profiles); err != nil {
		return nil, repairSummary{}, err
	}

	var conclusions []json.RawMessage
	if err := json.Unmarshal(root["Conclusions"], &conclusions); err != nil {
		return nil, repairSummary{}, fmt.Errorf("decode Conclusions: %w", err)
	}
	typeIndex := -1
	for index, raw := range conclusions {
		var header struct {
			Kind string `json:"Kind"`
		}
		if err := json.Unmarshal(raw, &header); err != nil {
			return nil, repairSummary{}, fmt.Errorf("decode Conclusions[%d]: %w", index, err)
		}
		if header.Kind != "type" {
			continue
		}
		if typeIndex >= 0 {
			return nil, repairSummary{}, fmt.Errorf("DefinitionV2 contains more than one type conclusion")
		}
		typeIndex = index
	}
	if typeIndex < 0 {
		return nil, repairSummary{}, fmt.Errorf("DefinitionV2 does not contain a type conclusion")
	}

	var typeConclusion map[string]json.RawMessage
	if err := json.Unmarshal(conclusions[typeIndex], &typeConclusion); err != nil {
		return nil, repairSummary{}, fmt.Errorf("decode type conclusion: %w", err)
	}
	var profiles []json.RawMessage
	if err := json.Unmarshal(typeConclusion["Profiles"], &profiles); err != nil {
		return nil, repairSummary{}, fmt.Errorf("decode type conclusion Profiles: %w", err)
	}
	if err := validateProfileCodes(profiles, catalog.Profiles); err != nil {
		return nil, repairSummary{}, err
	}

	summary := repairSummary{
		ProfileCount: len(profiles),
		NormalCount:  catalog.NormalCount,
		SpecialCount: catalog.SpecialCount,
	}
	for index, raw := range profiles {
		var profile map[string]json.RawMessage
		if err := json.Unmarshal(raw, &profile); err != nil {
			return nil, repairSummary{}, fmt.Errorf("decode Profiles[%d]: %w", index, err)
		}
		code, _, err := rawString(profile, "OutcomeCode")
		if err != nil {
			return nil, repairSummary{}, fmt.Errorf("decode Profiles[%d].OutcomeCode: %w", index, err)
		}
		seed := catalog.Profiles[code]
		pattern, patternPresent, err := rawString(profile, "Pattern")
		if err != nil {
			return nil, repairSummary{}, fmt.Errorf("decode profile %s Pattern: %w", code, err)
		}
		if seed.IsSpecial {
			if patternPresent && pattern != "" {
				delete(profile, "Pattern")
				summary.PatternChanges++
				summary.Changes = append(summary.Changes, fieldChange{OutcomeCode: code, Field: "Pattern", Before: quoteOrMissing(pattern, true), After: "<missing>"})
			}
		} else if !patternPresent || pattern != seed.Pattern {
			profile["Pattern"] = mustJSON(seed.Pattern)
			summary.PatternChanges++
			summary.Changes = append(summary.Changes, fieldChange{OutcomeCode: code, Field: "Pattern", Before: quoteOrMissing(pattern, patternPresent), After: fmt.Sprintf("%q", seed.Pattern)})
		}

		special, specialPresent, err := rawBool(profile, "IsSpecial")
		if err != nil {
			return nil, repairSummary{}, fmt.Errorf("decode profile %s IsSpecial: %w", code, err)
		}
		if special != seed.IsSpecial {
			profile["IsSpecial"] = mustJSON(seed.IsSpecial)
			summary.SpecialFlagChanges++
			summary.Changes = append(summary.Changes, fieldChange{
				OutcomeCode: code,
				Field:       "IsSpecial",
				Before:      boolOrMissing(special, specialPresent),
				After:       fmt.Sprintf("%t", seed.IsSpecial),
			})
		}

		trigger, triggerPresent, err := rawString(profile, "Trigger")
		if err != nil {
			return nil, repairSummary{}, fmt.Errorf("decode profile %s Trigger: %w", code, err)
		}
		if seed.Trigger == "" {
			if triggerPresent && trigger != "" {
				delete(profile, "Trigger")
				summary.TriggerChanges++
				summary.Changes = append(summary.Changes, fieldChange{OutcomeCode: code, Field: "Trigger", Before: quoteOrMissing(trigger, true), After: "<missing>"})
			}
		} else if !triggerPresent || trigger != seed.Trigger {
			profile["Trigger"] = mustJSON(seed.Trigger)
			summary.TriggerChanges++
			summary.Changes = append(summary.Changes, fieldChange{OutcomeCode: code, Field: "Trigger", Before: quoteOrMissing(trigger, triggerPresent), After: fmt.Sprintf("%q", seed.Trigger)})
		}

		updated, err := json.Marshal(profile)
		if err != nil {
			return nil, repairSummary{}, fmt.Errorf("encode profile %s: %w", code, err)
		}
		profiles[index] = updated
	}

	typeConclusion["Profiles"] = mustJSON(profiles)
	conclusions[typeIndex] = mustJSON(typeConclusion)
	root["Conclusions"] = mustJSON(conclusions)
	output, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, repairSummary{}, fmt.Errorf("encode repaired DefinitionV2: %w", err)
	}
	if err := verifyRepairedDefinition(output, catalog); err != nil {
		return nil, repairSummary{}, err
	}
	var typed modeldefinition.Definition
	if err := json.Unmarshal(output, &typed); err != nil {
		return nil, repairSummary{}, fmt.Errorf("decode repaired DefinitionV2 domain contract: %w", err)
	}
	if issues := modeldefinition.Validate(typed); len(issues) > 0 {
		return nil, repairSummary{}, fmt.Errorf("repaired DefinitionV2 remains structurally invalid: %s: %s", issues[0].Field, issues[0].Message)
	}
	return append(output, '\n'), summary, nil
}

func verifyRepairedDefinition(input []byte, catalog repairCatalog) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(input, &root); err != nil {
		return fmt.Errorf("verify repaired DefinitionV2: %w", err)
	}
	if err := validateFactorOrder(root["Measure"], catalog.DimensionOrder); err != nil {
		return err
	}
	if err := validateOutcomeRegistry(root["Outcomes"], catalog.Profiles); err != nil {
		return err
	}
	var conclusions []map[string]json.RawMessage
	if err := json.Unmarshal(root["Conclusions"], &conclusions); err != nil {
		return fmt.Errorf("verify Conclusions: %w", err)
	}
	found := false
	for _, item := range conclusions {
		kind, _, err := rawString(item, "Kind")
		if err != nil {
			return err
		}
		if kind != "type" {
			continue
		}
		if found {
			return fmt.Errorf("DefinitionV2 contains more than one type conclusion")
		}
		found = true
		var profiles []map[string]json.RawMessage
		if err := json.Unmarshal(item["Profiles"], &profiles); err != nil {
			return fmt.Errorf("verify type Profiles: %w", err)
		}
		for _, profile := range profiles {
			code, _, _ := rawString(profile, "OutcomeCode")
			seed := catalog.Profiles[code]
			pattern, _, err := rawString(profile, "Pattern")
			if err != nil {
				return fmt.Errorf("verify profile %s Pattern: %w", code, err)
			}
			special, _, err := rawBool(profile, "IsSpecial")
			if err != nil {
				return fmt.Errorf("verify profile %s IsSpecial: %w", code, err)
			}
			trigger, _, err := rawString(profile, "Trigger")
			if err != nil {
				return fmt.Errorf("verify profile %s Trigger: %w", code, err)
			}
			if pattern != seed.Pattern || special != seed.IsSpecial || trigger != seed.Trigger {
				return fmt.Errorf("profile %s does not match the canonical SBTI pattern/special flag/trigger", code)
			}
		}
	}
	if !found {
		return fmt.Errorf("DefinitionV2 does not contain a type conclusion")
	}
	return nil
}

func validateFactorOrder(raw json.RawMessage, expected []string) error {
	var measure struct {
		FactorGraph struct {
			Roots []string `json:"Roots"`
		} `json:"FactorGraph"`
	}
	if err := json.Unmarshal(raw, &measure); err != nil {
		return fmt.Errorf("decode Measure.FactorGraph.Roots: %w", err)
	}
	if !equalStrings(measure.FactorGraph.Roots, expected) {
		return fmt.Errorf("Measure.FactorGraph.Roots does not match canonical SBTI order: got [%s], want [%s]",
			strings.Join(measure.FactorGraph.Roots, ", "), strings.Join(expected, ", "))
	}
	return nil
}

func validateOutcomeRegistry(raw json.RawMessage, expected map[string]profileSeed) error {
	var outcomes []struct {
		Code string `json:"Code"`
	}
	if err := json.Unmarshal(raw, &outcomes); err != nil {
		return fmt.Errorf("decode Outcomes: %w", err)
	}
	codes := make([]string, 0, len(outcomes))
	for _, outcome := range outcomes {
		codes = append(codes, outcome.Code)
	}
	return validateExactCodes("Outcomes", codes, expected)
}

func validateProfileCodes(rawProfiles []json.RawMessage, expected map[string]profileSeed) error {
	codes := make([]string, 0, len(rawProfiles))
	for index, raw := range rawProfiles {
		var profile map[string]json.RawMessage
		if err := json.Unmarshal(raw, &profile); err != nil {
			return fmt.Errorf("decode Profiles[%d]: %w", index, err)
		}
		code, _, err := rawString(profile, "OutcomeCode")
		if err != nil {
			return fmt.Errorf("decode Profiles[%d].OutcomeCode: %w", index, err)
		}
		codes = append(codes, code)
	}
	return validateExactCodes("Conclusions[type].Profiles", codes, expected)
}

func validateExactCodes(label string, actual []string, expected map[string]profileSeed) error {
	if duplicates := duplicateStrings(actual); len(duplicates) > 0 {
		return fmt.Errorf("%s contains duplicate outcome codes: %s", label, strings.Join(duplicates, ", "))
	}
	actualSet := make(map[string]struct{}, len(actual))
	for _, code := range actual {
		if code == "" {
			return fmt.Errorf("%s contains an empty outcome code", label)
		}
		actualSet[code] = struct{}{}
	}
	missing := make([]string, 0)
	extra := make([]string, 0)
	for code := range expected {
		if _, ok := actualSet[code]; !ok {
			missing = append(missing, code)
		}
	}
	for code := range actualSet {
		if _, ok := expected[code]; !ok {
			extra = append(extra, code)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		return fmt.Errorf("%s outcome codes do not match canonical SBTI data: missing=[%s] extra=[%s]",
			label, strings.Join(missing, ", "), strings.Join(extra, ", "))
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

func rawBool(object map[string]json.RawMessage, key string) (bool, bool, error) {
	raw, ok := object[key]
	if !ok || string(raw) == "null" {
		return false, false, nil
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, true, err
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

func quoteOrMissing(value string, present bool) string {
	if !present {
		return "<missing>"
	}
	return fmt.Sprintf("%q", value)
}

func boolOrMissing(value, present bool) string {
	if !present {
		return "<missing>"
	}
	return fmt.Sprintf("%t", value)
}

func duplicateStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	duplicates := make(map[string]struct{})
	for _, value := range values {
		if _, ok := seen[value]; ok {
			duplicates[value] = struct{}{}
		}
		seen[value] = struct{}{}
	}
	out := make([]string, 0, len(duplicates))
	for value := range duplicates {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
