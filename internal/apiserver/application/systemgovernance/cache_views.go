package systemgovernance

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	govcomponent "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/component"
	cachemodel "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	sharedgovernance "github.com/FangcunMount/qs-server/internal/pkg/cache/governance"
	cachetopology "github.com/FangcunMount/qs-server/internal/pkg/cache/topology"
)

func BuildCacheRegistryView(components map[string]govcomponent.CacheGovernanceResult) CacheRegistryView {
	view := CacheRegistryView{
		ComponentRegistries: []ComponentRegistryRow{}, CapabilityRows: []RegistryCapabilityRow{}, RegistryDrift: []RegistryDrift{},
	}
	componentNames := sortedStringKeys(components)
	availableByComponent := make(map[string][]ComponentRegistryRow, len(components))
	driftedComponents := make(map[string]bool, len(components))
	for _, component := range componentNames {
		result := components[component]
		instances := result.Instances
		if len(instances) == 0 && result.Snapshot != nil {
			instances = map[string]*sharedgovernance.ComponentCacheGovernanceSnapshot{result.Snapshot.InstanceID: result.Snapshot}
		}
		for _, instanceID := range sortedStringKeys(instances) {
			snapshot := instances[instanceID]
			if snapshot == nil {
				continue
			}
			row := ComponentRegistryRow{
				Component: component, InstanceID: snapshot.InstanceID, Generation: snapshot.Generation, Available: true,
				SnapshotVersion: snapshot.EffectiveRegistry.SnapshotVersion, CatalogVersion: snapshot.EffectiveRegistry.CatalogVersion,
				PolicySource: cachemodel.FromSharedEffectiveRegistry(snapshot.EffectiveRegistry).PolicySource,
				Capabilities: cachemodel.FromSharedEffectiveRegistry(snapshot.EffectiveRegistry).Capabilities,
				L1Runtime:    cachemodel.FromSharedL1Runtime(snapshot.L1Runtime),
			}
			view.ComponentRegistries = append(view.ComponentRegistries, row)
			availableByComponent[component] = append(availableByComponent[component], row)
		}
		if len(instances) == 0 {
			view.ComponentRegistries = append(view.ComponentRegistries, ComponentRegistryRow{
				Component: component, Available: false, Reason: nonEmpty(result.Reason, "cache Registry unavailable"),
			})
		}
		if len(instances) == 0 || result.DiscoveredInstanceCount > result.AvailableInstanceCount || len(result.TargetErrors) > 0 {
			driftedComponents[component] = true
			view.RegistryDrift = append(view.RegistryDrift, RegistryDrift{
				Component: component, Kind: "missing_registry",
				Message: "one or more discovered instances did not return a valid Cache Registry",
				Values: map[string][]string{
					"discovered": {intString(result.DiscoveredInstanceCount)},
					"available":  {intString(result.AvailableInstanceCount)},
					"targets":    sortedStringKeys(result.TargetErrors),
				},
			})
		}
	}

	for _, component := range componentNames {
		rows := availableByComponent[component]
		identityDrift := registryIdentityDrift(component, rows)
		if len(identityDrift) > 0 {
			driftedComponents[component] = true
		}
		view.RegistryDrift = append(view.RegistryDrift, identityDrift...)
		view.CapabilityRows = append(view.CapabilityRows, mergeCapabilityRows(component, rows, driftedComponents[component])...)
	}
	sort.Slice(view.ComponentRegistries, func(i, j int) bool {
		if view.ComponentRegistries[i].Component != view.ComponentRegistries[j].Component {
			return view.ComponentRegistries[i].Component < view.ComponentRegistries[j].Component
		}
		return view.ComponentRegistries[i].InstanceID < view.ComponentRegistries[j].InstanceID
	})
	sort.Slice(view.RegistryDrift, func(i, j int) bool {
		if view.RegistryDrift[i].Component != view.RegistryDrift[j].Component {
			return view.RegistryDrift[i].Component < view.RegistryDrift[j].Component
		}
		return view.RegistryDrift[i].Kind < view.RegistryDrift[j].Kind
	})
	return view
}

