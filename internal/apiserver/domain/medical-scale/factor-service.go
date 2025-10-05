package medicalscale

import (
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/medical-scale/factor"
	errCode "github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
)

// FactorService 因子服务
type FactorService struct{}

// AddFactor 添加因子
func (FactorService) AddFactor(m *MedicalScale, newFactor factor.Factor) error {
	m.factors = append(m.factors, newFactor)
	return nil
}

// UpdateFactor 更新因子
func (FactorService) UpdateFactor(m *MedicalScale, updatedFactor factor.Factor) error {
	for i := range m.factors {
		if m.factors[i].GetCode() == updatedFactor.GetCode() {
			m.factors[i] = updatedFactor
			return nil
		}
	}
	return errors.WithCode(errCode.ErrMedicalScaleFactorNotFound, "找不到该因子")
}

// DeleteFactor 删除因子
func (FactorService) DeleteFactor(m *MedicalScale, factorCode string) error {
	for i := range m.factors {
		if m.factors[i].GetCode() == factorCode {
			m.factors = append(m.factors[:i], m.factors[i+1:]...)
			return nil
		}
	}
	return errors.WithCode(errCode.ErrMedicalScaleFactorNotFound, "找不到该因子")
}

// RemoveAllFactors 清除所有因子
func (FactorService) RemoveAllFactors(m *MedicalScale) {
	m.factors = make([]factor.Factor, 0)
}
