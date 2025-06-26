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

	// 注册用户服务（使用新的DDD架构）
	RegisterService(
		"user",
		func(container *AutoDiscoveryContainer) (interface{}, error) {
			repo, exists := container.GetRepository("user")
			if !exists {
				return nil, fmt.Errorf("user repository not found")
			}
			return userApp.NewService(repo.(storage.UserRepository)), nil
		},
		reflect.TypeOf((*userApp.Service)(nil)).Elem(),
		"user", // 依赖用户存储库
	)

	// 注册用户处理器
	RegisterHandler(
		"user",
		func(container *AutoDiscoveryContainer) (interface{}, error) {
			service, exists := container.GetService("user")
			if !exists {
				return nil, fmt.Errorf("user service not found")
			}
			return user.NewHandler(service.(*userApp.Service)), nil
		},
		"user", // 依赖用户服务
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

	// 注册问卷服务（使用DataCoordinator）
	RegisterService(
		"questionnaire",
		func(container *AutoDiscoveryContainer) (interface{}, error) {
			mysqlRepo, exists := container.GetRepository("questionnaire-mysql")
			if !exists {
				return nil, fmt.Errorf("questionnaire MySQL repository not found")
			}

			// 如果MongoDB可用，使用多数据源模式
			if mongoRepo, exists := container.GetRepository("questionnaire-mongo"); exists {
				return questionnaireApp.NewService(
					mysqlRepo.(storage.QuestionnaireRepository),
					mongoRepo.(storage.QuestionnaireDocumentRepository),
				), nil
			}

			// 否则使用单数据源模式
			return questionnaireApp.NewServiceWithSingleRepo(
				mysqlRepo.(storage.QuestionnaireRepository),
			), nil
		},
		reflect.TypeOf((*questionnaireApp.Service)(nil)).Elem(),
		"questionnaire-mysql", // 依赖MySQL存储库
	)

	// 注册问卷处理器
	RegisterHandler(
		"questionnaire",
		func(container *AutoDiscoveryContainer) (interface{}, error) {
			service, exists := container.GetService("questionnaire")
			if !exists {
				return nil, fmt.Errorf("questionnaire service not found")
			}
			return questionnaire.NewHandler(service.(*questionnaireApp.Service)), nil
		},
		"questionnaire", // 依赖问卷服务
	)
}
