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

const apiserverCachePolicyComponent = "qs-apiserver"

var apiserverCacheCapabilityPaths = [][]string{
	{"survey", "questionnaire"},
	{"modelcatalog", "published_model"},
	{"evaluation", "assessment_detail"},
	{"actor", "testee"},
	{"plan", "detail"},
	{"statistics", "query"},
}

type CachePolicySource interface {
	Read(context.Context) (*CacheOptions, sharedcache.PolicySource, error)
}

type fileCachePolicySource struct {
	path    string
	runtime app.RuntimeConfigContext
}

func newCachePolicySource(configuredPath string, runtime app.RuntimeConfigContext) (CachePolicySource, error) {
	path, err := policyfile.ResolvePath(runtime.MainConfigFile, configuredPath)
	if err != nil {
		return nil, err
	}
	return &fileCachePolicySource{path: path, runtime: runtime}, nil
}

func (s *fileCachePolicySource) Read(ctx context.Context) (*CacheOptions, sharedcache.PolicySource, error) {
	document, err := policyfile.Load(ctx, policyfile.LoadOptions{
		ConfiguredPath: s.path, ExpectedComponent: apiserverCachePolicyComponent,
		RequiredRoots: []string{"capabilities", "defaults", "governance"},
		Schema:        apiserverCachePolicySchema(), OverridePrefix: "cache", Runtime: s.runtime,
	})
	if err != nil {
		return nil, sharedcache.PolicySource{}, err
	}
	if err := validateApiserverPolicyDocument(document.Settings()); err != nil {
		return nil, sharedcache.PolicySource{}, fmt.Errorf("invalid %s cache policy %q: %w", apiserverCachePolicyComponent, document.Path(), err)
	}
	candidate := NewCacheOptions()
	if err := document.Unmarshal(candidate); err != nil {
		return nil, sharedcache.PolicySource{}, err
	}
	if errs := ValidateCacheOptions(candidate); len(errs) > 0 {
		messages := make([]string, 0, len(errs))
		for _, validationErr := range errs {
			messages = append(messages, validationErr.Error())
		}
		return nil, sharedcache.PolicySource{}, fmt.Errorf("invalid %s cache policy %q: %s", apiserverCachePolicyComponent, document.Path(), strings.Join(messages, "; "))
	}
	candidate.PolicyFile = s.path
	source, err := document.Source(normalizedApiserverCachePolicy(candidate))
	if err != nil {
		return nil, sharedcache.PolicySource{}, err
	}
	return candidate, source, nil
}

func apiserverCachePolicySchema() genericoptions.FieldSchema {
	leaf := genericoptions.FieldSchema(nil)
	policy := genericoptions.FieldSchema{
		"enabled": leaf, "ttl": leaf, "negative_ttl": leaf, "ttl_jitter_ratio": leaf,
		"compress": leaf, "singleflight": leaf, "negative": leaf,
	}
	family := genericoptions.FieldSchema{
		"negative_ttl": leaf, "ttl_jitter_ratio": leaf, "compress": leaf,
		"singleflight": leaf, "negative": leaf,
	}
	return genericoptions.FieldSchema{
		"capabilities": {
			"survey":       {"questionnaire": policy},
			"modelcatalog": {"published_model": policy},
			"evaluation":   {"assessment_detail": policy},
			"actor":        {"testee": policy},
			"plan":         {"detail": policy},
			"statistics":   {"query": policy},
		},
		"defaults": {
			"compress_payload": leaf, "ttl_jitter_ratio": leaf,
			"static": family, "object": family, "query": family,
		},
		"governance": {
			"statistics_warmup": {"enable": leaf, "warm_on_startup": leaf, "org_ids": leaf, "overview_presets": leaf},
			"warmup":            {"enable": leaf, "startup": {"static": leaf, "query": leaf}, "hotset": {"enable": leaf, "top_n": leaf, "max_items_per_kind": leaf}},
		},
	}
}

func validateApiserverPolicyDocument(settings map[string]any) error {
	for _, path := range apiserverCacheCapabilityPaths {
		capability, ok := nestedPolicyMap(settings, append([]string{"capabilities"}, path...)...)
		if !ok {
			return fmt.Errorf("capabilities.%s is required", strings.Join(path, "."))
		}
		if _, ok := lookupPolicyValue(capability, "enabled"); !ok {
			return fmt.Errorf("capabilities.%s.enabled is required", strings.Join(path, "."))
		}
	}
	for _, path := range [][]string{{"defaults", "static"}, {"defaults", "object"}, {"defaults", "query"}, {"governance", "statistics_warmup"}, {"governance", "warmup"}} {
		if _, ok := nestedPolicyMap(settings, path...); !ok {
			return fmt.Errorf("%s is required", strings.Join(path, "."))
		}
	}
	return nil
}

func normalizedApiserverCachePolicy(candidate *CacheOptions) *CacheOptions {
	if candidate == nil {
		return nil
	}
	cloned := *candidate
	cloned.PolicyFile = ""
	return &cloned
}

func nestedPolicyMap(values map[string]any, path ...string) (map[string]any, bool) {
	var current any = values
	for _, item := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = lookupPolicyValue(mapping, item)
		if !ok {
			return nil, false
		}
	}
	result, ok := current.(map[string]any)
	return result, ok
}

func lookupPolicyValue(values map[string]any, key string) (any, bool) {
	if value, ok := values[key]; ok {
		return value, true
	}
	value, ok := values[strings.ReplaceAll(key, "_", "-")]
	return value, ok
}
