package aibridge

// EvaluationCreationReceipt describes the original request, independently of current progress.
type EvaluationCreationReceipt struct {
	SchemaVersion      string            `json:"schema_version"`
	RunID              string            `json:"run_id"`
	Release            EvaluationRelease `json:"release"`
	ReleaseFingerprint string            `json:"release_fingerprint"`
	RequestedBy        string            `json:"requested_by"`
	RequestReason      string            `json:"request_reason"`
	CreatedAt          string            `json:"created_at"`
}
