# 兼容链退役工具退出说明

本轮四个旧对象已删除，最终 R4 已合并并上线，专用工具、测试和本批 ServerA 材料已退出。当前业务保留标准消息链及退休身份保护。下文记录此次执行的实际证据；文档整体的语义签署和其他基础设施验收仍按各自范围管理。

## 四对象范围

| 模块 | 退役对象 | 保留职责 |
| --- | --- | --- |
| 事件与可靠消息 | MySQL `domain_event_outbox`、MongoDB `domain_event_outbox` | 原业务事实、EventEvidence、标准 Outbox 与双向审计 |
| AI 运行时协作 | MySQL `ai_bridge_commands`、`ai_messaging_legacy_commands` | 当前 MQ 账本、原 command ID、退休证据及迟到回执隔离 |

此前 CBPT 清理档案、普通备份和非目标数据不属于本批材料销毁范围。36 个存量改名候选、AI 重复正文精简和人员维护对象仍属后续独立阶段。

## 源码退出与长期入口

[PR #346](https://github.com/FangcunMount/qs-server/pull/346) 已移除专用 history/retirement/handoff CLI、三个退役 Action、维护包、Python 编排、临时运行观察和仅保护这些实现的测试：449 个文件删除，仓库净减少 178,233 行。共享构建、镜像导出和 CI 撤掉临时分支，继续执行普通交付与长期业务检查；历史迁移文件保留。

现用 MQ 的目录、组织范围与关联检查见[MQ 只读数据库清单](../../cmd/qs-ai-bridge/README.md)。事件出站和业务审计见[标准 Outbox](../../docs/03-基础设施/event/30-Outbox可靠出站链路.md)。迁移及启动缺失保护见[迁移说明](../../internal/pkg/migration/README.md)。历史迁移文件保留原字节，退休 ID 不因旧源恢复而重新可用。

## Final historical readback and temporary material cleanup

2026-10-13，负责人明确授权直接删除本页四个旧对象，随后通过最终 R4 的普通三服务部署完成发布 B 与双库迁移。旧数据历史迁移与历史兼容已取消；完整恢复演练未完成，已按用户要求取消，未签发恢复成功证明。旧窗口退役链不再调用，失败的旧退役运行 `38316933488-1` 不作为本次删除、验收或材料清零的成功证明。

实际结果按以下范围分别记录：

1. MySQL `domain_event_outbox`、`ai_bridge_commands`、`ai_messaging_legacy_commands` 以及 MongoDB `domain_event_outbox` 已直接删除；上线后逐项回读均不存在，MySQL `100`／MongoDB `39` 均 clean。原 MySQL 库身份与 MongoDB 命名空间锚点保持一致。
2. 最终 R4 源码为 `627fefa44c22c36ef722a3d3d82cfd107b5f72b2`。[普通 CD 运行 `38323013758`](https://github.com/FangcunMount/qs-server/actions/runs/38323013758) 成功，包含三个服务的部署、治理检查及 K6 可观测性验证。后续只读回验确认 apiserver 1、collection 2、worker 3 个实例的实际程序版本均为该 SHA，六实例 ready 全部通过；apiserver／collection 的 Docker health 为 healthy，worker 未配置 Docker health，不能将其记为 Docker health 通过。
3. 历史回填未执行，不补造旧事件证据或恢复成功回执。当前标准事件证据、退休 ID 与 MQ 责任事实保留；部署和 ready 检查不代替全部业务场景验收。
4. 验收后已销毁本批两份备份及 ServerA 54 个固定材料范围：43 个范围删除、11 个原已缺失，删除 1,064 个文件和 63 个目录，最终剩余文件为 0。范围包括当前批次、已登记失败运行、保管材料及有限权限入口；普通备份、此前清理档案和共享父目录不在删除范围。该结果不宣称本机或 Mac mini 的全部临时材料已清零。
5. 临时 ServerA 路由覆盖和三个入口变量已删除，普通部署暂停变量保持原值 `true`。18 个仍有源码入口的工作流已恢复 active，12 个已无源码入口的旧工作流不重建。Mac mini 的 SonarQube 与 runner4 已实际恢复，runner5 已自行恢复，runner6 的 ops 标签由用户确认恢复。

无正文原始回执摘要：四对象缺失与双库 clean 验证为 `0db8758854385d684fb2a1c46f35f5080a158747a1550cf35046b15d3f8fb110`；六实例版本与 ready 验证为 `582ea0c0fb139e946abf1d4bb873c129e5241e5a815957955962394f1b4bcf4e`；54 范围退出为 `a57dfa0fde8331c68bb438fcd9f6693726b13f2aa39dec53c6158f6da2fe80e1`。退出回执的 12,821,309,650 字节为已删除登记文件大小之和，不能当作实测磁盘空间回收量。

源码检查、CI、部署健康、运行验收和临时材料退出分别记录。本批备份已销毁，后续故障采用向前修复；已经退出的一次性恢复入口不可再用。

## 无正文历史证明来源

原工具设计与当时的检查记录保留在[固定 B Git 快照](https://github.com/FangcunMount/qs-server/blob/6fdae9a1ebe74490309e88cdee4cec9b0c675715/scripts/database/compatibility-retirement.md)，source SHA 为 `6fdae9a1ebe74490309e88cdee4cec9b0c675715`。该来源标记用于定位旧证明，不能当作当前工具、生产运行或本次销毁已成功的结论；本页不复制旧执行器正文或旧验证清单。