func registryIdentityDrift(component string, rows []ComponentRegistryRow) []RegistryDrift {
	if len(rows) <= 1 {
		return nil
	}
	checks := []struct {
		kind    string
		message string
		value   func(ComponentRegistryRow) string
	}{
		{kind: "policy_sha256", message: "instances use different effective policy hashes", value: func(row ComponentRegistryRow) string {
			if row.PolicySource == nil {
				return "missing"
			}
			return nonEmpty(row.PolicySource.PolicySHA256, "missing")
		}},
		{kind: "catalog_version", message: "instances use different Cache Catalog versions", value: func(row ComponentRegistryRow) string {
			return nonEmpty(row.CatalogVersion, "missing")
		}},
		{kind: "capability_set", message: "instances expose different capability sets", value: func(row ComponentRegistryRow) string {
			ids := make([]string, 0, len(row.Capabilities))
			for _, capability := range row.Capabilities {
				ids = append(ids, capability.Capability)
			}
			sort.Strings(ids)
			return strings.Join(ids, ",")
		}},
	}
	result := make([]RegistryDrift, 0)
	for _, check := range checks {
		values := map[string][]string{}
		for _, row := range rows {
			value := check.value(row)
			values[value] = append(values[value], row.InstanceID)
		}
		if len(values) > 1 {
			result = append(result, RegistryDrift{Component: component, Kind: check.kind, Message: check.message, Values: values})
		}
	}

	structures := map[string]map[string][]string{}
	for _, row := range rows {
		for _, capability := range row.Capabilities {
			value := strings.Join([]string{
				capability.Owner, capability.Kind, capability.Layer, capability.Family, capability.MetricLabel,
				capability.TopologyGroup, strconv.Itoa(capability.TopologyOrder), capability.ReadModel,
			}, "|")
			if structures[capability.Capability] == nil {
				structures[capability.Capability] = map[string][]string{}
			}
			structures[capability.Capability][value] = append(structures[capability.Capability][value], row.InstanceID)
		}
	}
	for capability, values := range structures {
		if len(values) > 1 {
			result = append(result, RegistryDrift{
				Component: component, Kind: "capability_structure", Message: "capability structure differs: " + capability, Values: values,
			})
		}
	}
	return result
}

type capabilityVariantAccumulator struct {
	policySHA string
	view      cachemodel.CapabilityPolicyView
	instances []string
}

