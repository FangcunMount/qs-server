# govern_interpretation_template_releases

对 Interpretation `legacy-v1` 与 `2026-08-v1` ReportTemplate release 执行 manifest 驱动的审计、物化、验证和回滚。

目标集合固定为 `interpretation_report_templates`。工具只会：

1. 为既有 `standard`、`mbti`、`sbti`、`bigfive` release 增加 `report_type`、完整 manifest 和 SHA-256；
2. 补建代码中已注册但生产缺失的 `enneagram@legacy-v1` release；
3. 为新版本补建 `standard`、`mbti`、`sbti`、`bigfive`、`enneagram` 五个独立 release；
4. 保留现有 release 的 ID、生命周期时间、发布人和旧身份字段；
5. 不修改任何 Generation、Artifact、Outcome 或 ModelCatalog snapshot。

manifest 指纹由 Go Registry 契约测试和 Node 工具测试双向固定。任一 release 的状态、Builder、Adapter、原文哈希或 manifest 与预期不一致时，写操作拒绝执行。

## 环境变量

| 变量 | 说明 |
| --- | --- |
| `TEMPLATE_RELEASE_OPERATION` | `audit`、`apply`、`verify`、`rollback`；默认 `audit` |
| `TEMPLATE_RELEASE_TARGET_VERSION` | `legacy-v1`（默认）或 `2026-08-v1` |
| `TEMPLATE_RELEASE_MANIFEST_PATH` | 治理 manifest 路径；必填 |
| `TEMPLATE_RELEASE_CONFIRM` | apply/rollback 精确确认词 |

apply 确认词：

```text
materialize-interpretation-template-manifest-v1
```

`2026-08-v1` apply 确认词：

```text
publish-interpretation-template-2026-08-v1
```

rollback 确认词：

```text
rollback-interpretation-template-manifest-v1
```

`2026-08-v1` rollback 确认词：

```text
rollback-interpretation-template-2026-08-v1
```

## 本地纯函数测试

```bash
node --test scripts/oneoff/govern_interpretation_template_releases/govern.test.js
```

生产只允许通过受保护的 GitHub Actions 入口执行。不得提交生产 manifest、MongoDB URI 或密码。
