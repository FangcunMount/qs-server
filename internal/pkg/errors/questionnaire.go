package errors

import "net/http"

// 问卷相关错误码 (12xxxx)
const (
	// ErrQuestionnaireNotFound - 问卷不存在
	ErrQuestionnaireNotFound int = iota + 120000

	// ErrQuestionnaireAlreadyExists - 问卷已存在
	ErrQuestionnaireAlreadyExists
	// ErrQuestionnaireCodeAlreadyExists - 问卷代码已存在
	ErrQuestionnaireCodeAlreadyExists
	// ErrQuestionnaireInvalidStatus - 问卷状态无效
	ErrQuestionnaireInvalidStatus
	// ErrQuestionnaireAlreadyPublished - 问卷已发布
	ErrQuestionnaireAlreadyPublished
	// ErrQuestionnaireNotPublished - 问卷未发布
	ErrQuestionnaireNotPublished
	// ErrQuestionnaireExpired - 问卷已过期
	ErrQuestionnaireExpired
	// ErrQuestionnaireInactive - 问卷未激活
	ErrQuestionnaireInactive
	// ErrQuestionnaireValidationFailed - 问卷验证失败
	ErrQuestionnaireValidationFailed
	// ErrQuestionnaireCreateFailed - 问卷创建失败
	ErrQuestionnaireCreateFailed
	// ErrQuestionnaireUpdateFailed - 问卷更新失败
	ErrQuestionnaireUpdateFailed
	// ErrQuestionnaireDeleteFailed - 问卷删除失败
	ErrQuestionnaireDeleteFailed
	// ErrQuestionnairePublishFailed - 问卷发布失败
	ErrQuestionnairePublishFailed
	// ErrQuestionnaireQueryFailed - 问卷查询失败
	ErrQuestionnaireQueryFailed
	// ErrQuestionnaireInvalidID - 问卷ID无效
	ErrQuestionnaireInvalidID
	// ErrQuestionnaireInvalidCode - 问卷代码无效
	ErrQuestionnaireInvalidCode
	// ErrQuestionnaireInvalidTitle - 问卷标题无效
	ErrQuestionnaireInvalidTitle
	// ErrQuestionnaireInvalidDescription - 问卷描述无效
	ErrQuestionnaireInvalidDescription
	// ErrQuestionnaireInvalidCreator - 问卷创建者无效
	ErrQuestionnaireInvalidCreator
	// ErrQuestionnaireAccessDenied - 问卷访问权限不足
	ErrQuestionnaireAccessDenied
	// ErrQuestionnaireOperationNotAllowed - 问卷操作不被允许
	ErrQuestionnaireOperationNotAllowed
	// ErrQuestionnaireDataInconsistent - 问卷数据不一致
	ErrQuestionnaireDataInconsistent
	// ErrQuestionnaireSyncFailed - 问卷同步失败
	ErrQuestionnaireSyncFailed
	// ErrQuestionnaireBackupFailed - 问卷备份失败
	ErrQuestionnaireBackupFailed
	// ErrQuestionnaireRestoreFailed - 问卷恢复失败
	ErrQuestionnaireRestoreFailed
	// ErrQuestionnaireCloneFailed - 问卷克隆失败
	ErrQuestionnaireCloneFailed
	// ErrQuestionnaireImportFailed - 问卷导入失败
	ErrQuestionnaireImportFailed
	// ErrQuestionnaireExportFailed - 问卷导出失败
	ErrQuestionnaireExportFailed

	// 问题相关错误
	// ErrQuestionNotFound - 问题不存在
	ErrQuestionNotFound
	// ErrQuestionInvalidType - 问题类型无效
	ErrQuestionInvalidType
	// ErrQuestionInvalidOptions - 问题选项无效
	ErrQuestionInvalidOptions
	// ErrQuestionValidationFailed - 问题验证失败
	ErrQuestionValidationFailed
	// ErrQuestionCreateFailed - 问题创建失败
	ErrQuestionCreateFailed
	// ErrQuestionUpdateFailed - 问题更新失败
	ErrQuestionUpdateFailed
	// ErrQuestionDeleteFailed - 问题删除失败
	ErrQuestionDeleteFailed
	// ErrQuestionInvalidOrder - 问题顺序无效
	ErrQuestionInvalidOrder
	// ErrQuestionRequiredEmpty - 必填问题为空
	ErrQuestionRequiredEmpty
	// ErrQuestionLogicError - 问题逻辑错误
	ErrQuestionLogicError

	// 答案相关错误
	// ErrAnswerNotFound - 答案不存在
	ErrAnswerNotFound
	// ErrAnswerInvalidFormat - 答案格式无效
	ErrAnswerInvalidFormat
	// ErrAnswerValidationFailed - 答案验证失败
	ErrAnswerValidationFailed
	// ErrAnswerSubmitFailed - 答案提交失败
	ErrAnswerSubmitFailed
	// ErrAnswerUpdateFailed - 答案更新失败
	ErrAnswerUpdateFailed
	// ErrAnswerDeleteFailed - 答案删除失败
	ErrAnswerDeleteFailed

	// 额外的问卷相关错误码
	// ErrQuestionnairePublished - 问卷已发布，不能修改
	ErrQuestionnairePublished
	// ErrQuestionnaireInvalidQuestion - 问题无效
	ErrQuestionnaireInvalidQuestion
	// ErrQuestionnaireInvalidContent - 问卷内容无效
	ErrQuestionnaireInvalidContent
	// ErrQuestionnaireUnpublishFailed - 取消发布问卷失败
	ErrQuestionnaireUnpublishFailed
	// ErrQuestionnaireArchiveFailed - 归档问卷失败
	ErrQuestionnaireArchiveFailed
)

