package evaluation

import (
	"gorm.io/gorm"
	"strconv"
)

// Point reads use an exact batch of IDs from the page, without organization or
// deletion predicates. The cache prevents one scan per old source/event row.
func (c *SQLHistoricalResponsibilityCycle) checkPageOwners(tx *gorm.DB, start int) error {
	ids := make([]uint64, 0)
	seen := map[uint64]bool{}
	for i := start; i < len(c.observations); i++ {
		id := c.observations[i].AssessmentID
		if id == 0 {
			continue
		}
		if _, ok := c.owners[id]; !ok && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		rows, _, _, err := cycleQuery(tx, "SELECT id,org_id,testee_id,deleted_at FROM assessment WHERE id IN ? ORDER BY id", len(ids), ids)
		if err != nil {
			return err
		}
		for _, id := range ids {
			c.owners[id] = sqlResponsibilityOwner{absent: true}
			c.anchorDigests["assessment:"+strconv.FormatUint(id, 10)] = "absent"
		}
		for _, row := range rows {
			id := cyclePositive(row, "id")
			if !seen[id] || id == 0 {
				return ErrSQLResponsibilityInvalid
			}
			owner := c.owners[id]
			if !owner.absent {
				return ErrSQLResponsibilityInvalid
			}
			org, testee := cyclePositive(row, "org_id"), cyclePositive(row, "testee_id")
			c.owners[id] = sqlResponsibilityOwner{org: org, testee: testee, invalid: org == 0 || testee == 0 || row["deleted_at"] != nil}
			c.anchorDigests["assessment:"+strconv.FormatUint(id, 10)] = cycleRowDigest([]string{"id", "org_id", "testee_id", "deleted_at"}, row)
		}
	}
	for i := start; i < len(c.observations); i++ {
		v := &c.observations[i]
		if v.AssessmentID == 0 {
			continue
		}
		owner := c.owners[v.AssessmentID]
		if owner.absent {
			cycleReason(v, "assessment_owner_orphan")
		} else if owner.invalid || v.OrgID != owner.org || v.TesteeID != 0 && v.TesteeID != owner.testee {
			cycleReason(v, "assessment_owner_identity_conflict")
		}
	}
	// Exact Outcome/Run IDs on current wire must resolve; an Assessment alone
	// cannot hide an orphan original Outcome or a copied Run identity.
	outcomeIDs := []uint64{}
	outcomeSeen := map[uint64]bool{}
	runIDs := []string{}
	runSeen := map[string]bool{}
	for i := start; i < len(c.observations); i++ {
		v := c.observations[i]
		if v.outcomeID != 0 && !outcomeSeen[v.outcomeID] {
			outcomeSeen[v.outcomeID] = true
			outcomeIDs = append(outcomeIDs, v.outcomeID)
		}
		if v.runID != "" && !runSeen[v.runID] {
			runSeen[v.runID] = true
			runIDs = append(runIDs, v.runID)
		}
	}
	outcomes := map[uint64]historicalSQLRow{}
	runs := map[string][]historicalSQLRow{}
	if len(outcomeIDs) > 0 {
		for _, id := range outcomeIDs {
			c.anchorDigests["outcome:"+strconv.FormatUint(id, 10)] = "absent"
		}
		rows, _, _, err := cycleQuery(tx, "SELECT id,assessment_id,org_id,testee_id,evaluation_run_id FROM evaluation_outcome WHERE id IN ? ORDER BY id", len(outcomeIDs), outcomeIDs)
		if err != nil {
			return err
		}
		for _, row := range rows {
			id := cyclePositive(row, "id")
			if id == 0 || outcomes[id] != nil {
				return ErrSQLResponsibilityInvalid
			}
			outcomes[id] = row
			c.anchorDigests["outcome:"+strconv.FormatUint(id, 10)] = cycleRowDigest([]string{"id", "assessment_id", "org_id", "testee_id", "evaluation_run_id"}, row)
		}
	}
	if len(runIDs) > 0 {
		for _, id := range runIDs {
			c.anchorDigests["run_resource:"+id] = "absent"
		}
		rows, _, _, err := cycleQuery(tx, "SELECT id,resource_id,assessment_id,scope,attempt_no,deleted_at FROM runtime_checkpoint WHERE resource_id IN ? ORDER BY id", len(runIDs)*2, runIDs)
		if err != nil {
			return err
		}
		for _, row := range rows {
			key := valueOrEmpty(row["resource_id"])
			runs[key] = append(runs[key], row)
			c.anchorDigests["run_row:"+valueOrEmpty(row["id"])] = cycleRowDigest([]string{"id", "resource_id", "assessment_id", "scope", "attempt_no", "deleted_at"}, row)
			c.anchorDigests["run_resource:"+key] = "present"
		}
	}
	for i := start; i < len(c.observations); i++ {
		v := &c.observations[i]
		if v.outcomeID == 0 {
			continue
		}
		outcome := outcomes[v.outcomeID]
		if outcome == nil {
			cycleReason(v, "original_outcome_orphan")
		} else if cyclePositive(outcome, "assessment_id") != v.AssessmentID || cyclePositive(outcome, "org_id") != v.OrgID || cyclePositive(outcome, "testee_id") != v.TesteeID || valueOrEmpty(outcome["evaluation_run_id"]) != v.runID {
			cycleReason(v, "original_outcome_identity_conflict")
		}
		candidates := runs[v.runID]
		if len(candidates) != 1 {
			cycleReason(v, "original_run_absent_or_ambiguous")
		} else {
			run := candidates[0]
			if cyclePositive(run, "assessment_id") != v.AssessmentID || valueOrEmpty(run["scope"]) != "evaluation_run" || cyclePositive(run, "attempt_no") == 0 || run["deleted_at"] != nil {
				cycleReason(v, "original_run_identity_conflict")
			}
		}
	}
	return nil
}