func mergeCapabilityRows(component string, registries []ComponentRegistryRow, forceInconsistent bool) []RegistryCapabilityRow {
	type groupedCapability struct {
		capability string
		layer      string
		variants   map[string]*capabilityVariantAccumulator
	}
	groups := map[string]*groupedCapability{}
	for _, registry := range registries {
		policySHA := ""
		if registry.PolicySource != nil {
			policySHA = registry.PolicySource.PolicySHA256
		}
		for _, capability := range registry.Capabilities {
			groupKey := component + "\x00" + capability.Capability + "\x00" + capability.Layer
			group := groups[groupKey]
			if group == nil {
				group = &groupedCapability{capability: capability.Capability, layer: capability.Layer, variants: map[string]*capabilityVariantAccumulator{}}
				groups[groupKey] = group
			}
			encoded, _ := json.Marshal(struct {
				PolicySHA string                          `json:"policy_sha"`
				Policy    cachemodel.CapabilityPolicyView `json:"policy"`
			}{PolicySHA: policySHA, Policy: capability})
			variantKey := string(encoded)
			variant := group.variants[variantKey]
			if variant == nil {
				copyView := capability
				variant = &capabilityVariantAccumulator{policySHA: policySHA, view: copyView}
				group.variants[variantKey] = variant
			}
			variant.instances = append(variant.instances, registry.InstanceID)
		}
	}
	keys := sortedStringKeys(groups)
	result := make([]RegistryCapabilityRow, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		variantKeys := sortedStringKeys(group.variants)
		row := RegistryCapabilityRow{Component: component, Capability: group.capability, Layer: group.layer, InstanceIDs: []string{}}
		for _, variantKey := range variantKeys {
			variant := group.variants[variantKey]
			sort.Strings(variant.instances)
			row.InstanceIDs = append(row.InstanceIDs, variant.instances...)
			row.Variants = append(row.Variants, RegistryCapabilityVariant{
				PolicySHA256: variant.policySHA, InstanceIDs: variant.instances, Owner: variant.view.Owner, Kind: variant.view.Kind,
				Layer: variant.view.Layer, Family: variant.view.Family, Enabled: variant.view.Enabled,
				MetricLabel: variant.view.MetricLabel, Effective: variant.view.Effective,
				TopologyGroup: variant.view.TopologyGroup, TopologyOrder: variant.view.TopologyOrder,
				ReadModel: variant.view.ReadModel,
			})
		}
		sort.Strings(row.InstanceIDs)
		row.Consistent = !forceInconsistent && len(row.Variants) == 1 && len(row.InstanceIDs) == len(registries)
		if row.Consistent {
			variant := row.Variants[0]
			row.PolicySHA256, row.Owner, row.Kind, row.Family = variant.PolicySHA256, variant.Owner, variant.Kind, variant.Family
			row.MetricLabel = variant.MetricLabel
			row.TopologyGroup, row.TopologyOrder, row.ReadModel = variant.TopologyGroup, variant.TopologyOrder, variant.ReadModel
			enabled := variant.Enabled
			policy := variant.Effective
			row.Enabled = &enabled
			row.Effective = &policy
			row.Variants = nil
		}
		result = append(result, row)
	}
	return result
}

