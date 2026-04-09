package main

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	clog "github.com/FangcunMount/component-base/pkg/log"
	actorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	questionnaireApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	apiservercontainer "github.com/FangcunMount/qs-server/internal/apiserver/container"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
	redis "github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

const (
	planModeLocal                = "local"
	planModeRemote               = "remote"
	defaultLocalPlanEntryBaseURL = "https://collect.fangcunmount.cn/entry"
)

type PlanSeedGateway interface {
	GetPlan(ctx context.Context, planID string) (*PlanResponse, error)
	GetScale(ctx context.Context, code string) (*ScaleResponse, error)
	GetQuestionnaireDetail(ctx context.Context, code string) (*QuestionnaireDetailResponse, error)
	ListTesteesByOrg(ctx context.Context, orgID int64, page, pageSize int) (*ApiserverTesteeListResponse, error)
	GetTesteeByID(ctx context.Context, testeeID string) (*ApiserverTesteeResponse, error)
	EnrollTestee(ctx context.Context, req EnrollTesteeRequest) (*EnrollmentResponse, error)
	SchedulePendingTasks(ctx context.Context, req SchedulePendingTasksRequest) (*TaskListResponse, error)
	ListTasksByTesteeAndPlan(ctx context.Context, testeeID, planID string) (*TaskListResponse, error)
	GetTask(ctx context.Context, taskID string) (*TaskResponse, error)
	ExpireTask(ctx context.Context, taskID string) (*TaskResponse, error)
}

func resolvePlanMode(cliMode string, configMode string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(cliMode))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(configMode))
	}
	if mode == "" {
		mode = planModeLocal
	}
	switch mode {
	case planModeLocal, planModeRemote:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported plan mode %q", mode)
	}
}

func newPlanSeedGateway(ctx context.Context, deps *dependencies, mode string, silent bool) (PlanSeedGateway, func() error, error) {
	switch mode {
	case planModeRemote:
		return &remotePlanSeedGateway{
			api:        deps.APIClient,
			collection: deps.CollectionClient,
		}, func() error { return nil }, nil
	case planModeLocal:
		runtime, err := newLocalPlanRuntime(ctx, deps.Config.Local, deps.Config.API.CollectionBaseURL, silent)
		if err != nil {
			return nil, nil, err
		}
		return &localPlanSeedGateway{
			orgID:   deps.Config.Global.OrgID,
			runtime: runtime,
		}, runtime.Cleanup, nil
	default:
		return nil, nil, fmt.Errorf("unsupported plan mode %q", mode)
	}
}

type remotePlanSeedGateway struct {
	api        *APIClient
	collection *APIClient
}

func (g *remotePlanSeedGateway) GetPlan(ctx context.Context, planID string) (*PlanResponse, error) {
	return g.api.GetPlan(ctx, planID)
}

func (g *remotePlanSeedGateway) GetScale(ctx context.Context, code string) (*ScaleResponse, error) {
	return g.collection.GetScale(ctx, code)
}

func (g *remotePlanSeedGateway) GetQuestionnaireDetail(ctx context.Context, code string) (*QuestionnaireDetailResponse, error) {
	return g.collection.GetQuestionnaireDetail(ctx, code)
}

func (g *remotePlanSeedGateway) ListTesteesByOrg(ctx context.Context, orgID int64, page, pageSize int) (*ApiserverTesteeListResponse, error) {
	return g.api.ListTesteesByOrg(ctx, orgID, page, pageSize)
}

func (g *remotePlanSeedGateway) GetTesteeByID(ctx context.Context, testeeID string) (*ApiserverTesteeResponse, error) {
	return g.api.GetTesteeByID(ctx, testeeID)
}

func (g *remotePlanSeedGateway) EnrollTestee(ctx context.Context, req EnrollTesteeRequest) (*EnrollmentResponse, error) {
	return g.api.EnrollTestee(ctx, req)
}

func (g *remotePlanSeedGateway) SchedulePendingTasks(ctx context.Context, req SchedulePendingTasksRequest) (*TaskListResponse, error) {
	return g.api.SchedulePendingTasks(ctx, req)
}

func (g *remotePlanSeedGateway) ListTasksByTesteeAndPlan(ctx context.Context, testeeID, planID string) (*TaskListResponse, error) {
	return g.api.ListTasksByTesteeAndPlan(ctx, testeeID, planID)
}

