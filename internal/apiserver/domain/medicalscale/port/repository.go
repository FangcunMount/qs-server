package port

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medicalscale"
)

// Repository 医学量表仓储接口
type Repository interface {
	// Save 保存医学量表
	Save(ctx context.Context, scale *medicalscale.MedicalScale) error

	// FindByID 根据ID查找医学量表
	FindByID(ctx context.Context, id medicalscale.MedicalScaleID) (*medicalscale.MedicalScale, error)

	// FindByCode 根据代码查找医学量表
	FindByCode(ctx context.Context, code string) (*medicalscale.MedicalScale, error)

	// FindByQuestionnaireCode 根据问卷代码查找医学量表列表
	FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) ([]*medicalscale.MedicalScale, error)

	// FindAll 查找所有医学量表（支持分页）
	FindAll(ctx context.Context, offset, limit int) ([]*medicalscale.MedicalScale, int64, error)

	// Update 更新医学量表
	Update(ctx context.Context, scale *medicalscale.MedicalScale) error

	// Delete 删除医学量表
	Delete(ctx context.Context, id medicalscale.MedicalScaleID) error

	// ExistsByCode 检查代码是否已存在
	ExistsByCode(ctx context.Context, code string) (bool, error)

	// ExistsByQuestionnaireBinding 检查问卷绑定是否已存在
	ExistsByQuestionnaireBinding(ctx context.Context, questionnaireCode, questionnaireVersion string) (bool, error)
}
