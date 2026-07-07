package configured

import (
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// RegisterLegacyDetailAssemblers returns a registry copy with characterization-only legacy detail adapters.
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