func BuildCacheRuntimeView(components map[string]ComponentCache, instanceRows []CacheFamilyRow, registries map[string]govcomponent.CacheGovernanceResult) CacheRuntimeView {
	view := CacheRuntimeView{FamilyGroups: []CacheFamilyGroup{}, InstanceRows: make([]CacheFamilyRow, 0, len(instanceRows))}
	for _, row := range instanceRows {
		row.Component = canonicalCacheComponent(row.Component)
		view.InstanceRows = append(view.InstanceRows, row)
	}
	for _, component := range sortedStringKeys(registries) {
		result := registries[component]
		instances := result.Instances
		if len(instances) == 0 && result.Snapshot != nil {
			instances = map[string]*sharedgovernance.ComponentCacheGovernanceSnapshot{result.Snapshot.InstanceID: result.Snapshot}
		}
		for _, instanceID := range sortedStringKeys(instances) {
			snapshot := instances[instanceID]
			if snapshot == nil {
				continue
			}
			for _, runtime := range snapshot.L1Runtime {
				projectedRuntime := cachemodel.FromSharedL1CapabilityRuntime(runtime)
				view.L1CapabilityRuntime = append(view.L1CapabilityRuntime, CacheL1CapabilityRuntimeRow{
					Component: component, InstanceID: instanceID, Generation: snapshot.Generation,
					Capability: runtime.Capability, Enabled: runtime.Enabled, Buckets: projectedRuntime.Buckets,
					SignalWatcher: projectedRuntime.SignalWatcher,
				})
			}
		}
	}
	canonicalComponents := map[string]struct{}{}
	discoveredByComponent := map[string]int{}
	for component, status := range components {
		component = canonicalCacheComponent(component)
		canonicalComponents[component] = struct{}{}
		discovered := status.DiscoveredInstanceCount
		if discovered == 0 {
			discovered = len(status.Instances)
		}
		if discovered == 0 && status.Snapshot != nil {
			discovered = 1
		}
		discoveredByComponent[component] = discovered
		view.Summary.DiscoveredInstanceCount += discovered
		healthyInstances := 0
		if len(status.Instances) > 0 {
			for _, snapshot := range status.Instances {
				if snapshot != nil && snapshot.Summary.Ready {
					healthyInstances++
				}
			}
		} else if status.Snapshot != nil && status.Snapshot.Summary.Ready {
			healthyInstances++
		}
		view.Summary.HealthyInstanceCount += healthyInstances
		if status.Available && !status.Partial && discovered > 0 && healthyInstances == discovered {
			view.Summary.HealthyComponentCount++
		}
	}
	view.Summary.ComponentTotal = len(canonicalComponents)

	type groupAccumulator struct {
		group     CacheFamilyGroup
		instances map[string]struct{}
		evidence  map[string]MetricEvidence
	}
	groups := map[string]*groupAccumulator{}
	for _, row := range view.InstanceRows {
		key := strings.Join([]string{row.Component, row.Family, row.Profile, row.Namespace}, "\x00")
		group := groups[key]
		if group == nil {
			group = &groupAccumulator{
				group:     CacheFamilyGroup{Component: row.Component, Family: row.Family, Profile: row.Profile, Namespace: row.Namespace, Severity: SeverityHealthy},
				instances: map[string]struct{}{}, evidence: map[string]MetricEvidence{},
			}
			groups[key] = group
		}
		group.instances[row.InstanceID] = struct{}{}
		if row.Available && !row.Degraded {
			group.group.HealthyInstanceCount++
		}
		if row.Degraded {
			group.group.DegradedInstanceCount++
		}
		if !row.Available {
			group.group.UnavailableInstanceCount++
		}
		if row.LastError != "" {
			group.group.LastError = row.LastError
		}
		for _, evidence := range row.MetricEvidence {
			group.evidence[evidence.Name+"\x00"+evidence.Window] = evidence
		}
	}
	for _, key := range sortedStringKeys(groups) {
		item := groups[key]
		item.group.DiscoveredInstanceCount = discoveredByComponent[item.group.Component]
		if item.group.DiscoveredInstanceCount == 0 {
			item.group.DiscoveredInstanceCount = len(item.instances)
		}
		missing := item.group.DiscoveredInstanceCount - len(item.instances)
		if missing > 0 {
			item.group.UnavailableInstanceCount += missing
		}
		switch {
		case item.group.UnavailableInstanceCount > 0:
			item.group.Severity = SeverityCritical
		case item.group.DegradedInstanceCount > 0 || item.group.LastError != "":
			item.group.Severity = SeverityWarning
		}
		for _, evidenceKey := range sortedStringKeys(item.evidence) {
			item.group.MetricEvidence = append(item.group.MetricEvidence, item.evidence[evidenceKey])
		}
		view.FamilyGroups = append(view.FamilyGroups, item.group)
	}
	view.Summary.FamilyGroupCount = len(view.FamilyGroups)
	for _, group := range view.FamilyGroups {
		if group.Severity != SeverityHealthy {
			view.Summary.AbnormalFamilyGroupCount++
		}
	}
	for _, runtime := range view.L1CapabilityRuntime {
		health := l1RuntimeHealth(runtime)
		if health == "degraded" || health == "unknown" {
			view.Summary.AbnormalL1CapabilityCount++
		}
	}
	view.Summary.Ready = view.Summary.ComponentTotal > 0 && view.Summary.HealthyComponentCount == view.Summary.ComponentTotal && view.Summary.AbnormalFamilyGroupCount == 0 && view.Summary.AbnormalL1CapabilityCount == 0
	return view
}

func canonicalCacheComponent(component string) string {
	if component == "apiserver" {
		return "qs-apiserver"
	}
	return component
}

func AttachCacheL1WindowEvidence(ctx context.Context, view *CacheRuntimeView, evidence MetricEvidenceReader, window string, evalAt time.Time) {
	if view == nil {
		return
	}
	type pair struct {
		hitRate *MetricEvidence
		samples *MetricEvidence
	}
	cache := map[string]pair{}
	for index := range view.L1CapabilityRuntime {
		row := &view.L1CapabilityRuntime[index]
		key := row.Component + "\x00" + row.Capability
		item, ok := cache[key]
		if !ok {
			if metric, exists := evidence.CollectionL1HitRate(ctx, row.Capability, window, evalAt); exists {
				item.hitRate = &metric
			}
			if metric, exists := evidence.CollectionL1Samples(ctx, row.Capability, window, evalAt); exists {
				item.samples = &metric
			}
			if !hasWindowSamples(item.samples) {
				item.hitRate = nil
			}
			cache[key] = item
		}
		row.HitRate, row.Samples = item.hitRate, item.samples
	}
}

