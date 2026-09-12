# QS AI workflow contract

Source: `FangcunMount/qs-ai`, commit `77f3172172abc504114ad0d51c41a06f75a181c6`, path `integrations/workflow/proto/workflow.proto`.

This copy is byte-identical to the pinned source. qs-ai owns this contract; synchronize the source and regenerate both languages when changing it. Go generation uses `scripts/proto/generate.sh`. The runnable integration entry and limitations are documented in `cmd/qs-ai-bridge/README.md`.

PublicationManagement and PromptDraftManagement are served by qs-ai. QS provides authorized management proxies over one shared mTLS connection. Prompt drafts support create, revise, historical reads and original-command receipts; saving is not validation, asset freezing or publication. All management routes remain behind the disabled migration switch.

Freeze and GetFreezeReceipt are authorized QS proxies for native Prompt syntax validation and immutable asset creation. They do not approve a Profile or publish configuration. Freeze uses an explicit revision; timeouts require reading the original freeze command receipt, without retrying automatically.

ProfileManagement Register/GetReceipt 复用 QS 管理授权和共用 mTLS 连接，注册不代表质量批准、发布或生效。回执核对原机构/操作者、命令和完整定义摘要，超时后查询原命令。

SuiteManagement Register/GetReceipt 复用 QS OrgAdmin/解读审计授权，登记保留案例与新资产清单的绑定；注册不等于评测批准。回执绑定原命令、机构/操作者、套件版本及资产引用，超时查询原 command_id。共享现有 mTLS 连接，治理开关关闭时不注册路由。
