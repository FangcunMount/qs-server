# govern_interpretation_model_template_routes

将 active ModelCatalog `published_snapshot` 以新不可变 release 的方式切换到显式的 Interpretation `TemplateID + TemplateVersion`。

工具不会原地改写已发布定义，也不会删除历史 snapshot：

1. `audit` 固化全部 active snapshot 的内容哈希、旧/新 release 身份与新 ObjectID，并由 Go helper 复用服务端 `CanonicalContentHash` 固化变更前后的 DefinitionV2 哈希；
2. `apply` 在 Mongo 事务中归档旧 active release，并插入或重新激活显式绑定 `2026-08-v1` 的新 release；
3. `verify` 校验新旧 snapshot 内容、模板发布路由和 active 集合完整性；
4. `rollback` 只把后续选择切回旧 release，新 release 保留为 archived，保证已经产生的新版本 Outcome 仍可精确读取。

新 release version 由旧 version 确定性派生：

```text
<source-release-version>-report-202608-v1
```

## 环境变量

| 变量 | 说明 |
| --- | --- |
| `MODEL_TEMPLATE_ROUTE_OPERATION` | `audit`、`apply`、`verify`、`rollback` |
| `MODEL_TEMPLATE_ROUTE_MANIFEST_PATH` | 不可变治理 manifest 路径 |
| `MODEL_TEMPLATE_ROUTE_AFTER_ID` | 可选，按 source ObjectID 继续 |
| `MODEL_TEMPLATE_ROUTE_MAX_RECORDS` | `0` 为全部；canary 建议 `1` |
| `MODEL_TEMPLATE_ROUTE_REQUIRE_COMPLETE` | full verify 时设为 `true` |
| `MODEL_TEMPLATE_ROUTE_CONFIRM` | apply/rollback 精确确认词 |

apply：

```text
activate-model-template-route-2026-08-v1
```

rollback：

```text
rollback-model-template-route-2026-08-v1
```

生产执行前必须先验证 Mongo 全量备份，并确认五个 `2026-08-v1` ReportTemplate release 均已发布。生产 manifest、数据库 URI 和密码不得提交。

`main.go` 只在 `audit` 后读取 snapshot 并补充 canonical hash，不执行数据库写入；缺失/漂移的旧 hash 会直接阻断 manifest 固化。`apply`、`verify`、`rollback` 均拒绝未包含双 hash 的旧 manifest。
