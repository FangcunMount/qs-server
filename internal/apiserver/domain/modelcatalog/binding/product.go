package binding

import "fmt"

// Product is the normalized product taxonomy and does not select a runtime family.
type Product string

const (
	ProductMedicalScale    Product = Product(ProductChannelMedicalScale)
	ProductTypology        Product = Product(ProductChannelTypology)
	ProductBehaviorAbility Product = Product(ProductChannelBehaviorAbility)
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

// ProductFromChannel normalizes a persisted product channel to the product taxonomy.
func ProductFromChannel(channel ProductChannel) (Product, error) {
	product := Product(NormalizeProductChannel(channel))
	if !product.IsValid() {
		return "", fmt.Errorf("%w: product %q is invalid", ErrInvalidArgument, channel)
	}
	return product, nil
}

func (p Product) Channel() ProductChannel { return ProductChannel(p) }

// ProductChannelForIdentity resolves an explicit persisted channel or the model-family default.
func ProductChannelForIdentity(kind Kind, explicitChannel string) string {
	if explicitChannel != "" {
		return string(NormalizeProductChannel(ProductChannel(explicitChannel)))
	}
	if kind == "" {
		return ""
	}
	return string(DefaultProductChannelFor(kind))
}
