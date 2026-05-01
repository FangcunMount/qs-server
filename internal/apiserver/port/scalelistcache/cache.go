package scalelistcache

import (
	"context"
	"time"
)

// PublishedListCache 是量表应用层消费的已发布量表列表缓存端口。
type PublishedListCache interface {
	Rebuild(ctx context.Context) error
	GetPage(ctx context.Context, page, pageSize int) (*Page, bool)
}

// Page 表示缓存命中的量表摘要分页。
type Page struct {
	Items []Summary
	Total int64
}

// Summary 是缓存端口返回的量表摘要，不包含缓存实现细节。
type Summary struct {
	Code              string
	Title             string
	Description       string
	Category          string
	Stages            []string
	ApplicableAges    []string
	Reporters         []string
	Tags              []string
	QuestionnaireCode string
	Status            string
	CreatedBy         string
	CreatedAt         time.Time
	UpdatedBy         string
	UpdatedAt         time.Time
}
