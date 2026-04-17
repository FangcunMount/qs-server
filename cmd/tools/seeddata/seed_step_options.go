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

type assignmentSeedOptions struct {
	WorkerCount int
}

type planCreateOptions struct {
	PlanID           string
	PlanTesteeIDsRaw string
	PlanWorkers      int
	TesteePageSize   int
	TesteeOffset     int
	TesteeLimit      int
	Verbose          bool
}

type planProcessOptions struct {
	PlanID               string
	ScopeTesteeIDs       []string
	PlanWorkers          int
	PlanSubmitWorkers    int
	PlanWaitWorkers      int
	PlanMaxInFlightTasks int
	PlanSubmitQueueSize  int
	PlanSubmitQPS        float64
	PlanSubmitBurst      int
	PlanExpireRate       float64
	Verbose              bool
	Continuous           bool
}

type planFixupOptions struct {
	PlanID         string
	ScopeTesteeIDs []string
	Verbose        bool
}

type assessmentRetimeOptions struct {
	ScopeTesteeIDs []string
	CreatedAfter   string
	CreatedBefore  string
	Offset         string
	Limit          int
	AllowAll       bool
	DryRun         bool
	Verbose        bool
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
