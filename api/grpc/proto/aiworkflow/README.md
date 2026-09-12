# QS AI workflow contract

Source: `FangcunMount/qs-ai`, commit `746a376143c784bf5ba729953ffd52c906b3363b`, path `integrations/workflow/proto/workflow.proto`.

This copy is byte-identical to the pinned source. qs-ai owns this contract; synchronize the source and regenerate both languages when changing it. Go generation uses `scripts/proto/generate.sh`. The runnable integration entry and limitations are documented in `cmd/qs-ai-bridge/README.md`.

PublicationManagement and PromptDraftManagement are served by qs-ai. QS provides authorized management proxies over one shared mTLS connection. Prompt drafts support create, revise, historical reads and original-command receipts; saving is not validation, asset freezing or publication. All management routes remain behind the disabled migration switch.

Freeze and GetFreezeReceipt are authorized QS proxies for native Prompt syntax validation and immutable asset creation. They do not approve a Profile or publish configuration. Freeze uses an explicit revision; timeouts require reading the original freeze command receipt, without retrying automatically.
