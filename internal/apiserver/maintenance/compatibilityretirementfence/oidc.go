package compatibilityretirementfence

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"time"
)

type oidcHeader struct {
	Alg  string          `json:"alg"`
	Kid  string          `json:"kid"`
	Typ  string          `json:"typ"`
	Crit json.RawMessage `json:"crit"`
}
type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}
type jwks struct {
	Keys []jwk `json:"keys"`
}
type oidcClaims struct {
	Iss               string `json:"iss"`
	Aud               string `json:"aud"`
	Sub               string `json:"sub"`
	Exp               int64  `json:"exp"`
	Iat               int64  `json:"iat"`
	Nbf               int64  `json:"nbf"`
	JTI               string `json:"jti"`
	Repository        string `json:"repository"`
	RepositoryID      string `json:"repository_id"`
	OwnerID           string `json:"repository_owner_id"`
	RunID             string `json:"run_id"`
	RunAttempt        string `json:"run_attempt"`
	ActorID           string `json:"actor_id"`
	CheckRunID        string `json:"check_run_id"`
	Ref               string `json:"ref"`
	RefType           string `json:"ref_type"`
	HeadRef           string `json:"head_ref"`
	BaseRef           string `json:"base_ref"`
	WorkflowRef       string `json:"workflow_ref"`
	WorkflowSHA       string `json:"workflow_sha"`
	SHA               string `json:"sha"`
	EventName         string `json:"event_name"`
	Environment       string `json:"environment"`
	RunnerEnvironment string `json:"runner_environment"`
	JobWorkflowRef    string `json:"job_workflow_ref"`
	JobWorkflowSHA    string `json:"job_workflow_sha"`
}

func verifyOIDC(token string, keys []byte, p Policy, now time.Time) (oidcClaims, error) {
	var c oidcClaims
	if len(token) < 1 || len(token) > 16384 || strings.ContainsAny(token, " \r\n\t\x00") {
		return c, ErrToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return c, ErrToken
	}
	enc := base64.RawURLEncoding.Strict()
	header, e := enc.DecodeString(parts[0])
	if e != nil {
		return c, ErrToken
	}
	payload, e := enc.DecodeString(parts[1])
	if e != nil {
		return c, ErrToken
	}
	sig, e := enc.DecodeString(parts[2])
	if e != nil {
		return c, ErrToken
	}
	var h oidcHeader
	if decodeJSON(header, &h) != nil || h.Alg != "RS256" || h.Typ != "JWT" || len(h.Kid) < 1 || len(h.Kid) > 256 || len(h.Crit) != 0 {
		return c, ErrToken
	}
	var set jwks
	if decodeJSON(keys, &set) != nil || len(set.Keys) < 1 || len(set.Keys) > 32 {
		return c, ErrToken
	}
	var public *rsa.PublicKey
	seen := map[string]bool{}
	for _, k := range set.Keys {
		if k.Kid == "" || seen[k.Kid] {
			return c, ErrToken
		}
		seen[k.Kid] = true
		if k.Kid != h.Kid {
			continue
		}
		if k.Kty != "RSA" || k.Alg != "RS256" || k.Use != "sig" {
			return c, ErrToken
		}
		n, ne := enc.DecodeString(k.N)
		ex, ee := enc.DecodeString(k.E)
		if ne != nil || ee != nil || len(n) < 256 || len(n) > 1024 || len(ex) > 4 || len(ex) < 1 || n[0] == 0 || ex[0] == 0 {
			return c, ErrToken
		}
		exponent := new(big.Int).SetBytes(ex)
		if !exponent.IsInt64() || exponent.Int64() != 65537 {
			return c, ErrToken
		}
		public = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: 65537}
		if public.N.BitLen() < 2048 {
			return c, ErrToken
		}
	}
	if public == nil {
		return c, ErrToken
	}
	hashed := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(public, crypto.SHA256, hashed[:], sig) != nil || decodeJSON(payload, &c) != nil {
		return c, ErrToken
	}
	if c.Iss != Issuer || c.Aud != p.Audience || c.Sub != p.Subject || c.Repository != Repository || c.RepositoryID != p.RepositoryID || c.OwnerID != p.OwnerID || c.RunID != p.RunID || c.RunAttempt != p.RunAttempt || c.ActorID != p.ActorID || c.CheckRunID != p.CheckRunID || c.Ref != "refs/heads/main" || c.RefType != "branch" || c.HeadRef != "" || c.BaseRef != "" || c.WorkflowRef != Repository+"/"+WorkflowPath+"@refs/heads/main" || c.WorkflowSHA != p.SourceSHA || c.SHA != p.SourceSHA || c.EventName != "workflow_dispatch" || c.Environment != "production" || c.RunnerEnvironment != "self-hosted" || c.JobWorkflowRef != "" || c.JobWorkflowSHA != "" || len(c.JTI) < 1 || len(c.JTI) > 256 || strings.ContainsAny(c.JTI, "\r\n\x00") {
		return c, ErrToken
	}
	unix := now.Unix()
	if c.Exp <= unix || c.Nbf > unix || c.Iat > unix || c.Iat < unix-300 || c.Exp <= c.Iat || c.Exp-c.Iat > 600 || c.Nbf < c.Iat-30 || c.Nbf > c.Exp {
		return c, ErrToken
	}
	if c.Exp > p.ExpiresAt.Unix() {
		return c, ErrToken
	}
	return c, nil
}

func canonicalInteger(n int64) string { return strconv.FormatInt(n, 10) }
