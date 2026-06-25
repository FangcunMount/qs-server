package editable

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/definition"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

type SnapshotRepository interface {
	CreatePublishedSnapshot(ctx context.Context, scale *scaledefinition.MedicalScale, active bool) error
}

// EnsureHeadEditable preserves the currently published snapshot before a
// published head is forked into a new draft candidate.
func EnsureHeadEditable(ctx context.Context, repo SnapshotRepository, scale *scaledefinition.MedicalScale) error {
	if scale == nil {
		return nil
	}
	if scale.IsArchived() {
		return errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档，不能编辑")
	}
	if !scale.IsPublished() {
		return nil
	}
	if err := repo.CreatePublishedSnapshot(ctx, scale, true); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存已发布量表快照失败")
	}
	versioning := scaledefinition.Versioning{}
	if err := versioning.ForkDraftFromPublished(scale); err != nil {
		return err
	}
	return nil
}
