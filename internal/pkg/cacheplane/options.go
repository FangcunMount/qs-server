package cacheplane

import (
	"fmt"
	"slices"

	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

// CatalogFromOptions merges shared runtime options with component defaults.
func CatalogFromOptions(runtimeOpts *genericoptions.RedisRuntimeOptions, defaults map[Family]Route) *Catalog {
	rootNamespace := ""
	if runtimeOpts != nil {
		rootNamespace = runtimeOpts.Namespace
	}

	routes := make(map[Family]Route, len(defaults))
	for family, route := range defaults {
		routes[family] = route
	}

	if runtimeOpts != nil {
		for rawFamily, override := range runtimeOpts.Families {
			family := Family(rawFamily)
			route := routes[family]
			if override == nil {
				routes[family] = route
				continue
			}
			if override.RedisProfile != "" {
				route.RedisProfile = override.RedisProfile
			}
			if override.NamespaceSuffix != "" {
				route.NamespaceSuffix = override.NamespaceSuffix
			}
			if override.AllowFallbackDefault != nil {
				route.AllowFallbackDefault = *override.AllowFallbackDefault
			}
			if override.AllowWarmup != nil {
				route.AllowWarmup = *override.AllowWarmup
			}
			routes[family] = route
		}
	}

	return NewCatalog(rootNamespace, routes)
}

// ValidateRuntimeOptions validates family names and referenced redis profiles.
func ValidateRuntimeOptions(
	runtimeOpts *genericoptions.RedisRuntimeOptions,
	knownFamilies []Family,
	profiles map[string]*genericoptions.RedisOptions,
	fieldPrefix string,
) []error {
	if runtimeOpts == nil {
		return nil
	}

	var errs []error
	for rawFamily, route := range runtimeOpts.Families {
		family := Family(rawFamily)
		if !slices.Contains(knownFamilies, family) {
			errs = append(errs, fmt.Errorf("%s.families.%s is not a supported redis family", fieldPrefix, rawFamily))
			continue
		}
		if route == nil || route.RedisProfile == "" {
			continue
		}
		if route.AllowFallbackDefault != nil && *route.AllowFallbackDefault {
			continue
		}
		if _, ok := profiles[route.RedisProfile]; !ok {
			errs = append(errs, fmt.Errorf("%s.families.%s.redis_profile references missing redis_profiles entry %q", fieldPrefix, rawFamily, route.RedisProfile))
		}
	}
	return errs
}
