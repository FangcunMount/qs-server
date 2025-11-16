package repository

import (
	"context"

	answersheetpb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/answersheet"
	interpretreportpb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/interpret-report"
	medicalscalepb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/medical-scale"
	questionnairepb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// QuestionnaireRepository 问卷仓储接口
type QuestionnaireRepository interface {
	GetQuestionnaire(ctx context.Context, code string) (*questionnairepb.Questionnaire, error)
}

// AnswerSheetRepository 答卷仓储接口
type AnswerSheetRepository interface {
	GetAnswerSheet(ctx context.Context, id meta.ID) (*answersheetpb.AnswerSheet, error)
	SaveAnswerSheetScores(ctx context.Context, answerSheetID meta.ID, totalScore uint32, answers []*answersheetpb.Answer) error
}

// MedicalScaleRepository 医学量表仓储接口
type MedicalScaleRepository interface {
	GetMedicalScaleByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*medicalscalepb.MedicalScale, error)
}

// InterpretReportRepository 解读报告仓储接口
type InterpretReportRepository interface {
	SaveInterpretReport(ctx context.Context, answerSheetID meta.ID, medicalScaleCode, title, description string, interpretItems []*interpretreportpb.InterpretItem) (*interpretreportpb.InterpretReport, error)
}
