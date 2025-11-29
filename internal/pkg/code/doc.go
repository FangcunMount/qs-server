// Package code defines error codes for questionnaire-scale platform.
//
// Error code ranges:
//
//	100xxx: 基础错误 (base.go)
//	  - 100001~100099: 基础错误
//	  - 100101~100199: 数据库错误
//	  - 100201~100299: 认证授权错误
//	  - 100301~100399: 编码解码错误
//	  - 100401~100499: 模块错误
//
//	110xxx: 用户错误 (apiserver.go)
//	111xxx: 答卷错误 (answersheet.go)
//	112xxx: 测评错误 (assessment.go)
//	113xxx: 计算错误 (calculation.go)
//	114xxx: 量表错误 (medical-scale.go)
//	115xxx: 报告错误 (interpret-report.go)
//	120xxx: 问卷错误 (questionnaire.go)
//
// Allowed HTTP status codes:
//
//	StatusOK                  = 200 // RFC 7231, 6.3.1
//	StatusBadRequest          = 400 // RFC 7231, 6.5.1
//	StatusUnauthorized        = 401 // RFC 7235, 3.1
//	StatusForbidden           = 403 // RFC 7231, 6.5.3
//	StatusNotFound            = 404 // RFC 7231, 6.5.4
//	StatusConflict            = 409 // RFC 7231, 6.5.8
//	StatusInternalServerError = 500 // RFC 7231, 6.6.1
package code
