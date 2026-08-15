# govern_interpretation_presentation_profiles

对 Interpretation 历史 `presentation_profile` 执行 manifest 驱动的审计、等价物化、验证和回滚。

工具只允许操作：

- `interpret_report_artifacts`

默认操作是只读 `audit`。写入只会设置或撤销 `presentation_profile`，不会读取当前 ModelCatalog，也不会重新生成报告。

## 不变量

1. profile 只从同一份不可变文档的 `dimensions[].factor_code` 生成；
2. factor code 保持原顺序，忽略空字符串并按首次出现去重；
3. manifest 保存除 `presentation_profile` 外整份文档的 SHA-256；
4. apply/verify/rollback 前均重新验证 protected hash；
5. apply 使用 `domain_id + deleted_at + 原 dimensions + profile 缺失` CAS；
6. rollback 只撤销与 manifest 完全相同的 `legacy_artifact_dimensions/v1` profile；
7. ID 始终使用十进制字符串，禁止经过 JavaScript Number 精度转换。

## 环境变量

| 变量 | 说明 |
| --- | --- |
| `PRESENTATION_OPERATION` | `audit`、`apply`、`verify`、`rollback`；默认 `audit` |
| `PRESENTATION_COLLECTION` | 目标集合；默认 `interpret_report_artifacts` |
| `PRESENTATION_MANIFEST_PATH` | manifest 路径；audit 可选，其他操作必填 |
| `PRESENTATION_AFTER_ID` | 只处理大于该 domain ID 的记录；默认 `0` |
| `PRESENTATION_MAX_RECORDS` | 本轮最多处理条数；`0` 表示不限 |
| `PRESENTATION_REQUIRE_COMPLETE` | verify 是否要求 manifest 全量完成且剩余缺口等于无 dimensions 数；默认 `false` |
| `PRESENTATION_CONFIRM` | apply/rollback 的精确确认词 |

apply 确认词：

```text
materialize-legacy-artifact-dimensions-v1
```

rollback 确认词：

```text
rollback-legacy-artifact-dimensions-v1
```

## 本地纯函数测试

```bash
node --test scripts/oneoff/govern_interpretation_presentation_profiles/govern.test.js
```

生产只允许通过受保护的 GitHub Actions 入口执行。不得把 MongoDB URI、密码或生产 manifest 提交到仓库。
