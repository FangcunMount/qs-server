package waiter

import (
	"context"
	"sync"
	"time"
)

// Logger 日志接口（避免循环依赖）
type Logger interface {
	Debugw(msg string, keysAndValues ...interface{})
	Infow(msg string, keysAndValues ...interface{})
	Warnw(msg string, keysAndValues ...interface{})
}

// StatusSummary 状态摘要（用于长轮询响应）
type StatusSummary struct {
	Status     string   `json:"status"` // pending/submitted/interpreted/failed
	TotalScore *float64 `json:"total_score,omitempty"`
	RiskLevel  *string  `json:"risk_level,omitempty"`
	UpdatedAt  int64    `json:"updated_at"` // Unix timestamp
}

// WaiterRegistry 等待队列注册表
// 用于长轮询机制：当评估完成时，通知所有等待该测评的客户端
type WaiterRegistry struct {
	mu      sync.RWMutex
	waiters map[uint64][]chan StatusSummary
	logger  Logger
}

// NewWaiterRegistry 创建等待队列注册表
func NewWaiterRegistry(log Logger) *WaiterRegistry {
	return &WaiterRegistry{
		waiters: make(map[uint64][]chan StatusSummary),
		logger:  log,
	}
}

// Add 添加等待者
// assessmentID: 测评ID
// ch: 用于接收状态更新的 channel
func (r *WaiterRegistry) Add(assessmentID uint64, ch chan StatusSummary) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.waiters[assessmentID] = append(r.waiters[assessmentID], ch)
	r.logger.Debugw("waiter added",
		"assessment_id", assessmentID,
		"total_waiters", len(r.waiters[assessmentID]),
	)
}

// Remove 移除等待者
func (r *WaiterRegistry) Remove(assessmentID uint64, ch chan StatusSummary) {
	r.mu.Lock()
	defer r.mu.Unlock()

	channels, ok := r.waiters[assessmentID]
	if !ok {
		return
	}

	// 从切片中移除指定的 channel
	for i, c := range channels {
		if c == ch {
			r.waiters[assessmentID] = append(channels[:i], channels[i+1:]...)
			break
		}
	}

	// 如果没有等待者了，删除该测评的条目
	if len(r.waiters[assessmentID]) == 0 {
		delete(r.waiters, assessmentID)
	}

	r.logger.Debugw("waiter removed",
		"assessment_id", assessmentID,
		"remaining_waiters", len(r.waiters[assessmentID]),
	)
}

// Notify 通知所有等待者
// 当评估完成时调用此方法，唤醒所有等待该测评的客户端
func (r *WaiterRegistry) Notify(ctx context.Context, assessmentID uint64, summary StatusSummary) {
	r.mu.RLock()
	channels, ok := r.waiters[assessmentID]
	r.mu.RUnlock()

	if !ok || len(channels) == 0 {
		return
	}

	// 设置更新时间戳
	summary.UpdatedAt = time.Now().Unix()

	// 非阻塞写入所有等待的 channel
	notifiedCount := 0
	for _, ch := range channels {
		select {
		case ch <- summary:
			notifiedCount++
		case <-ctx.Done():
			return
		default:
			// channel 已满或已关闭，跳过
			r.logger.Warnw("failed to notify waiter, channel full or closed",
				"assessment_id", assessmentID,
			)
		}
	}

	r.logger.Infow("notified waiters",
		"assessment_id", assessmentID,
		"notified_count", notifiedCount,
		"total_waiters", len(channels),
		"status", summary.Status,
	)

	// 清理已通知的等待者（可选，也可以让客户端自己清理）
	// 这里不清理，让客户端在收到通知后自己调用 Remove
}

// GetWaiterCount 获取指定测评的等待者数量（用于监控）
func (r *WaiterRegistry) GetWaiterCount(assessmentID uint64) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.waiters[assessmentID])
}

// GetTotalWaiterCount 获取总等待者数量（用于监控）
func (r *WaiterRegistry) GetTotalWaiterCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	total := 0
	for _, channels := range r.waiters {
		total += len(channels)
	}
	return total
}