func (c *SQLHistoricalResponsibilityCycle) checkReverse() {
	requests := map[string][]int{}
	standard := map[string][]int{}
	messages := map[string][]int{}
	refs := map[string][]int{}
	for i, v := range c.observations {
		if v.EventID != "" {
			c.byEvent[v.EventID] = append(c.byEvent[v.EventID], i)
		}
		if v.AssessmentID != 0 {
			c.byOwner[v.AssessmentID] = append(c.byOwner[v.AssessmentID], i)
		}
		switch v.Store {
		case "qs_rm_replay_requests":
			key := cyclePair(v.OrgID, v.link.requestID)
			requests[key] = append(requests[key], i)
		case "rm_outbox":
			standard[v.EventID] = append(standard[v.EventID], i)
			messages[v.EventID] = append(messages[v.EventID], i)
		case "retry_event_hold", "event_delivery_dead_letter":
			messages[v.EventID] = append(messages[v.EventID], i)
		case "qs_rm_evaluation_request_ref":
			refs[v.EventID] = append(refs[v.EventID], i)
		}
	}
	// Multiple deliveries of one original event can be retained. They cannot
	// disagree about its typed immutable identity or original payload bytes.
	for id, indexes := range messages {
		if id == "" {
			continue
		}
		first := c.observations[indexes[0]]
		conflict := false
		for _, i := range indexes {
			current := c.observations[i]
			if current.EventType != first.EventType || current.OrgID != first.OrgID || current.AssessmentID != first.AssessmentID || current.OwnerKind != first.OwnerKind || current.OwnerID != first.OwnerID || current.payloadSHA256 != first.payloadSHA256 {
				conflict = true
				break
			}
		}
		if conflict {
			for _, i := range indexes {
				cycleReason(&c.observations[i], "duplicate_event_identity_conflict")
			}
		}
		if len(standard[id]) > 1 {
			for _, i := range standard[id] {
				cycleReason(&c.observations[i], "duplicate_standard_event_reference")
			}
		}
	}
	for id, indexes := range refs {
		if len(indexes) > 1 {
			for _, i := range indexes {
				cycleReason(&c.observations[i], "duplicate_business_event_reference")
			}
		}
		for _, i := range indexes {
			ref := &c.observations[i]
			candidates := standard[id]
			if len(candidates) != 1 {
				cycleReason(ref, "current_business_reference_unbound")
				continue
			}
			current := c.observations[candidates[0]]
			if current.Invalid || current.EventType != "evaluation.requested" || current.OrgID != ref.OrgID || current.AssessmentID != ref.AssessmentID {
				cycleReason(ref, "business_reference_current_identity_conflict")
			}
		}
	}
	// A new requested message's canonical durable reference is independent of
	// transport settlement. Reverse coverage must not silently skip an orphan.
	for id, indexes := range standard {
		for _, i := range indexes {
			current := &c.observations[i]
			if current.EventType != "evaluation.requested" {
				continue
			}
			if len(refs[id]) != 1 {
				cycleReason(current, "requested_business_reference_absent_or_ambiguous")
			} else {
				ref := c.observations[refs[id][0]]
				if ref.OrgID != current.OrgID || ref.AssessmentID != current.AssessmentID {
					cycleReason(current, "requested_business_reference_identity_conflict")
				}
			}
		}
	}
	itemsByRequest := map[string]int{}
	for i := range c.observations {
		v := &c.observations[i]
		switch v.Store {
		case "qs_rm_gap_recovery_request":
			if !v.link.authorized {
				continue
			}
			candidates := standard[v.EventID]
			if len(candidates) != 1 {
				cycleReason(v, "authorized_gap_current_event_absent_or_ambiguous")
				v.Unfinished = true
				continue
			}
			current := c.observations[candidates[0]]
			if current.Invalid || current.EventType != "evaluation.requested" || current.OrgID != v.OrgID || current.AssessmentID != v.AssessmentID {
				cycleReason(v, "authorized_gap_current_event_conflict")
			}
			v.Unfinished = current.Unfinished || current.LeasePresent
		case "qs_rm_replay_items":
			key := cyclePair(v.OrgID, v.link.requestID)
			itemsByRequest[key]++
			parents := requests[key]
			if len(parents) != 1 {
				cycleReason(v, "replay_parent_absent_or_ambiguous")
				continue
			}
			parent := &c.observations[parents[0]]
			if parent.Invalid {
				cycleReason(v, "replay_parent_invalid")
			}
			v.link.store = parent.link.store
			if parent.link.store == "mongo-domain-events" {
				v.OwnerUnproven = true
				v.ScopeClass = "coordination_required"
				continue
			}
			if !v.link.authorized && len(standard[v.EventID]) == 0 && v.link.code == "not_found" {
				v.ScopeClass = "coordination_required"
				v.OwnerUnproven = true
				continue
			}
			candidates := standard[v.EventID]
			if len(candidates) != 1 {
				cycleReason(v, "replay_current_event_absent_or_ambiguous")
				continue
			}
			current := c.observations[candidates[0]]
			v.AssessmentID, v.TesteeID, v.EventType, v.OwnerKind, v.OwnerID = current.AssessmentID, current.TesteeID, current.EventType, current.OwnerKind, current.OwnerID
			v.ScopeClass = current.ScopeClass
			if current.Invalid || current.OrgID != v.OrgID {
				cycleReason(v, "replay_current_event_identity_conflict")
			}
			if v.link.authorized {
				v.Unfinished = current.Unfinished || current.LeasePresent
			}
			if v.AssessmentID != 0 {
				c.byOwner[v.AssessmentID] = append(c.byOwner[v.AssessmentID], i)
			}
		}
	}
	for key, indexes := range requests {
		for _, i := range indexes {
			if len(indexes) != 1 {
				cycleReason(&c.observations[i], "replay_request_identity_ambiguous")
			}
			if itemsByRequest[key] == 0 {
				cycleReason(&c.observations[i], "replay_request_items_absent")
			}
		}
	}
	// Unfinished organization-wide actions remain visible even when their
	// targets cannot safely be reconstructed from editable input JSON.
	for i, v := range c.observations {
		if v.Store == "system_governance_action_runs" && v.Unfinished && v.ScopeClass != "scope_outside_retirement" {
			c.byOrgActions[v.OrgID] = append(c.byOrgActions[v.OrgID], i)
		}
	}
	for _, v := range c.observations {
		c.report.Observed++
		switch v.ScopeClass {
		case "retirement_related":
			c.report.RetirementRelated++
		case "scope_outside_retirement":
			c.report.OutsideRetirement++
		default:
			c.report.Unknown++
		}
		if v.Invalid || v.ScopeClass == "retirement_related" && (v.Unfinished || v.LeasePresent) {
			c.report.Blocking++
		}
	}
}

// Fixed categories only; identifiers/body/error strings never enter reports.
func (c *SQLHistoricalResponsibilityCycle) UnboundEventCount() uint64 {
	if c == nil {
		return 0
	}
	var count uint64
	for _, v := range c.observations {
		if v.Invalid || v.ScopeClass == "coordination_required" || v.OwnerUnproven {
			count++
		}
	}
	return count
}
