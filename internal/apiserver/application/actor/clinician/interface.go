package clinician

import (
	"context"
	"time"
)

// ClinicianLifecycleService 从业者生命周期服务。
type ClinicianLifecycleService interface {
	Register(ctx context.Context, dto RegisterClinicianDTO) (*ClinicianResult, error)
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
	ListAssignedTestees(ctx context.Context, dto ListAssignedTesteeDTO) (*AssignedTesteeListResult, error)
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

// ListClinicianDTO 从业者列表查询。
type ListClinicianDTO struct {
	OrgID  int64
	Offset int
	Limit  int
}

// ClinicianResult 从业者结果。
type ClinicianResult struct {
	ID            uint64
	OrgID         int64
	OperatorID    *uint64
	Name          string
	Department    string
	Title         string
	ClinicianType string
	EmployeeCode  string
	IsActive      bool
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
