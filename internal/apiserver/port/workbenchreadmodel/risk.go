// Package workbenchreadmodel defines read contracts owned by the operator workbench.
package workbenchreadmodel

import (
	"context"
	"time"
)

type PageRequest struct {
	Page     int
	PageSize int
}

func (p PageRequest) Offset() int {
	page := p.Page
	if page < 1 {
		page = 1
	}
	return (page - 1) * p.Limit()
}

func (p PageRequest) Limit() int {
	if p.PageSize < 1 {
		return 10
	}
	if p.PageSize > 100 {
		return 100
	}
	return p.PageSize
}

type LatestRiskFilter struct {
	OrgID     int64
	TesteeIDs []uint64
}

type LatestRiskQueueFilter struct {
	OrgID               int64
	TesteeIDs           []uint64
	RestrictToTesteeIDs bool
	RiskLevels          []string
}

type LatestRiskRow struct {
	AssessmentID uint64
	OrgID        int64
	TesteeID     uint64
	RiskLevel    string
	OccurredAt   time.Time
}

type LatestRiskPage struct {
	Items    []LatestRiskRow
	Total    int64
	Page     int
	PageSize int
}

type LatestRiskReader interface {
	ListLatestRisksByTesteeIDs(context.Context, LatestRiskFilter) ([]LatestRiskRow, error)
	ListLatestRiskQueue(context.Context, LatestRiskQueueFilter, PageRequest) (LatestRiskPage, error)
}
