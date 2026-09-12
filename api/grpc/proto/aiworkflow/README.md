# QS AI workflow contract

Source: `FangcunMount/qs-ai`, commit `9d3718804d12892c582b86f75c7827a20eced9bf`, path `integrations/workflow/proto/workflow.proto`.

This copy is byte-identical to the pinned source. qs-ai owns this contract; synchronize the source and regenerate both languages when changing it. Go generation uses `scripts/proto/generate.sh`. The runnable integration entry and limitations are documented in `cmd/qs-ai-bridge/README.md`.

PublicationManagement and PromptDraftManagement are served by qs-ai. QS provides authorized management proxies over one shared mTLS connection. Prompt drafts support create, revise, historical reads and original-command receipts; saving is not validation, asset freezing or publication. All management routes remain behind the disabled migration switch.

Freeze and GetFreezeReceipt are authorized QS proxies for native Prompt syntax validation and immutable asset creation. They do not approve a Profile or publish configuration. Freeze uses an explicit revision; timeouts require reading the original freeze command receipt, without retrying automatically.

ProfileManagement Register/GetReceipt 复用 QS 管理授权和共用 mTLS 连接，注册不代表质量批准、发布或生效。回执核对原机构/操作者、命令和完整定义摘要，超时后查询原命令。
