package statistics

import (
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"gorm.io/gorm"
)

// AnalyticsScanWatermarkPO stores scan progress for behavior journey projection.
type AnalyticsScanWatermarkPO struct {
	ID              uint64         `gorm:"column:id;primaryKey;autoIncrement"`
	SourceName      string         `gorm:"column:source_name;size:64;not null;uniqueIndex:uk_source_org,priority:1"`
	OrgID           int64          `gorm:"column:org_id;not null;default:0;uniqueIndex:uk_source_org,priority:2"`
	LastSeenID      uint64         `gorm:"column:last_seen_id;not null;default:0"`
	LastSeenTime    *time.Time     `gorm:"column:last_seen_time"`
	ScanWindowStart *time.Time     `gorm:"column:scan_window_start"`
	ScanWindowEnd   *time.Time     `gorm:"column:scan_window_end"`
	Status          string         `gorm:"column:status;size:32;not null;default:idle;index:idx_status_updated_at,priority:1"`
	LastError       string         `gorm:"column:last_error;type:text"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime;index:idx_status_updated_at,priority:2"`
	DeletedAt       gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (AnalyticsScanWatermarkPO) TableName() string {
	return "analytics_scan_watermarks"
}

func scanWatermarkToDomain(po *AnalyticsScanWatermarkPO) *domainStatistics.ScanWatermark {
	if po == nil {
		return nil
	}
	return &domainStatistics.ScanWatermark{
		ID:              po.ID,
		SourceName:      po.SourceName,
		OrgID:           po.OrgID,
		LastSeenID:      po.LastSeenID,
		LastSeenTime:    po.LastSeenTime,
		ScanWindowStart: po.ScanWindowStart,
		ScanWindowEnd:   po.ScanWindowEnd,
		Status:          po.Status,
		LastError:       po.LastError,
	}
}

func scanWatermarkFromDomain(watermark *domainStatistics.ScanWatermark) *AnalyticsScanWatermarkPO {
	if watermark == nil {
		return nil
	}
	return &AnalyticsScanWatermarkPO{
		ID:              watermark.ID,
		SourceName:      watermark.SourceName,
		OrgID:           watermark.OrgID,
		LastSeenID:      watermark.LastSeenID,
		LastSeenTime:    watermark.LastSeenTime,
		ScanWindowStart: watermark.ScanWindowStart,
		ScanWindowEnd:   watermark.ScanWindowEnd,
		Status:          watermark.Status,
		LastError:       watermark.LastError,
	}
}
