package medicalscale

import (
	"context"
	"fmt"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale/port"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// Queryer 医学量表查询器
type Queryer struct {
	repo port.Repository
}

// NewQueryer 创建医学量表查询器
func NewQueryer(repo port.Repository) *Queryer {
	return &Queryer{
		repo: repo,
	}
}

// QueryRequest 查询请求
type QueryRequest struct {
	Page              int    `json:"page" form:"page"`
	PageSize          int    `json:"page_size" form:"page_size"`
	Code              string `json:"code" form:"code"`
	Title             string `json:"title" form:"title"`
	QuestionnaireCode string `json:"questionnaire_code" form:"questionnaire_code"`
}

// QueryResponse 查询响应
type QueryResponse struct {
	MedicalScales []*medicalscale.MedicalScale `json:"medical_scales"`
	Total         int64                        `json:"total"`
	Page          int                          `json:"page"`
	PageSize      int                          `json:"page_size"`
}

// Query 查询医学量表列表
func (q *Queryer) Query(ctx context.Context, req *QueryRequest) (*QueryResponse, error) {
	log.L(ctx).Infof("Querying medical scales with request: %+v", req)

	// 设置默认分页参数
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}

	offset := (req.Page - 1) * req.PageSize

	// 根据不同条件查询
	var scales []*medicalscale.MedicalScale
	var total int64
	var err error

	if req.QuestionnaireCode != "" {
		// 根据问卷代码查询
		scales, err = q.repo.FindByQuestionnaireCode(ctx, req.QuestionnaireCode)
		if err != nil {
			return nil, fmt.Errorf("failed to find by questionnaire code: %w", err)
		}
		total = int64(len(scales))

		// 手动分页
		start := offset
		end := offset + req.PageSize
		if start > len(scales) {
			scales = []*medicalscale.MedicalScale{}
		} else {
			if end > len(scales) {
				end = len(scales)
			}
			scales = scales[start:end]
		}
	} else {
		// 查询所有
		scales, total, err = q.repo.FindAll(ctx, offset, req.PageSize)
		if err != nil {
			return nil, fmt.Errorf("failed to find all medical scales: %w", err)
		}
	}

	log.L(ctx).Infof("Found %d medical scales (total: %d)", len(scales), total)

	return &QueryResponse{
		MedicalScales: scales,
		Total:         total,
		Page:          req.Page,
		PageSize:      req.PageSize,
	}, nil
}

// GetByID 根据ID获取医学量表
func (q *Queryer) GetByID(ctx context.Context, id medicalscale.MedicalScaleID) (*medicalscale.MedicalScale, error) {
	log.L(ctx).Infof("Getting medical scale by ID: %s", id.String())

	scale, err := q.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to find medical scale by ID: %w", err)
	}

	log.L(ctx).Infof("Found medical scale: %s", scale.Code())
	return scale, nil
}

// GetByCode 根据代码获取医学量表
func (q *Queryer) GetByCode(ctx context.Context, code string) (*medicalscale.MedicalScale, error) {
	log.L(ctx).Infof("Getting medical scale by code: %s", code)

	if code == "" {
		return nil, fmt.Errorf("code cannot be empty")
	}

	scale, err := q.repo.FindByCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to find medical scale by code: %w", err)
	}

	log.L(ctx).Infof("Found medical scale: %s", scale.Title())
	return scale, nil
}

// GetByQuestionnaireCode 根据问卷代码获取医学量表列表
func (q *Queryer) GetByQuestionnaireCode(ctx context.Context, questionnaireCode string) ([]*medicalscale.MedicalScale, error) {
	log.L(ctx).Infof("Getting medical scales by questionnaire code: %s", questionnaireCode)

	if questionnaireCode == "" {
		return nil, fmt.Errorf("questionnaire code cannot be empty")
	}

	scales, err := q.repo.FindByQuestionnaireCode(ctx, questionnaireCode)
	if err != nil {
		return nil, fmt.Errorf("failed to find medical scales by questionnaire code: %w", err)
	}

	log.L(ctx).Infof("Found %d medical scales for questionnaire: %s", len(scales), questionnaireCode)
	return scales, nil
}

// ExistsByCode 检查代码是否已存在
func (q *Queryer) ExistsByCode(ctx context.Context, code string) (bool, error) {
	log.L(ctx).Infof("Checking if medical scale code exists: %s", code)

	if code == "" {
		return false, fmt.Errorf("code cannot be empty")
	}

	exists, err := q.repo.ExistsByCode(ctx, code)
	if err != nil {
		return false, fmt.Errorf("failed to check if code exists: %w", err)
	}

	log.L(ctx).Infof("Medical scale code %s exists: %v", code, exists)
	return exists, nil
}
