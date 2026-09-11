# AI 持久投递桥接入口

本入口用于内部跨服务联调，尚未接入现有产品路由。调用者必须来自已授权的业务用例；CLI 不提供终端用户认证。AI 执行、问题及状态权威由 qs-ai 持有，QS 只保存关联、命令 outbox、接收凭证和投影。

先向隔离 QS 测试库应用 `internal/pkg/migration/migrations/mysql/000072_ai_bridge_delivery.up.sql`，配置 `QS_AI_BRIDGE_DSN`（Go MySQL DSN，启用 `parseTime=true`），再构建：

```sh
go build -o /tmp/qs-ai-bridge ./cmd/qs-ai-bridge
/tmp/qs-ai-bridge -mode stage-start -input start.json
/tmp/qs-ai-bridge -mode relay -address localhost:50061 -ca "$CA_FILE" -cert "$QS_CERT_FILE" -key "$QS_KEY_FILE"
/tmp/qs-ai-bridge -mode receive -address localhost:50062 -ca "$CA_FILE" -cert "$QS_CERT_FILE" -key "$QS_KEY_FILE"
/tmp/qs-ai-bridge -mode projection -request-id "$REQUEST_ID"
/tmp/qs-ai-bridge -mode stage-change -request-id "$REQUEST_ID" -input answer.json
```

Start JSON 字段：`request_id`（稳定 UUID）、`actor`（org_id、subject_id）、`testee_id`、`assessment_ids`、`goal`。业务编号为字符串。Change 字段：`command_id`、`session_id`、`actor`、`action`（answer/cancel）、`expected_version`；回答附 `question_id` 和 `answer`，跳过使用 `skip=true` 且省略 answer。

relay 每次处理至多 20 个待投递命令；失败保留并退避，需外部调度重复执行。receive 常驻且只接受 qs-ai.svc 的受信任客户端证书。TLS 服务器名称仍须匹配证书 SAN，不能用 CN 白名单代替服务器名称校验。

数据库集成测试使用 `QS_AI_BRIDGE_TEST_DSN` 指向已迁移的隔离测试库：

```sh
go test -race ./internal/apiserver/application/aibridge ./internal/apiserver/infra/mysql/aibridge ./internal/apiserver/infra/aibridge ./internal/apiserver/transport/grpc/aibridge ./cmd/qs-ai-bridge
```

协议源文件由 qs-ai 的 `integrations/workflow/proto/workflow.proto` 持有，本仓复制到 `api/grpc/proto/aiworkflow/workflow.proto`，使用现有生成脚本生成。跨语言回归入口位于 qs-ai 的 `tests/integration/test_delivery.py`，覆盖重复请求、确认丢失、乱序回传、接收端重启及错误证书身份。

当前结果契约仅含状态、问题和阻断原因；正式 Artifact、真实资源授权/事实读取、产品入口切换和生产调度在后续批次实现。旧 AI 引擎及其流量保持原状。
