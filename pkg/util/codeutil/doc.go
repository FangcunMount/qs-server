// Package codeutil 提供编码生成和管理的工具函数
//
// 功能特性:
// 1. Sonyflake 算法生成全局唯一编码
// 2. Base62 编码,生成短小的字符串ID
// 3. 统一的 Code 类型,支持各种实体编码
// 4. 类型别名支持:QuestionnaireCode, QuestionCode, OptionCode
//
// 使用示例:
//
//	// 生成全局唯一编码
//	code, err := codeutil.GenerateNewCode()
//	if err != nil {
//		// 处理错误
//	}
//	fmt.Println(code.Value()) // 例如: "2KxFg3N"
//
//	// 使用预定义值创建编码
//	code := codeutil.NewCode("CUSTOM001")
//
//	// 生成问卷编码
//	qCode, err := codeutil.GenerateQuestionnaireCode()
//
//	// 生成问题编码
//	questCode, err := codeutil.GenerateQuestionCode()
//
//	// 生成选项编码
//	optCode, err := codeutil.GenerateOptionCode()
//
//	// 编码比较
//	if code1.Equals(code2) {
//		// 相等
//	}
//
//	// 检查空编码
//	if code.IsEmpty() {
//		// 空编码
//	}
package codeutil
