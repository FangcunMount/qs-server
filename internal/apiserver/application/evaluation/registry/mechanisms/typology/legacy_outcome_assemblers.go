package typology

import (
	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// RegisterLegacyOutcomeAdapters 返回注册表副本 使用 仅用于表征 旧版 结果 adapters。
func RegisterLegacyOutcomeAdapters(registry OutcomeAdapterRegistry) OutcomeAdapterRegistry {
	return registry.
		Register(modeltypology.DetailAdapterMBTI, typologylegacy.AssemblePersonalityTypeFromMBTI).
		Register(modeltypology.DetailAdapterSBTI, typologylegacy.AssemblePersonalityTypeFromSBTI).
		Register(modeltypology.DetailAdapterBigFive, typologylegacy.AssembleTraitProfileFromBigFive)
}
