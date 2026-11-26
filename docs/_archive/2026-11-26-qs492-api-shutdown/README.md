# QS #492 API 关闭隔离复验

本记录归档 2026-11-26 对 [QS #492](https://github.com/FangcunMount/qs-server/pull/492) 后续修复的隔离复验。用 MySQL 111、MongoDB 44、Redis、RocketMQ 和 IAM 组成一次性夹具。用补丁构建的 Linux amd64 API 通过只读挂载运行，Worker 和 Collection 保持夹具原有镜像。

## 源码与证据

| 项目 | 标识 |
| --- | --- |
| 源码基线 | `45eb08ecd00e8e1b3515a0b2f365e0cecb04fd96` |
| 与现场复验源码对应的修复提交 | `65e53ce1022d069e32eed51a39b76efd516d612b` |
| 复验 API 二进制 SHA-256 | `4747229f05562113af28576c88d065a865fba14cb81b1536876d51f01af0ce39` |
| 本地隔离原始回执 SHA-256 | `ab103c14e1c627bf8697ed9ebeb713bac3c224d5deed3179ca8df0a576ad0524` |

## 进程关闭结果

| 核对项 | 正常 TERM | TLS 连接重置后 TERM | 恢复控制组容器后 TERM |
| --- | --- | --- | --- |
| 事务排空 | 成功 | 成功 | 成功 |
| 容器内部清理 | 成功 | 成功 | 成功 |
| 发布者退出 | 成功 | 成功 | 成功 |
| MySQL 关闭 | 成功 | 成功 | 成功 |
| MongoDB 关闭 | 成功 | 成功 | 成功 |
| Redis 默认连接关闭 | 成功 | 成功 | 成功 |
| Redis `lock_cache` 关闭 | 成功 | 成功 | 成功 |
| Redis `meta_cache` 关闭 | 成功 | `closeNotify` 报错 | 成功 |
| Redis `object_cache` 关闭 | 成功 | 成功 | 成功 |
| Redis `ops_runtime` 关闭 | 成功 | 成功 | 成功 |
| Redis `query_cache` 关闭 | 成功 | 成功 | 成功 |
| Redis `rank_cache` 关闭 | 成功 | 成功 | 成功 |
| Redis `sdk_cache` 关闭 | 成功 | 成功 | 成功 |
| Redis `static_cache` 关闭 | 成功 | `closeNotify` 报错 | 成功 |
| API 退出码 | 0 | 1 | 0 |

- 隔离代理重置 42 条 TLS 连接。两个 Redis 错误均出现在独立回执和最终合并错误中。所有阶段的 Worker 和 Collection 均退出 0，API 未发生 OOM。
- 两次 Checker 交接均验证停机前后 MySQL 和 MongoDB 的事务 owner 可用。事务绑定及两个保留 outbox 样本摘要一致。最终隔离容器、网络和卷已清退。
- 首轮在 Broker 路由准备阶段中止，原因是夹具的期望结果漏列 `ops_runtime`。改为按实际配置生成期望结果后完成全程复跑。首轮隔离对象单独核对，Docker 默认 bridge ID 在该轮发生变化。

## 源码回归

API 的 236 个 Go 测试包通过。使用部署构建标签对 `bootstrap`、`container`、`eventing/subsystem`、`process` 进行竞态检查，全部通过。多错误用例同时验证 MySQL、MongoDB 和两个 Redis profile 的错误仍可追溯。事务排空或发布者退出失败的用例验证后续依赖继续保留。

本记录用于源码审查与隔离验收。QS 生产发布仍禁用，AI 自动发布仍关闭，本轮生产窗口保持关闭。
