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

// Fingerprint returns a stable fingerprint of the submission's business
// intent. Generated IDs, timestamps and calculated scores are excluded.
func Fingerprint(sheet *domainanswersheet.AnswerSheet) (string, error) {
	if sheet == nil {
		return "", errors.New("answer sheet is required")
	}
	ctx := sheet.SubmissionContext()
	code, version, _ := sheet.QuestionnaireInfo()
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
		QuestionnaireCode    string            `json:"questionnaire_code"`
		QuestionnaireVersion string            `json:"questionnaire_version"`
		Answers              []canonicalAnswer `json:"answers"`
	}
	answers := make([]canonicalAnswer, 0, len(sheet.Answers()))
	for _, answer := range sheet.Answers() {
		value, err := json.Marshal(answer.Value().Raw())
		if err != nil {
			return "", err
		}
		answers = append(answers, canonicalAnswer{
			QuestionCode: answer.QuestionCode(),
			QuestionType: answer.QuestionType(),
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
		WriterID:             ctx.Filler().UserID(),
		TesteeID:             ctx.TesteeID().Uint64(),
		OrgID:                ctx.OrgID().Uint64(),
		TaskID:               ctx.TaskID(),
		QuestionnaireCode:    code,
		QuestionnaireVersion: version,
		Answers:              answers,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
