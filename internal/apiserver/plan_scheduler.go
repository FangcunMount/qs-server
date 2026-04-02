package apiserver

import "github.com/FangcunMount/component-base/pkg/log"

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
