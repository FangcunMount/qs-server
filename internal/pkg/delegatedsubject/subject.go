package delegatedsubject

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/serviceidentity"
)

const (
	MetadataKey = "x-qs-delegated-subject"

	PurposeGetAssessmentReport = "participant_report.get_assessment_report"
	PurposeListMyReports       = "participant_report.list_my_reports"

	TrustedCallerQSCollection = serviceidentity.CollectionServerServiceID
)

var (
	ErrMissingToken      = errors.New("delegated subject token is required")
	ErrInvalidToken      = errors.New("delegated subject token is invalid")
	ErrExpiredToken      = errors.New("delegated subject token expired")
	ErrPurposeMismatch   = errors.New("delegated subject purpose mismatch")
	ErrTesteeMismatch    = errors.New("delegated subject testee mismatch")
	ErrUntrustedWorkload = errors.New("untrusted workload identity")
)

// Token carries a verified end-user delegation bound to a testee.
type Token struct {
	UserID   string
	TesteeID uint64
	OrgID    uint64
	Purpose  string
	Expiry   time.Time
	Nonce    string
}

type tokenPayload struct {
	UserID   string `json:"uid"`
	TesteeID uint64 `json:"tid"`
	OrgID    uint64 `json:"oid,omitempty"`
	Purpose  string `json:"pur"`
	Exp      int64  `json:"exp"`
	Nonce    string `json:"nonce"`
}

// SignInput is the delegation material collection-server signs after ProfileLink checks.
type SignInput struct {
	UserID   string
	TesteeID uint64
	OrgID    uint64
	Purpose  string
	TTL      time.Duration
}

func (in SignInput) validate() error {
	if strings.TrimSpace(in.UserID) == "" {
		return fmt.Errorf("user id is required")
	}
	if in.TesteeID == 0 {
		return fmt.Errorf("testee id is required")
	}
	if strings.TrimSpace(in.Purpose) == "" {
		return fmt.Errorf("purpose is required")
	}
	if in.TTL < 0 {
		return fmt.Errorf("ttl must be positive")
	}
	return nil
}

func encodeToken(payload tokenPayload, key []byte) (string, error) {
	if len(key) == 0 {
		return "", fmt.Errorf("signing key is required")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(encoded))
	signature := hex.EncodeToString(mac.Sum(nil))
	return encoded + "." + signature, nil
}

func decodeToken(raw string, keys ...[]byte) (Token, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Token{}, ErrMissingToken
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Token{}, ErrInvalidToken
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Token{}, ErrInvalidToken
	}
	if !verifySignature(parts[0], parts[1], keys...) {
		return Token{}, ErrInvalidToken
	}
	var payload tokenPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return Token{}, ErrInvalidToken
	}
	if payload.UserID == "" || payload.TesteeID == 0 || payload.Purpose == "" || payload.Nonce == "" || payload.Exp <= 0 {
		return Token{}, ErrInvalidToken
	}
	expiry := time.Unix(payload.Exp, 0)
	if time.Now().After(expiry) {
		return Token{}, ErrExpiredToken
	}
	return Token{
		UserID:   payload.UserID,
		TesteeID: payload.TesteeID,
		OrgID:    payload.OrgID,
		Purpose:  payload.Purpose,
		Expiry:   expiry,
		Nonce:    payload.Nonce,
	}, nil
}

func verifySignature(payload, signature string, keys ...[]byte) bool {
	for _, key := range keys {
		if len(key) == 0 {
			continue
		}
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(payload))
		expected := hex.EncodeToString(mac.Sum(nil))
		if hmac.Equal([]byte(expected), []byte(signature)) {
			return true
		}
	}
	return false
}

func newNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func parseUserID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("user id is required")
	}
	if _, err := strconv.ParseUint(raw, 10, 64); err != nil {
		return "", fmt.Errorf("user id must be numeric")
	}
	return raw, nil
}
