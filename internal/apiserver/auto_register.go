package apiserver

import (
	"fmt"
	"reflect"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers/user"
	mongoQuestionnaireAdapter "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/storage/mongodb/questionnaire"
	mysqlQuestionnaireAdapter "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/storage/mysql/questionnaire"
	mysqlUserAdapter "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/storage/mysql/user"
	questionnaireApp "github.com/yshujie/questionnaire-scale/internal/apiserver/application/questionnaire"
	userApp "github.com/yshujie/questionnaire-scale/internal/apiserver/application/user"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

func init() {
	// 注册所有组件
	registerUserComponents()
	registerQuestionnaireComponents()
}

// registerUserComponents 注册用户相关组件
func registerUserComponents() {
	// 注册用户存储库
	RegisterRepository(
		"user",
		func(container *AutoDiscoveryContainer) (interface{}, error) {
			return mysqlUserAdapter.NewRepository(container.GetMySQLDB()), nil
		},
		reflect.TypeOf((*storage.UserRepository)(nil)).Elem(),
	)

	// 注册用户编辑器
	RegisterService(
		"user-editor",
		func(container *AutoDiscoveryContainer) (interface{}, error) {
			repo, exists := container.GetRepository("user")
			if !exists {
				return nil, fmt.Errorf("user repository not found")
			}
			return userApp.NewUserEditor(repo.(storage.UserRepository)), nil
		},
		reflect.TypeOf((*userApp.UserEditor)(nil)).Elem(),
		"user", // 依赖用户存储库
	)

	// 注册用户查询器
	RegisterService(
		"user-query",
		func(container *AutoDiscoveryContainer) (interface{}, error) {
			repo, exists := container.GetRepository("user")
			if !exists {
				return nil, fmt.Errorf("user repository not found")
			}
			return userApp.NewUserQuery(repo.(storage.UserRepository)), nil
		},
		reflect.TypeOf((*userApp.UserQuery)(nil)).Elem(),
		"user", // 依赖用户存储库
	)

	// 注册用户处理器
	RegisterHandler(
		"user",
		func(container *AutoDiscoveryContainer) (interface{}, error) {
			editor, editorExists := container.GetService("user-editor")
			if !editorExists {
				return nil, fmt.Errorf("user editor service not found")
			}
			query, queryExists := container.GetService("user-query")
			if !queryExists {
				return nil, fmt.Errorf("user query service not found")
			}
			return user.NewHandler(
				editor.(*userApp.UserEditor),
				query.(*userApp.UserQuery),
			), nil
		},
		"user-editor", "user-query", // 依赖用户编辑器和查询器
	)
}

// registerQuestionnaireComponents 注册问卷相关组件
func registerQuestionnaireComponents() {
	// 注册MySQL问卷存储库
	RegisterRepository(
		"questionnaire-mysql",
		func(container *AutoDiscoveryContainer) (interface{}, error) {
			return mysqlQuestionnaireAdapter.NewRepository(container.GetMySQLDB(), nil, ""), nil
		},
		reflect.TypeOf((*storage.QuestionnaireRepository)(nil)).Elem(),
	)

	// 注册MongoDB问卷文档存储库（如果可用）
	RegisterRepository(
		"questionnaire-mongo",
		func(container *AutoDiscoveryContainer) (interface{}, error) {
			if container.GetMongoClient() == nil {
				return nil, fmt.Errorf("MongoDB not available")
			}
			return mongoQuestionnaireAdapter.NewRepository(
				container.GetMongoClient(),
				container.GetMongoDatabase(),
			), nil
		},
		reflect.TypeOf((*storage.QuestionnaireDocumentRepository)(nil)).Elem(),
	)

	// 注册问卷编辑器
	RegisterService(
		"questionnaire-editor",
		func(container *AutoDiscoveryContainer) (interface{}, error) {
			mysqlRepo, exists := container.GetRepository("questionnaire-mysql")
			if !exists {
				return nil, fmt.Errorf("questionnaire MySQL repository not found")
			}
			return questionnaireApp.NewQuestionnaireEditor(
				mysqlRepo.(storage.QuestionnaireRepository),
			), nil
		},
		reflect.TypeOf((*questionnaireApp.QuestionnaireEditor)(nil)).Elem(),
		"questionnaire-mysql", // 依赖MySQL存储库
	)

	// 注册问卷查询器
	RegisterService(
		"questionnaire-query",
		func(container *AutoDiscoveryContainer) (interface{}, error) {
			mysqlRepo, exists := container.GetRepository("questionnaire-mysql")
			if !exists {
				return nil, fmt.Errorf("questionnaire MySQL repository not found")
			}
			return questionnaireApp.NewQuestionnaireQuery(
				mysqlRepo.(storage.QuestionnaireRepository),
			), nil
		},
		reflect.TypeOf((*questionnaireApp.QuestionnaireQuery)(nil)).Elem(),
		"questionnaire-mysql", // 依赖MySQL存储库
	)

	// 注册问卷处理器
	RegisterHandler(
		"questionnaire",
		func(container *AutoDiscoveryContainer) (interface{}, error) {
			editor, editorExists := container.GetService("questionnaire-editor")
			if !editorExists {
				return nil, fmt.Errorf("questionnaire editor service not found")
			}
			query, queryExists := container.GetService("questionnaire-query")
			if !queryExists {
				return nil, fmt.Errorf("questionnaire query service not found")
			}
			return questionnaire.NewHandler(
				editor.(*questionnaireApp.QuestionnaireEditor),
				query.(*questionnaireApp.QuestionnaireQuery),
			), nil
		},
		"questionnaire-editor", "questionnaire-query", // 依赖问卷编辑器和查询器
	)
}
