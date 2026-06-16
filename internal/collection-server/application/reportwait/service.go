package reportwait

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

const (
	defaultPollInterval = 500 * time.Millisecond
)

type QueryService interface {
	GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentDetailResponse, error)
}

type StatusCache interface {
	Get(ctx context.Context, assessmentID string) (*reportstatus.Snapshot, error)
	Set(ctx context.Context, snapshot *reportstatus.Snapshot, ttl time.Duration) error
	SetIfHigherPriority(ctx context.Context, snapshot *reportstatus.Snapshot, ttl time.Duration) error
}

type WaitHub interface {
	Register(assessmentID string) (<-chan reportstatus.ChangedSignal, func())
	Notify(signal reportstatus.ChangedSignal)
	ActiveWaiters() int
}

type Config struct {
	PollInterval       time.Duration
	StatusTTL          time.Duration
	DefaultTimeout     time.Duration
	MinTimeout         time.Duration
	MaxTimeout         time.Duration
	MaxActiveWaiters   int
	SignalingEnabled   bool
	RedisMissRetryWait time.Duration
}

func DefaultConfig() Config {
	return Config{
		PollInterval:       defaultPollInterval,
		StatusTTL:          reportstatus.DefaultTTL,
		DefaultTimeout:     20 * time.Second,
		MinTimeout:         1 * time.Second,
		MaxTimeout:         25 * time.Second,
		MaxActiveWaiters:   3000,
		SignalingEnabled:   false,
		RedisMissRetryWait: defaultPollInterval,
	}
}

type Service struct {
	query    QueryService
	cache    StatusCache
	waitHub  WaitHub
	signaler *signalredis.Signaler[reportstatus.ChangedSignal]
	cfg      Config
}

func NewService(
	query QueryService,
	cache StatusCache,
	waitHub WaitHub,
	signaler *signalredis.Signaler[reportstatus.ChangedSignal],
	cfg Config,
) *Service {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	return &Service{
		query:    query,
		cache:    cache,
		waitHub:  waitHub,
		signaler: signaler,
		cfg:      cfg,
	}
}

func (s *Service) NormalizeTimeout(raw string) time.Duration {
	timeout := s.cfg.DefaultTimeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if raw == "" {
		return timeout
	}
	parsed, err := time.ParseDuration(raw + "s")
	if err == nil {
		timeout = parsed
	}
	if timeout < s.cfg.MinTimeout {
		timeout = s.cfg.MinTimeout
	}
	if timeout > s.cfg.MaxTimeout {
		timeout = s.cfg.MaxTimeout
	}
	return timeout
}

func (s *Service) Wait(ctx context.Context, testeeID, assessmentID uint64, timeout time.Duration) (*evaluation.AssessmentStatusResponse, error) {
	reportstatus.IncWaitReportRequest()
	if s == nil {
		return pendingResponse("pending", "报告生成中", 3000), nil
	}
	if timeout <= 0 {
		timeout = s.cfg.DefaultTimeout
	}

	assessmentKey := fmt.Sprintf("%d", assessmentID)
	if result, done, err := s.checkCurrentStatus(ctx, testeeID, assessmentID, assessmentKey); err != nil {
		return nil, err
	} else if done {
		return result, nil
	}

	if s.waitHub == nil || !s.cfg.SignalingEnabled {
		return s.waitByPolling(ctx, testeeID, assessmentID, assessmentKey, timeout)
	}
	if s.cfg.MaxActiveWaiters > 0 && s.waitHub.ActiveWaiters() >= s.cfg.MaxActiveWaiters {
		reportstatus.IncWaitReportProcessing()
		return pendingResponse("queued", "系统繁忙，报告生成中", 5000), nil
	}

	waitCh, unregister := s.waitHub.Register(assessmentKey)
	defer unregister()

	if result, done, err := s.checkCurrentStatus(ctx, testeeID, assessmentID, assessmentKey); err != nil {
		return nil, err
	} else if done {
		return result, nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			reportstatus.IncWaitReportProcessing()
			return pendingResponse("processing", "报告生成中", 3000), nil
		case <-timer.C:
			reportstatus.IncWaitTimeout()
			reportstatus.IncWaitReportProcessing()
			return s.processingFromCache(ctx, assessmentKey), nil
		case signal, ok := <-waitCh:
			if !ok {
				reportstatus.IncWaitReportProcessing()
				return s.processingFromCache(ctx, assessmentKey), nil
			}
			reportstatus.IncWaitReportSignalWakeup()
			if signal.Status != "completed" && signal.Status != "failed" {
				continue
			}
			result, done, err := s.checkCurrentStatus(ctx, testeeID, assessmentID, assessmentKey)
			if err != nil {
				return nil, err
			}
			if done {
				return result, nil
			}
		}
	}
}

