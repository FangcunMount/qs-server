package main

import (
	"context"
	"errors"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mysqlBase "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/mysqltls"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	mysqldriver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openOriginalHost(ctx context.Context, a arguments, getenv func(string) string) (executor, func() error, error) {
	if a.binding.Owner == "qs.mysql" {
		raw := getenv("QS_RETRY_HANDOFF_MYSQL_DSN")
		cfg, err := mysqldriver.ParseDSN(raw)
		if raw == "" || err != nil || cfg.DBName != a.database {
			return nil, nil, errors.New("explicit original MySQL maintenance binding required")
		}
		cfg.ParseTime, cfg.Loc = true, time.UTC
		cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, 5*time.Second, 5*time.Second
		pool, err := mysqltls.Open(cfg.FormatDSN())
		if err != nil {
			return nil, nil, err
		}
		pool.SetMaxOpenConns(2)
		primary, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: pool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			return nil, nil, errors.Join(err, pool.Close())
		}
		host, err := container.NewMySQLRetryHandoff(primary, a.binding, mysqlBase.BaseRepositoryOptions{Limiter: backpressure.NewLimiter(1, 2*time.Second)})
		if err != nil {
			return nil, nil, errors.Join(err, pool.Close())
		}
		return host, pool.Close, nil
	}
	if a.binding.Owner != "qs.mongo" || a.database == "" {
		return nil, nil, errors.New("explicit original Mongo maintenance binding required")
	}
	uri := getenv("QS_RETRY_HANDOFF_MONGO_URI")
	if uri == "" {
		return nil, nil, errors.New("explicit original Mongo maintenance URI required")
	}
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetMaxPoolSize(2).SetConnectTimeout(5*time.Second).SetServerSelectionTimeout(5*time.Second).SetTimeout(5*time.Second))
	if err != nil {
		return nil, nil, err
	}
	closeClient := func() error {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return client.Disconnect(cleanup)
	}
	host, err := container.NewMongoRetryHandoff(client.Database(a.database), a.binding, mongoBase.BaseRepositoryOptions{Limiter: backpressure.NewLimiter(1, 2*time.Second)})
	if err != nil {
		return nil, nil, errors.Join(err, closeClient())
	}
	return host, closeClient, nil
}
