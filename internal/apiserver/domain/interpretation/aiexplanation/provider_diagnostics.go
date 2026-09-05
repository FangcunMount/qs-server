package aiexplanation

import (
	"fmt"
	"regexp"
)

// ProviderFailureDiagnostics contains bounded metadata, never response bodies,
// reasoning text, credentials or user content. It is independent of a success receipt.
type ProviderFailureDiagnostics struct {
	Code           string `json:"code" bson:"code"`
	RequestID      string `json:"request_id,omitempty" bson:"request_id,omitempty"`
	ResponseStatus string `json:"response_status,omitempty" bson:"response_status,omitempty"`
	ResponseShape  string `json:"response_shape,omitempty" bson:"response_shape,omitempty"`
}

var providerDiagnosticIdentifier = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,256}$`)

func (d ProviderFailureDiagnostics) Validate() error {
	if !providerDiagnosticIdentifier.MatchString(d.Code) {
		return fmt.Errorf("invalid Provider diagnostic code")
	}
	for _, v := range []string{d.RequestID, d.ResponseStatus, d.ResponseShape} {
		if v != "" && !providerDiagnosticIdentifier.MatchString(v) {
			return fmt.Errorf("invalid Provider diagnostic metadata")
		}
	}
	return nil
}
