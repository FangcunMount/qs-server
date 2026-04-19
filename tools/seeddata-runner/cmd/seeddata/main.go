package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"golang.org/x/sync/errgroup"

	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedconfig"
	"github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/seedruntime"
)

type cliOptions struct {
	configPath string
	verbose    bool
}

func main() {
	opts, err := parseCLIOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	logger := seedruntime.NewLogger(opts.verbose)

	cfg, err := seedconfig.Load(opts.configPath)
	if err != nil {
		logger.Errorw("Load seeddata config failed", "config", opts.configPath, "error", err.Error())
		os.Exit(1)
	}

	ctx, cancel := seedruntime.NewSignalContext()
	defer cancel()

	deps, err := seedruntime.LoadDependencies(ctx, cfg, logger)
	if err != nil {
		logger.Errorw("Initialize seeddata dependencies failed", "error", err.Error())
		os.Exit(1)
	}

	if err := runSeedSupervisor(ctx, deps, opts.verbose); err != nil {
		logger.Errorw("Seeddata supervisor exited with error", "error", err.Error())
		os.Exit(1)
	}
}

func parseCLIOptions(args []string) (cliOptions, error) {
	var opts cliOptions
	fs := flag.NewFlagSet("seeddata", flag.ContinueOnError)
	fs.StringVar(&opts.configPath, "config", "./configs/seeddata.yaml", "path to seeddata config yaml")
	fs.BoolVar(&opts.verbose, "verbose", false, "enable verbose logging")
	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}
	return opts, nil
}

func runSeedSupervisor(ctx context.Context, deps *seedruntime.Dependencies, verbose bool) error {
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return seedDailySimulationDaemon(groupCtx, deps)
	})
	group.Go(func() error {
		return seedPlanSubmitOpenTasksDaemon(groupCtx, deps, verbose)
	})
	return group.Wait()
}
