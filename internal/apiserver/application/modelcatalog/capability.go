package modelcatalog

import catalogoption "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/option"

func registeredOptionForAPIKind(apiKind string) (catalogoption.RegisteredOption, bool) {
	return catalogRegistry.ByAPIKind(apiKind)
}

func shouldListModelKind(filterKind, apiKind string) bool {
	if filterKind != "" && normalizeAPIKind(filterKind) != normalizeAPIKind(apiKind) {
		return false
	}
	entry, ok := registeredOptionForAPIKind(apiKind)
	return ok && entry.Operations.ListSupported
}
