package answeringstart

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answeringstart"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"strconv"
)

type ContentAdmission interface {
	ValidateStartContent(context.Context, *domain.Intent) error
}
type TesteeReader interface {
	FindByID(context.Context, testee.ID) (*testee.Testee, error)
}
type ProfileLinks interface {
	IsEnabled() bool
	HasActiveProfileLink(context.Context, string, string) (bool, error)
}
type participantAdmission struct {
	content  ContentAdmission
	subjects TesteeReader
	links    ProfileLinks
}

func NewParticipantAdmission(content ContentAdmission, subjects TesteeReader, links ProfileLinks) Admission {
	return &participantAdmission{content: content, subjects: subjects, links: links}
}
func (a *participantAdmission) ValidateStart(ctx context.Context, intent *domain.Intent) error {
	if a.content == nil || a.subjects == nil || a.links == nil || !a.links.IsEnabled() {
		return errors.WithCode(code.ErrInternalServerError, "作答准入不可用")
	}
	subject, err := a.subjects.FindByID(ctx, meta.FromUint64(intent.TesteeID))
	if err != nil {
		return err
	}
	if subject == nil || subject.OrgID() != intent.OrgID || subject.ProfileID() == nil {
		return errors.WithCode(code.ErrPermissionDenied, "无权为该受试者开始作答")
	}
	allowed, err := a.links.HasActiveProfileLink(ctx, strconv.FormatUint(intent.UserID, 10), strconv.FormatUint(*subject.ProfileID(), 10))
	if err != nil {
		return err
	}
	if !allowed {
		return errors.WithCode(code.ErrPermissionDenied, "无权为该受试者开始作答")
	}
	return a.content.ValidateStartContent(ctx, intent)
}