func hasWindowSamples(samples *MetricEvidence) bool {
	return samples != nil && samples.Available && samples.Value != nil && *samples.Value > 0
}

func AttachCacheL2OperationEvidence(ctx context.Context, view *CacheRuntimeView, evidence MetricEvidenceReader, window string, evalAt time.Time) {
	if view == nil {
		return
	}
	for index := range view.FamilyGroups {
		group := &view.FamilyGroups[index]
		if metric, ok := evidence.CacheFamilyOperationP95(ctx, group.Component, group.Family, window, evalAt); ok {
			group.OperationP95 = &metric
			group.MetricEvidence = append(group.MetricEvidence, metric)
		}
		if metric, ok := evidence.CacheFamilyOperationErrors(ctx, group.Component, group.Family, window, evalAt); ok {
			group.OperationErrors = &metric
			group.MetricEvidence = append(group.MetricEvidence, metric)
			if metric.Available && metric.Value != nil && *metric.Value > 0 && group.Severity == SeverityHealthy {
				group.Severity = SeverityWarning
				view.Summary.AbnormalFamilyGroupCount++
				view.Summary.Ready = false
			}
		}
	}
}

func BuildCacheTopologyView(registry CacheRegistryView, runtime CacheRuntimeView, l2Workloads []CacheCapabilityRow) CacheTopologyView {
	view := CacheTopologyView{Topologies: []CacheTopology{}}
	l2ByCapability := map[string]CacheCapabilityWorkload{}
	for _, row := range l2Workloads {
		l2ByCapability[row.Capability] = row.Workload
	}
	policyPath := map[string]string{}
	conflictingPolicyPath := map[string]bool{}
	for _, component := range registry.ComponentRegistries {
		if !component.Available || component.PolicySource == nil {
			continue
		}
		path := component.PolicySource.Path
		if existing, ok := policyPath[component.Component]; ok && existing != path {
			conflictingPolicyPath[component.Component] = true
			continue
		}
		policyPath[component.Component] = path
	}
	for component := range conflictingPolicyPath {
		delete(policyPath, component)
	}
	for _, source := range cachetopology.Sources() {
		topology := CacheTopology{
			TopologyGroup: source.TopologyGroup, ReadModel: source.ReadModel, Status: "unknown",
			Nodes: []CacheTopologyNode{}, Edges: []CacheTopologyEdge{},
			Source:         CacheTopologySource{ID: "source:" + source.TopologyGroup, ReadModel: source.ReadModel, SourceKind: source.SourceKind},
			WindowEvidence: map[string]*MetricEvidence{},
		}
		for _, capability := range registry.CapabilityRows {
			group := capability.TopologyGroup
			order := capability.TopologyOrder
			readModel := capability.ReadModel
			family := capability.Family
			if !capability.Consistent {
				var ok bool
				group, order, readModel, family, ok = consistentVariantTopology(capability.Variants)
				if !ok {
					continue
				}
			}
			if group != source.TopologyGroup {
				continue
			}
			node := CacheTopologyNode{
				ID:        capability.Component + ":" + capability.Capability + ":" + capability.Layer,
				Component: capability.Component, Capability: capability.Capability, Layer: capability.Layer,
				Enabled: capability.Enabled, RegistryConsistent: capability.Consistent, RuntimeHealth: "unknown",
				PolicySource: policyPath[capability.Component], Order: order,
			}
			if readModel != "" {
				topology.ReadModel = readModel
			}
			if capability.Layer == "L1" {
				node.RuntimeHealth = ""
				matchedInstances := map[string]struct{}{}
				for _, item := range runtime.L1CapabilityRuntime {
					if item.Component != capability.Component || item.Capability != capability.Capability {
						continue
					}
					matchedInstances[item.InstanceID] = struct{}{}
					node.RuntimeHealth = mergeRuntimeHealth(node.RuntimeHealth, l1RuntimeHealth(item))
					node.HitRate, node.Samples = item.HitRate, item.Samples
				}
				if len(matchedInstances) == 0 {
					node.RuntimeHealth = "unknown"
				} else if len(matchedInstances) < len(capability.InstanceIDs) {
					node.RuntimeHealth = mergeRuntimeHealth(node.RuntimeHealth, "degraded")
				}
			} else {
				node.RuntimeHealth = l2RuntimeHealth(runtime.FamilyGroups, capability.Component, family)
				if workload, ok := l2ByCapability[capability.Capability]; ok {
					node.HitRate, node.Samples = workload.HitRate, workload.Samples
				}
			}
			if node.HitRate != nil {
				topology.WindowEvidence[node.ID+":hit_rate"] = node.HitRate
			}
			if node.Samples != nil {
				topology.WindowEvidence[node.ID+":samples"] = node.Samples
			}
			topology.Nodes = append(topology.Nodes, node)
		}
		sort.Slice(topology.Nodes, func(i, j int) bool { return topology.Nodes[i].Order < topology.Nodes[j].Order })
		for index := range topology.Nodes {
			to := topology.Source.ID
			if index+1 < len(topology.Nodes) {
				to = topology.Nodes[index+1].ID
			}
			topology.Edges = append(topology.Edges, CacheTopologyEdge{From: topology.Nodes[index].ID, To: to, Kind: "miss_fallback"})
		}
		topology.Status = topologyStatus(topology.Nodes)
		view.Topologies = append(view.Topologies, topology)
	}
	return view
}

