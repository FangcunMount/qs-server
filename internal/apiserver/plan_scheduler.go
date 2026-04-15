package apiserver

import "github.com/FangcunMount/component-base/pkg/log"

// startPlanScheduler is kept only as a deprecated stub for historical context.
// The apiserver no longer starts a local plan scheduler; qs-worker owns that responsibility.
func (s *apiServer) startPlanScheduler() {
	if s == nil || s.config == nil {
		return
	}

	opts := s.config.PlanScheduler
	if opts == nil || !opts.Enable {
		return
	}

	log.Warnf("apiserver plan scheduler is deprecated and no longer starts locally; enable qs-worker plan_scheduler instead (org_ids=%v, lock_key=%s)",
		opts.OrgIDs, opts.LockKey)
}
