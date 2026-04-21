package cacheobservability

import "time"

// RuntimeSnapshot captures one component's current Redis runtime governance state.
type RuntimeSnapshot struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Component   string         `json:"component"`
	Summary     RuntimeSummary `json:"summary"`
	Families    []FamilyStatus `json:"families"`
}

// RuntimeSummary aggregates family availability for one component.
type RuntimeSummary struct {
	FamilyTotal      int  `json:"family_total"`
	AvailableCount   int  `json:"available_count"`
	DegradedCount    int  `json:"degraded_count"`
	UnavailableCount int  `json:"unavailable_count"`
	Ready            bool `json:"ready"`
}

// SnapshotForComponent returns a filtered runtime snapshot for one component.
func SnapshotForComponent(component string, registry *FamilyStatusRegistry) RuntimeSnapshot {
	snapshot := RuntimeSnapshot{
		GeneratedAt: time.Now(),
		Component:   component,
		Families:    []FamilyStatus{},
		Summary: RuntimeSummary{
			Ready: true,
		},
	}
	if registry == nil {
		return snapshot
	}

	all := registry.Snapshot()
	if component == "" {
		snapshot.Families = append(snapshot.Families, all...)
	} else {
		for _, family := range all {
			if family.Component == component {
				snapshot.Families = append(snapshot.Families, family)
			}
		}
	}
	snapshot.Summary = SummarizeFamilies(snapshot.Families)
	return snapshot
}

// SummarizeFamilies converts detailed family state into an endpoint/metric-friendly summary.
func SummarizeFamilies(families []FamilyStatus) RuntimeSummary {
	summary := RuntimeSummary{
		FamilyTotal: len(families),
		Ready:       true,
	}
	for _, family := range families {
		if family.Available && !family.Degraded {
			summary.AvailableCount++
		}
		if family.Degraded {
			summary.DegradedCount++
		}
		if !family.Available {
			summary.UnavailableCount++
		}
	}
	if summary.DegradedCount > 0 || summary.UnavailableCount > 0 {
		summary.Ready = false
	}
	return summary
}