func (g *remotePlanSeedGateway) GetTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	return g.api.GetTask(ctx, taskID)
}

func (g *remotePlanSeedGateway) ExpireTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	return g.api.ExpireTask(ctx, taskID)
}

type localPlanRuntime struct {
	mysqlDB     *gorm.DB
	mongoClient *mongo.Client
	mongoDB     *mongo.Database
	redisClient redis.UniversalClient
	container   *apiservercontainer.Container
	quietLogger clog.Logger
}

func newLocalPlanRuntime(ctx context.Context, cfg LocalRuntimeConfig, collectionBaseURL string, silent bool) (*localPlanRuntime, error) {
	if strings.TrimSpace(cfg.MySQLDSN) == "" {
		return nil, fmt.Errorf("seeddata local.mysql_dsn is required when plan-mode=local")
	}
	if strings.TrimSpace(cfg.MongoURI) == "" {
		return nil, fmt.Errorf("seeddata local.mongo_uri is required when plan-mode=local")
	}
	if strings.TrimSpace(cfg.MongoDatabase) == "" {
		return nil, fmt.Errorf("seeddata local.mongo_database is required when plan-mode=local")
	}
	if strings.TrimSpace(cfg.RedisAddr) == "" {
		return nil, fmt.Errorf("seeddata local.redis_addr is required when plan-mode=local")
	}

	mysqlDB, err := openLocalSeedMySQL(cfg.MySQLDSN)
	if err != nil {
		return nil, err
	}

	mongoClient, mongoDB, err := openLocalSeedMongo(ctx, cfg.MongoURI, cfg.MongoDatabase)
	if err != nil {
		closeLocalSeedMySQL(mysqlDB)
		return nil, err
	}

	redisClient, err := openLocalSeedRedis(ctx, cfg)
	if err != nil {
		_ = mongoClient.Disconnect(ctx)
		closeLocalSeedMySQL(mysqlDB)
		return nil, err
	}

	container := apiservercontainer.NewContainerWithOptions(
		mysqlDB,
		mongoDB,
		redisClient,
		apiservercontainer.ContainerOptions{
			PublisherMode:    eventconfig.PublishModeNop,
			PlanEntryBaseURL: resolveLocalPlanEntryBaseURL(cfg, collectionBaseURL),
			Silent:           silent,
		},
	)
	if err := container.Initialize(); err != nil {
		_ = redisClient.Close()
		_ = mongoClient.Disconnect(ctx)
		closeLocalSeedMySQL(mysqlDB)
		return nil, fmt.Errorf("initialize local apiserver container: %w", err)
	}

	return &localPlanRuntime{
		mysqlDB:     mysqlDB,
		mongoClient: mongoClient,
		mongoDB:     mongoDB,
		redisClient: redisClient,
		container:   container,
		quietLogger: newLocalSeedQuietLogger(silent),
	}, nil
}

