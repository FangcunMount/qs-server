package operatorretire

import (
	"context"
	"encoding/json"
	"fmt"
)

type binding struct {
	ClinicianID uint64 `json:"clinician_id"`
	OrgID       int64  `json:"org_id"`
	OperatorID  uint64 `json:"operator_id"`
}

func (t *Tool) bindingDigest(ctx context.Context, id string) (string, error) {
	rows := []binding{}
	var err error
	if t.table == "staff" {
		err = t.db.WithContext(ctx).Table("clinician").Select("id AS clinician_id,org_id,operator_id").Where("operator_id IS NOT NULL").Order("id").Find(&rows).Error
	} else {
		err = t.db.WithContext(ctx).Table("clinician_operator_binding_archive").Select("clinician_id,org_id,operator_id").Where("migration_id=?", id).Order("clinician_id").Find(&rows).Error
	}
	return hash(rows), err
}

func (t *Tool) verifyArchive(ctx context.Context, r *Report) error {
	var rows []struct {
		ClinicianID  uint64
		OrgID        int64
		OperatorID   uint64
		BeforeRecord string
		RecordSHA256 string `gorm:"column:record_sha256"`
	}
	if err := t.db.WithContext(ctx).Table("clinician_operator_binding_archive").Where("migration_id=?", r.MigrationID).Order("clinician_id").Find(&rows).Error; err != nil {
		return err
	}
	facts := []binding{}
	if int64(len(rows)) != r.BindingCount {
		return fmt.Errorf("binding archive count mismatch")
	}
	for _, row := range rows {
		if rawHash(row.BeforeRecord) != row.RecordSHA256 {
			return fmt.Errorf("binding archive checksum mismatch")
		}
		var original struct {
			ID         uint64 `json:"id"`
			OrgID      int64  `json:"org_id"`
			OperatorID uint64 `json:"operator_id"`
		}
		if err := json.Unmarshal([]byte(row.BeforeRecord), &original); err != nil {
			return fmt.Errorf("invalid binding archive: %w", err)
		}
		if original.ID != row.ClinicianID || original.OrgID != row.OrgID || original.OperatorID != row.OperatorID {
			return fmt.Errorf("binding archive identity mismatch")
		}
		facts = append(facts, binding{ClinicianID: row.ClinicianID, OrgID: row.OrgID, OperatorID: row.OperatorID})
	}
	if hash(facts) != r.BindingHash {
		return fmt.Errorf("binding archive does not match original inventory")
	}
	return nil
}
