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

	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
)

const (
	IdentityRefPrefix = "isn:v2:"
)

// InputSnapshotIdentity is the structured, digest-backed identity of one
// materialized InputSnapshot (EV-R009). Any semantic component change yields
// a different CompositeDigest, so Run/Outcome refs can prove that retries
// executed against the same input.
type InputSnapshotIdentity struct {
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
	return IdentityRefPrefix + id.CompositeDigest
}

// IsIdentityRef reports whether ref is a complete EV-R009 v2 identity.
func IsIdentityRef(ref string) bool {
	if !strings.HasPrefix(ref, IdentityRefPrefix) {
		return false
	}
	digest := strings.TrimPrefix(ref, IdentityRefPrefix)
	if len(digest) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

// NewInputSnapshotIdentity derives the identity from a resolved snapshot.
// It hashes an explicit whitelist of semantic fields in fixed order and never
// depends on JSON map iteration order. Incomplete snapshots are not executable.
func NewInputSnapshotIdentity(input *InputSnapshot) (InputSnapshotIdentity, bool) {
	if input == nil || input.Model == nil || input.DefinitionV2 == nil ||
		input.AnswerSheet == nil || input.Questionnaire == nil || !input.Model.HasFrozenRuntime() {
		return InputSnapshotIdentity{}, false
	}
	m := input.Model
	if m.Kind == "" || m.Algorithm == "" || m.Code == "" || m.Version == "" {
		return InputSnapshotIdentity{}, false
	}
	definitionDigest, err := modeldefinition.CanonicalContentHash(input.DefinitionV2)
	if err != nil || definitionDigest == "" {
		return InputSnapshotIdentity{}, false
	}
	id := InputSnapshotIdentity{}
	id.ModelCode = m.Code
	id.ModelVersion = m.Version
	id.ModelDigest = digestFields(
		"model:v2",
		string(m.Kind), m.SubKind, m.Algorithm,
		m.AlgorithmFamily, m.DecisionKind,
		m.ProductChannel, m.Code, m.Version, definitionDigest,
	)
	q := input.Questionnaire
	id.QuestionnaireCode = q.Code
	id.QuestionnaireVersion = q.Version
	id.QuestionnaireDigest = digestQuestionnaire(q)
	s := input.AnswerSheet
	id.AnswerSheetID = s.ID
	id.AnswerSheetDigest = digestAnswerSheet(s)
	if n := input.NormSubject; n != nil {
		ageState, ageValue := "missing", ""
		if n.AgeMonths != nil {
			ageState, ageValue = "known", strconv.Itoa(*n.AgeMonths)
		}
		id.SubjectDigest = digestFields("subject:v2", ageState, ageValue, n.Gender)
	}
	id.CompositeDigest = digestFields(
		"isn:v2",
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
