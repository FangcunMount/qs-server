package answersheet

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func validateListMyAnswerSheetsDTO(l *logger.RequestLogger, dto ListMyAnswerSheetsDTO) error {
	if dto.FillerID == 0 {
		l.Warnw("填写人 ID 为空",
			"action", "list_my_answersheets",
			"result", "invalid_params",
		)
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "填写人ID不能为空")
	}
	if dto.Page <= 0 {
		l.Warnw("页码有效性检查失败",
			"action", "list_my_answersheets",
			"page", dto.Page,
			"result", "invalid_params",
		)
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "页码必须大于0")
	}
	if dto.PageSize <= 0 {
		l.Warnw("每页数量有效性检查失败",
			"action", "list_my_answersheets",
			"page_size", dto.PageSize,
			"result", "invalid_params",
		)
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "每页数量必须大于0")
	}
	if dto.PageSize > 100 {
		l.Warnw("每页数量超限",
			"action", "list_my_answersheets",
			"page_size", dto.PageSize,
			"max_size", 100,
			"result", "invalid_params",
		)
		return errors.WithCode(errorCode.ErrAnswerSheetInvalid, "每页数量不能超过100")
	}
	return nil
}

func (s *submissionService) listMyAnswerSheetRows(
	ctx context.Context,
	l *logger.RequestLogger,
	dto ListMyAnswerSheetsDTO,
) ([]surveyreadmodel.AnswerSheetSummaryRow, int64, error) {
	l.Debugw("开始查询答卷列表",
		"filler_id", dto.FillerID,
		"page", dto.Page,
		"page_size", dto.PageSize,
	)
	filter := surveyreadmodel.AnswerSheetFilter{FillerID: &dto.FillerID}
	sheets, err := s.reader.ListAnswerSheets(ctx, filter, surveyreadmodel.PageRequest{Page: dto.Page, PageSize: dto.PageSize})
	if err != nil {
		l.Errorw("查询答卷列表失败",
			"action", "list_my_answersheets",
			"filler_id", dto.FillerID,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, 0, errors.WrapC(err, errorCode.ErrDatabase, "查询答卷列表失败")
	}

	l.Debugw("查询答卷总数",
		"filler_id", dto.FillerID,
	)
	total, err := s.reader.CountAnswerSheets(ctx, filter)
	if err != nil {
		l.Errorw("获取答卷总数失败",
			"action", "list_my_answersheets",
			"filler_id", dto.FillerID,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, 0, errors.WrapC(err, errorCode.ErrDatabase, "获取答卷总数失败")
	}
	return sheets, total, nil
}
