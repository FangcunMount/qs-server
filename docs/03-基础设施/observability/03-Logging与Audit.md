# Logging 与 Audit

**本文回答**：qs-server 中结构化日志与审计日志分别负责什么；日志字段应该如何命名；哪些敏感信息不能写入日志；Audit 应覆盖哪些安全/管理/服务间访问行为；普通业务日志和审计日志为什么不能混用。

---

## 30 秒结论

| 类型 | 目标 | 示例 |
| ---- | ---- | ---- |
| Logging | 排查运行时行为、错误、降级、外部调用 | component、action、error、duration、result |
| Audit | 追踪高价值操作：谁在何时对什么做了什么 | actor、operation、resource、decision、before/after |
| 当前实现基础 | component-base `log` / `logger` 在多处使用；gRPC server 可启用 AuditInterceptor |
| 不应记录 | token、secret、password、authorization header、raw JWT、appSecret、access key secret |
| 谨慎记录 | openid、手机号、邮箱、量表答案、儿童档案信息 |
| 核心原则 | 业务定位信息可以进日志；高基数字段不要进 metrics label；敏感字段要脱敏 |

一句话概括：

> **Logging 用于排障，Audit 用于追责；两者都不能成为敏感信息泄漏通道。**

---

## 1. Logging 的职责

Logging 回答：

```text
发生了什么？
在哪个组件？
哪个动作？
结果是什么？
错误是什么？
耗时多少？
与哪个请求/资源有关？
```

适合记录：

- 启动阶段。
- 外部集成调用。
- Redis degraded。
- MQ publisher fallback。
- cache warmup failure。
- permission denied context。
- worker handler error。
- shutdown cleanup error。

---

## 2. 推荐结构化字段

| 字段 | 说明 |
| ---- | ---- |
| component | apiserver / collection-server / worker |
| action | 稳定动作名 |
| request_id | 请求 ID |
| trace_id | 如后续接入 tracing |
| user_id_hash | 如必须，优先 hash |
| org_id | 低风险时可记录 |
| resource | 资源类型 |
| resource_id_hash | 高基数时 hash |
| result | success / failure / skipped / partial |
| outcome | bounded outcome |
| duration_ms | 耗时 |
| error | 错误信息 |
| reason | bounded reason |

---

## 3. 不应记录的内容

禁止明文记录：

- password。
- appSecret。
- access token。
- service token。
- Authorization header。
- raw JWT。
- AccessKeySecret。
- SessionToken。
- private key。
- raw cookie。

谨慎记录：

- openid。
- phone。
- email。
- child profile id。
- scale answer。
- psychological report content。
- precise location。

如果必须记录，优先：

- hash。
- masked。
- count。
- truncated。
- internal-only access。

---

## 4. Audit 的职责

Audit 回答：

```text
谁
在什么时候
通过什么入口
对哪个高价值资源
做了什么操作
结果是什么
```

适合审计：

- 登录/认证关键事件。
- 权限变更。
- Operator lifecycle。
- capability-sensitive 管理操作。
- service-to-service gRPC 调用。
- manual warmup / repair complete。
- ACL allowed/denied。
- 数据导出。
- 删除/归档/发布/撤销发布。

---

## 5. Logging 与 Audit 对比

| 维度 | Logging | Audit |
| ---- | ------- | ----- |
| 目的 | 排障 | 追责/合规 |
| 频率 | 高频 | 中低频 |
| 内容 | 技术上下文 | 行为事实 |
| 保存 | 可短一些 | 通常更长 |
| 敏感性 | 需脱敏 | 更需脱敏和权限控制 |
| 示例 | Redis get error | 某管理员发布量表 |
| 是否可采样 | 可 | 通常不可随意采样 |

---

## 6. gRPC Audit

gRPC server 在 Audit.Enabled 时安装：

```text
basegrpc.AuditInterceptor
```

它适合记录服务间访问行为。

注意：

- AuditInterceptor 记录的是 gRPC 层访问。
- 用户级业务审计仍可能需要 application 层补充。
- 不要把完整 request payload 全量写入 audit。

---

## 7. 日志与 Metrics 的边界

| 信息 | 放哪里 |
| ---- | ------ |
| 低基数 outcome | metrics label |
| request_id | log |
| user_id | log 中 hash / 谨慎 |
| raw error | log |
| error category | metrics label |
| duration | metrics histogram + log |
| detailed payload | 通常不记录 |
| openid/token | 不记录或 hash |

---

## 8. 常见反模式

| 反模式 | 风险 |
| ------ | ---- |
| metrics label 放 request_id | 高基数爆炸 |
| 日志打印 Authorization | 严重安全泄漏 |
| audit 记录完整报告内容 | 隐私风险 |
| error 只有 “failed” | 无法排障 |
| 日志字段名每处不同 | 检索困难 |
| 普通日志代替审计 | 无法追责 |
| audit 被采样 | 合规风险 |

---

## 9. 修改指南

### 9.1 新增日志字段

必须：

1. 字段名稳定。
2. 无敏感信息。
3. 高基数字段谨慎。
4. 业务 ID 优先 hash。
5. 与 metrics label 区分。

### 9.2 新增 Audit Event

必须定义：

1. actor。
2. operation。
3. resource type。
4. resource id。
5. scope。
6. decision/result。
7. reason。
8. timestamp。
9. request correlation。
10. 脱敏策略。

---

## 10. Verify

```bash
go test ./internal/pkg/grpc
go test ./internal/apiserver/transport/rest/middleware
```
