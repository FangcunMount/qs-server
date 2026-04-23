package answersheetsubmit

type DurableSubmitMeta struct {
	IdempotencyKey string
	WriterID       uint64
	TesteeID       uint64
	OrgID          uint64
	TaskID         string
}
