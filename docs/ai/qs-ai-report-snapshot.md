# QS → qs-ai：授权报告快照入口

新增入口 `POST /api/v1/assessments/{id}/ai-workflows?testee_id=...`，请求体为 `request_id`（调用端生成并复用的 UUID）和 `report_id`（十进制字符串）。HTTP 202 仅表示 QS 已将命令持久化，不能视为 AI 已接收或生成成功。旧 AI 解读接口保持独立，不双写旧 Generation。

入口沿用 reportIdentity、现有请求限流、collection-server 委托主体签名与 apiserver 服务身份验证。apiserver 复用 AuthorizeOwnAssessment，再读取当前标准报告及 Outcome；验证组织、Testee、Assessment 和客户端选定报告 ID 一致。请求体不接受 actor、组织或报告内容。

首次请求将标准报告 Content 序列化为 standard_report 事实，携带 report_id 和 `content_schema_version:outcome_id` 来源版本，同业务请求和 outbox 一起提交。重试先重新授权，已有请求匹配主体/报告后沿用已保存快照，不重新读取新版本。相同请求号不同主体或报告返回冲突。

qs-ai 在经 mTLS 验证的内部 Start 入口接收 evidence，并与会话、任务、回传 outbox 同事务冻结。qs-snapshot-v1 按提交时授权执行，不建设用户权限表、不回查用户 token；动态事实读取的旧技术路径仍要求 EvidenceSource 授权。未来新增事实必须再次经 QS 授权，结果展示也必须经 QS 授权。

上线开关为 `ai_explanation.workflow_enabled`，默认 false，独立于旧模型与 participant_enabled。启用前须部署兼容的 qs-ai gRPC 服务、两端独立证书及命令 relay。当前模型和正式成果不属于此批次，禁止向用户展示“生成成功”。新入口已实现，但本批不切换线上产品流量。

验证：应用测试覆盖拒绝访问时不读取报告/不入队、组织/主体/报告版本绑定、重复请求不换版；传输测试拒绝无可信委托的请求；QS 包测试包含路由、ACL 和 OpenAPI 契约。qs-ai 验证快照与任务原子提交、篡改重放冲突、无动态授权后端执行，以及真实双向 TLS Go/Python/MySQL 投递与回传。

当前快照使用标准报告 Content 字段；模型 Prompt 适配及更细的引用结构在正式模型批次实现。此批没有生产真实测评验收、模型调用或新成果展示证据。
