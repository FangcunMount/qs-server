package delegatedsubject

import (
	"fmt"
	"time"
)

// Options configures delegated-subject signing and verification.
type Options struct {
	Enabled     bool          `json:"enabled" mapstructure:"enabled"`
	CurrentKey  string        `json:"current_key" mapstructure:"current-key"`
	PreviousKey string        `json:"previous_key" mapstructure:"previous-key"`
	TTL         time.Duration `json:"ttl" mapstructure:"ttl"`
}

// DefaultTTL is used when ttl is not configured.
const DefaultTTL = 5 * time.Minute

// Validate returns an error when an enabled production-facing delegated
// subject configuration cannot both sign and verify newly issued tokens.
// PreviousKey is deliberately optional: it only exists for a bounded rotation
// window and must never be the sole configured key.
func (o *Options) Validate() error {
	if o == nil || !o.Enabled {
		return nil
	}
	if len(o.CurrentKey) == 0 {
		return fmt.Errorf("delegated subject current key is required when enabled")
	}
	if o.TTL <= 0 {
		return fmt.Errorf("delegated subject ttl must be greater than zero when enabled")
	}
	return nil
}

// Signer signs delegated-subject tokens for outbound internal RPCs.
type Signer struct {
	key []byte
	ttl time.Duration
}

// Verifier validates delegated-subject tokens on inbound internal RPCs.
type Verifier struct {
	currentKey  []byte
	previousKey []byte
	callers     map[string]struct{}
}

// NewSignerFromOptions returns nil when signing is disabled or misconfigured.
func NewSignerFromOptions(opts *Options) (*Signer, error) {
	if opts == nil || !opts.Enabled {
		return nil, nil
	}
	key := []byte(opts.CurrentKey)
	if len(key) == 0 {
		return nil, fmt.Errorf("delegated subject current key is required when enabled")
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Signer{key: key, ttl: ttl}, nil
}

// NewVerifierFromOptions returns nil when verification is disabled or misconfigured.
func NewVerifierFromOptions(opts *Options) (*Verifier, error) {
	if opts == nil || !opts.Enabled {
		return nil, nil
	}
	current := []byte(opts.CurrentKey)
	previous := []byte(opts.PreviousKey)
	if len(current) == 0 && len(previous) == 0 {
		return nil, fmt.Errorf("delegated subject verification requires current or previous key")
	}
	return &Verifier{
		currentKey:  current,
		previousKey: previous,
		callers: map[string]struct{}{
			TrustedCallerQSCollection: {},
		},
	}, nil
}

func (s *Signer) Enabled() bool {
	return s != nil && len(s.key) > 0
}

func (v *Verifier) Enabled() bool {
	return v != nil && (len(v.currentKey) > 0 || len(v.previousKey) > 0)
}

func (s *Signer) Sign(in SignInput) (string, error) {
	if s == nil || !s.Enabled() {
		return "", fmt.Errorf("delegated subject signer is not configured")
	}
	if err := in.validate(); err != nil {
		return "", err
	}
	userID, err := parseUserID(in.UserID)
	if err != nil {
		return "", err
	}
	ttl := in.TTL
	if ttl <= 0 {
		ttl = s.ttl
	}
	nonce, err := newNonce()
	if err != nil {
		return "", err
	}
	return encodeToken(tokenPayload{
		UserID:   userID,
		TesteeID: in.TesteeID,
		OrgID:    in.OrgID,
		Purpose:  in.Purpose,
		Exp:      time.Now().Add(ttl).Unix(),
		Nonce:    nonce,
	}, s.key)
}

func (v *Verifier) Verify(raw string, purpose string, testeeID uint64) (Token, error) {
	if v == nil || !v.Enabled() {
		return Token{}, fmt.Errorf("delegated subject verifier is not configured")
	}
	token, err := decodeToken(raw, v.currentKey, v.previousKey)
	if err != nil {
		return Token{}, err
	}
	if token.Purpose != purpose {
		return Token{}, ErrPurposeMismatch
	}
	if token.TesteeID != testeeID {
		return Token{}, ErrTesteeMismatch
	}
	return token, nil
}

func (v *Verifier) AllowWorkload(serviceID string) error {
	if v == nil || !v.Enabled() || serviceID == "" {
		return nil
	}
	if _, ok := v.callers[serviceID]; ok {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrUntrustedWorkload, serviceID)
}

// SignWithExpiryForTest signs a delegated token with an explicit expiry (tests only).
func SignWithExpiryForTest(key string, in SignInput, expiry time.Time) (string, error) {
	if err := in.validate(); err != nil {
		return "", err
	}
	userID, err := parseUserID(in.UserID)
	if err != nil {
		return "", err
	}
	nonce, err := newNonce()
	if err != nil {
		return "", err
	}
	return encodeToken(tokenPayload{
		UserID:   userID,
		TesteeID: in.TesteeID,
		OrgID:    in.OrgID,
		Purpose:  in.Purpose,
		Exp:      expiry.Unix(),
		Nonce:    nonce,
	}, []byte(key))
}
