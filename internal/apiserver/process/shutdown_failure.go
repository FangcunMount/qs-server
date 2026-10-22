package process

import (
	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/shutdown"
	"os"
)

func transactionShutdownFailure(exit func(int)) shutdown.ErrorFunc {
	return shutdown.ErrorFunc(func(error) {
		// The POSIX manager otherwise exits0 after callback errors. A timed-out
		// drain must not look like a successful process termination.
		log.Errorw("transaction host shutdown incomplete or failed", "exit_code", 1)
		log.Flush()
		exit(1)
	})
}
func configureTransactionShutdownFailure(gs *shutdown.GracefulShutdown) {
	gs.SetErrorHandler(transactionShutdownFailure(os.Exit))
}
