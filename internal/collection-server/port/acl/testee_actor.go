package acl

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
)

// actorReaderBridge 共享受试者读侧 gRPC 调用，供 answersheet 与 testee 复用。
type actorReaderBridge struct {
	inner grpcbridge.ActorReader
}

func newActorReaderBridge(inner grpcbridge.ActorReader) actorReaderBridge {
	return actorReaderBridge{inner: inner}
}

func (b actorReaderBridge) getTestee(ctx context.Context, testeeID uint64) (*grpcbridge.TesteeResponse, error) {
	return grpcbridge.CallBridge(b.inner,
		func() (*grpcbridge.TesteeResponse, error) { return b.inner.GetTestee(ctx, testeeID) },
		func(out *grpcbridge.TesteeResponse) *grpcbridge.TesteeResponse { return out },
	)
}

func (b actorReaderBridge) testeeExists(ctx context.Context, orgID, iamProfileID uint64) (bool, uint64, error) {
	if b.inner == nil {
		return false, 0, nil
	}
	return b.inner.TesteeExists(ctx, orgID, iamProfileID)
}

// TesteeActorLookup 将 ActorReader 适配为 answersheet.ActorLookup。
type TesteeActorLookup struct {
	bridge actorReaderBridge
}

// NewTesteeActorLookup 构造受试者查询 ACL 适配器。
func NewTesteeActorLookup(inner grpcbridge.ActorReader) *TesteeActorLookup {
	return &TesteeActorLookup{bridge: newActorReaderBridge(inner)}
}

func (a *TesteeActorLookup) GetTestee(ctx context.Context, testeeID uint64) (*answersheet.ActorTestee, error) {
	if a == nil {
		return nil, nil
	}
	out, err := a.bridge.getTestee(ctx, testeeID)
	if err != nil || out == nil {
		return nil, err
	}
	return &answersheet.ActorTestee{
		OrgID:        out.OrgID,
		IAMProfileID: out.IAMProfileID,
		Name:         out.Name,
	}, nil
}

func (a *TesteeActorLookup) TesteeExists(ctx context.Context, orgID, iamProfileID uint64) (bool, uint64, error) {
	if a == nil {
		return false, 0, nil
	}
	return a.bridge.testeeExists(ctx, orgID, iamProfileID)
}
