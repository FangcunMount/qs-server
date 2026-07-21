package rendering

import (
	"context"

	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

type versionedBuilder struct {
	inner   Builder
	version policy.TemplateVersion
}

// Versioned registers the same builder implementation under an explicit template
// release without duplicating build logic.
func Versioned(inner Builder, version policy.TemplateVersion) Builder {
	if inner == nil || version.IsEmpty() {
		return inner
	}
	return versionedBuilder{inner: inner, version: version}
}

// ExpandTemplateVersions clones builders for each requested release.
func ExpandTemplateVersions(builders []Builder, versions ...policy.TemplateVersion) []Builder {
	if len(versions) == 0 {
		return builders
	}
	out := make([]Builder, 0, len(builders)*len(versions))
	for _, version := range versions {
		for _, builder := range builders {
			out = append(out, Versioned(builder, version))
		}
	}
	return out
}

func (b versionedBuilder) ReportType() policy.ReportType { return b.inner.ReportType() }
func (b versionedBuilder) TemplateVersion() policy.TemplateVersion {
	return b.version
}
func (b versionedBuilder) BuilderIdentity() string    { return b.inner.BuilderIdentity() }
func (b versionedBuilder) ContentSchemaVersion() string { return b.inner.ContentSchemaVersion() }
func (b versionedBuilder) Build(ctx context.Context, input interpinput.InterpretationInput) (*report.Draft, error) {
	return b.inner.Build(ctx, input)
}

func (b versionedBuilder) MechanismKey() Key {
	keyed, ok := b.inner.(KeyedBuilder)
	if !ok {
		return Key{ReportType: b.ReportType(), TemplateVersion: b.version}
	}
	key := keyed.MechanismKey()
	if key.ReportType == "" {
		key.ReportType = b.ReportType()
	}
	key.TemplateVersion = b.version
	return key
}

func (b versionedBuilder) MechanismKeys() []Key {
	multi, ok := b.inner.(MultiKeyedBuilder)
	if !ok {
		return []Key{b.MechanismKey()}
	}
	keys := multi.MechanismKeys()
	out := make([]Key, 0, len(keys))
	for _, key := range keys {
		if key.ReportType == "" {
			key.ReportType = b.ReportType()
		}
		key.TemplateVersion = b.version
		out = append(out, key)
	}
	return out
}
