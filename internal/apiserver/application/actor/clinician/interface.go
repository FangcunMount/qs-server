package clinician

import (
	"context"
	"time"
)

// ClinicianLifecycleService 从业者生命周期服务。
type ClinicianLifecycleService interface {
	Register(ctx context.Context, dto RegisterClinicianDTO) (*ClinicianResult, error)
	Update(ctx context.Context, dto UpdateClinicianDTO) (*ClinicianResult, error)
	Activate(ctx context.Context, clinicianID uint64) (*ClinicianResult, error)
	Deactivate(ctx context.Context, clinicianID uint64) (*ClinicianResult, error)
	BindOperator(ctx context.Context, dto BindClinicianOperatorDTO) (*ClinicianResult, error)
	UnbindOperator(ctx context.Context, clinicianID uint64) (*ClinicianResult, error)
	Delete(ctx context.Context, clinicianID uint64) error
}

// ClinicianQueryService 从业者查询服务。
type ClinicianQueryService interface {
	GetByID(ctx context.Context, clinicianID uint64) (*ClinicianResult, error)
	GetByOperator(ctx context.Context, orgID int64, operatorID uint64) (*ClinicianResult, error)
	ListClinicians(ctx context.Context, dto ListClinicianDTO) (*ClinicianListResult, error)
}

// ClinicianRelationshipService 从业者关系服务。
type ClinicianRelationshipService interface {
	AssignTestee(ctx context.Context, dto AssignTesteeDTO) (*RelationResult, error)
	AssignPrimary(ctx context.Context, dto AssignTesteeDTO) (*RelationResult, error)
	AssignAttending(ctx context.Context, dto AssignTesteeDTO) (*RelationResult, error)
	AssignCollaborator(ctx context.Context, dto AssignTesteeDTO) (*RelationResult, error)
	TransferPrimary(ctx context.Context, dto TransferPrimaryDTO) (*RelationResult, error)
	UnbindRelation(ctx context.Context, relationID uint64) (*RelationResult, error)
	ListAssignedTestees(ctx context.Context, dto ListAssignedTesteeDTO) (*AssignedTesteeListResult, error)
	ListAssignedTesteeIDs(ctx context.Context, orgID int64, clinicianID uint64) ([]uint64, error)
	ListTesteeRelations(ctx context.Context, dto ListTesteeRelationDTO) (*TesteeRelationListResult, error)
	ListClinicianRelations(ctx context.Context, dto ListClinicianRelationDTO) (*ClinicianRelationListResult, error)
	GetTesteeCareContext(ctx context.Context, orgID int64, testeeID uint64) (*TesteeCareContextResult, error)
}

// BehaviorEventStager 将看护关系行为事件暂存到 outbox。
type BehaviorEventStager interface {
	StageCareRelationshipTransferred(ctx context.Context, orgID int64, fromClinicianID, toClinicianID, testeeID uint64, occurredAt time.Time) error
}

// RegisterClinicianDTO 注册从业者。
type RegisterClinicianDTO struct {
	OrgID         int64
	OperatorID    *uint64
	Name          string
	Department    string
	Title         string
	ClinicianType string
	EmployeeCode  string
	IsActive      bool
}

// UpdateClinicianDTO 更新从业者。
type UpdateClinicianDTO struct {
	ClinicianID   uint64
	Name          string
	Department    string
	Title         string
	ClinicianType string
	EmployeeCode  string
}

// BindClinicianOperatorDTO 绑定从业者与后台操作者。
type BindClinicianOperatorDTO struct {
	ClinicianID uint64
	OperatorID  uint64
}

// ListClinicianDTO 从业者列表查询。
type ListClinicianDTO struct {
	OrgID  int64
	Offset int
	Limit  int
}

// ClinicianResult 从业者结果。
type ClinicianResult struct {
	ID                   uint64
	OrgID                int64
	OperatorID           *uint64
	Name                 string
	Department           string
	Title                string
	ClinicianType        string
	EmployeeCode         string
	IsActive             bool
	AssignedTesteeCount  int64
	AssessmentEntryCount int64
}

// ClinicianListResult 从业者列表结果。
type ClinicianListResult struct {
	Items      []*ClinicianResult
	TotalCount int64
	Offset     int
	Limit      int
}

// AssignTesteeDTO 建立从业者与受试者关系。
type AssignTesteeDTO struct {
	OrgID        int64
	ClinicianID  uint64
	TesteeID     uint64
	RelationType string
	SourceType   string
	SourceID     *uint64
}

// TransferPrimaryDTO 转移主责从业者。
type TransferPrimaryDTO struct {
	OrgID         int64
	ToClinicianID uint64
	TesteeID      uint64
	SourceType    string
	SourceID      *uint64
}

// RelationResult 关系结果。
type RelationResult struct {
	ID           uint64
	OrgID        int64
	ClinicianID  uint64
	TesteeID     uint64
	RelationType string
	SourceType   string
	SourceID     *uint64
	IsActive     bool
	BoundAt      time.Time
	UnboundAt    *time.Time
}

// ListAssignedTesteeDTO 查询从业者名下受试者。
type ListAssignedTesteeDTO struct {
	OrgID       int64
	ClinicianID uint64
	Offset      int
	Limit       int
}

// ListTesteeRelationDTO 查询受试者关系列表。
type ListTesteeRelationDTO struct {
	OrgID      int64
	TesteeID   uint64
	ActiveOnly bool
}

// ListClinicianRelationDTO 查询从业者关系列表。
type ListClinicianRelationDTO struct {
	OrgID       int64
	ClinicianID uint64
	Offset      int
	Limit       int
	ActiveOnly  bool
}

// AssignedTesteeResult 从业者名下受试者结果。
type AssignedTesteeResult struct {
	ID         uint64
	OrgID      int64
	ProfileID  *uint64
	Name       string
	Gender     int8
	Birthday   *time.Time
	Age        int
	Tags       []string
	Source     string
	IsKeyFocus bool
}

// AssignedTesteeListResult 从业者名下受试者列表结果。
type AssignedTesteeListResult struct {
	Items      []*AssignedTesteeResult
	TotalCount int64
	Offset     int
	Limit      int
}

// TesteeRelationResult 受试者关系列表项。
type TesteeRelationResult struct {
	Relation  *RelationResult
	Clinician *ClinicianResult
}

// TesteeRelationListResult 受试者关系列表结果。
type TesteeRelationListResult struct {
	Items []*TesteeRelationResult
}

// ClinicianRelationResult 从业者关系列表项。
type ClinicianRelationResult struct {
	Relation *RelationResult
	Testee   *AssignedTesteeResult
}

// ClinicianRelationListResult 从业者关系列表结果。
type ClinicianRelationListResult struct {
	Items      []*ClinicianRelationResult
	TotalCount int64
	Offset     int
	Limit      int
}

type TesteeCareContextResult struct {
	ClinicianName   string
	ClinicianRole   string
	RelationType    string
	EntrySourceType string
	EntryTitle      string
}
