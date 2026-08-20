# Migration 与恢复

本目录集中维护 schema migration、dirty 状态、one-off、恢复和生产证据边界。它不提供绕过审批的“万能修复命令”。

## 阅读顺序

1. [Migration、dirty 与回退边界](./10-Migration、dirty与回退边界.md)
2. [One-off、恢复与生产证据](./20-One-off、恢复与生产证据.md)

实现细节以 `internal/pkg/migration` 和各 one-off 源码为准；其 sidecar 只说明包契约、参数和安全前提。某次执行的对象数、主机数、耗时和结果进入[基础设施生产证据台账](../../00-总览/10-基础设施生产证据台账.md)，不回写稳定说明页。

## Canonical 边界

| 主题 | 主文档 | 投影 |
| --- | --- | --- |
| migration / dirty / rollback | [10](./10-Migration、dirty与回退边界.md) | `internal/pkg/migration/README.md` 仅保留包级使用与开发约束 |
| one-off / recovery / production evidence | [20](./20-One-off、恢复与生产证据.md) | `scripts/oneoff/README.md` 仅保留工具索引和逐工具安全级别 |

## 最低验证

```bash
go test -count=1 ./internal/pkg/migration
go test -count=1 ./scripts/oneoff/...
```

带 DSN、Mongo Replica Set 或生产数据的用例若因环境变量缺失而 skip，必须在证据中明确写为未运行，不能把包级绿色概括成真实数据库通过。
