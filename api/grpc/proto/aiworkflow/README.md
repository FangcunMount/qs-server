# QS AI workflow contract

Source: `FangcunMount/qs-ai`, commit `a99827d9e032d20e13bc9db8e8c86f7d91403352`, path `integrations/workflow/proto/workflow.proto`.

This copy is byte-identical to the pinned source. qs-ai owns this contract; synchronize the source and regenerate both languages when changing it. Go generation uses `scripts/proto/generate.sh`. The runnable integration entry and limitations are documented in `cmd/qs-ai-bridge/README.md`.

PublicationManagement and PromptDraftManagement are served by qs-ai. QS provides authorized management proxies over one shared mTLS connection. Prompt drafts support create, revise, historical reads and original-command receipts; saving is not validation, asset freezing or publication. All management routes remain behind the disabled migration switch.

Freeze and GetFreezeReceipt are authorized QS proxies for native Prompt syntax validation and immutable asset creation. They do not approve a Profile or publish configuration. Freeze uses an explicit revision; timeouts require reading the original freeze command receipt, without retrying automatically.

ProfileManagement Register/GetReceipt 复用 QS 管理授权和共用 mTLS 连接，注册不代表质量批准、发布或生效。回执核对原机构/操作者、命令和完整定义摘要，超时后查询原命令。

SuiteManagement Register/GetReceipt 复用 QS OrgAdmin/解读审计授权，登记保留案例与新资产清单的绑定；注册不等于评测批准。回执绑定原命令、机构/操作者、套件版本及资产引用，超时查询原 command_id。共享现有 mTLS 连接，治理开关关闭时不注册路由。

AssetCatalog List/Get 提供共享不可变配置目录，复用解读审计授权及 mTLS 连接。Go 校验分页顺序、筛选与游标、详情正文 SHA256；Prompt 源指纹与包摘要保持区分。目录不含命令审计和发布状态，不代表评测批准。

GetLifecycle 读取当前草稿修订与冻结摘要，复用解读审计授权；不返回原冻结命令审计、不接受历史 revision。状态、修订、资产身份及时间不一致时拒绝响应。该只读快照不授予后续编辑权。

EvaluationManagement.Prepare 复用审计授权，显式传递套件、生成路线与语义评测路线。AI 返回完整 11 项 release、整体指纹及策略预算；QS 核对原查询、完整引用、策略正文摘要和预算投影一致性，不重新计算质量门槛。REST 为 POST `/internal/v2/interpretation/ai-workflow/evaluations/prepare`，请求限 8 KiB、RPC 响应限 32 KiB、期限 5 秒且不自动重试。预算不表示机构可用额度，不创建任务或调用模型；Create/Start 仍分别要求管理员授权及明确确认。

EvaluationState.creation_json 增量携带原始创建回执（qs-ai-evaluation-creation-receipt/v1）；QS 核对任务 ID、11 项引用及整体摘要、原作者/原因/带时区时间，再透出 creation。Create 还核对原请求的引用、操作者和原因；Get 不把当前查询人当作原创建人。空字段兼容旧 AI，但不构造可恢复凭证；不返回可执行资产正文，不重发创建或启动命令。

PublicationManagement.ListHistory/GetHistory 提供精确 selector 的发布历史摘要和原始变更证据，复用当前解读审计授权。GET publications/history 使用排他 before_version 游标、每页 1–20 条（默认 20）；GET publications/history/{version} 核对原前后版本、内容摘要及审计，保留原操作者。历史读取不要求当前查询人是原发布者，也不授予发布/回退权限；原命令 GetReceipt 仍限制原组织/操作者。RPC 期限 5 秒，不自动重试；历史页正文最多 64 KiB，详情最多 1 MiB。治理开关关闭时两条路由均不注册。
