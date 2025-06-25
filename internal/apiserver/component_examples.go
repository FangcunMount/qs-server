package apiserver

// 这个文件展示如何使用新的容器架构轻松添加其他业务模块
// 当需要添加新模块时，只需要按照这个模式注册组件即可

// "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/api/http/handlers"
// "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/storage/mysql"
// "github.com/yshujie/questionnaire-scale/internal/apiserver/application/services"
// "github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"

// registerScaleComponents 注册量表相关组件
// 当需要添加量表功能时，取消注释并实现这个方法
func (c *Container) registerScaleComponents() {
	// // 注册量表仓储
	// c.RegisterComponent("scaleRepo", RepositoryType, func(container *Container) (interface{}, error) {
	// 	return mysqlAdapter.NewScaleRepository(container.mysqlDB), nil
	// })

	// // 注册量表服务
	// c.RegisterComponent("scaleService", ServiceType, func(container *Container) (interface{}, error) {
	// 	repo, err := container.GetComponent("scaleRepo")
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	return services.NewScaleService(repo.(storage.ScaleRepository)), nil
	// })

	// // 注册量表处理器
	// c.RegisterComponent("scaleHandler", HandlerType, func(container *Container) (interface{}, error) {
	// 	service, err := container.GetComponent("scaleService")
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	return handlers.NewScaleHandler(service.(*services.ScaleService)), nil
	// })
}

// registerResponseComponents 注册答卷相关组件
// 当需要添加答卷功能时，取消注释并实现这个方法
func (c *Container) registerResponseComponents() {
	// // 注册答卷仓储
	// c.RegisterComponent("responseRepo", RepositoryType, func(container *Container) (interface{}, error) {
	// 	return mysqlAdapter.NewResponseRepository(container.mysqlDB), nil
	// })

	// // 注册答卷文档仓储（MongoDB）
	// if c.mongoClient != nil {
	// 	c.RegisterComponent("responseDocumentRepo", RepositoryType, func(container *Container) (interface{}, error) {
	// 		return mongoAdapter.NewResponseDocumentRepository(container.mongoClient, container.mongoDatabase), nil
	// 	})
	// }

	// // 注册答卷服务
	// c.RegisterComponent("responseService", ServiceType, func(container *Container) (interface{}, error) {
	// 	repo, err := container.GetComponent("responseRepo")
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	return services.NewResponseService(repo.(storage.ResponseRepository)), nil
	// })

	// // 注册答卷处理器
	// c.RegisterComponent("responseHandler", HandlerType, func(container *Container) (interface{}, error) {
	// 	service, err := container.GetComponent("responseService")
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	return handlers.NewResponseHandler(service.(*services.ResponseService)), nil
	// })
}

// registerEvaluationComponents 注册评估相关组件
// 当需要添加评估功能时，取消注释并实现这个方法
func (c *Container) registerEvaluationComponents() {
	// // 注册评估仓储
	// c.RegisterComponent("evaluationRepo", RepositoryType, func(container *Container) (interface{}, error) {
	// 	return mysqlAdapter.NewEvaluationRepository(container.mysqlDB), nil
	// })

	// // 注册评估服务
	// c.RegisterComponent("evaluationService", ServiceType, func(container *Container) (interface{}, error) {
	// 	repo, err := container.GetComponent("evaluationRepo")
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	return services.NewEvaluationService(repo.(storage.EvaluationRepository)), nil
	// })

	// // 注册评估处理器
	// c.RegisterComponent("evaluationHandler", HandlerType, func(container *Container) (interface{}, error) {
	// 	service, err := container.GetComponent("evaluationService")
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	return handlers.NewEvaluationHandler(service.(*services.EvaluationService)), nil
	// })
}

// 扩展路由注册方法的示例
// 在 router.go 中可以添加这些方法

/*
// RegisterScaleRoutes 注册量表处理器路由
func (r *Router) RegisterScaleRoutes(handler interface{}) error {
	scaleHandler, ok := handler.(*handlers.ScaleHandler)
	if !ok {
		return fmt.Errorf("invalid scale handler type")
	}

	r.handlers["scale"] = scaleHandler

	// 注册量表路由
	v1 := r.engine.Group("/api/v1")
	scales := v1.Group("/scales")
	{
		scales.POST("", scaleHandler.CreateScale)
		scales.GET("", scaleHandler.GetScale)
		scales.GET("/list", scaleHandler.ListScales)
		scales.PUT("/:id", scaleHandler.UpdateScale)
		scales.DELETE("/:id", scaleHandler.DeleteScale)
	}

	return nil
}

// RegisterResponseRoutes 注册答卷处理器路由
func (r *Router) RegisterResponseRoutes(handler interface{}) error {
	responseHandler, ok := handler.(*handlers.ResponseHandler)
	if !ok {
		return fmt.Errorf("invalid response handler type")
	}

	r.handlers["response"] = responseHandler

	// 注册答卷路由
	v1 := r.engine.Group("/api/v1")
	responses := v1.Group("/responses")
	{
		responses.POST("", responseHandler.SubmitResponse)
		responses.GET("/:id", responseHandler.GetResponse)
		responses.GET("/questionnaire/:questionnaire_id", responseHandler.GetResponsesByQuestionnaire)
	}

	return nil
}

// RegisterEvaluationRoutes 注册评估处理器路由
func (r *Router) RegisterEvaluationRoutes(handler interface{}) error {
	evaluationHandler, ok := handler.(*handlers.EvaluationHandler)
	if !ok {
		return fmt.Errorf("invalid evaluation handler type")
	}

	r.handlers["evaluation"] = evaluationHandler

	// 注册评估路由
	v1 := r.engine.Group("/api/v1")
	evaluations := v1.Group("/evaluations")
	{
		evaluations.POST("/calculate/:response_id", evaluationHandler.CalculateScore)
		evaluations.GET("/:id", evaluationHandler.GetEvaluation)
		evaluations.GET("/response/:response_id", evaluationHandler.GetEvaluationByResponse)
	}

	return nil
}
*/

// 使用示例：
//
// 1. 在 registerCoreComponents 方法中添加新模块的注册：
//    func (c *Container) registerCoreComponents() error {
//        c.registerQuestionnaireComponents()
//        c.registerUserComponents()
//        c.registerScaleComponents()        // 新增
//        c.registerResponseComponents()     // 新增
//        c.registerEvaluationComponents()   // 新增
//        return nil
//    }
//
// 2. 在 registerHandlerRoutes 方法中添加新的路由注册逻辑：
//    func (c *Container) registerHandlerRoutes(name string, handler interface{}) error {
//        switch name {
//        case "questionnaireHandler":
//            return c.router.RegisterQuestionnaireRoutes(handler)
//        case "scaleHandler":               // 新增
//            return c.router.RegisterScaleRoutes(handler)
//        case "responseHandler":            // 新增
//            return c.router.RegisterResponseRoutes(handler)
//        case "evaluationHandler":          // 新增
//            return c.router.RegisterEvaluationRoutes(handler)
//        default:
//            return c.router.RegisterGenericRoutes(name, handler)
//        }
//    }
//
// 就这样！不需要修改 Container 的核心逻辑，完全符合开放封闭原则。
