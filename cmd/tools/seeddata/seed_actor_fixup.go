package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	actorMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	"gorm.io/gorm"
)

type actorFixupRelationRow struct {
	RelationID      uint64    `gorm:"column:relation_id"`
	ClinicianID     uint64    `gorm:"column:clinician_id"`
	OperatorID      *uint64   `gorm:"column:operator_id"`
	RelationType    string    `gorm:"column:relation_type"`
	TesteeCreatedAt time.Time `gorm:"column:testee_created_at"`
	SourceType      string    `gorm:"column:source_type"`
}

type actorFixupClinicianAnchor struct {
	ClinicianID uint64
	OperatorID  *uint64
	FirstBound  time.Time
}

func seedActorFixupTimestamps(ctx context.Context, deps *dependencies) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	if deps.Config.Global.OrgID == 0 {
		return fmt.Errorf("global.orgId is required for actor_fixup_timestamps")
	}
	if strings.TrimSpace(deps.Config.Local.MySQLDSN) == "" {
		return fmt.Errorf("seeddata local.mysql_dsn is required for actor_fixup_timestamps")
	}

	mysqlDB, err := openLocalSeedMySQL(deps.Config.Local.MySQLDSN)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeLocalSeedMySQL(mysqlDB); closeErr != nil {
			deps.Logger.Warnw("Failed to close local mysql after actor timestamp fixup", "error", closeErr.Error())
		}
	}()

	rows, err := loadActorFixupRelations(ctx, mysqlDB, deps.Config.Global.OrgID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		deps.Logger.Infow("No eligible actor relations found for timestamp fixup", "org_id", deps.Config.Global.OrgID)
		return nil
	}

	anchors := make(map[uint64]actorFixupClinicianAnchor, len(rows))
	relationsUpdated := 0
	relationsSkipped := 0
	relationProgress := newSeedProgressBar("actor_fixup relations", len(rows))
	defer relationProgress.Close()

	for _, row := range rows {
		if row.TesteeCreatedAt.IsZero() {
			relationsSkipped++
			deps.Logger.Warnw("Skipping relation timestamp fixup because testee.created_at is zero",
				"relation_id", row.RelationID,
				"clinician_id", row.ClinicianID,
				"relation_type", row.RelationType,
			)
			relationProgress.Increment()
			continue
		}

		boundAt, err := deriveRelationBoundAt(row.TesteeCreatedAt, row.RelationType)
		if err != nil {
			relationsSkipped++
			deps.Logger.Warnw("Skipping relation timestamp fixup because relation type is unsupported",
				"relation_id", row.RelationID,
				"clinician_id", row.ClinicianID,
				"relation_type", row.RelationType,
				"error", err.Error(),
			)
			relationProgress.Increment()
			continue
		}
		if err := updateActorFixupRelation(ctx, mysqlDB, row.RelationID, boundAt); err != nil {
			return err
		}
		relationsUpdated++

		anchor := anchors[row.ClinicianID]
		if anchor.FirstBound.IsZero() || boundAt.Before(anchor.FirstBound) {
			anchors[row.ClinicianID] = actorFixupClinicianAnchor{
				ClinicianID: row.ClinicianID,
				OperatorID:  row.OperatorID,
				FirstBound:  boundAt,
			}
		}
		relationProgress.Increment()
	}
	relationProgress.Complete()

	cliniciansUpdated := 0
	staffUpdated := 0
	anchorProgress := newSeedProgressBar("actor_fixup anchors", len(anchors))
	defer anchorProgress.Close()
	for _, anchor := range anchors {
		if anchor.FirstBound.IsZero() {
			anchorProgress.Increment()
			continue
		}
		clinicianCreatedAt := deriveClinicianCreatedAt(anchor.FirstBound)
		if err := updateActorFixupClinician(ctx, mysqlDB, anchor.ClinicianID, clinicianCreatedAt); err != nil {
			return err
		}
		cliniciansUpdated++

		if anchor.OperatorID != nil && *anchor.OperatorID > 0 {
			staffCreatedAt := deriveStaffCreatedAt(clinicianCreatedAt)
			updated, err := updateActorFixupStaff(ctx, mysqlDB, *anchor.OperatorID, staffCreatedAt)
			if err != nil {
				return err
			}
			if updated {
				staffUpdated++
			}
		}
		anchorProgress.Increment()
	}
	anchorProgress.Complete()

	deps.Logger.Infow("Actor timestamp fixup completed",
		"org_id", deps.Config.Global.OrgID,
		"relations_loaded", len(rows),
		"relations_updated", relationsUpdated,
		"relations_skipped", relationsSkipped,
		"clinicians_updated", cliniciansUpdated,
		"staff_updated", staffUpdated,
	)
	return nil
}

