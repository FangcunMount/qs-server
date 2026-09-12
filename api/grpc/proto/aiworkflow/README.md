# QS AI workflow contract

Source: `FangcunMount/qs-ai`, commit `456a84f8f53d1fc48fcc76fcfe9edbb77e4399b7`, path `integrations/workflow/proto/workflow.proto`.

This copy is byte-identical to the pinned source. qs-ai owns this contract; synchronize the source and regenerate both languages when changing it. Go generation uses `scripts/proto/generate.sh`. The runnable integration entry and limitations are documented in `cmd/qs-ai-bridge/README.md`.

The PublicationManagement service is synchronized for the forthcoming QS adapter. QS does not yet route publication governance to it; the existing business authorization and disabled migration switches remain in place.
