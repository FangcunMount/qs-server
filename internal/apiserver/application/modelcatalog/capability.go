package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

func capabilityForAPIKind(apiKind string) (domain.KindCapability, bool) {
	domainKind, ok := APIKindToDomainKind(apiKind)
	if !ok {
		return domain.KindCapability{}, false
	}
	return domain.CapabilityByKind(domainKind)
}

func apiKindOptions() []Option {
	caps := domain.DefaultCapabilities()
	options := make([]Option, 0, len(caps))
	for _, cap := range caps {
		apiKind := cap.APIKind
		if apiKind == "" {
			apiKind = DomainKindToAPIKind(cap.Kind)
		}
		if apiKind == "" {
			continue
		}
		options = append(options, Option{
			Label:    cap.DisplayName,
			Value:    apiKind,
			Disabled: !cap.OptionsEnabled,
		})
	}
	return options
}

func shouldListModelKind(filterKind, apiKind string) bool {
	if filterKind != "" && filterKind != apiKind {
		return false
	}
	cap, ok := capabilityForAPIKind(apiKind)
	return ok && cap.ListSupported
}