// 问卷错误码注册
func init() {
	register(ErrQuestionnaireNotFound, http.StatusNotFound, "问卷不存在", "")
	register(ErrQuestionnaireAlreadyExists, http.StatusConflict, "问卷已存在", "")
	register(ErrQuestionnaireCodeAlreadyExists, http.StatusConflict, "问卷代码已存在", "")
	register(ErrQuestionnaireInvalidStatus, http.StatusBadRequest, "问卷状态无效", "")
	register(ErrQuestionnaireAlreadyPublished, http.StatusConflict, "问卷已发布", "")
	register(ErrQuestionnaireNotPublished, http.StatusBadRequest, "问卷未发布", "")
	register(ErrQuestionnaireExpired, http.StatusGone, "问卷已过期", "")
	register(ErrQuestionnaireInactive, http.StatusForbidden, "问卷未激活", "")
	register(ErrQuestionnaireValidationFailed, http.StatusBadRequest, "问卷验证失败", "")
	register(ErrQuestionnaireCreateFailed, http.StatusInternalServerError, "问卷创建失败", "")
	register(ErrQuestionnaireUpdateFailed, http.StatusInternalServerError, "问卷更新失败", "")
	register(ErrQuestionnaireDeleteFailed, http.StatusInternalServerError, "问卷删除失败", "")
	register(ErrQuestionnairePublishFailed, http.StatusInternalServerError, "问卷发布失败", "")
	register(ErrQuestionnaireQueryFailed, http.StatusInternalServerError, "问卷查询失败", "")
	register(ErrQuestionnaireInvalidID, http.StatusBadRequest, "问卷ID无效", "")
	register(ErrQuestionnaireInvalidCode, http.StatusBadRequest, "问卷代码无效", "")
	register(ErrQuestionnaireInvalidTitle, http.StatusBadRequest, "问卷标题无效", "")
	register(ErrQuestionnaireInvalidDescription, http.StatusBadRequest, "问卷描述无效", "")
	register(ErrQuestionnaireInvalidCreator, http.StatusBadRequest, "问卷创建者无效", "")
	register(ErrQuestionnaireAccessDenied, http.StatusForbidden, "问卷访问权限不足", "")
	register(ErrQuestionnaireOperationNotAllowed, http.StatusForbidden, "问卷操作不被允许", "")
	register(ErrQuestionnaireDataInconsistent, http.StatusConflict, "问卷数据不一致", "")
	register(ErrQuestionnaireSyncFailed, http.StatusInternalServerError, "问卷同步失败", "")
	register(ErrQuestionnaireBackupFailed, http.StatusInternalServerError, "问卷备份失败", "")
	register(ErrQuestionnaireRestoreFailed, http.StatusInternalServerError, "问卷恢复失败", "")
	register(ErrQuestionnaireCloneFailed, http.StatusInternalServerError, "问卷克隆失败", "")
	register(ErrQuestionnaireImportFailed, http.StatusInternalServerError, "问卷导入失败", "")
	register(ErrQuestionnaireExportFailed, http.StatusInternalServerError, "问卷导出失败", "")

	// 问题相关错误
	register(ErrQuestionNotFound, http.StatusNotFound, "问题不存在", "")
	register(ErrQuestionInvalidType, http.StatusBadRequest, "问题类型无效", "")
	register(ErrQuestionInvalidOptions, http.StatusBadRequest, "问题选项无效", "")
	register(ErrQuestionValidationFailed, http.StatusBadRequest, "问题验证失败", "")
	register(ErrQuestionCreateFailed, http.StatusInternalServerError, "问题创建失败", "")
	register(ErrQuestionUpdateFailed, http.StatusInternalServerError, "问题更新失败", "")
	register(ErrQuestionDeleteFailed, http.StatusInternalServerError, "问题删除失败", "")
	register(ErrQuestionInvalidOrder, http.StatusBadRequest, "问题顺序无效", "")
	register(ErrQuestionRequiredEmpty, http.StatusBadRequest, "必填问题不能为空", "")
	register(ErrQuestionLogicError, http.StatusBadRequest, "问题逻辑错误", "")

	// 答案相关错误
	register(ErrAnswerNotFound, http.StatusNotFound, "答案不存在", "")
	register(ErrAnswerInvalidFormat, http.StatusBadRequest, "答案格式无效", "")
	register(ErrAnswerValidationFailed, http.StatusBadRequest, "答案验证失败", "")
	register(ErrAnswerSubmitFailed, http.StatusInternalServerError, "答案提交失败", "")
	register(ErrAnswerUpdateFailed, http.StatusInternalServerError, "答案更新失败", "")
	register(ErrAnswerDeleteFailed, http.StatusInternalServerError, "答案删除失败", "")

	// 额外的问卷相关错误码
	register(ErrQuestionnairePublished, http.StatusBadRequest, "问卷已发布，不能修改", "")
	register(ErrQuestionnaireInvalidQuestion, http.StatusBadRequest, "问题无效", "")
	register(ErrQuestionnaireInvalidContent, http.StatusBadRequest, "问卷内容无效", "")
	register(ErrQuestionnaireUnpublishFailed, http.StatusInternalServerError, "取消发布问卷失败", "")
	register(ErrQuestionnaireArchiveFailed, http.StatusInternalServerError, "归档问卷失败", "")
}
