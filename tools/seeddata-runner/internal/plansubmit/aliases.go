package plansubmit

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedapi"
	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedconfig"
	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedruntime"
)

type Dependencies = seedruntime.Dependencies
type dependencies = Dependencies
type SeedConfig = seedconfig.Config
type GlobalConfig = seedconfig.GlobalConfig
type APIClient = seedapi.APIClient
type ScaleResponse = seedapi.ScaleResponse
type PlanResponse = seedapi.PlanResponse
type TaskResponse = seedapi.TaskResponse
type PlanTaskWindowResponse = seedapi.PlanTaskWindowResponse
type ListPlanTaskWindowRequest = seedapi.ListPlanTaskWindowRequest
type QuestionnaireDetailResponse = seedapi.QuestionnaireDetailResponse
type SubmitAnswerSheetRequest = seedapi.SubmitAnswerSheetRequest
type AdminSubmitAnswerSheetRequest = seedapi.AdminSubmitAnswerSheetRequest
type Answer = seedapi.Answer
type SubmitAnswerSheetResponse = seedapi.SubmitAnswerSheetResponse
type QuestionResponse = seedapi.QuestionResponse
type OptionResponse = seedapi.OptionResponse
type Response = seedapi.Response

var NewAPIClient = seedapi.NewAPIClient

func newSeeddataLogger(verbose bool) log.Logger {
	return seedruntime.NewLogger(verbose)
}

type Options struct {
	PlanIDs    []string
	Workers    int
	Verbose    bool
	Continuous bool
}

type planOpenTaskSubmitOptions = Options

func optionsFromConfig(cfg *seedconfig.Config) Options {
	return Options{
		PlanIDs:    normalizePlanIDs(cfg.PlanSubmit.PlanIDStrings()),
		Workers:    cfg.PlanSubmit.Workers,
		Continuous: true,
	}
}

func RunDaemon(ctx context.Context, deps *Dependencies, verbose bool) (*planOpenTaskSubmitStats, error) {
	opts := optionsFromConfig(deps.Config)
	opts.Verbose = verbose
	return seedPlanSubmitOpenTasksDaemon(ctx, deps, opts)
}

func normalizePlanID(planID string) string {
	return strings.TrimSpace(planID)
}

func normalizePlanIDs(planIDs []string) []string {
	if len(planIDs) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(planIDs))
	seen := make(map[string]struct{}, len(planIDs))
	for _, planID := range planIDs {
		value := normalizePlanID(planID)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	return seedruntime.SleepWithContext(ctx, d)
}

func parseID(raw string) uint64 {
	return seedruntime.ParseID(raw)
}

func normalizePlanWorkers(workers, taskCount int) int {
	return seedruntime.NormalizePlanWorkers(workers, taskCount)
}

func prewarmAPIToken(ctx context.Context, client *APIClient, orgID int64, logger interface{ Warnw(string, ...interface{}) }) {
	if client == nil || orgID <= 0 {
		return
	}
	_, err := client.ListTesteesByOrg(ctx, orgID, 1, 1)
	if err != nil {
		logger.Warnw("Prewarm API token failed", "error", err)
	}
}

func sortTasksBySeq(tasks []TaskResponse) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Seq == tasks[j].Seq {
			return parseID(tasks[i].ID) < parseID(tasks[j].ID)
		}
		return tasks[i].Seq < tasks[j].Seq
	})
}