func consistentVariantTopology(variants []RegistryCapabilityVariant) (string, int, string, string, bool) {
	if len(variants) == 0 {
		return "", 0, "", "", false
	}
	group, order, readModel, family := variants[0].TopologyGroup, variants[0].TopologyOrder, variants[0].ReadModel, variants[0].Family
	for _, variant := range variants[1:] {
		if variant.TopologyGroup != group || variant.TopologyOrder != order || variant.ReadModel != readModel || variant.Family != family {
			return "", 0, "", "", false
		}
	}
	return group, order, readModel, family, group != ""
}

func mergeRuntimeHealth(current, next string) string {
	if current == "" {
		return next
	}
	priority := map[string]int{"healthy": 1, "disabled": 2, "unknown": 3, "degraded": 4}
	if priority[next] > priority[current] {
		return next
	}
	return current
}

func l2RuntimeHealth(groups []CacheFamilyGroup, component, family string) string {
	status := ""
	for _, group := range groups {
		if group.Component != component || group.Family != family {
			continue
		}
		next := "healthy"
		if group.Severity != SeverityHealthy {
			next = "degraded"
		}
		status = mergeRuntimeHealth(status, next)
	}
	if status == "" {
		return "unknown"
	}
	return status
}

func l1RuntimeHealth(runtime CacheL1CapabilityRuntimeRow) string {
	if !runtime.Enabled {
		return "disabled"
	}
	if len(runtime.Buckets) == 0 {
		return "unknown"
	}
	if runtime.SignalWatcher.LastError != "" || runtime.SignalWatcher.Status == "reconnecting" {
		return "degraded"
	}
	if runtime.SignalWatcher.Configured && runtime.SignalWatcher.Status != "running" {
		return "unknown"
	}
	return "healthy"
}

func topologyStatus(nodes []CacheTopologyNode) string {
	if len(nodes) == 0 {
		return "unknown"
	}
	disabled := false
	for _, node := range nodes {
		if !node.RegistryConsistent || node.RuntimeHealth == "degraded" || node.RuntimeHealth == "unknown" {
			return "degraded"
		}
		if node.Enabled != nil && !*node.Enabled {
			disabled = true
		}
	}
	if disabled {
		return "disabled"
	}
	return "healthy"
}

func sortedStringKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func intString(value int) string {
	return strconv.Itoa(value)
}
