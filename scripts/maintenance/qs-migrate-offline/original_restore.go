package main

import (
	"errors"
	"flag"
	"path/filepath"
	"time"
)

type originalRestoreArguments struct {
	original, originalSHA, approval, approvalSHA string
	apply                                        bool
}

func registerOriginalRestoreFlags(f *flag.FlagSet, a *originalRestoreArguments) {
	f.StringVar(&a.original, "restore-original", "", "private exact original M5 material; defaults to inspection")
	f.StringVar(&a.originalSHA, "restore-original-sha256", "", "reviewed original material raw hash")
	f.BoolVar(&a.apply, "restore-apply", false, "independently approved pre-start exact empty-target/full-ledger restoration")
	f.StringVar(&a.approval, "restore-approval-file", "", "separate private M5 restoration approval")
	f.StringVar(&a.approvalSHA, "restore-approval-sha256", "", "independent restoration approval raw hash")
}

func (r originalRestoreArguments) validate(a arguments) error {
	if r.original == "" {
		if r.originalSHA != "" || r.apply || r.approval != "" || r.approvalSHA != "" {
			return errors.New("explicit separate original material required")
		}
		return nil
	}
	if !filepath.IsAbs(r.original) || !drainDigest.MatchString(r.originalSHA) || a.apply || a.protocolObservation || a.retainedBaseline || a.ownerDrain.enabled || a.binding.action != "" || a.timeout > 10*time.Minute {
		return errors.New("separate bounded original M5 restore action required")
	}
	if r.apply && (!filepath.IsAbs(r.approval) || !drainDigest.MatchString(r.approvalSHA)) {
		return errors.New("independent original restore approval required")
	}
	if !r.apply && (r.approval != "" || r.approvalSHA != "") {
		return errors.New("readonly original inspection does not consume approvals")
	}
	return nil
}
