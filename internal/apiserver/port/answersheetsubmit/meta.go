package answersheetsubmit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
)

var ErrIdempotencyConflict = errors.New("answersheet idempotency key reused with different submission content")

type DurableSubmitMeta struct {
	IdempotencyKey string
	WriterID       uint64
	Fingerprint    string
	RequestID      string
}

// CompletedSubmission is the durable idempotency fact returned for a
// writer-scoped submission key. Fingerprint is the acceptance-time value
// stored with the submission, or the historical fallback reconstructed for a
// legacy row that predates the embedded fingerprint.
type CompletedSubmission struct {
	Sheet       *domainanswersheet.AnswerSheet
	Fingerprint string
}

// SubmissionIntent is the canonical business input covered by the durable
// submission fingerprint. Derived IDs, timestamps, titles and scores are
// deliberately excluded.
type SubmissionIntent struct {
	WriterID             int64
	TesteeID             uint64
	OrgID                uint64
	TaskID               string
	OriginType           string
	OriginID             string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Answers              []SubmissionAnswer
}

type SubmissionAnswer struct {
	QuestionCode string
	QuestionType string
	Value        any
}

// Fingerprint returns a stable fingerprint of the submission's business
// intent. Generated IDs, timestamps and calculated scores are excluded.
func Fingerprint(sheet *domainanswersheet.AnswerSheet) (string, error) {
	if sheet == nil {
		return "", errors.New("answer sheet is required")
	}
	ctx := sheet.SubmissionContext()
	code, version, _ := sheet.QuestionnaireInfo()
	attribution := ctx.Attribution()
	intent := SubmissionIntent{
		WriterID:             ctx.Filler().UserID(),
		TesteeID:             ctx.TesteeID().Uint64(),
		OrgID:                ctx.OrgID().Uint64(),
		TaskID:               ctx.TaskID(),
		OriginType:           string(attribution.OriginType()),
		OriginID:             attribution.OriginID(),
		QuestionnaireCode:    code,
		QuestionnaireVersion: version,
		Answers:              make([]SubmissionAnswer, 0, len(sheet.Answers())),
	}
	for _, answer := range sheet.Answers() {
		intent.Answers = append(intent.Answers, SubmissionAnswer{
			QuestionCode: answer.QuestionCode(),
			QuestionType: answer.QuestionType(),
			Value:        answer.Value().Raw(),
		})
	}
	return FingerprintIntent(intent)
}

// FingerprintIntent hashes an explicit submission intent using the exact
// canonical JSON shape historically produced by Fingerprint.
func FingerprintIntent(intent SubmissionIntent) (string, error) {
	type canonicalAnswer struct {
		QuestionCode string `json:"question_code"`
		QuestionType string `json:"question_type"`
		Value        string `json:"value"`
	}
	type canonicalSubmission struct {
		WriterID             int64             `json:"writer_id"`
		TesteeID             uint64            `json:"testee_id"`
		OrgID                uint64            `json:"org_id"`
		TaskID               string            `json:"task_id"`
		OriginType           string            `json:"origin_type"`
		OriginID             string            `json:"origin_id"`
		QuestionnaireCode    string            `json:"questionnaire_code"`
		QuestionnaireVersion string            `json:"questionnaire_version"`
		Answers              []canonicalAnswer `json:"answers"`
	}
	answers := make([]canonicalAnswer, 0, len(intent.Answers))
	for _, answer := range intent.Answers {
		value, err := json.Marshal(answer.Value)
		if err != nil {
			return "", err
		}
		answers = append(answers, canonicalAnswer{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Value:        string(value),
		})
	}
	sort.Slice(answers, func(i, j int) bool {
		if answers[i].QuestionCode != answers[j].QuestionCode {
			return answers[i].QuestionCode < answers[j].QuestionCode
		}
		if answers[i].QuestionType != answers[j].QuestionType {
			return answers[i].QuestionType < answers[j].QuestionType
		}
		return answers[i].Value < answers[j].Value
	})
	payload, err := json.Marshal(canonicalSubmission{
		WriterID:             intent.WriterID,
		TesteeID:             intent.TesteeID,
		OrgID:                intent.OrgID,
		TaskID:               intent.TaskID,
		OriginType:           intent.OriginType,
		OriginID:             intent.OriginID,
		QuestionnaireCode:    intent.QuestionnaireCode,
		QuestionnaireVersion: intent.QuestionnaireVersion,
		Answers:              answers,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