func (s *Service) waitByPolling(
	ctx context.Context,
	testeeID, assessmentID uint64,
	assessmentKey string,
	timeout time.Duration,
) (*evaluation.AssessmentStatusResponse, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()

	for {
		if result, done, err := s.checkCurrentStatus(ctx, testeeID, assessmentID, assessmentKey); err != nil {
			return nil, err
		} else if done {
			return result, nil
		}
		if time.Now().After(deadline) {
			reportstatus.IncWaitTimeout()
			reportstatus.IncWaitReportProcessing()
			return s.processingFromCache(ctx, assessmentKey), nil
		}
		select {
		case <-ctx.Done():
			reportstatus.IncWaitReportProcessing()
			return pendingResponse("processing", "报告生成中", 3000), nil
		case <-ticker.C:
		}
	}
}

func (s *Service) checkCurrentStatus(
	ctx context.Context,
	testeeID, assessmentID uint64,
	assessmentKey string,
) (*evaluation.AssessmentStatusResponse, bool, error) {
	if s.cache != nil {
		snapshot, err := s.cache.Get(ctx, assessmentKey)
		if err != nil {
			logger.L(ctx).Warnw("wait-report redis get failed",
				"assessment_id", assessmentID,
				"error", err.Error(),
			)
		} else if snapshot != nil {
			reportstatus.IncWaitReportRedisHit()
			resp := snapshotToResponse(snapshot)
			if isTerminalStatus(snapshot.Status) {
				recordTerminalResponse(resp)
				return resp, true, nil
			}
			return resp, false, nil
		} else {
			reportstatus.IncWaitReportRedisMiss()
		}
	}
	reportstatus.IncWaitReportDBFallback()
	return s.loadStatusFromDB(ctx, testeeID, assessmentID, assessmentKey)
}

func (s *Service) loadStatusFromDB(
	ctx context.Context,
	testeeID, assessmentID uint64,
	assessmentKey string,
) (*evaluation.AssessmentStatusResponse, bool, error) {
	if s.query == nil {
		return pendingResponse("queued", "报告排队生成中", 3000), false, nil
	}
	result, err := s.query.GetMyAssessment(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, false, err
	}
	if result == nil {
		return pendingResponse("queued", "报告排队生成中", 3000), false, nil
	}

	resp := fromAssessment(result)
	if s.cache != nil {
		_ = s.cache.SetIfHigherPriority(ctx, &reportstatus.Snapshot{
			AssessmentID: assessmentKey,
			Status:       resp.Status,
			Stage:        resp.Stage,
			Message:      resp.Message,
			Reason:       resp.Reason,
			UpdatedAt:    time.Now().UTC(),
		}, s.cfg.StatusTTL)
	}
	if isTerminalStatus(resp.Status) {
		recordTerminalResponse(resp)
		return resp, true, nil
	}
	return resp, false, nil
}

