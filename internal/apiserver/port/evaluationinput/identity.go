package evaluationinput

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"sort"
	"strconv"
	"strings"
)

const (
	IdentityRefV1Prefix = "isn:v1:"
	IdentityRefV2Prefix = "isn:v2:"
	// IdentityRefPrefix is the prefix used for new assessment attempts.
	IdentityRefPrefix = IdentityRefV2Prefix
)

// InputSnapshotIdentity is the structured, digest-backed identity of one
// materialized InputSnapshot (EV-R009). Any semantic component change yields
// a different CompositeDigest, so Run/Outcome refs can prove that retries
// executed against the same input.
type InputSnapshotIdentity struct {
	Version              string
	ModelCode            string
	ModelVersion         string
	ModelDigest          string
	QuestionnaireCode    string
	QuestionnaireVersion string
	QuestionnaireDigest  string
	AnswerSheetID        uint64
	AnswerSheetDigest    string
	SubjectDigest        string
	CompositeDigest      string
}

// Ref renders the single-string form persisted into input_snapshot_ref.
func (id InputSnapshotIdentity) Ref() string {
	if id.CompositeDigest == "" {
		return ""
	}
	if id.Version == "v1" {
		return IdentityRefV1Prefix + id.CompositeDigest
	}
	return IdentityRefV2Prefix + id.CompositeDigest
}

// IsIdentityRef reports whether ref is an EV-R009 verifiable reference, as
// opposed to a legacy "model:..." / "answersheet:..." readable label.
func IsIdentityRef(ref string) bool {
	return strings.HasPrefix(ref, IdentityRefV1Prefix) || strings.HasPrefix(ref, IdentityRefV2Prefix)
}

func IsV1IdentityRef(ref string) bool { return strings.HasPrefix(ref, IdentityRefV1Prefix) }

func IsV2IdentityRef(ref string) bool { return strings.HasPrefix(ref, IdentityRefV2Prefix) }

// NewInputSnapshotIdentity derives the identity from a resolved snapshot.
// It hashes an explicit whitelist of semantic fields in fixed order and never
// depends on JSON map iteration order. ok is false when the snapshot carries
// no identifiable component.
func NewInputSnapshotIdentity(input *InputSnapshot) (InputSnapshotIdentity, bool) {
	return newInputSnapshotIdentity(input, "v2")
}

// NewLegacyV1InputSnapshotIdentity reproduces the historical EV-R009 digest.
// It is used only to keep already-started v1 retry chains stable after v2 is
// deployed; new assessments must use NewInputSnapshotIdentity.
func NewLegacyV1InputSnapshotIdentity(input *InputSnapshot) (InputSnapshotIdentity, bool) {
	return newInputSnapshotIdentity(input, "v1")
}

func newInputSnapshotIdentity(input *InputSnapshot, version string) (InputSnapshotIdentity, bool) {
	if input == nil || (input.Model == nil && input.AnswerSheet == nil) {
		return InputSnapshotIdentity{}, false
	}
	id := InputSnapshotIdentity{Version: version}
	if m := input.Model; m != nil {
		id.ModelCode = m.Code
		id.ModelVersion = m.Version
		id.ModelDigest = digestFields(
			"model",
			string(m.Kind), m.SubKind, m.Algorithm,
			m.AlgorithmFamily, m.DecisionKind, m.PayloadFormat,
			m.ProductChannel, m.Code, m.Version,
		)
	}
	if q := input.Questionnaire; q != nil {
		id.QuestionnaireCode = q.Code
		id.QuestionnaireVersion = q.Version
		id.QuestionnaireDigest = digestQuestionnaire(q)
	}
	if s := input.AnswerSheet; s != nil {
		id.AnswerSheetID = s.ID
		id.AnswerSheetDigest = digestAnswerSheet(s)
	}
	if n := input.NormSubject; n != nil {
		if version == "v1" {
			ageMonths := 0
			if n.AgeMonths != nil {
				ageMonths = *n.AgeMonths
			}
			id.SubjectDigest = digestFields("subject", strconv.Itoa(ageMonths), n.Gender)
		} else {
			ageState, ageValue := "missing", ""
			if n.AgeMonths != nil {
				ageState, ageValue = "known", strconv.Itoa(*n.AgeMonths)
			}
			id.SubjectDigest = digestFields("subject:v2", ageState, ageValue, n.Gender)
		}
	}
	compositeVersion := "isn:v2"
	if version == "v1" {
		compositeVersion = "isn:v1"
	}
	id.CompositeDigest = digestFields(
		compositeVersion,
		id.ModelCode, id.ModelVersion, id.ModelDigest,
		id.QuestionnaireCode, id.QuestionnaireVersion, id.QuestionnaireDigest,
		strconv.FormatUint(id.AnswerSheetID, 10), id.AnswerSheetDigest,
		id.SubjectDigest,
	)
	return id, true
}

func digestQuestionnaire(q *QuestionnaireSnapshot) string {
	h := newDigest("questionnaire")
	writeField(h, q.Code)
	writeField(h, q.Version)
	for _, question := range q.Questions {
		writeField(h, question.Code)
		writeField(h, question.Type)
		for _, option := range question.Options {
			writeField(h, option.Code)
			writeField(h, formatFloat(option.Score))
		}
	}
	return finishDigest(h)
}

func digestAnswerSheet(s *AnswerSheetSnapshot) string {
	h := newDigest("answersheet")
	writeField(h, strconv.FormatUint(s.ID, 10))
	writeField(h, s.QuestionnaireCode)
	writeField(h, s.QuestionnaireVersion)
	answers := make([]AnswerSnapshot, len(s.Answers))
	copy(answers, s.Answers)
	sort.SliceStable(answers, func(i, j int) bool { return answers[i].QuestionCode < answers[j].QuestionCode })
	for _, answer := range answers {
		writeField(h, answer.QuestionCode)
		writeField(h, formatFloat(answer.Score))
		writeField(h, canonicalValue(answer.Value))
	}
	return finishDigest(h)
}

// canonicalValue serializes an arbitrary answer value deterministically:
// encoding/json sorts map keys, so JSON-decoded values are stable.
func canonicalValue(value any) string {
	if value == nil {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("!unencodable:%T", value)
	}
	return string(encoded)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func digestFields(domain string, fields ...string) string {
	h := newDigest(domain)
	for _, field := range fields {
		writeField(h, field)
	}
	return finishDigest(h)
}

func newDigest(domain string) hash.Hash {
	h := sha256.New()
	h.Write([]byte(domain))
	return h
}

// writeField length-prefixes each field so adjacent fields can never collide
// regardless of their content.
func writeField(h hash.Hash, field string) {
	_, _ = fmt.Fprintf(h, "|%d:", len(field))
	_, _ = h.Write([]byte(field))
}

func finishDigest(h hash.Hash) string {
	return hex.EncodeToString(h.Sum(nil))
}