func (r *localPlanRuntime) Cleanup() error {
	var errs []string
	if r.container != nil {
		if err := r.container.Cleanup(); err != nil {
			errs = append(errs, fmt.Sprintf("cleanup local container: %v", err))
		}
	}
	if r.redisClient != nil {
		if err := r.redisClient.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("close local redis client: %v", err))
		}
	}
	if r.mongoClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := r.mongoClient.Disconnect(ctx); err != nil {
			errs = append(errs, fmt.Sprintf("disconnect local mongo client: %v", err))
		}
		cancel()
	}
	if err := closeLocalSeedMySQL(r.mysqlDB); err != nil {
		errs = append(errs, fmt.Sprintf("close local mysql connection: %v", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

type localPlanSeedGateway struct {
	orgID   int64
	runtime *localPlanRuntime
}

func (g *localPlanSeedGateway) GetPlan(ctx context.Context, planID string) (*PlanResponse, error) {
	result, err := g.runtime.container.PlanModule.QueryService.GetPlan(g.runtime.planContext(ctx), g.orgID, planID)
	if err != nil {
		return nil, err
	}
	return toSeedPlanResponse(result), nil
}

func (g *localPlanSeedGateway) GetScale(ctx context.Context, code string) (*ScaleResponse, error) {
	result, err := g.runtime.container.ScaleModule.QueryService.GetByCode(g.runtime.planContext(ctx), code)
	if err != nil {
		return nil, err
	}
	return toSeedScaleResponse(result), nil
}

func (g *localPlanSeedGateway) GetQuestionnaireDetail(ctx context.Context, code string) (*QuestionnaireDetailResponse, error) {
	result, err := g.runtime.container.SurveyModule.Questionnaire.QueryService.GetByCode(g.runtime.planContext(ctx), code)
	if err != nil {
		return nil, err
	}
	return toSeedQuestionnaireDetailResponse(result), nil
}

func (g *localPlanSeedGateway) ListTesteesByOrg(ctx context.Context, orgID int64, page, pageSize int) (*ApiserverTesteeListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	result, err := g.runtime.container.ActorModule.TesteeQueryService.ListTestees(g.runtime.planContext(ctx), actorApp.ListTesteeDTO{
		OrgID:  orgID,
		Offset: offset,
		Limit:  pageSize,
	})
	if err != nil {
		return nil, err
	}
	return toSeedTesteeListResponse(result, page, pageSize), nil
}

func (g *localPlanSeedGateway) GetTesteeByID(ctx context.Context, testeeID string) (*ApiserverTesteeResponse, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(testeeID), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse testee_id %q: %w", testeeID, err)
	}
	result, err := g.runtime.container.ActorModule.TesteeQueryService.GetByID(g.runtime.planContext(ctx), id)
	if err != nil {
		return nil, err
	}
	return toSeedTesteeResponse(result), nil
}

func (g *localPlanSeedGateway) EnrollTestee(ctx context.Context, req EnrollTesteeRequest) (*EnrollmentResponse, error) {
	result, err := g.runtime.container.PlanModule.CommandService.EnrollTestee(g.runtime.planContext(ctx), planApp.EnrollTesteeDTO{
		OrgID:     g.orgID,
		PlanID:    req.PlanID,
		TesteeID:  req.TesteeID,
		StartDate: req.StartDate,
	})
	if err != nil {
		return nil, err
	}
	return toSeedEnrollmentResponse(result), nil
}

func (g *localPlanSeedGateway) SchedulePendingTasks(ctx context.Context, req SchedulePendingTasksRequest) (*TaskListResponse, error) {
	scheduleCtx := g.runtime.planContext(ctx)
	if source := strings.TrimSpace(req.Source); source != "" {
		scheduleCtx = planApp.WithTaskSchedulerSource(scheduleCtx, source)
	}
	if req.PlanID != "" || len(req.TesteeIDs) > 0 {
		scheduleCtx = planApp.WithTaskSchedulerScope(scheduleCtx, req.PlanID, req.TesteeIDs)
	}
	result, err := g.runtime.container.PlanModule.CommandService.SchedulePendingTasks(scheduleCtx, g.orgID, req.Before)
	if err != nil {
		return nil, err
	}
	return toSeedScheduledTaskListResponse(result), nil
}

func (g *localPlanSeedGateway) ListTasksByTesteeAndPlan(ctx context.Context, testeeID, planID string) (*TaskListResponse, error) {
	results, err := g.runtime.container.PlanModule.QueryService.ListTasksByTesteeAndPlan(g.runtime.planContext(ctx), testeeID, planID)
	if err != nil {
		return nil, err
	}
	return toSeedTaskListResponse(results), nil
}

func (g *localPlanSeedGateway) GetTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	result, err := g.runtime.container.PlanModule.QueryService.GetTask(g.runtime.planContext(ctx), g.orgID, taskID)
	if err != nil {
		return nil, err
	}
	return toSeedTaskResponse(result), nil
}

func (g *localPlanSeedGateway) ExpireTask(ctx context.Context, taskID string) (*TaskResponse, error) {
	result, err := g.runtime.container.PlanModule.CommandService.ExpireTask(g.runtime.planContext(ctx), g.orgID, taskID)
	if err != nil {
		return nil, err
	}
	return toSeedTaskResponse(result), nil
}

func (r *localPlanRuntime) planContext(ctx context.Context) context.Context {
	if r == nil || r.quietLogger == nil {
		return ctx
	}
	return r.quietLogger.WithContext(ctx)
}

func newLocalSeedQuietLogger(silent bool) clog.Logger {
	if !silent {
		return nil
	}
	opts := clog.NewOptions()
	opts.Level = "warn"
	return clog.New(opts)
}

func resolveLocalPlanEntryBaseURL(cfg LocalRuntimeConfig, collectionBaseURL string) string {
	if baseURL := strings.TrimSpace(cfg.PlanEntryBaseURL); baseURL != "" {
		return baseURL
	}
	if trimmed := strings.TrimSpace(collectionBaseURL); trimmed != "" {
		return strings.TrimSuffix(trimmed, "/") + "/entry"
	}
	return defaultLocalPlanEntryBaseURL
}

func openLocalSeedMySQL(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		DisableAutomaticPing: false,
	})
	if err != nil {
		return nil, fmt.Errorf("open local mysql for seeddata: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql DB for seeddata local mysql: %w", err)
	}
	sqlDB.SetConnMaxIdleTime(30 * time.Second)
	sqlDB.SetConnMaxLifetime(10 * time.Minute)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(50)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping local mysql for seeddata: %w", err)
	}
	return db, nil
}

