package reporttemplate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

const ManifestSchemaVersion = "interpretation-report-template-manifest/v1"

var templateIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
var templateVersionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// ManifestRoute binds one frozen runtime decision to its exact report builder
// and content schema. AdapterKey is empty for non-typology mechanisms.
type ManifestRoute struct {
	DecisionKind         modelcatalog.DecisionKind `json:"decision_kind" bson:"decision_kind"`
	BuilderIdentity      string                    `json:"builder_identity" bson:"builder_identity"`
	ContentSchemaVersion string                    `json:"content_schema_version" bson:"content_schema_version"`
	AdapterKey           string                    `json:"adapter_key,omitempty" bson:"adapter_key,omitempty"`
}

// ReleaseManifest is the immutable, self-contained identity of one report
// template release. Its fingerprint is calculated from the canonical form.
type ReleaseManifest struct {
	SchemaVersion   string                 `json:"schema_version" bson:"schema_version"`
	TemplateID      string                 `json:"template_id" bson:"template_id"`
	TemplateVersion policy.TemplateVersion `json:"template_version" bson:"template_version"`
	ReportType      policy.ReportType      `json:"report_type" bson:"report_type"`
	Routes          []ManifestRoute        `json:"routes" bson:"routes"`
}

func NewReleaseManifest(
	templateID string,
	templateVersion policy.TemplateVersion,
	reportType policy.ReportType,
	routes []ManifestRoute,
) (ReleaseManifest, error) {
	manifest := ReleaseManifest{
		SchemaVersion:   ManifestSchemaVersion,
		TemplateID:      strings.TrimSpace(templateID),
		TemplateVersion: policy.TemplateVersion(strings.TrimSpace(templateVersion.String())),
		ReportType:      reportType,
		Routes:          append([]ManifestRoute(nil), routes...),
	}
	manifest.normalizeRoutes()
	if err := manifest.Validate(); err != nil {
		return ReleaseManifest{}, err
	}
	return manifest, nil
}

func (m ReleaseManifest) Validate() error {
	if m.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("unsupported report template manifest schema: %s", m.SchemaVersion)
	}
	if !templateIDPattern.MatchString(m.TemplateID) {
		return fmt.Errorf("report template manifest template_id is invalid")
	}
	if !templateVersionPattern.MatchString(m.TemplateVersion.String()) {
		return fmt.Errorf("report template manifest template_version is invalid")
	}
	if m.ReportType != policy.ReportTypeStandard {
		return fmt.Errorf("unsupported report template manifest report_type: %s", m.ReportType)
	}
	if len(m.Routes) == 0 {
		return fmt.Errorf("report template manifest routes are required")
	}
	seen := make(map[modelcatalog.DecisionKind]struct{}, len(m.Routes))
	for _, route := range m.Routes {
		if _, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(route.DecisionKind); !ok {
			return fmt.Errorf("report template manifest decision_kind is invalid: %s", route.DecisionKind)
		}
		if _, duplicate := seen[route.DecisionKind]; duplicate {
			return fmt.Errorf("report template manifest decision_kind is duplicated: %s", route.DecisionKind)
		}
		seen[route.DecisionKind] = struct{}{}
		if strings.TrimSpace(route.BuilderIdentity) == "" {
			return fmt.Errorf("report template manifest builder_identity is required for %s", route.DecisionKind)
		}
		if strings.TrimSpace(route.ContentSchemaVersion) == "" {
			return fmt.Errorf("report template manifest content_schema_version is required for %s", route.DecisionKind)
		}
		if route.BuilderIdentity != strings.TrimSpace(route.BuilderIdentity) ||
			route.ContentSchemaVersion != strings.TrimSpace(route.ContentSchemaVersion) ||
			route.AdapterKey != strings.TrimSpace(route.AdapterKey) {
			return fmt.Errorf("report template manifest route values must be normalized")
		}
	}
	canonical := m
	canonical.normalizeRoutes()
	for index := range canonical.Routes {
		if canonical.Routes[index] != m.Routes[index] {
			return fmt.Errorf("report template manifest routes must be canonically sorted")
		}
	}
	return nil
}

func (m ReleaseManifest) Fingerprint() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("marshal report template manifest: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func (m ReleaseManifest) RouteFor(decisionKind modelcatalog.DecisionKind) (ManifestRoute, bool) {
	for _, route := range m.Routes {
		if route.DecisionKind == decisionKind {
			return route, true
		}
	}
	return ManifestRoute{}, false
}

func (m *ReleaseManifest) normalizeRoutes() {
	if m == nil {
		return
	}
	for index := range m.Routes {
		m.Routes[index].BuilderIdentity = strings.TrimSpace(m.Routes[index].BuilderIdentity)
		m.Routes[index].ContentSchemaVersion = strings.TrimSpace(m.Routes[index].ContentSchemaVersion)
		m.Routes[index].AdapterKey = strings.TrimSpace(m.Routes[index].AdapterKey)
	}
	sort.Slice(m.Routes, func(left, right int) bool {
		return string(m.Routes[left].DecisionKind) < string(m.Routes[right].DecisionKind)
	})
}
