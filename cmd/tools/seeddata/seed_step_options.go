package main

import "strings"

type assessmentSeedOptions struct {
	MinPerTestee      int
	MaxPerTestee      int
	WorkerCount       int
	SubmitWorkerCount int
	TesteePageSize    int
	TesteeOffset      int
	TesteeLimit       int
	CategoryFilter    string
	Verbose           bool
}

type planCreateOptions struct {
	PlanID                  string
	PlanMode                string
	PlanTesteeIDsRaw        string
	PlanWorkers             int
	PlanProcessExistingOnly bool
	TesteePageSize          int
	TesteeOffset            int
	TesteeLimit             int
	Verbose                 bool
}

type planProcessOptions struct {
	PlanID               string
	PlanMode             string
	ScopeTesteeIDs       []string
	PlanWorkers          int
	PlanSubmitWorkers    int
	PlanWaitWorkers      int
	PlanMaxInFlightTasks int
	PlanExpireRate       float64
	Verbose              bool
	Continuous           bool
}

func (o planProcessOptions) withScope(scopeTesteeIDs []string, continuous bool) planProcessOptions {
	o.ScopeTesteeIDs = append([]string(nil), scopeTesteeIDs...)
	o.Continuous = continuous
	return o
}

func normalizePlanID(planID string) string {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return defaultPlanID
	}
	return planID
}
