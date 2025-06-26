package errors

import (
	"context"
	"fmt"
)

// 这个文件提供新错误码系统的使用示例

// ExampleUsage 演示新错误码系统的使用方法
func ExampleUsage() {
	ctx := context.Background()

	// 1. 直接创建错误码
	err1 := NewWithCode(ErrUserNotFound, "用户ID为%s的用户不存在", "12345")
	fmt.Printf("错误1: %v\n", err1)
	fmt.Printf("错误码: %d\n", CodeFromError(err1))
	fmt.Printf("HTTP状态码: %d\n", HTTPStatusFromError(err1))
	fmt.Printf("错误消息: %s\n", MessageFromError(err1))

	// 2. 包装现有错误
	originalErr := fmt.Errorf("database connection failed")
	err2 := WrapWithCode(originalErr, ErrDatabaseConnection, "连接MySQL数据库失败")
	fmt.Printf("\n错误2: %v\n", err2)
	fmt.Printf("错误码: %d\n", CodeFromError(err2))
	fmt.Printf("HTTP状态码: %d\n", HTTPStatusFromError(err2))

	// 3. 检查错误码
	if IsCode(err1, ErrUserNotFound) {
		fmt.Printf("\n错误1确实是用户不存在错误\n")
	}

	// 4. 解析错误信息
	if coder := ParseCoder(err2); coder != nil {
		fmt.Printf("\n解析的错误信息:\n")
		fmt.Printf("- 错误码: %d\n", coder.Code())
		fmt.Printf("- HTTP状态码: %d\n", coder.HTTPStatus())
		fmt.Printf("- 错误消息: %s\n", coder.String())
		fmt.Printf("- 参考文档: %s\n", coder.Reference())
	}

	// 在实际业务代码中的使用场景：
	ExampleBusinessLogic(ctx)
}

// ExampleBusinessLogic 演示在业务逻辑中的实际使用
func ExampleBusinessLogic(ctx context.Context) {
	// 模拟用户服务
	userID := "nonexistent-user"

	// 业务逻辑中的错误处理
	user, err := findUser(ctx, userID)
	if err != nil {
		// 错误会被自动传播到Handler层，Handler会智能解析错误码
		fmt.Printf("\n业务逻辑错误: %v\n", err)
		return
	}

	fmt.Printf("找到用户: %v\n", user)
}

// 模拟的查找用户函数
func findUser(ctx context.Context, userID string) (interface{}, error) {
	// 模拟数据库查询失败
	if userID == "nonexistent-user" {
		return nil, NewWithCode(ErrUserNotFound, "用户 %s 不存在", userID)
	}

	// 模拟网络错误
	if userID == "network-error" {
		dbErr := fmt.Errorf("connection timeout")
		return nil, WrapWithCode(dbErr, ErrDatabaseTimeout, "查询用户时数据库超时")
	}

	return map[string]string{"id": userID, "name": "测试用户"}, nil
}

// ExampleErrorCodeRanges 展示错误码范围分配
func ExampleErrorCodeRanges() {
	fmt.Println("\n错误码范围分配:")
	fmt.Printf("通用错误 (10xxxx): %d - %d\n", ErrSuccess, ErrGatewayTimeout)
	fmt.Printf("用户错误 (11xxxx): %d - %d\n", ErrUserNotFound, ErrUserPhoneNotVerified)
	fmt.Printf("问卷错误 (12xxxx): %d - %d\n", ErrQuestionnaireNotFound, ErrAnswerDeleteFailed)
	fmt.Printf("数据库错误 (15xxxx): %d - %d\n", ErrDatabase, ErrRedisScript)
}

// ExampleHandlerIntegration 展示与Handler层的集成
func ExampleHandlerIntegration() {
	fmt.Println("\n=== Handler层集成示例 ===")

	// 在Handler中，你只需要调用：
	// h.ErrorResponse(c, err)
	//
	// BaseHandler会自动：
	// 1. 解析错误类型（内部错误码 vs 应用层错误）
	// 2. 选择合适的HTTP状态码
	// 3. 生成标准化的响应格式
	// 4. 记录错误日志

	fmt.Println("Handler使用方式:")
	fmt.Println("```go")
	fmt.Println("func (h *UserHandler) GetUser(c *gin.Context) {")
	fmt.Println("    user, err := h.userService.GetUser(ctx, query)")
	fmt.Println("    if err != nil {")
	fmt.Println("        h.ErrorResponse(c, err)  // 智能错误处理")
	fmt.Println("        return")
	fmt.Println("    }")
	fmt.Println("    h.SuccessResponse(c, user)")
	fmt.Println("}")
	fmt.Println("```")
}

// ExampleResponseFormat 展示标准化响应格式
func ExampleResponseFormat() {
	fmt.Println("\n=== 标准化响应格式 ===")

	fmt.Println("成功响应:")
	fmt.Println(`{
  "code": 100000,
  "message": "操作成功",
  "data": {...}
}`)

	fmt.Println("\n错误响应:")
	fmt.Println(`{
  "code": 110000,
  "message": "用户不存在", 
  "reference": ""
}`)
}
