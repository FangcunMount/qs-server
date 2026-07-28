package historicalseed

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	HeaderContext     = "X-QS-Historical-Context"
	HeaderRequestedAt = "X-QS-Historical-Requested-At"
	HeaderSignature   = "X-QS-Historical-Signature"
)

var (
	ErrDisabled          = errors.New("historical seed context is disabled")
	ErrIncompleteHeaders = errors.New("historical seed headers are incomplete")
	ErrStaleRequest      = errors.New("historical seed request is outside the freshness window")
	ErrInvalidSignature  = errors.New("historical seed signature is invalid")
)

type Verifier struct {
	Enabled       bool
	Secret        []byte
	AllowedOrgIDs map[uint64]struct{}
	Earliest      time.Time
	Latest        time.Time
	Location      *time.Location
	MaxSkew       time.Duration
	Now           func() time.Time
}

type Headers struct {
	EncodedContext string
	RequestedAt    string
	Signature      string
}

func Encode(historical Context) (string, error) {
	payload, err := json.Marshal(historical)
	if err != nil {
		return "", fmt.Errorf("marshal historical seed context: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func Decode(encoded string) (Context, error) {
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return Context{}, fmt.Errorf("decode historical seed context: %w", err)
	}
	var historical Context
	if err := json.Unmarshal(payload, &historical); err != nil {
		return Context{}, fmt.Errorf("unmarshal historical seed context: %w", err)
	}
	return historical, nil
}

func Sign(method, requestURI string, body []byte, requestedAt, encodedContext string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical(method, requestURI, body, requestedAt, encodedContext)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (v Verifier) Verify(method, requestURI string, body []byte, headers Headers) (Context, bool, error) {
	present := strings.TrimSpace(headers.EncodedContext) != "" || strings.TrimSpace(headers.RequestedAt) != "" || strings.TrimSpace(headers.Signature) != ""
	if !present {
		return Context{}, false, nil
	}
	if !v.Enabled {
		return Context{}, false, ErrDisabled
	}
	if strings.TrimSpace(headers.EncodedContext) == "" || strings.TrimSpace(headers.RequestedAt) == "" || strings.TrimSpace(headers.Signature) == "" {
		return Context{}, false, ErrIncompleteHeaders
	}
	if len(v.Secret) == 0 {
		return Context{}, false, errors.New("historical seed verifier secret is empty")
	}

	requestedAt, err := time.Parse(time.RFC3339Nano, headers.RequestedAt)
	if err != nil {
		return Context{}, false, fmt.Errorf("parse historical requested-at: %w", err)
	}
	now := time.Now()
	if v.Now != nil {
		now = v.Now()
	}
	maxSkew := v.MaxSkew
	if maxSkew <= 0 {
		maxSkew = 5 * time.Minute
	}
	if delta := now.Sub(requestedAt); delta > maxSkew || delta < -maxSkew {
		return Context{}, false, ErrStaleRequest
	}

	want := Sign(method, requestURI, body, headers.RequestedAt, headers.EncodedContext, v.Secret)
	wantBytes, _ := hex.DecodeString(want)
	gotBytes, err := hex.DecodeString(strings.TrimSpace(headers.Signature))
	if err != nil || !hmac.Equal(gotBytes, wantBytes) {
		return Context{}, false, ErrInvalidSignature
	}
	historical, err := Decode(headers.EncodedContext)
	if err != nil {
		return Context{}, false, err
	}
	if err := historical.Validate(v.Earliest, v.Latest, v.Location); err != nil {
		return Context{}, false, err
	}
	if len(v.AllowedOrgIDs) > 0 {
		if _, ok := v.AllowedOrgIDs[historical.OrgID]; !ok {
			return Context{}, false, fmt.Errorf("historical seed org_id %d is not allowed", historical.OrgID)
		}
	}
	return historical, true, nil
}

// VerifyForwarded validates an already authenticated context received over an
// internal transport. The REST signature is intentionally not forwarded, but
// the receiving process must still enforce the feature switch, date window
// and organization allow-list instead of trusting arbitrary protobuf input.
func (v *Verifier) VerifyForwarded(historical Context) error {
	if v == nil || !v.Enabled {
		return ErrDisabled
	}
	if err := historical.Validate(v.Earliest, v.Latest, v.Location); err != nil {
		return err
	}
	if len(v.AllowedOrgIDs) == 0 {
		return fmt.Errorf("historical seed allowed organization list is empty")
	}
	if _, ok := v.AllowedOrgIDs[historical.OrgID]; !ok {
		return fmt.Errorf("historical seed org_id %d is not allowed", historical.OrgID)
	}
	return nil
}

func canonical(method, requestURI string, body []byte, requestedAt, encodedContext string) string {
	bodyHash := sha256.Sum256(body)
	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(method)),
		requestURI,
		hex.EncodeToString(bodyHash[:]),
		requestedAt,
		encodedContext,
	}, "\n")
}

func ParseOrgIDs(values []int64) (map[uint64]struct{}, error) {
	result := make(map[uint64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			return nil, fmt.Errorf("historical seed allowed org id must be positive: %s", strconv.FormatInt(value, 10))
		}
		result[uint64(value)] = struct{}{}
	}
	return result, nil
}
