# 兼容链维护隔离说明的退出记录

本页对应 R4 源码退出候选，`doc_status=needs_review`、`runtime_status=unknown`。本批专用平台隔离、SSH 管理、服务停止、数据库 writer lease 与恢复执行器随其调用者和测试退出。它们的旧入口清单不再描述当前源码能力。

## 保留的正常职责

普通部署和数据库操作继续使用现有受保护工作流、宿主配置和原权限边界。[GitHub Actions 交付契约](../../.github/workflows/README.md)维护现行 workflow；[基础设施生产证据台账](../../docs/00-总览/10-基础设施生产证据台账.md)保存独立的运行版本和观测。

四对象的退休身份、标准读取和迁移启动保护继续保留。删除临时控制代码不会授予普通 workflow 绕过 schema dirty、源身份或部署健康检查的权限。

## 应用前需保存的实际结论

R4 必须在本批实际停止、删除、验收、材料清零和设置恢复之后应用。原窗口的 writer 归属、数据库准入恢复、原服务实例、排队及旧 ref 隔离和最终 workflow 设置需绑定实际 owner 与回执；全部门禁见[四对象退出说明](compatibility-retirement.md#final-historical-readback-and-temporary-material-cleanup)。

本候选未给这些生产结论签发成功，也没有保留通用认证、全 Broker 历史或新的运维平台作为清理交付物。

## 无正文历史证明来源

原入口分析和当时未覆盖项见[固定 B Git 快照](https://github.com/FangcunMount/qs-server/blob/6fdae9a1ebe74490309e88cdee4cec9b0c675715/scripts/database/compatibility-retirement-fence-analysis.md)，source SHA 为 `6fdae9a1ebe74490309e88cdee4cec9b0c675715`。该旧证明来源保留原身份，当前状态为 needs_review；不把历史入口数量或旧脚本检查当作现行隔离能力。
