package actorreadmodel

import (
	"context"
	"time"
)

type TesteeFilter struct {
	OrgID                 int64
	Name                  string
	Tags                  []string
	KeyFocus              *bool
	CreatedAtStart        *time.Time
	CreatedAtEnd          *time.Time
	AccessibleTesteeIDs   []uint64
	RestrictToAccessScope bool
	Offset                int
	Limit                 int
}

type TesteeRow struct {
	ID               uint64
	OrgID            int64
	ProfileID        *uint64
	Name             string
	Gender           int8
	Birthday         *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Tags             []string
	Source           string
	IsKeyFocus       bool
	LastAssessmentAt *time.Time
	TotalAssessments int
	LastRiskLevel    string
}

type TesteeReader interface {
	GetTestee(ctx context.Context, id uint64) (*TesteeRow, error)
	FindTesteeByProfile(ctx context.Context, orgID int64, profileID uint64) (*TesteeRow, error)
	ListTestees(ctx context.Context, filter TesteeFilter) ([]TesteeRow, error)
	CountTestees(ctx context.Context, filter TesteeFilter) (int64, error)
	ListTesteesByProfileIDs(ctx context.Context, profileIDs []uint64, offset, limit int) ([]TesteeRow, error)
	CountTesteesByProfileIDs(ctx context.Context, profileIDs []uint64) (int64, error)
}

type OperatorFilter struct {
	OrgID  int64
	Role   string
	Offset int
	Limit  int
}

type OperatorRow struct {
	ID       uint64
	OrgID    int64
	UserID   int64
	Roles    []string
	Name     string
	Email    string
	Phone    string
	IsActive bool
}

type OperatorReader interface {
	GetOperator(ctx context.Context, id uint64) (*OperatorRow, error)
	FindOperatorByUser(ctx context.Context, orgID int64, userID int64) (*OperatorRow, error)
	ListOperators(ctx context.Context, filter OperatorFilter) ([]OperatorRow, error)
	CountOperators(ctx context.Context, orgID int64) (int64, error)
}

type ClinicianFilter struct {
	OrgID  int64
	Offset int
	Limit  int
}

type ClinicianRow struct {
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

type ClinicianReader interface {
	GetClinician(ctx context.Context, id uint64) (*ClinicianRow, error)
	FindClinicianByOperator(ctx context.Context, orgID int64, operatorID uint64) (*ClinicianRow, error)
	ListClinicians(ctx context.Context, filter ClinicianFilter) ([]ClinicianRow, error)
	CountClinicians(ctx context.Context, orgID int64) (int64, error)
}

type RelationFilter struct {
	OrgID         int64
	ClinicianID   uint64
	TesteeID      uint64
	RelationTypes []string
	ActiveOnly    bool
	Offset        int
	Limit         int
}

type RelationRow struct {
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

type TesteeRelationRow struct {
	Relation  RelationRow
	Clinician ClinicianRow
}

type ClinicianRelationRow struct {
	Relation RelationRow
	Testee   TesteeRow
}

type RelationReader interface {
	ListAssignedTestees(ctx context.Context, filter RelationFilter) ([]TesteeRow, int64, error)
	ListActiveTesteeIDsByClinician(ctx context.Context, orgID int64, clinicianID uint64, relationTypes []string) ([]uint64, error)
	ListTesteeRelations(ctx context.Context, filter RelationFilter) ([]TesteeRelationRow, error)
	ListClinicianRelations(ctx context.Context, filter RelationFilter) ([]ClinicianRelationRow, int64, error)
	HasActiveRelationForTestee(ctx context.Context, orgID int64, clinicianID, testeeID uint64, relationTypes []string) (bool, error)
}

type AssessmentEntryFilter struct {
	OrgID       int64
	ClinicianID uint64
	Offset      int
	Limit       int
}

type AssessmentEntryRow struct {
	ID            uint64
	OrgID         int64
	ClinicianID   uint64
	Token         string
	TargetType    string
	TargetCode    string
	TargetVersion string
	IsActive      bool
	ExpiresAt     *time.Time
}

type AssessmentEntryReader interface {
	ListAssessmentEntriesByClinician(ctx context.Context, filter AssessmentEntryFilter) ([]AssessmentEntryRow, error)
	CountAssessmentEntriesByClinician(ctx context.Context, orgID int64, clinicianID uint64) (int64, error)
	GetAssessmentEntryTitle(ctx context.Context, id uint64) (string, error)
}

type ReadModel interface {
	TesteeReader
	OperatorReader
	ClinicianReader
	RelationReader
	AssessmentEntryReader
}
