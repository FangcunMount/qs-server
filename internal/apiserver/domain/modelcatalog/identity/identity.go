package identity

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// Product 是面向产品的测评分类，不参与运行时执行家族选择。
type Product string

const (
	ProductMedicalScale    Product = Product(binding.ProductChannelMedicalScale)
	ProductTypology        Product = Product(binding.ProductChannelTypology)
	ProductBehaviorAbility Product = Product(binding.ProductChannelBehaviorAbility)
)

func (p Product) String() string { return string(p) }

func (p Product) IsValid() bool {
	switch p {
	case ProductMedicalScale, ProductTypology, ProductBehaviorAbility:
		return true
	default:
		return false
	}
}

// ProductFromChannel normalizes legacy persisted channel values into the
// target three-product taxonomy.
func ProductFromChannel(channel binding.ProductChannel) (Product, error) {
	normalized := binding.NormalizeProductChannel(channel)
	product := Product(normalized)
	if !product.IsValid() {
		return "", fmt.Errorf("%w: product %q is invalid", binding.ErrInvalidArgument, channel)
	}
	return product, nil
}

func (p Product) Channel() binding.ProductChannel {
	return binding.ProductChannel(p)
}

// Identity 是测评模型的算法身份，不表达产品概念。
type Identity struct {
	Kind      binding.Kind
	SubKind   binding.SubKind
	Algorithm binding.Algorithm
}

func New(kind binding.Kind, subKind binding.SubKind, algorithm binding.Algorithm) Identity {
	return Identity{Kind: kind, SubKind: subKind, Algorithm: algorithm}
}

func (i Identity) IsZero() bool {
	return i.Kind == "" && i.SubKind == "" && i.Algorithm == ""
}

func (i Identity) Family() (Family, bool) {
	return FamilyFromIdentity(i)
}

func (i Identity) DecisionKind() (binding.DecisionKind, bool) {
	return DecisionKindForIdentity(i.Kind, i.SubKind, i.Algorithm)
}

// Family 是运行执行机制家族，始终由 Identity 或 DecisionKind 派生。
type AlgorithmFamily string

const (
	AlgorithmFamilyFactorScoring        AlgorithmFamily = "factor_scoring"
	AlgorithmFamilyFactorClassification AlgorithmFamily = "factor_classification"
	AlgorithmFamilyFactorNorm           AlgorithmFamily = "factor_norm"
	AlgorithmFamilyTaskPerformance      AlgorithmFamily = "task_performance"
)

func (f AlgorithmFamily) String() string { return string(f) }

func (f AlgorithmFamily) IsValid() bool {
	switch f {
	case AlgorithmFamilyFactorScoring,
		AlgorithmFamilyFactorClassification,
		AlgorithmFamilyFactorNorm,
		AlgorithmFamilyTaskPerformance:
		return true
	default:
		return false
	}
}

type Family = AlgorithmFamily

const (
	FamilyFactorScoring        Family = AlgorithmFamilyFactorScoring
	FamilyFactorClassification Family = AlgorithmFamilyFactorClassification
	FamilyFactorNorm           Family = AlgorithmFamilyFactorNorm
	FamilyTaskPerformance      Family = AlgorithmFamilyTaskPerformance
)

func FamilyFromDecisionKind(decision binding.DecisionKind) (Family, bool) {
	return AlgorithmFamilyFromDecisionKind(decision)
}

func FamilyFromIdentity(identity Identity) (Family, bool) {
	return AlgorithmFamilyFromIdentity(identity.Kind, identity.SubKind, identity.Algorithm)
}

// AlgorithmFamilyFromDecisionKind 映射 published 判定策略到执行家族。
func AlgorithmFamilyFromDecisionKind(decision binding.DecisionKind) (AlgorithmFamily, bool) {
	switch decision {
	case binding.DecisionKindScoreRange, binding.DecisionKindScoreRangeInterpretation:
		return AlgorithmFamilyFactorScoring, true
	case binding.DecisionKindPoleComposition, binding.DecisionKindTraitProfile, binding.DecisionKindNearestPattern:
		return AlgorithmFamilyFactorClassification, true
	case binding.DecisionKindNormLookup:
		return AlgorithmFamilyFactorNorm, true
	case binding.DecisionKindAbilityLevel:
		return AlgorithmFamilyTaskPerformance, true
	default:
		return "", false
	}
}

// DecisionKindForIdentity mirrors publish-builder decision selection for non-typology draft binding.
// Personality typology requires explicit decision.kind in payload; no algorithm fallback.
// Cognitive/projection currently uses task_performance as the implementation family.
func DecisionKindForIdentity(kind binding.Kind, subKind binding.SubKind, algorithm binding.Algorithm) (binding.DecisionKind, bool) {
	switch kind {
	case binding.KindScale:
		return binding.DecisionKindScoreRange, true
	case binding.KindTypology:
		if subKind != binding.SubKindTypology {
			return "", false
		}
		return "", false
	case binding.KindBehavioralRating:
		return binding.DecisionKindNormLookup, true
	case binding.KindCognitive:
		return binding.DecisionKindAbilityLevel, true
	case binding.KindCustom:
		return "", false
	default:
		return "", false
	}
}

// AlgorithmFamilyFromIdentity 推导执行家族 from draft model binding.
func AlgorithmFamilyFromIdentity(kind binding.Kind, subKind binding.SubKind, algorithm binding.Algorithm) (AlgorithmFamily, bool) {
	if binding.IsTypologyKind(kind) && subKind == binding.SubKindTypology {
		return AlgorithmFamilyFactorClassification, true
	}
	decision, ok := DecisionKindForIdentity(kind, subKind, algorithm)
	if !ok {
		return "", false
	}
	return AlgorithmFamilyFromDecisionKind(decision)
}

// AllAlgorithmFamilies 返回 supported 算法家族 values 用于 API 选项。
func AllAlgorithmFamilies() []AlgorithmFamily {
	return []AlgorithmFamily{
		AlgorithmFamilyFactorScoring,
		AlgorithmFamilyFactorClassification,
		AlgorithmFamilyFactorNorm,
		AlgorithmFamilyTaskPerformance,
	}
}

// ProductChannelForIdentity resolves the product channel from an explicit snapshot value or kind fallback.
func ProductChannelForIdentity(kind binding.Kind, explicitChannel string) string {
	if explicitChannel != "" {
		return string(binding.NormalizeProductChannel(binding.ProductChannel(explicitChannel)))
	}
	if kind == "" {
		return ""
	}
	return string(binding.DefaultProductChannelFor(kind))
}

// AlgorithmFamilyStringFromIdentity derives the algorithm family string from model identity fields.
func AlgorithmFamilyStringFromIdentity(kind binding.Kind, subKind binding.SubKind, algorithm binding.Algorithm) string {
	if kind == "" {
		return ""
	}
	family, ok := AlgorithmFamilyFromIdentity(kind, subKind, algorithm)
	if !ok {
		return ""
	}
	return string(family)
}
