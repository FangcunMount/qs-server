# Interpretation 历史 Presentation 与 Template 治理台账

> 状态：执行中  
> 生产只读基线：2026-08-06  
> 证据运行：[Database Operations 31075388732](https://github.com/FangcunMount/qs-server/actions/runs/31075388732)  
> 审计版本：`c9eb01eac16b166986644b49771ca8933aa54d24`

## 1. 结论

本轮治理必须拆成两个互不混写的批次：

1. 历史 `presentation_profile` 缺口可以只使用不可变 Artifact 自身的 `dimensions` 等价物化，不需要读取当前 ModelCatalog，也不需要重新生成历史报告；
2. `legacy-v1` 是 212,457 份 Generation/Artifact 的真实历史身份，不得批量改名、覆盖或删除。Template 治理应补齐发布身份后，通过新版本承接未来写入。

## 2. Presentation 生产基线

### 2.1 总量与分类

| 指标 | 结果 |
| --- | ---: |
| active artifacts | 212,457 |
| 已有 `frozen` profile | 175,597 |
| 缺 `presentation_profile` | 36,860 |
| 可由 `dimensions` 等价回填 | 36,860 |
| 无 dimensions | 0 |
| dimensions 非数组 | 0 |
| dimensions 非空但 factor code 全空 | 0 |
| active archives | 0 |
| 最近 24 小时新增缺口 | 0 |
| 最近 7 天缺口 | 86 |
| 最新缺口生成时间 | `2026-08-01T03:33:08Z` |

36,860 条缺口全部属于同一运行时分组：

```text
model_kind       = scale
builder_identity = factor-scoring
template_version = legacy-v1
```

因此本批不存在需要人工判断的 ambiguous 记录，目标集合精确为：

```text
interpret_report_artifacts
  where deleted_at = null
    and presentation_profile is missing/null
    and dimensions is a non-empty array
```

### 2.2 等价物化规则

回填值必须逐条从原 Artifact 的 dimensions 计算：

```text
presentation_profile.visible_factor_codes
  = dimensions[].factor_code 按原顺序去空、去重

presentation_profile.source
  = legacy_artifact_dimensions/v1
```

禁止使用当前 ModelCatalog visibility，禁止修改正文、维度、结论、风险、主体关联、Generation/Run 引用与生成时间。

## 3. Template 生产基线

### 3.1 发布记录

当前有 4 个 published release，均为 `legacy-v1`：

| TemplateID | BuilderIdentity | AdapterKey |
| --- | --- | --- |
| `standard` | `factor-scoring` | - |
| `mbti` | `typology` | `personality_type` |
| `sbti` | `typology` | `personality_type` |
| `bigfive` | `typology` | `trait_profile` |

### 3.2 历史引用

| 运行事实 | 数量 |
| --- | ---: |
| `legacy-v1` generations | 212,457 |
| `legacy-v1 / factor-scoring / report-content/v1` artifacts | 205,185 |
| `legacy-v1 / norm-profile / report-content/v1` artifacts | 7,272 |

这些历史记录必须继续诚实保留 `legacy-v1`。

### 3.3 当前 ModelCatalog 引用

24 个 active published snapshot 都没有显式 `template_version`：

| 模型类型 / 模板 | 数量 |
| --- | ---: |
| scale / 无 TemplateID | 17 |
| behavioral_rating / 无 TemplateID | 2 |
| typology / `mbti` | 2 |
| typology / `bigfive` | 1 |
| typology / `sbti` | 1 |
| typology / `enneagram` | 1 |

`enneagram` 已有 active ModelCatalog 引用，但当前 4 条 ReportTemplate release 中没有 `enneagram@legacy-v1`。这进一步证明现有 release 集合只是兼容目录，不是完整的生成资产清单。

## 4. 分阶段执行与门禁

| 阶段 | 内容 | 状态 | 退出条件 |
| --- | --- | --- | --- |
| A | 生产只读分类与引用矩阵 | 已完成 | run 31075388732 成功，缺口完整分类 |
| B | presentation 治理工具与表征测试 | 执行中 | audit/apply/verify/rollback 均有测试，默认 dry-run |
| C | 备份、canary、批量回填 | 待执行 | 36,860/36,860 精确更新并验证，正文 hash 不变 |
| D | 退役动态 presentation 兼容 | 待执行 | 缺口为 0，24 小时新增为 0，接口等价 |
| E | Template manifest 与发布门禁 | 待执行 | 发布资产能证明 Builder/schema/manifest，所有模型发布 fail-closed |
| F | 新 TemplateVersion 灰度 | 待执行 | 新 snapshot 显式冻结版本；新旧版本均可精确解析和回滚 |

## 5. 回滚边界

- Presentation 回滚只允许操作本次 manifest 中的 Artifact ID；
- 只有当已落库 profile 与 manifest 预期完全一致时，才允许 `$unset`；
- Template 回滚只改变后续 ModelCatalog 选择，不修改历史 Generation、Artifact 或 Outcome；
- `legacy-v1` release 与 Builder 在历史重放保留期内不得删除。

## 6. 关闭标准

只有同时满足以下条件，才能关闭 `FINAL-P2-001`：

- 36,860 条 presentation 缺口全部完成等价物化；
- 写后审计确认缺口 0、异常 0、正文变化 0；
- 动态 presentation 兼容分支已退役，持久化 source 仍可读取；
- active ModelCatalog snapshot 均显式冻结可发布的 TemplateID/TemplateVersion；
- Template release 与精确 Builder、ContentSchema、manifest 一致；
- 新版本 canary、回滚和旧版本重放验证通过。
