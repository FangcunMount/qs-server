package main

import (
	"context"

	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/plansubmit"
	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedruntime"
)

func seedPlanSubmitOpenTasksDaemon(ctx context.Context, deps *seedruntime.Dependencies, verbose bool) error {
	_, err := plansubmit.RunDaemon(ctx, deps, verbose)
	return err
}
