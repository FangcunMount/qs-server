package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

func capabilityForAPIKind(apiKind string) (domain.KindCapability, bool) {
	domainKind, ok := APIKindToDomainKind(apiKind)
	if !ok {
		return domain.KindCapability{}, false
	}
	return domain.CapabilityByKind(domainKind)
}

func shouldListModelKind(filterKind, apiKind string) bool {
	if filterKind != "" && filterKind != apiKind {
		return false
	}
	cap, ok := capabilityForAPIKind(apiKind)
	return ok && cap.ListSupported
}
