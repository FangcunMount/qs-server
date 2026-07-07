package configured

import (
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/patterns"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// RegisterLegacyDetailAssemblers 返回注册表副本 使用 仅用于表征 旧明细适配器。
func RegisterLegacyDetailAssemblers(registry DetailAssemblerRegistry) DetailAssemblerRegistry {
	return registry.
		Register(modeltypology.DetailAdapterMBTI, assembleMBTIDetail).
		Register(modeltypology.DetailAdapterSBTI, assembleSBTIDetail).
		Register(modeltypology.DetailAdapterBigFive, assembleBigFiveDetail)
}

func assembleMBTIDetail(input DetailInput) (any, error) {
	generic, err := assemblePersonalityTypeDetail(input)
	if err != nil {
		return nil, err
	}
	return evaluationtypology.MBTIResultDetailFromPersonalityType(generic.(evaluationtypology.PersonalityTypeDetail)), nil
}

func assembleSBTIDetail(input DetailInput) (any, error) {
	generic, err := assemblePersonalityTypeDetail(input)
	if err != nil {
		return nil, err
	}
	return evaluationtypology.SBTIResultDetailFromPersonalityType(generic.(evaluationtypology.PersonalityTypeDetail)), nil
}

func assembleBigFiveDetail(input DetailInput) (any, error) {
	generic, err := assembleTraitProfileDetail(input)
	if err != nil {
		return nil, err
	}
	return evaluationtypology.BigFiveResultDetailFromTraitProfile(generic.(evaluationtypology.TraitProfileDetail)), nil
}
