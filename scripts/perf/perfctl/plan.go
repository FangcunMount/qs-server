package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

type phaseSpec struct {
	ID            string
	Profile       string
	TargetQPS     int
	Duration      string
	ThresholdTier string
	Dynamic       bool
}

var canonicalWorkloadOrder = []string{
	"medicalQuery",
	"personalityQuery",
	"questionnaireQuery",
	"personalityQuestionnaireQuery",
	"medicalSubmit",
	"personalitySubmit",
	"medicalWaitReport",
	"behaviorWaitReport",
	"personalityWaitReport",
	"stats",
}

func phasesForPlan(plan string) ([]phaseSpec, error) {
	smoke := phaseSpec{ID: "smoke", Profile: "smoke_4", TargetQPS: 4, Duration: "30s", ThresholdTier: "none"}
	experience := phaseSpec{ID: "experience_60", Profile: "experience_60", TargetQPS: 60, Duration: "5m", ThresholdTier: "experience"}
	switch plan {
	case "quick":
		return []phaseSpec{smoke}, nil
	case "baseline":
		return []phaseSpec{smoke, experience}, nil
	case "ceiling-120":
		return []phaseSpec{
			{ID: "capacity_110", Profile: "capacity_110", TargetQPS: 110, Duration: "2m", ThresholdTier: "protection", Dynamic: true},
			{ID: "capacity_120", Profile: "capacity_120", TargetQPS: 120, Duration: "2m", ThresholdTier: "protection", Dynamic: true},
		}, nil
	case "admission":
		return []phaseSpec{
			smoke,
			experience,
			{ID: "capacity_80", Profile: "capacity_80", TargetQPS: 80, Duration: "2m", ThresholdTier: "protection", Dynamic: true},
			{ID: "capacity_100", Profile: "capacity_100", TargetQPS: 100, Duration: "2m", ThresholdTier: "protection", Dynamic: true},
			{ID: "capacity_110", Profile: "capacity_110", TargetQPS: 110, Duration: "2m", ThresholdTier: "protection", Dynamic: true},
			{ID: "capacity_120", Profile: "capacity_120", TargetQPS: 120, Duration: "2m", ThresholdTier: "protection", Dynamic: true},
			{ID: "capacity_200", Profile: "capacity_200", TargetQPS: 200, Duration: "3m", ThresholdTier: "protection", Dynamic: true},
			{ID: "capacity_240", Profile: "capacity_240", TargetQPS: 240, Duration: "4m", ThresholdTier: "protection", Dynamic: true},
			{ID: "capacity_280", Profile: "capacity_280", TargetQPS: 280, Duration: "3m", ThresholdTier: "protection", Dynamic: true},
			{ID: "admission_300", Profile: "admission_300", TargetQPS: 300, Duration: "10m", ThresholdTier: "protection"},
		}, nil
	default:
		return nil, fmt.Errorf("unknown PLAN %q; use quick, baseline, ceiling-120, admission, or diagnose", plan)
	}
}

type perfConfig map[string]any

func loadAndPrepareConfig(path, output string, phases []phaseSpec) (perfConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read perf config: %w", err)
	}
	var config perfConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("decode perf config: %w", err)
	}
	absolutizeConfigFiles(config, filepath.Dir(path))
	profiles, ok := config["qpsProfiles"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("perf config has no qpsProfiles object")
	}
	canonical, ok := profiles["admission_300"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("perf config has no admission_300 profile")
	}
	for _, phase := range phases {
		if !phase.Dynamic {
			if _, exists := profiles[phase.Profile]; !exists {
				return nil, fmt.Errorf("perf profile %q is missing", phase.Profile)
			}
			continue
		}
		profile := deepCopyMap(canonical)
		delete(profile, "vusers")
		qps, err := scaledWorkload(profileQPS(canonical), phase.TargetQPS)
		if err != nil {
			return nil, fmt.Errorf("build %s: %w", phase.ID, err)
		}
		profile["duration"] = phase.Duration
		profile["description"] = fmt.Sprintf("generated capacity stage at %d business QPS", phase.TargetQPS)
		profile["thresholdTier"] = phase.ThresholdTier
		profile["qps"] = qps
		profiles[phase.Profile] = profile
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode effective perf config: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(output, encoded, 0o600); err != nil {
		return nil, fmt.Errorf("write effective perf config: %w", err)
	}
	return config, nil
}

func profileQPS(profile map[string]any) map[string]float64 {
	result := map[string]float64{}
	qps, _ := profile["qps"].(map[string]any)
	for key, value := range qps {
		if number, ok := numericFloat64(value); ok {
			result[key] = number
		}
	}
	return result
}

func numericFloat64(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int8:
		return float64(number), true
	case int16:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint8:
		return float64(number), true
	case uint16:
		return float64(number), true
	case uint32:
		return float64(number), true
	case uint64:
		return float64(number), true
	case json.Number:
		parsed, err := strconv.ParseFloat(number.String(), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func scaledWorkload(canonical map[string]float64, target int) (map[string]any, error) {
	if target < 2 {
		return nil, fmt.Errorf("target QPS must be at least 2 so chainProbe can remain 1")
	}
	if canonical["chainProbe"] != 1 {
		return nil, fmt.Errorf("admission_300 chainProbe must equal 1")
	}
	available := target - 1
	weightTotal := 0.0
	for _, key := range canonicalWorkloadOrder {
		weightTotal += canonical[key]
	}
	if weightTotal <= 0 {
		return nil, fmt.Errorf("canonical workload has no scalable QPS")
	}
	type remainder struct {
		key      string
		fraction float64
		order    int
	}
	result := make(map[string]any, len(canonicalWorkloadOrder)+1)
	remainders := make([]remainder, 0, len(canonicalWorkloadOrder))
	allocated := 0
	for index, key := range canonicalWorkloadOrder {
		exact := canonical[key] * float64(available) / weightTotal
		base := int(math.Floor(exact))
		result[key] = base
		allocated += base
		remainders = append(remainders, remainder{key: key, fraction: exact - float64(base), order: index})
	}
	sort.SliceStable(remainders, func(i, j int) bool {
		if remainders[i].fraction == remainders[j].fraction {
			return remainders[i].order < remainders[j].order
		}
		return remainders[i].fraction > remainders[j].fraction
	})
	for index := 0; index < available-allocated; index++ {
		key := remainders[index].key
		result[key] = result[key].(int) + 1
	}
	result["chainProbe"] = 1
	return result, nil
}

func deepCopyMap(input map[string]any) map[string]any {
	raw, _ := json.Marshal(input)
	result := map[string]any{}
	_ = json.Unmarshal(raw, &result)
	return result
}

func absolutizeConfigFiles(config perfConfig, baseDir string) {
	for _, key := range []string{"tokensFile", "collectionTokensFile", "apiserverTokensFile"} {
		value, ok := config[key].(string)
		if !ok || value == "" || filepath.IsAbs(value) {
			continue
		}
		config[key] = filepath.Clean(filepath.Join(baseDir, value))
	}
}
