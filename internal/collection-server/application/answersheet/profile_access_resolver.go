package answersheet

import (
	"context"
	"fmt"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ProfileAccessResolver 校验提交人对受试者的 ProfileLink 访问权限。
type ProfileAccessResolver struct {
	actorClient        ActorLookup
	profileLinkService profileLinkChecker
}

func NewProfileAccessResolver(actorClient ActorLookup, profileLinkService profileLinkChecker) *ProfileAccessResolver {
	return &ProfileAccessResolver{
		actorClient:        actorClient,
		profileLinkService: profileLinkService,
	}
}

func (r *ProfileAccessResolver) Resolve(ctx context.Context, writerID, testeeID uint64) (*ActorTestee, uint64, error) {
	if r == nil {
		return nil, 0, status.Error(codes.Unavailable, "profile access resolver is not configured")
	}
	return r.validateProfileAccess(ctx, writerID, testeeID)
}

func (r *ProfileAccessResolver) validateProfileAccess(ctx context.Context, writerID, testeeID uint64) (*ActorTestee, uint64, error) {
	l := logger.L(ctx)

	testee, resolvedTesteeID, err := r.resolveCanonicalTestee(ctx, testeeID)
	if err != nil {
		l.Errorw("查询受试者信息失败",
			"action", "submit_answersheet",
			"testee_id", testeeID,
			"error", err.Error(),
		)
		if status.Code(err) == codes.NotFound {
			return nil, 0, err
		}
		return nil, 0, status.Error(codes.Unavailable, "查询受试者信息失败")
	}

	if r.profileLinkService == nil || !r.profileLinkService.IsEnabled() {
		return testee, resolvedTesteeID, nil
	}

	if testee.IAMProfileID == "" {
		l.Warnw("受试者未绑定 IAM Profile，跳过权限校验",
			"testee_id", resolvedTesteeID,
			"testee_name", testee.Name,
		)
		return testee, resolvedTesteeID, nil
	}

	if err := r.checkProfileLinkAccess(ctx, writerID, resolvedTesteeID, testee.IAMProfileID, testee.Name); err != nil {
		return nil, 0, err
	}

	return testee, resolvedTesteeID, nil
}

func (r *ProfileAccessResolver) resolveCanonicalTestee(ctx context.Context, rawTesteeID uint64) (*ActorTestee, uint64, error) {
	if r.actorClient == nil {
		return nil, 0, status.Error(codes.Unavailable, "testee lookup is unavailable")
	}
	testee, err := r.actorClient.GetTestee(ctx, rawTesteeID)
	if err == nil {
		return testee, rawTesteeID, nil
	}
	if status.Code(err) != codes.NotFound || r.profileLinkService == nil {
		return nil, 0, fmt.Errorf("查询受试者信息失败: %w", err)
	}

	orgID := r.profileLinkService.GetDefaultOrgID()
	exists, canonicalTesteeID, existsErr := r.actorClient.TesteeExists(ctx, orgID, rawTesteeID)
	if existsErr != nil {
		return nil, 0, fmt.Errorf("查询受试者信息失败: %w", existsErr)
	}
	if !exists || canonicalTesteeID == 0 {
		return nil, 0, fmt.Errorf("查询受试者信息失败: %w", err)
	}

	canonicalTestee, canonicalErr := r.actorClient.GetTestee(ctx, canonicalTesteeID)
	if canonicalErr != nil {
		return nil, 0, fmt.Errorf("查询受试者信息失败: %w", canonicalErr)
	}

	logger.L(ctx).Warnw("提交答卷时检测到 profile_id 被误作 testee_id，已自动回退到 canonical testee_id",
		"action", "submit_answersheet",
		"submitted_testee_id", rawTesteeID,
		"canonical_testee_id", canonicalTesteeID,
		"org_id", orgID,
	)
	return canonicalTestee, canonicalTesteeID, nil
}

func (r *ProfileAccessResolver) checkProfileLinkAccess(ctx context.Context, writerID, testeeID uint64, iamProfileID, testeeName string) error {
	l := logger.L(ctx)

	userIDStr := strconv.FormatUint(writerID, 10)
	hasActiveProfileLink, err := r.profileLinkService.HasActiveProfileLink(ctx, userIDStr, iamProfileID)
	if err != nil {
		l.Errorw("校验 ProfileLink 权限失败",
			"action", "submit_answersheet",
			"writer_id", writerID,
			"testee_id", testeeID,
			"iam_profile_id", iamProfileID,
			"error", err.Error(),
		)
		return status.Error(codes.Unavailable, "校验 ProfileLink 权限失败")
	}

	if !hasActiveProfileLink {
		l.Warnw("无权为该受试者提交答卷：缺少 active ProfileLink",
			"action", "submit_answersheet",
			"writer_id", writerID,
			"testee_id", testeeID,
			"iam_profile_id", iamProfileID,
			"testee_name", testeeName,
			"result", "forbidden",
		)
		return status.Error(codes.PermissionDenied, "无权为该受试者提交答卷")
	}

	l.Infow("ProfileLink 权限验证通过",
		"action", "submit_answersheet",
		"writer_id", writerID,
		"testee_id", testeeID,
		"iam_profile_id", iamProfileID,
	)
	return nil
}
