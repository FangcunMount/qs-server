package grpcclient

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/actor"
)

// ActorClient Actor 服务客户端
type ActorClient struct {
	client pb.ActorServiceClient
	base   *Client
}

// NewActorClient 创建 Actor 服务客户端
func NewActorClient(base *Client) *ActorClient {
	return &ActorClient{
		client: pb.NewActorServiceClient(base.Conn()),
		base:   base,
	}
}

// CreateTesteeRequest 创建受试者请求参数
type CreateTesteeRequest struct {
	OrgID      uint64     // 机构ID
	IAMUserID  string     // IAM用户ID（成人）- 字符串格式
	IAMChildID string     // IAM儿童ID - 字符串格式
	Name       string     // 姓名
	Gender     int32      // 性别：1-男，2-女，3-其他
	Birthday   *time.Time // 出生日期
	Tags       []string   // 标签列表
	Source     string     // 来源：online_form/plan/screening/imported
	IsKeyFocus bool       // 是否重点关注
}

// TesteeResponse 受试者响应
type TesteeResponse struct {
	ID         uint64    // 受试者ID
	OrgID      uint64    // 机构ID
	IAMUserID  string    // IAM用户ID - 字符串格式
	IAMChildID string    // IAM儿童ID - 字符串格式
	Name       string    // 姓名
	Gender     int32     // 性别
	Birthday   time.Time // 出生日期
	Tags       []string  // 标签列表
	Source     string    // 来源
	IsKeyFocus bool      // 是否重点关注

	// 测评统计信息
	AssessmentStats *AssessmentStats

	CreatedAt time.Time // 创建时间
	UpdatedAt time.Time // 更新时间
}

// AssessmentStats 测评统计信息
type AssessmentStats struct {
	TotalCount       int32     // 总测评次数
	LastAssessmentAt time.Time // 最后测评时间
	LastRiskLevel    string    // 最后风险等级
}

// CreateTestee 创建受试者
func (c *ActorClient) CreateTestee(ctx context.Context, req *CreateTesteeRequest) (*TesteeResponse, error) {
	ctx, cancel := c.base.ContextWithTimeout(ctx)
	defer cancel()

	// 转换字符串 ID 为 uint64
	var iamUserID uint64
	var iamChildID uint64
	var err error

	if req.IAMUserID != "" {
		iamUserID, err = strconv.ParseUint(req.IAMUserID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid iam_user_id format: %w", err)
		}
	}

	if req.IAMChildID != "" {
		iamChildID, err = strconv.ParseUint(req.IAMChildID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid iam_child_id format: %w", err)
		}
	}

	pbReq := &pb.CreateTesteeRequest{
		OrgId:      req.OrgID,
		IamUserId:  iamUserID,
		IamChildId: iamChildID,
		Name:       req.Name,
		Gender:     req.Gender,
		Tags:       req.Tags,
		Source:     req.Source,
		IsKeyFocus: req.IsKeyFocus,
	}

	if req.Birthday != nil {
		pbReq.Birthday = timestamppb.New(*req.Birthday)
	}

	resp, err := c.client.CreateTestee(ctx, pbReq)
	if err != nil {
		return nil, err
	}

	return convertTesteeResponse(resp), nil
}

// GetTestee 获取受试者详情
func (c *ActorClient) GetTestee(ctx context.Context, testeeID uint64) (*TesteeResponse, error) {
	ctx, cancel := c.base.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.client.GetTestee(ctx, &pb.GetTesteeRequest{
		Id: testeeID,
	})
	if err != nil {
		return nil, err
	}

	return convertTesteeResponse(resp), nil
}

// UpdateTesteeRequest 更新受试者请求参数
type UpdateTesteeRequest struct {
	ID         uint64     // 受试者ID
	Name       string     // 姓名
	Gender     int32      // 性别
	Birthday   *time.Time // 出生日期
	Tags       []string   // 标签列表
	IsKeyFocus bool       // 是否重点关注
}