func closeLocalSeedMySQL(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func openLocalSeedMongo(ctx context.Context, uri string, database string) (*mongo.Client, *mongo.Database, error) {
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client, err := mongo.Connect(connectCtx, mongooptions.Client().ApplyURI(uri))
	if err != nil {
		return nil, nil, fmt.Errorf("connect local mongo for seeddata: %w", err)
	}
	if err := client.Ping(connectCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("ping local mongo for seeddata: %w", err)
	}
	return client, client.Database(database), nil
}

func openLocalSeedRedis(ctx context.Context, cfg LocalRuntimeConfig) (redis.UniversalClient, error) {
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client := redis.NewClient(&redis.Options{
		Addr:                     strings.TrimSpace(cfg.RedisAddr),
		Username:                 strings.TrimSpace(cfg.RedisUsername),
		Password:                 cfg.RedisPassword,
		DB:                       cfg.RedisDB,
		DialTimeout:              10 * time.Second,
		ReadTimeout:              10 * time.Second,
		WriteTimeout:             10 * time.Second,
		PoolTimeout:              30 * time.Second,
		MinIdleConns:             5,
		MaxRetries:               3,
		DisableIdentity:          true,
		MaintNotificationsConfig: &maintnotifications.Config{Mode: maintnotifications.ModeDisabled},
	})
	if err := client.Ping(connectCtx).Err(); err != nil {
		_ = client.Close()
		if shouldHintRedisUsername(err) {
			return nil, fmt.Errorf("ping local redis for seeddata: %w; if your Redis uses ACL, pass --local-redis-username (or set local.redis_username)", err)
		}
		return nil, fmt.Errorf("ping local redis for seeddata: %w", err)
	}
	return client, nil
}

func shouldHintRedisUsername(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "wrongpass") ||
		strings.Contains(msg, "noauth") ||
		strings.Contains(msg, "authentication required") ||
		strings.Contains(msg, "invalid username-password pair")
}

func toSeedPlanResponse(result *planApp.PlanResult) *PlanResponse {
	if result == nil {
		return nil
	}
	return &PlanResponse{
		ID:            result.ID,
		OrgID:         result.OrgID,
		ScaleCode:     result.ScaleCode,
		ScheduleType:  result.ScheduleType,
		Interval:      result.Interval,
		TotalTimes:    result.TotalTimes,
		FixedDates:    append([]string(nil), result.FixedDates...),
		RelativeWeeks: append([]int(nil), result.RelativeWeeks...),
		Status:        result.Status,
	}
}

func toSeedTaskResponse(result *planApp.TaskResult) *TaskResponse {
	if result == nil {
		return nil
	}
	resp := &TaskResponse{
		ID:         result.ID,
		PlanID:     result.PlanID,
		Seq:        result.Seq,
		OrgID:      result.OrgID,
		TesteeID:   result.TesteeID,
		ScaleCode:  result.ScaleCode,
		PlannedAt:  result.PlannedAt,
		Status:     result.Status,
		EntryToken: result.EntryToken,
		EntryURL:   result.EntryURL,
	}
	if result.OpenAt != nil {
		openAt := *result.OpenAt
		resp.OpenAt = &openAt
	}
	if result.ExpireAt != nil {
		expireAt := *result.ExpireAt
		resp.ExpireAt = &expireAt
	}
	if result.CompletedAt != nil {
		completedAt := *result.CompletedAt
		resp.CompletedAt = &completedAt
	}
	if result.AssessmentID != nil {
		assessmentID := *result.AssessmentID
		resp.AssessmentID = &assessmentID
	}
	return resp
}

