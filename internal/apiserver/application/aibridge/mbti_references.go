package aibridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// This is transport integrity and finite reference resolution, not theory approval.
// The original selection is supplied by qs-ai's frozen input; QS never fetches sources.
type mbtiReferenceSelection struct {
	SchemaVersion string                `json:"schema_version"`
	Version       string                `json:"version"`
	ModelCode     string                `json:"model_code"`
	ModelVersion  string                `json:"model_version"`
	TypeCode      string                `json:"type_code"`
	Sources       []mbtiReferenceSource `json:"sources"`
	Entries       []mbtiReferenceEntry  `json:"entries"`
}
type mbtiReferenceSource struct {
	ID           string `json:"source_id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	AccessedOn   string `json:"accessed_on"`
	SupportScope string `json:"support_scope"`
}
type mbtiReferenceEntry struct {
	ID            string   `json:"entry_id"`
	Topic         string   `json:"topic"`
	Axis          string   `json:"axis"`
	Pole          string   `json:"pole"`
	Content       string   `json:"content"`
	SourceIDs     []string `json:"source_ids"`
	UsageBoundary string   `json:"usage_boundary"`
}

var referenceIdentity = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
var referenceVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)

func referenceText(s string, limit int) bool {
	return strings.TrimSpace(s) != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= limit && !strings.ContainsAny(s, "<>")
}

func validateMBTIReferences(a Artifact) error {
	if len(a.ReferenceMaterialJSON) == 0 || len(a.ReferenceMaterialJSON) > 131072 || !digest(a.ReferenceMaterialFingerprint, true) {
		return ErrInvalid
	}
	sum := sha256.Sum256([]byte(a.ReferenceMaterialJSON))
	if a.ReferenceMaterialFingerprint != "sha256:"+hex.EncodeToString(sum[:]) {
		return ErrInvalid
	}
	var material mbtiReferenceSelection
	dec := json.NewDecoder(bytes.NewBufferString(a.ReferenceMaterialJSON))
	dec.DisallowUnknownFields()
	if dec.Decode(&material) != nil || dec.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	if material.SchemaVersion != "mbti-reference-selection/v1" || material.ModelCode != "MBTI_OEJTS" || material.ModelVersion != "v64-report-202608-v1" || !referenceVersion.MatchString(material.Version) || len(material.TypeCode) != 4 || len(material.Sources) < 1 || len(material.Sources) > 8 || len(material.Entries) < 12 || len(material.Entries) > 24 {
		return ErrInvalid
	}
	axes := []string{"EI", "SN", "TF", "JP"}
	poles := make(map[string]string, 4)
	for i, axis := range axes {
		pole := string(material.TypeCode[i])
		if !strings.Contains(axis, pole) {
			return ErrInvalid
		}
		poles[axis] = pole
	}
	sources := make(map[string]bool, len(material.Sources))
	for _, source := range material.Sources {
		parsed, err := url.Parse(source.URL)
		parsedDate, dateErr := time.Parse("2006-01-02", source.AccessedOn)
		if !referenceIdentity.MatchString(source.ID) || sources[source.ID] || !referenceText(source.Title, 255) || !referenceText(source.URL, 2048) || !referenceText(source.SupportScope, 1000) || err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || strings.IndexFunc(source.URL, unicode.IsSpace) >= 0 || dateErr != nil || parsedDate.Format("2006-01-02") != source.AccessedOn {
			return ErrInvalid
		}
		sources[source.ID] = true
	}
	entries := make(map[string]mbtiReferenceEntry, len(material.Entries))
	coverage, usedSources := map[string]bool{}, map[string]bool{}
	for _, entry := range material.Entries {
		_, duplicate := entries[entry.ID]
		if !referenceIdentity.MatchString(entry.ID) || duplicate || (entry.Topic != "personality" && entry.Topic != "career" && entry.Topic != "relationships") || poles[entry.Axis] == "" || poles[entry.Axis] != entry.Pole || !referenceText(entry.Content, 1000) || !referenceText(entry.UsageBoundary, 500) || len(entry.SourceIDs) < 1 || len(entry.SourceIDs) > 3 {
			return ErrInvalid
		}
		linked := map[string]bool{}
		for _, id := range entry.SourceIDs {
			if !sources[id] || linked[id] {
				return ErrInvalid
			}
			linked[id], usedSources[id] = true, true
		}
		entries[entry.ID] = entry
		coverage[entry.Topic+":"+entry.Axis] = true
	}
	if len(coverage) != 12 || len(usedSources) != len(sources) {
		return ErrInvalid
	}
	var output struct {
		Sections []struct {
			Topic     string               `json:"topic"`
			Insights  []mbtiReferenceLinks `json:"insights"`
			Questions []mbtiReferenceLinks `json:"reflection_questions"`
			Actions   []mbtiReferenceLinks `json:"actions"`
		} `json:"sections"`
	}
	if json.Unmarshal([]byte(a.ContentJSON), &output) != nil {
		return ErrInvalid
	}
	for _, section := range output.Sections {
		items := append(append(section.Insights, section.Questions...), section.Actions...)
		for _, item := range items {
			for _, ref := range item.ReferenceRefs {
				entry, found := entries[strings.TrimPrefix(ref, "reference:")]
				if !strings.HasPrefix(ref, "reference:") || !found || entry.Topic != section.Topic {
					return ErrInvalid
				}
				supported := false
				for _, evidence := range item.EvidenceRefs {
					if evidence.Ref == "model_result" || evidence.Ref == "dimension:"+entry.Axis {
						supported = true
					}
				}
				if !supported {
					return ErrInvalid
				}
			}
		}
	}
	return nil
}

type mbtiReferenceLinks struct {
	ReferenceRefs []string `json:"reference_refs"`
	EvidenceRefs  []struct {
		Ref string `json:"ref"`
	} `json:"evidence_refs"`
}
