# InputSnapshot 与 Contract 测试

**本文回答**：`evaluationinput` 当前为什么还保留 command repo 兼容 adapter，后续如何迁到 Survey/Scale catalog/read model adapter，以及真实数据库 contract 测试如何运行。

## 30 秒结论

| 主题 | 当前事实 | 目标形态 |
| ---- | -------- | -------- |
| Engine 输入 | Engine 只消费 `port/evaluationinput` snapshot DTO | 保持 port 中立，不暴露 Survey/Scale 聚合 |
| 兼容 adapter | `infra/evaluationinput.NewRepositoryResolver` 从 Survey/Scale command repo 映射 snapshot | 逐步替换为 catalog/read model adapter |
| Contract 测试 | 默认 CI 跑 unit/dry-run contract，真实 DB 测试 env-gated skip | CI 日志能看到 skip reason 和本地运行命令 |

## InputSnapshot 边界

`port/evaluationinput` 是 evaluation engine 的输入防腐层。它只定义 `InputRef`、`InputSnapshot`、`ScaleSnapshot`、`AnswerSheetSnapshot`、`QuestionnaireSnapshot` 等中立 DTO，不依赖 `domain/survey` 或 `domain/scale`。

当前 `infra/evaluationinput` 仍保留 command repo 兼容 adapter：

- `ScaleSnapshotCatalog` 从 scale command repo 读取 `MedicalScale` 后映射为 `ScaleSnapshot`。
- `AnswerSheetSnapshotReader` 从 answersheet command repo 读取答卷后映射为 `AnswerSheetSnapshot`。
- `QuestionnaireSnapshotReader` 从 questionnaire command repo 读取精确版本问卷后映射为 `QuestionnaireSnapshot`。

这条路径是兼容桥，不是最终形态。架构护栏要求新的 Survey/Scale domain dependency 只能留在兼容 adapter 文件内，避免把旧聚合导航重新扩散回 engine。

## 迁移路线

1. 补齐 snapshot contract tests：factor interpret rules、`cnt` option content、answer raw value、question option score、exact questionnaire version miss。
2. 新增 catalog/read model adapter，与 repository adapter 并存：
   - `ScaleSnapshotCatalog` 从 scale application catalog 或 scale read model 获取已发布量表 snapshot。
   - `AnswerSheetSnapshotReader` 从 survey answersheet catalog/read model 获取答卷 snapshot。
   - `QuestionnaireSnapshotReader` 从 survey questionnaire catalog 获取 exact version snapshot。
3. container 通过显式 deps 切换 adapter，engine 和 pipeline 不感知来源变化。
4. 删除 command repo adapter 和对应架构 allowlist，只保留 catalog/read model adapter。

## Contract 测试

默认 CI 不依赖真实 MySQL/Mongo，只运行 unit contract 和 dry-run contract。真实数据库 contract 测试通过环境变量开启，未配置时测试会 skip，并在日志中说明缺失变量、覆盖内容和运行命令。

### MySQL Evaluation Read Model

覆盖内容：

- assessment org/testee/status/date/access-scope filters
- pagination/order/count
- score order
- factor trend

运行方式：

```bash
QS_SERVER_TEST_MYSQL_DSN='user:pass@tcp(127.0.0.1:3306)/qs_server_contract?charset=utf8mb4&parseTime=True&loc=Local' \
  go test ./internal/apiserver/infra/mysql/evaluation -run 'Integration|AgainstDatabase' -v
```

### Mongo Evaluation Report Read Model

覆盖内容：

- testee/testeeIDs filter
- high-risk/risk/scale filter
- pagination/sort
- not-found mapping
- legacy nil field fallback

运行方式：

```bash
QS_SERVER_TEST_MONGO_URI='mongodb://127.0.0.1:27017' QS_SERVER_TEST_MONGO_DB='qs_server_contract_test' \
  go test ./internal/apiserver/infra/mongo/evaluation -run 'Integration|AgainstMongo' -v
```

`QS_SERVER_TEST_MONGO_DB` 可省略，默认使用 `qs_server_contract_test`。

## 代码锚点

- Input snapshot port：[port/evaluationinput](../../../internal/apiserver/port/evaluationinput/)
- 兼容 adapter：[infra/evaluationinput](../../../internal/apiserver/infra/evaluationinput/)
- MySQL contract tests：[read_model_integration_test.go](../../../internal/apiserver/infra/mysql/evaluation/read_model_integration_test.go)
- Mongo contract tests：[read_model_integration_test.go](../../../internal/apiserver/infra/mongo/evaluation/read_model_integration_test.go)

## Verify

```bash
go test ./internal/apiserver/infra/evaluationinput
go test ./internal/apiserver/infra/mysql/evaluation
go test ./internal/apiserver/infra/mongo/evaluation
```
