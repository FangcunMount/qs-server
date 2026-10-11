# 兼容链退役工具退出说明

本页对应 R4 源码退出候选。专用盘点、历史回填、交接、备份恢复和删除工具随本批退出；当前业务保留标准消息链及退休身份保护。本页的 `runtime_status=unknown`、`doc_status=needs_review`：生产执行和验收结论仍需绑定实际回执。

## 四对象范围

| 模块 | 退役对象 | 保留职责 |
| --- | --- | --- |
| 事件与可靠消息 | MySQL `domain_event_outbox`、MongoDB `domain_event_outbox` | 原业务事实、EventEvidence、标准 Outbox 与双向审计 |
| AI 运行时协作 | MySQL `ai_bridge_commands`、`ai_messaging_legacy_commands` | 当前 MQ 账本、原 command ID、退休证据及迟到回执隔离 |

此前 CBPT 清理档案、普通备份和非目标数据不属于本批材料销毁范围。36 个存量改名候选、AI 重复正文精简和人员维护对象仍属后续独立阶段。

## 源码退出与长期入口

本候选移除专用 history/retirement/handoff CLI、三个退役 Action、维护包、Python 编排、临时运行观察和仅保护这些实现的测试。共享构建、镜像导出和 CI 撤掉临时分支，继续执行普通交付与长期业务检查。

现用 MQ 的目录、组织范围与关联检查见[MQ 只读数据库清单](../../cmd/qs-ai-bridge/README.md)。事件出站和业务审计见[标准 Outbox](../../docs/03-基础设施/event/30-Outbox可靠出站链路.md)。迁移及启动缺失保护见[迁移说明](../../internal/pkg/migration/README.md)。历史迁移文件保留原字节，退休 ID 不因旧源恢复而重新可用。

## Final historical readback and temporary material cleanup

R4 应用前须逐项保存以下真实结论：

1. 四对象确实不存在，双库 schema head 正确且 clean，正常启动不重建旧对象。
2. 原历史证据和退休 ID 已落盘，未知执行结果、身份冲突及未决责任已解决。
3. 非目标数据符合冻结基线，正常业务读取、标准审计和当前消息处理完成实际验收。
4. 仅本批备份、恢复副本和临时材料按原 owner 销毁，逐处复读确认零剩余。
5. 普通工作流、平台和主机设置已恢复，原 owner 没有恢复待办。

源码检查、CI 和容器就绪分别记录。上述生产结论在本候选中仍为 unknown。材料销毁后的故障采用向前修复，不能再使用已经退出的一次性恢复入口。

## 无正文历史证明来源

原工具设计与当时的检查记录保留在[固定 B Git 快照](https://github.com/FangcunMount/qs-server/blob/6fdae9a1ebe74490309e88cdee4cec9b0c675715/scripts/database/compatibility-retirement.md)，source SHA 为 `6fdae9a1ebe74490309e88cdee4cec9b0c675715`。该来源标记用于定位旧证明，不能当作当前工具、生产运行或本次销毁已成功的结论；本页不复制旧执行器正文或旧验证清单。
