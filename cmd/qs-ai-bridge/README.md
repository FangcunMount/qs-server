# AI 持久投递桥接入口

本 CLI 保留用于隔离环境联调；正式参与者入口为 Collection `/api/v1/assessments/{id}/ai-workflows`。调用者必须来自已授权的业务用例；CLI 不提供终端用户认证。AI 执行、问题及状态权威由 qs-ai 持有，QS 只保存关联、命令 outbox、接收凭证和投影。

生产使用 qs-apiserver 内置投递循环和成果接收接口，无需部署此 CLI 进程。`ai_explanation.workflow_enabled=true` 时，进程启动常驻投递，每批最多 20 条、单批截止 60 秒，空闲间隔 1 秒、数据库错误退避上限 30 秒；单次 gRPC 仍为 5 秒。关闭进程会取消并等待循环退出，再释放连接和数据库。命令标识、失败重试时间和确认继续使用原 outbox 协议，不重复创建请求。

投递与管理共用 `ai_explanation.workflow_management` 下的地址和 TLS 文件；即使管理开关关闭，参与者入口启用也必须配置这些连接参数。管理接口仍单独受 `workflow_management.enabled` 控制。生产文件已配置 `qs-ai-grpc:50061` 和现有 QS 服务证书路径，但两个开关均保持关闭，等待集中切换。

成果接收注册在正式 `qs-apiserver:9090`，精确 ACL 和接收器均只接受 `qs-ai.svc`。它不依赖旧 AI 总开关，也不随新准入开关关闭，以继续接收已被接受任务的成果。关闭旧 AI 时同时关闭旧 `participant_enabled`、`evaluation.enabled`；保留标准报告、权限和新链路。生产旧数据不删除。

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

CLI relay 每次只处理一批待投递命令；常驻生产调度由上述正式进程负责。CLI receive 常驻且只接受 qs-ai.svc 的受信任客户端证书。TLS 服务器名称仍须匹配证书 SAN，不能用 CN 白名单代替服务器名称校验。

数据库集成测试使用 `QS_AI_BRIDGE_TEST_DSN` 指向已迁移的隔离测试库：

```sh
go test -race ./internal/apiserver/application/aibridge ./internal/apiserver/infra/mysql/aibridge ./internal/apiserver/infra/aibridge ./internal/apiserver/transport/grpc/aibridge ./cmd/qs-ai-bridge
```

协议源文件由 qs-ai 的 `integrations/workflow/proto/workflow.proto` 持有，本仓复制到 `api/grpc/proto/aiworkflow/workflow.proto`，使用现有生成脚本生成。跨语言回归入口位于 qs-ai 的 `tests/integration/test_delivery.py`，覆盖重复请求、确认丢失、乱序回传、接收端重启及错误证书身份。

结果契约包含状态、问题、阻断原因和完整 Artifact。代码与配置就绪不代表生产已切换；真实授权报告、管理闭环、成果展示、回退和观察证据仍需逐项验收。

发布镜像 `qs-apiserver` 仍携带 `/app/qs-ai-bridge` 供联调，默认入口为正式 apiserver。发布镜像不等于启用新 AI 流量。
