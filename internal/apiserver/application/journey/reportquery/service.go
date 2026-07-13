// Package reportquery preserves the cross-module Assessment status projection
// while delegating all authorized report reads to the administration actor.
package reportquery

import (
	"context"
	"errors"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	interpretationAdmin "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/administration"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// AssessmentProjection 评估投影
type AssessmentProjection struct {
	Assessment    *evaluationoperator.Assessment // 评估
	Status        string                         // 状态
	InterpretedAt *time.Time                     // 解读时间
}

// AssessmentListProjection 评估列表投影
type AssessmentListProjection struct {
	Items                             []*AssessmentProjection // 评估列表
	Total, Page, PageSize, TotalPages int                     // 总页数
}

// Scope 范围
type Scope struct{ OrgID, OperatorUserID int64 }

// ListQuery 列表查询
type ListQuery = interpretationAdmin.ListQuery

// Report 报告
type Report = interpretationAdmin.Report

// ReportList 报告列表
type ReportList = interpretationAdmin.ListResult

// AssessmentQuery 评估查询
type AssessmentQuery interface {
	// GetAssessment 获取评估
	GetAssessment(context.Context, evaluationoperator.Actor, uint64) (*evaluationoperator.Assessment, error)
	// ListAssessments 获取评估列表
	ListAssessments(context.Context, evaluationoperator.Actor, evaluationoperator.ListQuery) (*evaluationoperator.AssessmentList, error)
}

// Service 服务
type Service interface {
	// GetAssessmentProjection 获取评估投影
	GetAssessmentProjection(context.Context, Scope, uint64) (*AssessmentProjection, error)
	// ListAssessmentProjection 获取评估列表投影
	ListAssessmentProjection(context.Context, Scope, evaluationoperator.ListQuery) (*AssessmentListProjection, error)
	// ProjectAssessment 投影评估
	ProjectAssessment(context.Context, *evaluationoperator.Assessment) (*AssessmentProjection, error)
	// GetReport 获取报告
	GetReport(context.Context, Scope, uint64) (*Report, error)
	// ListReports 获取报告列表
	ListReports(context.Context, Scope, ListQuery) (*ReportList, error)
}

// service 服务实现
type service struct {
	reader   interpretationreadmodel.ReportReader
	admin    interpretationAdmin.Service
	operator AssessmentQuery
}

// NewAdministrationService 创建服务
func NewAdministrationService(reader interpretationreadmodel.ReportReader, admin interpretationAdmin.Service, operator AssessmentQuery) Service {
	return &service{reader: reader, admin: admin, operator: operator}
}

// GetAssessmentProjection 获取评估投影
func (s *service) GetAssessmentProjection(ctx context.Context, scope Scope, id uint64) (*AssessmentProjection, error) {
	result, err := s.operator.GetAssessment(ctx, evaluationoperator.Actor{OrgID: scope.OrgID, OperatorUserID: scope.OperatorUserID}, id)
	if err != nil {
		return nil, err
	}
	return s.ProjectAssessment(ctx, result)
}

// ListAssessmentProjection 获取评估列表投影
func (s *service) ListAssessmentProjection(ctx context.Context, scope Scope, query evaluationoperator.ListQuery) (*AssessmentListProjection, error) {
	result, err := s.operator.ListAssessments(ctx, evaluationoperator.Actor{OrgID: scope.OrgID, OperatorUserID: scope.OperatorUserID}, query)
	if err != nil {
		return nil, err
	}
	out := &AssessmentListProjection{Items: make([]*AssessmentProjection, 0, len(result.Items)), Total: result.Total, Page: result.Page, PageSize: result.PageSize, TotalPages: result.TotalPages}
	for _, item := range result.Items {
		projected, projectErr := s.ProjectAssessment(ctx, item)
		if projectErr != nil {
			return nil, projectErr
		}
		out.Items = append(out.Items, projected)
	}
	return out, nil
}

// ProjectAssessment 投影评估
func (s *service) ProjectAssessment(ctx context.Context, result *evaluationoperator.Assessment) (*AssessmentProjection, error) {
	p := &AssessmentProjection{Assessment: result}
	if result != nil {
		p.Status = result.Status
	}
	if result == nil || result.Status != "evaluated" || s.reader == nil {
		return p, nil
	}
	row, err := s.reader.GetReportByAssessmentID(ctx, result.ID)
	if err != nil {
		if errors.Is(err, interpretationreadmodel.ErrReportNotFound) || cberrors.IsCode(err, code.ErrInterpretReportNotFound) {
			return p, nil
		}
		return nil, err
	}
	if row != nil {
		p.Status = "interpreted"
		at := row.CreatedAt
		p.InterpretedAt = &at
	}
	return p, nil
}

// GetReport 获取报告
func (s *service) GetReport(ctx context.Context, scope Scope, id uint64) (*Report, error) {
	return s.admin.GetReport(ctx, actor(scope), interpretationAdmin.GetQuery{AssessmentID: id})
}

// ListReports 获取报告列表
func (s *service) ListReports(ctx context.Context, scope Scope, q ListQuery) (*ReportList, error) {
	return s.admin.ListReports(ctx, actor(scope), q)
}

// actor 获取管理员
func actor(s Scope) interpretationAdmin.Actor {
	return interpretationAdmin.Actor{OrgID: s.OrgID, OperatorUserID: s.OperatorUserID}
}
