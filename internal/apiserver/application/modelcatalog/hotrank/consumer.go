package hotrank

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/eventcodec"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/hotrank"
)

// NewEventConsumer 创建事件消费者，用于将答案卡提交事件投影为提交事实
func NewEventConsumer(projection hotrank.Projection) func(context.Context, string, []byte) error {
	return func(ctx context.Context, eventType string, payload []byte) error {
		// 验证投影和事件类型
		if eventType != domainAnswerSheet.EventTypeSubmitted {
			return nil
		}
		if projection == nil {
			return fmt.Errorf("modelcatalog hot-rank projection is unavailable")
		}

		// 解码事件
		env, err := eventcodec.DecodeEnvelope(payload)
		if err != nil {
			return err
		}

		// 解码数据
		var data domainAnswerSheet.AnswerSheetSubmittedData
		if err := json.Unmarshal(env.Data, &data); err != nil {
			return fmt.Errorf("decode answersheet submitted payload: %w", err)
		}

		// 获取提交时间
		submittedAt := data.SubmittedAt
		if submittedAt.IsZero() {
			submittedAt = env.OccurredAt
		}

		// 投影提交事实
		return projection.ProjectSubmission(ctx, hotrank.SubmissionFact{EventID: env.ID, QuestionnaireCode: data.QuestionnaireCode, SubmittedAt: submittedAt})
	}
}
