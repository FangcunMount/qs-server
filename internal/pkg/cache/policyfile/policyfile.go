package policyfile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/pkg/app"
	"github.com/spf13/viper"
)

const SchemaVersion = "1.0"

type LoadOptions struct {
	ConfiguredPath    string
	ExpectedComponent string
	RequiredRoots     []string
	Schema            genericoptions.FieldSchema
	OverridePrefix    string
	Runtime           app.RuntimeConfigContext
}

// Document is one validated cache policy file with runtime overrides applied.
type Document struct {
	path      string
	component string
	settings  map[string]any
}

func Load(ctx context.Context, opts LoadOptions) (*Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := ResolvePath(opts.Runtime.MainConfigFile, opts.ConfiguredPath)
	if err != nil {
		return nil, err
	}
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read %s cache policy %q: %w", opts.ExpectedComponent, path, err)
	}
	settings := v.AllSettings()
	if err := validateEnvelope(settings, opts.ExpectedComponent, opts.RequiredRoots, opts.Schema); err != nil {
		return nil, fmt.Errorf("invalid %s cache policy %q: %w", opts.ExpectedComponent, path, err)
	}
	applyOverrides(v, opts.Runtime, opts.OverridePrefix, opts.Schema.LeafPaths())
	return &Document{path: path, component: opts.ExpectedComponent, settings: v.AllSettings()}, nil
}

func ResolvePath(mainConfigFile, configuredPath string) (string, error) {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath == "" {
		return "", fmt.Errorf("cache.policy_file is required")
	}
	path := configuredPath
	if !filepath.IsAbs(path) {
		if strings.TrimSpace(mainConfigFile) == "" {
			return "", fmt.Errorf("main configuration path is required to resolve cache.policy_file")
		}
		path = filepath.Join(filepath.Dir(mainConfigFile), path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve cache policy path %q: %w", configuredPath, err)
	}
	return filepath.Clean(absolute), nil
}

func (d *Document) Unmarshal(target any) error {
	if d == nil {
		return fmt.Errorf("cache policy document is nil")
	}
	v := viper.New()
	if err := v.MergeConfigMap(d.settings); err != nil {
		return err
	}
	if err := v.Unmarshal(target); err != nil {
		return fmt.Errorf("decode %s cache policy %q: %w", d.component, d.path, err)
	}
	return nil
}

func (d *Document) Source(normalizedPolicy any) (sharedcache.PolicySource, error) {
	if d == nil {
		return sharedcache.PolicySource{}, fmt.Errorf("cache policy document is nil")
	}
	canonical, err := json.Marshal(normalizedPolicy)
	if err != nil {
		return sharedcache.PolicySource{}, fmt.Errorf("encode normalized %s cache policy: %w", d.component, err)
	}
	digest := sha256.Sum256(canonical)
	return sharedcache.PolicySource{
		Component: d.component, SchemaVersion: SchemaVersion, Path: d.path,
		PolicySHA256: hex.EncodeToString(digest[:]),
	}, nil
}

func (d *Document) Path() string {
	if d == nil {
		return ""
	}
	return d.path
}

func (d *Document) Settings() map[string]any {
	if d == nil {
		return nil
	}
	return d.settings
}

func validateEnvelope(settings map[string]any, expectedComponent string, requiredRoots []string, schema genericoptions.FieldSchema) error {
	version, _ := settings["version"].(string)
	if version != SchemaVersion {
		return fmt.Errorf("version = %q, want %q", version, SchemaVersion)
	}
	component, _ := settings["component"].(string)
	if component != expectedComponent {
		return fmt.Errorf("component = %q, want %q", component, expectedComponent)
	}
	allowed := map[string]struct{}{"version": {}, "component": {}}
	for root := range schema {
		allowed[root] = struct{}{}
	}
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown configuration field %s", key)
		}
	}
	for _, root := range requiredRoots {
		if _, ok := settings[root]; !ok {
			return fmt.Errorf("%s is required", root)
		}
	}
	for root, child := range schema {
		if err := genericoptions.ValidateRawSection(settings, root, child); err != nil {
			return err
		}
	}
	return nil
}

func applyOverrides(v *viper.Viper, runtime app.RuntimeConfigContext, prefix string, paths []string) {
	prefix = strings.TrimSuffix(strings.TrimSpace(prefix), ".")
	known := make(map[string]string, len(paths))
	for _, path := range paths {
		fullPath := path
		if prefix != "" {
			fullPath = prefix + "." + path
		}
		known[normalizeName(fullPath)] = path
		envName := strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(fullPath))
		if runtime.EnvPrefix != "" {
			envName = runtime.EnvPrefix + "_" + envName
		}
		if value, ok := os.LookupEnv(envName); ok {
			v.Set(path, value)
		}
	}
	for flagName, value := range runtime.ExplicitFlags {
		if path, ok := known[normalizeName(flagName)]; ok {
			v.Set(path, value)
		}
	}
}

func normalizeName(value string) string {
	return strings.ReplaceAll(strings.ToLower(value), "-", "_")
}