// UpdateTestee 更新受试者信息
func (c *ActorClient) UpdateTestee(ctx context.Context, req *UpdateTesteeRequest) (*TesteeResponse, error) {
	ctx, cancel := c.base.ContextWithTimeout(ctx)
	defer cancel()

	pbReq := &pb.UpdateTesteeRequest{
		Id:         req.ID,
		Name:       req.Name,
		Gender:     req.Gender,
		Tags:       req.Tags,
		IsKeyFocus: req.IsKeyFocus,
	}

	if req.Birthday != nil {
		pbReq.Birthday = timestamppb.New(*req.Birthday)
	}

	resp, err := c.client.UpdateTestee(ctx, pbReq)
	if err != nil {
		return nil, err
	}

	return convertTesteeResponse(resp), nil
}

// TesteeExists 检查受试者是否存在
func (c *ActorClient) TesteeExists(ctx context.Context, orgID, iamChildID uint64) (exists bool, testeeID uint64, err error) {
	ctx, cancel := c.base.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.client.TesteeExists(ctx, &pb.TesteeExistsRequest{
		OrgId:      orgID,
		IamChildId: iamChildID,
	})
	if err != nil {
		return false, 0, err
	}

	return resp.Exists, resp.TesteeId, nil
}

// ListTesteesByOrg 根据机构查询受试者列表
func (c *ActorClient) ListTesteesByOrg(ctx context.Context, orgID uint64, offset, limit int32) ([]*TesteeResponse, int64, error) {
	ctx, cancel := c.base.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.client.ListTesteesByOrg(ctx, &pb.ListTesteesByOrgRequest{
		OrgId:  orgID,
		Offset: offset,
		Limit:  limit,
	})
	if err != nil {
		return nil, 0, err
	}

	testees := make([]*TesteeResponse, 0, len(resp.Items))
	for _, t := range resp.Items {
		testees = append(testees, convertTesteeResponse(t))
	}

	return testees, resp.Total, nil
}

// ListTesteesByUser 根据用户（监护人）的孩子ID列表查询受试者列表
// 用于 collection-server 查询当前用户的所有受试者
func (c *ActorClient) ListTesteesByUser(ctx context.Context, childIDs []uint64, offset, limit int32) ([]*TesteeResponse, int64, error) {
	ctx, cancel := c.base.ContextWithTimeout(ctx)
	defer cancel()

	resp, err := c.client.ListTesteesByUser(ctx, &pb.ListTesteesByUserRequest{
		IamChildIds: childIDs,
		Offset:      offset,
		Limit:       limit,
	})
	if err != nil {
		return nil, 0, err
	}

	testees := make([]*TesteeResponse, 0, len(resp.Items))
	for _, t := range resp.Items {
		testees = append(testees, convertTesteeResponse(t))
	}

	return testees, resp.Total, nil
}

// convertTesteeResponse 转换 protobuf 响应为本地结构
func convertTesteeResponse(resp *pb.TesteeResponse) *TesteeResponse {
	if resp == nil {
		return nil
	}

	result := &TesteeResponse{
		ID:         resp.Id,
		OrgID:      resp.OrgId,
		IAMUserID:  strconv.FormatUint(resp.IamUserId, 10),
		IAMChildID: strconv.FormatUint(resp.IamChildId, 10),
		Name:       resp.Name,
		Gender:     resp.Gender,
		Tags:       resp.Tags,
		Source:     resp.Source,
		IsKeyFocus: resp.IsKeyFocus,
	}

	if resp.Birthday != nil {
		result.Birthday = resp.Birthday.AsTime()
	}

	if resp.CreatedAt != nil {
		result.CreatedAt = resp.CreatedAt.AsTime()
	}

	if resp.UpdatedAt != nil {
		result.UpdatedAt = resp.UpdatedAt.AsTime()
	}

	// 转换测评统计信息
	if resp.AssessmentStats != nil {
		result.AssessmentStats = &AssessmentStats{
			TotalCount:    resp.AssessmentStats.TotalCount,
			LastRiskLevel: resp.AssessmentStats.LastRiskLevel,
		}
		if resp.AssessmentStats.LastAssessmentAt != nil {
			result.AssessmentStats.LastAssessmentAt = resp.AssessmentStats.LastAssessmentAt.AsTime()
		}
	}

	return result
}