func toSeedTaskResponses(results []*planApp.TaskResult) []TaskResponse {
	if len(results) == 0 {
		return nil
	}
	items := make([]TaskResponse, 0, len(results))
	for _, result := range results {
		if task := toSeedTaskResponse(result); task != nil {
			items = append(items, *task)
		}
	}
	return items
}

func toSeedEnrollmentResponse(result *planApp.EnrollmentResult) *EnrollmentResponse {
	if result == nil {
		return nil
	}
	return &EnrollmentResponse{
		PlanID: result.PlanID,
		Tasks:  toSeedTaskResponses(result.Tasks),
	}
}

func toSeedTaskScheduleStats(stats planApp.TaskScheduleStats) *TaskScheduleStatsResponse {
	return &TaskScheduleStatsResponse{
		PendingCount:      stats.PendingCount,
		OpenedCount:       stats.OpenedCount,
		FailedCount:       stats.FailedCount,
		ExpiredCount:      stats.ExpiredCount,
		ExpireFailedCount: stats.ExpireFailedCount,
	}
}

func toSeedTaskListResponse(results []*planApp.TaskResult) *TaskListResponse {
	tasks := toSeedTaskResponses(results)
	return &TaskListResponse{
		Tasks:      tasks,
		TotalCount: int64(len(tasks)),
		Page:       1,
		PageSize:   len(tasks),
	}
}

func toSeedScheduledTaskListResponse(result *planApp.TaskScheduleResult) *TaskListResponse {
	if result == nil {
		return &TaskListResponse{}
	}
	tasks := toSeedTaskResponses(result.Tasks)
	return &TaskListResponse{
		Tasks:      tasks,
		TotalCount: int64(len(tasks)),
		Page:       1,
		PageSize:   len(tasks),
		Stats:      toSeedTaskScheduleStats(result.Stats),
	}
}

func toSeedScaleResponse(result *scaleApp.ScaleResult) *ScaleResponse {
	if result == nil {
		return nil
	}
	return &ScaleResponse{
		Code:                 result.Code,
		Title:                result.Title,
		Status:               result.Status,
		Version:              "",
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
	}
}

func toSeedQuestionnaireDetailResponse(result *questionnaireApp.QuestionnaireResult) *QuestionnaireDetailResponse {
	if result == nil {
		return nil
	}
	resp := &QuestionnaireDetailResponse{
		Code:      result.Code,
		Title:     result.Title,
		Status:    result.Status,
		Version:   result.Version,
		Type:      result.Type,
		Questions: make([]QuestionResponse, 0, len(result.Questions)),
	}
	for _, question := range result.Questions {
		q := QuestionResponse{
			Code:    question.Code,
			Type:    question.Type,
			Title:   question.Stem,
			Options: make([]OptionResponse, 0, len(question.Options)),
		}
		for _, option := range question.Options {
			q.Options = append(q.Options, OptionResponse{
				Code:    option.Value,
				Content: option.Label,
				Score:   int32(option.Score),
			})
		}
		resp.Questions = append(resp.Questions, q)
	}
	return resp
}

func toSeedTesteeResponse(result *actorApp.TesteeResult) *ApiserverTesteeResponse {
	if result == nil {
		return nil
	}
	return &ApiserverTesteeResponse{
		ID:        strconv.FormatUint(result.ID, 10),
		CreatedAt: result.CreatedAt,
		UpdatedAt: result.UpdatedAt,
	}
}

func toSeedTesteeListResponse(result *actorApp.TesteeListResult, page int, pageSize int) *ApiserverTesteeListResponse {
	if result == nil {
		return &ApiserverTesteeListResponse{
			Page:       page,
			PageSize:   pageSize,
			TotalPages: 0,
		}
	}
	items := make([]*ApiserverTesteeResponse, 0, len(result.Items))
	for _, item := range result.Items {
		if mapped := toSeedTesteeResponse(item); mapped != nil {
			items = append(items, mapped)
		}
	}
	totalPages := 0
	if pageSize > 0 && result.TotalCount > 0 {
		totalPages = int(math.Ceil(float64(result.TotalCount) / float64(pageSize)))
	}
	return &ApiserverTesteeListResponse{
		Items:      items,
		Total:      result.TotalCount,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}
