// Package queryerror owns the public error mapping for Interpretation reads.
// Storage consistency details stay in the read model logs and are never
// downgraded to not-found or returned to callers.
package queryerror

import (
	"errors"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// MapReadError preserves the distinction between an absent report, a corrupt
// catalog association, and an ordinary storage dependency failure.
func MapReadError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, interpretationreadmodel.ErrReportNotFound) {
		return cberrors.WithCode(code.ErrInterpretReportNotFound, "报告不存在")
	}
	if IsConsistencyError(err) {
		return cberrors.WithCode(code.ErrInterpretReportConsistency, "report temporarily unavailable")
	}
	return cberrors.WrapC(err, code.ErrDatabase, "查询报告失败")
}

// IsConsistencyError reports whether the read was blocked by a catalog/source
// invariant violation rather than by absence or dependency failure.
func IsConsistencyError(err error) bool {
	var dangling *interpretationreadmodel.CatalogDanglingSourceError
	if errors.As(err, &dangling) {
		return true
	}
	var mismatch *interpretationreadmodel.CatalogSourceAssociationMismatchError
	return errors.As(err, &mismatch)
}
