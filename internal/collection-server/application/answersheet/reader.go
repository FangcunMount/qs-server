package answersheet

import "context"

// AnswerSheetReader 答卷读端口（application-owned DTO）。
type AnswerSheetReader interface {
	GetAnswerSheet(ctx context.Context, id uint64) (*AnswerSheetResponse, error)
}

// ActorLookup 受试者查询端口（提交链路权限校验）。
type ActorLookup interface {
	GetTestee(ctx context.Context, testeeID uint64) (*ActorTestee, error)
	TesteeExists(ctx context.Context, orgID, iamProfileID uint64) (exists bool, testeeID uint64, err error)
}

// ActorTestee 提交链路所需的受试者字段。
type ActorTestee struct {
	OrgID        uint64
	IAMProfileID string
	Name         string
}
