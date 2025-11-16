package interpretreport

import (
	"go.mongodb.org/mongo-driver/bson/primitive"

	interpretreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpret-report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/user"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/user/role"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Mapper 解读报告映射器
type Mapper struct{}

// NewMapper 创建映射器
func NewMapper() *Mapper {
	return &Mapper{}
}

// ToEntity 将持久化对象转换为领域对象
func (m *Mapper) ToEntity(po *InterpretReportPO) (*interpretreport.InterpretReport, error) {
	if po == nil {
		return nil, nil
	}

	// 转换解读项
	items := make([]interpretreport.InterpretItem, len(po.InterpretItems))
	for i, itemPO := range po.InterpretItems {
		items[i] = m.interpretItemPOToEntity(itemPO)
	}

	// 创建选项
	var options []interpretreport.InterpretReportOption
	options = append(options, interpretreport.WithID(meta.ID(po.DomainID)))
	options = append(options, interpretreport.WithDescription(po.Description))
	options = append(options, interpretreport.WithInterpretItems(items))

	// 如果有被试者信息
	if po.Testee != nil {
		testee := role.Testee{
			UserID: user.NewUserID(po.Testee.UserID),
		}
		options = append(options, interpretreport.WithTestee(testee))
	}

	// 创建解读报告
	report := interpretreport.NewInterpretReport(
		po.AnswerSheetId,
		po.MedicalScaleCode,
		po.Title,
		options...,
	)

	return report, nil
}

// ToPO 将领域对象转换为持久化对象
func (m *Mapper) ToPO(entity *interpretreport.InterpretReport) (*InterpretReportPO, error) {
	if entity == nil {
		return nil, nil
	}

	// 转换ID
	var objectID primitive.ObjectID
	if !entity.GetID().IsZero() {
		var err error
		objectID, err = base.Uint64ToObjectID(entity.GetID().Uint64())
		if err != nil {
			return nil, err
		}
	}

	// 转换解读项
	items := make([]InterpretItemPO, len(entity.GetInterpretItems()))
	for i, item := range entity.GetInterpretItems() {
		items[i] = m.interpretItemEntityToPO(item)
	}

	// 转换被试者
	var testeePO *TesteePO
	testee := entity.GetTestee()
	if !testee.GetUserID().IsZero() {
		testeePO = &TesteePO{
			UserID: testee.GetUserID().Uint64(),
		}
	}

	po := &InterpretReportPO{
		BaseDocument: base.BaseDocument{
			ID:       objectID,
			DomainID: entity.GetID(),
		},
		AnswerSheetId:    entity.GetAnswerSheetId(),
		MedicalScaleCode: entity.GetMedicalScaleCode(),
		Title:            entity.GetTitle(),
		Description:      entity.GetDescription(),
		Testee:           testeePO,
		InterpretItems:   items,
	}

	return po, nil
}

// interpretItemPOToEntity 将解读项持久化对象转换为领域对象
func (m *Mapper) interpretItemPOToEntity(po InterpretItemPO) interpretreport.InterpretItem {
	return interpretreport.NewInterpretItem(
		po.FactorCode,
		po.Title,
		po.Score,
		po.Content,
	)
}

// interpretItemEntityToPO 将解读项领域对象转换为持久化对象
func (m *Mapper) interpretItemEntityToPO(entity interpretreport.InterpretItem) InterpretItemPO {
	return InterpretItemPO{
		FactorCode: entity.GetFactorCode(),
		Title:      entity.GetTitle(),
		Score:      entity.GetScore(),
		Content:    entity.GetContent(),
	}
}

// ToEntityList 将持久化对象列表转换为领域对象列表
func (m *Mapper) ToEntityList(pos []*InterpretReportPO) ([]*interpretreport.InterpretReport, error) {
	if pos == nil {
		return nil, nil
	}

	entities := make([]*interpretreport.InterpretReport, len(pos))
	for i, po := range pos {
		entity, err := m.ToEntity(po)
		if err != nil {
			return nil, err
		}
		entities[i] = entity
	}

	return entities, nil
}

// ToPOList 将领域对象列表转换为持久化对象列表
func (m *Mapper) ToPOList(entities []*interpretreport.InterpretReport) ([]*InterpretReportPO, error) {
	if entities == nil {
		return nil, nil
	}

	pos := make([]*InterpretReportPO, len(entities))
	for i, entity := range entities {
		po, err := m.ToPO(entity)
		if err != nil {
			return nil, err
		}
		pos[i] = po
	}

	return pos, nil
}
