// Package answeringstart owns the immutable beginning of an answering intent.
package answeringstart

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	sheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"math"
	"strings"
	"time"
)

// Intent contains only stable request facts; current ownership and server time
// must never be included in its hash, including during a replay after transfer.
type Intent struct {
	OrgID                int64           `json:"org_id"`
	UserID               uint64          `json:"user_id"`
	TesteeID             uint64          `json:"testee_id"`
	RequestKey           string          `json:"request_key"`
	QuestionnaireCode    string          `json:"questionnaire_code"`
	QuestionnaireVersion string          `json:"questionnaire_version"`
	ModelCode            string          `json:"model_code"`
	ModelVersion         string          `json:"model_version"`
	Origin               sheet.OriginRef `json:"origin"`
}

func (i Intent) Validate() error {
	if i.OrgID <= 0 || i.UserID == 0 || i.UserID > math.MaxInt64 || i.TesteeID == 0 || i.TesteeID > math.MaxInt64 {
		return fmt.Errorf("valid start company and subjects required")
	}
	if len(i.RequestKey) == 0 || len(i.RequestKey) > 128 {
		return fmt.Errorf("start request key must contain 1..128 ASCII characters")
	}
	for _, c := range i.RequestKey {
		if c < 33 || c > 126 {
			return fmt.Errorf("invalid start request key")
		}
	}
	for _, s := range []string{i.QuestionnaireCode, i.QuestionnaireVersion} {
		if strings.TrimSpace(s) == "" || strings.TrimSpace(s) != s || len(s) > 128 {
			return fmt.Errorf("exact questionnaire version required")
		}
	}
	if (i.ModelCode == "") != (i.ModelVersion == "") || len(i.ModelCode) > 128 || len(i.ModelVersion) > 128 {
		return fmt.Errorf("exact optional model version required")
	}
	if strings.TrimSpace(i.ModelCode) != i.ModelCode || strings.TrimSpace(i.ModelVersion) != i.ModelVersion || strings.TrimSpace(i.Origin.ID) != i.Origin.ID {
		return fmt.Errorf("noncanonical start reference")
	}
	if len(i.Origin.ID) > 128 {
		return fmt.Errorf("start origin reference too long")
	}
	return i.Origin.Validate()
}
func (i Intent) Hash() (string, error) {
	if err := i.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(struct {
		Version uint32 `json:"version"`
		Intent  Intent `json:"intent"`
	}{1, i})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

type Record struct {
	intent   Intent
	hash     string
	snapshot sheet.StartContext
}

func New(i Intent, id uint64, storeID *uint64, ownershipVersion uint32, at time.Time) (*Record, error) {
	hash, err := i.Hash()
	if err != nil {
		return nil, err
	}
	snapshot, err := sheet.NewStartContext(id, at, storeID, ownershipVersion)
	if err != nil {
		return nil, err
	}
	return &Record{intent: i, hash: hash, snapshot: snapshot}, nil
}
func Restore(i Intent, hash string, snapshot sheet.StartContext) (*Record, error) {
	expected, err := i.Hash()
	if err != nil {
		return nil, err
	}
	if hash != expected || snapshot.IsZero() || snapshot.Version() != 1 {
		return nil, fmt.Errorf("corrupt answering start")
	}
	return &Record{intent: i, hash: hash, snapshot: snapshot}, nil
}
func (r *Record) Intent() Intent              { return r.intent }
func (r *Record) Hash() string                { return r.hash }
func (r *Record) Context() sheet.StartContext { return r.snapshot }
func (r *Record) Matches(i Intent) bool       { h, e := i.Hash(); return e == nil && h == r.hash }
