# AI MQ 维护命令族

源码位于 `scripts/maintenance/qs-ai-messaging/` 和相邻的独立 `qs-ai-messaging-preflight/`。`make build-maintenance` 统一构建数据库维护命令和离线伴随工具；它们不作为常驻服务启动。

| 入口 | 资源与副作用 |
| --- | --- |
| `qs-ai-messaging audit -limit 100` | 仅使用 QS_AI_MESSAGING_AUDIT_DSN；固定只读一致快照，不读取消息正文 |
| `qs-ai-messaging admission inspect` | 仅使用 QS_AI_MESSAGING_CONTROL_DSN；只读查看准入行 |
| `qs-ai-messaging admission close -expected-revision N` | 同一独立门禁连接修改准入状态；成功提交后才报告结果 |
| `qs-ai-messaging admission open -expected-revision N` | 要求已审核版本；不触发消息重投或模型调用 |
| `qs-ai-messaging-preflight --source-sha` 或 `--binding ...` | 独立离线进程，保留原发布 ABI；不读取上述 DSN、不连接数据库/Broker/模型 |

数据库子命令在解析和校验操作后才选择对应凭据。audit 不能接受写门禁参数；admission 必须明确 inspect、close 或 open，写操作必须携带 expected-revision。错误输出保持固定摘要，不泄漏连接串和驱动错误。

离线工具继续由发布脚本以非 root、只读文件系统、无网络和无环境凭据的方式运行；不调用数据库命令族，也不走在线宿主启动。其原有间接包依赖不代表建立了数据库或消息连接。

关闭准入只阻止新操作，已提交消息、接收、ACK、原幂等操作和查询继续工作。完整事务、预算、密钥和冻结配置规则见[AI 消息运行与维护](../../../docs/03-基础设施/event/40-MQ发布消费与结算.md#ai-workflow-消息运行与维护)。旧 audit/control 可执行文件不再构建；离线 preflight 路径和兼容回退绑定保持。

RocketMQ 普通消息绑定新增显式 provider、publication gate 与三个只读传输文件（credentials、CA、routes）。离线预检校验完整可信路由、CA、权限与内容摘要，不连接 Broker。固定 root 工具升级仍需安装新摘要文件，保留旧工具；不得以本地修改上传脚本绕过生产门禁。

## Public MySQL TLS

Production Actions use organization `MYSQL_PUBLIC_HOST`, `MYSQL_PUBLIC_PORT`,
`MYSQL_TLS_MODE=VERIFY_IDENTITY` and immutable `MYSQL_TLS_CA_REF`. The host verifies
the pinned infra CA before deployment or maintenance. Containers receive a read-only
CA mount; Go clients share component-base v0.8.2 verification. Credentials, query
budgets, timezone, transactions, schema and existing approval boundaries are preserved.
This connection change does not authorize a migration or business write. Local and
disposable test databases retain explicit `DISABLED`. Production cutover and business
acceptance require separate runtime evidence.
