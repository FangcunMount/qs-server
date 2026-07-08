package legacy

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/runtime/configured"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

// RegisterLegacyDetailAssemblers 返回注册表副本 使用 仅用于表征 旧明细适配器。
func RegisterLegacyDetailAssemblers(registry configured.DetailAssemblerRegistry) configured.DetailAssemblerRegistry {
	return registry.
		Register(modeltypology.DetailAdapterMBTI, assembleMBTIDetail).
		Register(modeltypology.DetailAdapterSBTI, assembleSBTIDetail).
		Register(modeltypology.DetailAdapterBigFive, assembleBigFiveDetail)
}

func assembleMBTIDetail(input configured.DetailInput) (any, error) {
	generic, err := configured.AssemblePersonalityTypeDetail(input)
	if err != nil {
		return nil, err
	}
	return MBTIResultDetailFromPersonalityType(generic), nil
}

func assembleSBTIDetail(input configured.DetailInput) (any, error) {
	generic, err := configured.AssemblePersonalityTypeDetail(input)
	if err != nil {
		return nil, err
	}
	return SBTIResultDetailFromPersonalityType(generic), nil
}

func assembleBigFiveDetail(input configured.DetailInput) (any, error) {
	generic, err := configured.AssembleTraitProfileDetail(input)
	if err != nil {
		return nil, err
	}
	return BigFiveResultDetailFromTraitProfile(generic), nil
}
