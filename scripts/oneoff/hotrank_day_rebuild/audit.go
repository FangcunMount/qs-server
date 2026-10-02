package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type operationIntent struct {
	Mode              string `json:"mode"`
	Day               string `json:"day"`
	RequestID         string `json:"request_id"`
	Operator          string `json:"operator"`
	Fingerprint       string `json:"fingerprint"`
	TargetFingerprint string `json:"target_fingerprint"`
}
type operationJournal struct {
	dir    string
	intent operationIntent
}

// No connection credentials or source answers enter the journal. An intent is
// fsynced before mutation; a response lost before confirmation leaves that
// original intent available for reconciliation, never a fictitious success.
func openOperationJournal(dir string, intent operationIntent) (*operationJournal, error) {
	info, err := os.Lstat(dir)
	if err != nil || !filepath.IsAbs(dir) || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("existing absolute private audit directory required")
	}
	id, err := uuid.Parse(intent.RequestID)
	if err != nil || id.String() != intent.RequestID {
		return nil, fmt.Errorf("canonical operation UUID required")
	}
	day, err := time.Parse("20060102", intent.Day)
	if err != nil || day.Format("20060102") != intent.Day {
		return nil, fmt.Errorf("valid audit day required")
	}
	hash, err := hex.DecodeString(intent.Fingerprint)
	if err != nil || len(hash) != 32 {
		return nil, fmt.Errorf("valid approval fingerprint required")
	}
	j := &operationJournal{dir: dir, intent: intent}
	if err := j.persist("intent", intent); err != nil {
		return nil, err
	}
	return j, nil
}
func (j *operationJournal) confirm(receipt any) error { return j.persist("confirmed", receipt) }
func (j *operationJournal) persist(kind string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	path := filepath.Join(j.dir, j.intent.RequestID+"."+kind+".json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("unsafe existing audit record")
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(existing, encoded) {
			return fmt.Errorf("operation audit conflict")
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		syncErr := file.Sync()
		closeErr := file.Close()
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
		return j.syncDirectory()
	}
	if err != nil {
		return err
	}
	if _, err = file.Write(encoded); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return j.syncDirectory()
}
func (j *operationJournal) syncDirectory() error {
	directory, err := os.Open(j.dir)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