func loadActorFixupRelations(ctx context.Context, mysqlDB *gorm.DB, orgID int64) ([]actorFixupRelationRow, error) {
	rows := make([]actorFixupRelationRow, 0, 128)
	err := mysqlDB.WithContext(ctx).
		Table((actorMySQL.ClinicianRelationPO{}).TableName()+" AS cr").
		Select("cr.id AS relation_id, cr.clinician_id, c.operator_id, cr.relation_type, cr.source_type, t.created_at AS testee_created_at").
		Joins("JOIN "+(actorMySQL.TesteePO{}).TableName()+" AS t ON t.id = cr.testee_id AND t.deleted_at IS NULL").
		Joins("JOIN "+(actorMySQL.ClinicianPO{}).TableName()+" AS c ON c.id = cr.clinician_id AND c.deleted_at IS NULL").
		Where("cr.org_id = ? AND cr.is_active = 1 AND cr.deleted_at IS NULL", orgID).
		Where("(cr.source_type IS NULL OR cr.source_type <> ?)", "assessment_entry").
		Order("cr.clinician_id ASC, t.created_at ASC, cr.id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load actor fixup relations: %w", err)
	}
	return rows, nil
}

func updateActorFixupRelation(ctx context.Context, mysqlDB *gorm.DB, relationID uint64, boundAt time.Time) error {
	err := mysqlDB.WithContext(ctx).
		Table((actorMySQL.ClinicianRelationPO{}).TableName()).
		Where("id = ? AND deleted_at IS NULL", relationID).
		Updates(map[string]interface{}{
			"bound_at":   boundAt,
			"created_at": boundAt,
			"updated_at": boundAt,
		}).Error
	if err != nil {
		return fmt.Errorf("update clinician_relation %d timestamps: %w", relationID, err)
	}
	return nil
}

func updateActorFixupClinician(ctx context.Context, mysqlDB *gorm.DB, clinicianID uint64, createdAt time.Time) error {
	err := mysqlDB.WithContext(ctx).
		Table((actorMySQL.ClinicianPO{}).TableName()).
		Where("id = ? AND deleted_at IS NULL", clinicianID).
		Updates(map[string]interface{}{
			"created_at": createdAt,
			"updated_at": createdAt,
		}).Error
	if err != nil {
		return fmt.Errorf("update clinician %d timestamps: %w", clinicianID, err)
	}
	return nil
}

func updateActorFixupStaff(ctx context.Context, mysqlDB *gorm.DB, operatorID uint64, createdAt time.Time) (bool, error) {
	result := mysqlDB.WithContext(ctx).
		Table((actorMySQL.OperatorPO{}).TableName()).
		Where("id = ? AND deleted_at IS NULL", operatorID).
		Updates(map[string]interface{}{
			"created_at": createdAt,
			"updated_at": createdAt,
		})
	if result.Error != nil {
		return false, fmt.Errorf("update staff %d timestamps: %w", operatorID, result.Error)
	}
	return result.RowsAffected > 0, nil
}
