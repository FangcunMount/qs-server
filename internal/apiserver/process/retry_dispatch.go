package process

import (
	"context"
	"errors"

	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	scheduler "github.com/FangcunMount/qs-server/internal/apiserver/runtime/scheduler"
)

// buildRetryDispatchStart is inert. The actual runtime stage validates explicit
// schema before starting scans; constructors never install it or create clients.
func (s *server) buildRetryDispatchStart(host containerOutput) func(*runtimeOutput) error {
	if host.container == nil || !host.container.HasTransactionOwners() {
		return nil
	}
	return func(output *runtimeOutput) error {
		if s == nil || s.config == nil || s.config.Options == nil {
			return errors.New("retry dispatch requires the explicit host configuration")
		}
		var governance *opts.RetryGovernanceOptions
		if s.config.SystemGovernance != nil {
			governance = s.config.SystemGovernance.Retry
		}
		limits, automatic, err := governance.DispatchLimits()
		if err != nil {
			return err
		}
		c := host.container
		dispatchers, err := c.BuildRetryDispatchers(automatic)
		if err != nil {
			return err
		}
		if err = dispatchers.Validate(context.Background()); err != nil {
			return err
		}
		mysqlRunner, err := scheduler.NewRetryDispatchRunner("qs.mysql", dispatchers.MySQL.Once, func() bool { return c.EvaluationTransactionOwner().Runtime().Stats().Ready }, limits)
		if err != nil {
			return err
		}
		mongoRunner, err := scheduler.NewRetryDispatchRunner("qs.mongo", dispatchers.Mongo.Once, func() bool { return c.ReportTransactionOwner().Runtime().Stats().Ready }, limits)
		if err != nil {
			return err
		}
		startSchedulerManager(scheduler.NewManager(mysqlRunner, mongoRunner), output)
		return nil
	}
}