func (s *Service) processingFromCache(ctx context.Context, assessmentKey string) *evaluation.AssessmentStatusResponse {
	if s.cache == nil {
		return pendingResponse("processing", "报告生成中", 3000)
	}
	snapshot, err := s.cache.Get(ctx, assessmentKey)
	if err != nil || snapshot == nil {
		return pendingResponse("processing", "报告生成中", 3000)
	}
	resp := snapshotToResponse(snapshot)
	if resp.NextPollAfterMs == 0 && !isTerminalStatus(resp.Status) {
		resp.NextPollAfterMs = nextPollAfterMs(resp.Stage, resp.Status)
	}
	return resp
}

func (s *Service) StartSignalWatcher(ctx context.Context) {
	if s == nil || !s.cfg.SignalingEnabled {
		return
	}
	StartSignalWatcher(ctx, s.signaler, s.waitHub)
}

func snapshotToResponse(s *reportstatus.Snapshot) *evaluation.AssessmentStatusResponse {
	if s == nil {
		return pendingResponse("processing", "报告生成中", 3000)
	}
	return &evaluation.AssessmentStatusResponse{
		Status:          s.Status,
		Stage:           s.Stage,
		Message:         s.Message,
		Reason:          s.Reason,
		NextPollAfterMs: nextPollAfterMs(s.Stage, s.Status),
		UpdatedAt:       s.UpdatedAt.Unix(),
	}
}

func recordTerminalResponse(resp *evaluation.AssessmentStatusResponse) {
	if resp == nil {
		return
	}
	switch resp.Status {
	case "completed":
		reportstatus.IncWaitReportCompleted()
	case "failed":
		reportstatus.IncWaitReportFailed()
	default:
		reportstatus.IncWaitReportProcessing()
	}
}

func fromAssessment(result *evaluation.AssessmentDetailResponse) *evaluation.AssessmentStatusResponse {
	if result == nil {
		return pendingResponse("queued", "报告排队生成中", 3000)
	}
	resp := &evaluation.AssessmentStatusResponse{
		Status:          mapAssessmentStatus(result.Status),
		Stage:           mapAssessmentStage(result.Status),
		Message:         mapAssessmentMessage(result.Status),
		NextPollAfterMs: nextPollAfterMs(mapAssessmentStage(result.Status), mapAssessmentStatus(result.Status)),
		UpdatedAt:       time.Now().Unix(),
	}
	if result.TotalScore != 0 {
		total := result.TotalScore
		resp.TotalScore = &total
	}
	if result.RiskLevel != "" {
		risk := result.RiskLevel
		resp.RiskLevel = &risk
	}
	if resp.Status == "failed" {
		resp.Reason = result.FailureReason
	}
	return resp
}

func mapAssessmentStatus(status string) string {
	switch status {
	case "interpreted":
		return "completed"
	case "failed":
		return "failed"
	case "submitted":
		return "processing"
	default:
		return "queued"
	}
}

func mapAssessmentStage(status string) string {
	switch status {
	case "interpreted":
		return "completed"
	case "failed":
		return "failed"
	case "submitted":
		return "processing"
	default:
		return "queued"
	}
}

func mapAssessmentMessage(status string) string {
	switch status {
	case "interpreted":
		return "报告已生成"
	case "failed":
		return "报告生成失败"
	case "submitted":
		return "报告生成中"
	default:
		return "报告排队生成中"
	}
}

func pendingResponse(stage, message string, next int) *evaluation.AssessmentStatusResponse {
	return &evaluation.AssessmentStatusResponse{
		Status:          "processing",
		Stage:           stage,
		Message:         message,
		NextPollAfterMs: next,
		UpdatedAt:       time.Now().Unix(),
	}
}

func nextPollAfterMs(stage, status string) int {
	if isTerminalStatus(status) {
		return 0
	}
	switch stage {
	case "scoring", "interpreting":
		return 2000
	case "queued", "processing":
		return 3000
	default:
		return 5000
	}
}

func isTerminalStatus(status string) bool {
	return status == "completed" || status == "failed"
}
