package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	sbtiWikiRepository  = "https://github.com/serenakeyitan/sbti-wiki"
	sbtiWikiRevision    = "6fbd41d63c60b322bb695e92457baa1b72fc3917"
	sbtiWikiLicense     = "CC BY-NC-SA 4.0"
	sbtiWikiAttribution = "SBTI 原始文案版权归 B 站 up 主 蛆肉儿串儿 UID 417038183；配置整理归 serenakeyitan/sbti-wiki"
)

// These files are a minimal, reviewable projection of the upstream wiki data
// at sbtiWikiRevision. They intentionally exclude result prose, images, and
// simulated rarity data because the repair only owns executable profile
// configuration.
//
//go:embed data/dimensions.json
var wikiDimensionsJSON []byte

//go:embed data/patterns.json
var wikiPatternsJSON []byte

type wikiDimensions struct {
	Order []string                 `json:"order"`
	Meta  map[string]wikiDimension `json:"meta"`
}

type wikiDimension struct {
	Name  string `json:"name"`
	Model string `json:"model"`
}

type wikiPatterns struct {
	NormalTypes  []wikiProfile `json:"normal_types"`
	SpecialTypes []wikiProfile `json:"special_types"`
}

type wikiProfile struct {
	Code    string `json:"code"`
	Pattern string `json:"pattern"`
	Trigger string `json:"trigger"`
}

func loadWikiRepairCatalog() (repairCatalog, error) {
	var dimensions wikiDimensions
	if err := json.Unmarshal(wikiDimensionsJSON, &dimensions); err != nil {
		return repairCatalog{}, fmt.Errorf("decode embedded sbti-wiki dimensions: %w", err)
	}
	var patterns wikiPatterns
	if err := json.Unmarshal(wikiPatternsJSON, &patterns); err != nil {
		return repairCatalog{}, fmt.Errorf("decode embedded sbti-wiki patterns: %w", err)
	}
	if len(dimensions.Order) != 15 {
		return repairCatalog{}, fmt.Errorf("sbti-wiki dimension order contains %d entries, want 15", len(dimensions.Order))
	}
	if duplicates := duplicateStrings(dimensions.Order); len(duplicates) > 0 {
		return repairCatalog{}, fmt.Errorf("sbti-wiki dimension order contains duplicates: %s", strings.Join(duplicates, ", "))
	}
	for _, code := range dimensions.Order {
		meta, ok := dimensions.Meta[code]
		if !ok {
			return repairCatalog{}, fmt.Errorf("sbti-wiki dimension %s has no metadata", code)
		}
		if strings.TrimSpace(meta.Name) == "" || strings.TrimSpace(meta.Model) == "" {
			return repairCatalog{}, fmt.Errorf("sbti-wiki dimension %s has incomplete metadata", code)
		}
	}
	if len(dimensions.Meta) != len(dimensions.Order) {
		return repairCatalog{}, fmt.Errorf("sbti-wiki dimension metadata contains entries outside the canonical order")
	}
	if len(patterns.NormalTypes) != 25 || len(patterns.SpecialTypes) != 2 {
		return repairCatalog{}, fmt.Errorf("sbti-wiki profiles contain normal=%d special=%d, want normal=25 special=2", len(patterns.NormalTypes), len(patterns.SpecialTypes))
	}

	catalog := repairCatalog{
		DimensionOrder: append([]string(nil), dimensions.Order...),
		Profiles:       make(map[string]profileSeed, len(patterns.NormalTypes)+len(patterns.SpecialTypes)),
		NormalCount:    len(patterns.NormalTypes),
		SpecialCount:   len(patterns.SpecialTypes),
		Source:         sbtiWikiRepository,
		Revision:       sbtiWikiRevision,
		License:        sbtiWikiLicense,
		Attribution:    sbtiWikiAttribution,
	}
	for _, profile := range patterns.NormalTypes {
		if strings.TrimSpace(profile.Trigger) != "" {
			return repairCatalog{}, fmt.Errorf("normal sbti-wiki profile %s must not have a trigger", profile.Code)
		}
		if err := addSeedProfile(catalog.Profiles, profile.Code, profile.Pattern, "", false, len(dimensions.Order)); err != nil {
			return repairCatalog{}, err
		}
	}
	for _, profile := range patterns.SpecialTypes {
		if strings.TrimSpace(profile.Pattern) != "" {
			return repairCatalog{}, fmt.Errorf("special sbti-wiki profile %s must not have a pattern", profile.Code)
		}
		if strings.TrimSpace(profile.Trigger) == "" {
			return repairCatalog{}, fmt.Errorf("special sbti-wiki profile %s must have a trigger", profile.Code)
		}
		if err := addSeedProfile(catalog.Profiles, profile.Code, "", profile.Trigger, true, len(dimensions.Order)); err != nil {
			return repairCatalog{}, err
		}
	}
	return catalog, nil
}
