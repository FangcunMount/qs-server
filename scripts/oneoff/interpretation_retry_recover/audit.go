package main

import "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/recoveryjournal"

type operationIntent = recoveryjournal.Intent[recoveryPlan]
type operationReceipt = recoveryjournal.Receipt

func (p recoveryPlan) RecoveryEventID() string { return p.EventID }

func reserveOperation(dir string, intent operationIntent) error {
	return recoveryjournal.Reserve(dir, intent)
}
func persistExclusive(dir, name string, value any) error {
	return recoveryjournal.PersistExclusive(dir, name, value)
}
func readOperation(dir, requestID string) (operationIntent, *operationReceipt, error) {
	return recoveryjournal.Read[recoveryPlan](dir, requestID)
}
