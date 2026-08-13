package options

import (
	"context"
	"fmt"
	"strings"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/cache/policyfile"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/pkg/app"
)

const collectionCachePolicyComponent = "collection-server"

var collectionCacheCapabilityNames = []string{"questionnaire", "published_model", "typology"}

func loadCollectionCachePolicy(ctx context.Context, configuredPath string, runtime app.RuntimeConfigContext) (*CacheOptions, sharedcache.PolicySource, error) {
	document, err := policyfile.Load(ctx, policyfile.LoadOptions{
		ConfiguredPath: configuredPath, ExpectedComponent: collectionCachePolicyComponent,
		RequiredRoots: []string{"capabilities"}, Schema: collectionCachePolicySchema(),
		OverridePrefix: "cache", Runtime: runtime,
	})
	if err != nil {
		return nil, sharedcache.PolicySource{}, err
	}
	if err := validateCollectionPolicyDocument(document.Settings()); err != nil {
		return nil, sharedcache.PolicySource{}, fmt.Errorf("invalid %s cache policy %q: %w", collectionCachePolicyComponent, document.Path(), err)
	}
	candidate := NewCacheOptions()
	if err := document.Unmarshal(candidate); err != nil {
		return nil, sharedcache.PolicySource{}, err
	}
	ensureCollectionCacheCapabilities(candidate)
	var errs []error
	errs = append(errs, validateQuestionnaireCacheOptions(candidate.Capabilities.Catalog.Questionnaire)...)
	errs = append(errs, validatePublishedModelCacheOptions(candidate.Capabilities.Catalog.PublishedModel)...)
	errs = append(errs, validateTypologyCacheOptions(candidate.Capabilities.Catalog.Typology)...)
	if len(errs) > 0 {
		messages := make([]string, 0, len(errs))
		for _, validationErr := range errs {
			messages = append(messages, validationErr.Error())
		}
		return nil, sharedcache.PolicySource{}, fmt.Errorf("invalid %s cache policy %q: %s", collectionCachePolicyComponent, document.Path(), strings.Join(messages, "; "))
	}
	candidate.PolicyFile = document.Path()
	source, err := document.Source(normalizedCollectionCachePolicy(candidate))
	if err != nil {
		return nil, sharedcache.PolicySource{}, err
	}
	return candidate, source, nil
}

func collectionCachePolicySchema() genericoptions.FieldSchema {
	leaf := genericoptions.FieldSchema(nil)
	catalog := genericoptions.FieldSchema{
		"enabled": leaf, "ttl_seconds": leaf, "ttl_jitter_ratio": leaf,
		"max_entries": leaf, "singleflight": leaf, "signal_evict_enabled": leaf,
	}
	return genericoptions.FieldSchema{
		"capabilities": {"catalog": {
			"questionnaire": catalog, "published_model": catalog, "typology": catalog,
		}},
	}
}

func validateCollectionPolicyDocument(settings map[string]any) error {
	for _, name := range collectionCacheCapabilityNames {
		capability, ok := nestedCollectionPolicyMap(settings, "capabilities", "catalog", name)
		if !ok {
			return fmt.Errorf("capabilities.catalog.%s is required", name)
		}
		for _, field := range []string{"enabled", "ttl_seconds", "ttl_jitter_ratio", "max_entries", "singleflight", "signal_evict_enabled"} {
			if _, ok := lookupCollectionPolicyValue(capability, field); !ok {
				return fmt.Errorf("capabilities.catalog.%s.%s is required", name, field)
			}
		}
	}
	return nil
}

func ensureCollectionCacheCapabilities(cache *CacheOptions) {
	defaults := NewCacheOptions()
	if cache.Capabilities == nil {
		cache.Capabilities = defaults.Capabilities
	}
	if cache.Capabilities.Catalog == nil {
		cache.Capabilities.Catalog = defaults.Capabilities.Catalog
	}
	if cache.Capabilities.Catalog.Questionnaire == nil {
		cache.Capabilities.Catalog.Questionnaire = defaults.Capabilities.Catalog.Questionnaire
	}
	if cache.Capabilities.Catalog.PublishedModel == nil {
		cache.Capabilities.Catalog.PublishedModel = defaults.Capabilities.Catalog.PublishedModel
	}
	if cache.Capabilities.Catalog.Typology == nil {
		cache.Capabilities.Catalog.Typology = defaults.Capabilities.Catalog.Typology
	}
}

func normalizedCollectionCachePolicy(candidate *CacheOptions) *CacheOptions {
	if candidate == nil {
		return nil
	}
	cloned := *candidate
	cloned.PolicyFile = ""
	return &cloned
}

func nestedCollectionPolicyMap(values map[string]any, path ...string) (map[string]any, bool) {
	var current any = values
	for _, item := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = lookupCollectionPolicyValue(mapping, item)
		if !ok {
			return nil, false
		}
	}
	result, ok := current.(map[string]any)
	return result, ok
}

func lookupCollectionPolicyValue(values map[string]any, key string) (any, bool) {
	if value, ok := values[key]; ok {
		return value, true
	}
	value, ok := values[strings.ReplaceAll(key, "_", "-")]
	return value, ok
}
