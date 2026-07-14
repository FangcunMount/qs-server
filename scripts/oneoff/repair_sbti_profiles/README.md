# SBTI 结果 Profile 定点修复

`repair_sbti_profiles` 修复历史 SBTI DefinitionV2 中缺失的结果 `Pattern` 和错误的 `IsSpecial` 标记。

脚本以服务内嵌的 `sbti_fun.json` 为唯一事实源：

- 为 25 个普通结果写入对应的 15 位 L/M/H Pattern；
- 将普通结果保持为 `IsSpecial=false`；
- 将 `HHHH`、`DRUNK` 等 seed 中的特殊结果设置为 `IsSpecial=true`，并移除 Pattern；
- 保留 DefinitionV2 中题目贡献、结果文案、报告配置及未知扩展字段；
- 只保存草稿，不自动发布。

## 安全约束

脚本写入前必须满足以下条件，否则立即停止：

1. `Measure.FactorGraph.Roots` 与内置 SBTI 因子顺序完全一致；
2. 顶层 `Outcomes` 和 type conclusion 的 `Profiles` Code 集合与 seed 完全一致；
3. Code 不得为空或重复；
4. 每个普通 Pattern 去掉连字符后必须与因子数量一致，且只能包含 `L/M/H`；
5. 修复结果必须通过 DefinitionV2 领域结构校验。

写入使用受保护的模型定义 API，不直接修改 MongoDB。`--apply` 前会以 `0600` 权限保存原始 DefinitionV2 备份；写入后会调用模型校验接口。如果校验未通过，草稿会保留但脚本返回失败，禁止继续发布。

## 执行

operator token 必须具有模型定义读取、编辑和校验权限。token 只从环境变量读取，不接受命令行参数。

先执行 dry-run：

```bash
QS_APISERVER_URL=https://qs.example.com \
QS_OPERATOR_TOKEN="$QS_OPERATOR_TOKEN" \
go run ./scripts/oneoff/repair_sbti_profiles/ \
  --model-code SBTI_FUN
```

确认输出的 25 个 Pattern 变更和 2 个特殊标记变更后写入草稿：

```bash
QS_APISERVER_URL=https://qs.example.com \
QS_OPERATOR_TOKEN="$QS_OPERATOR_TOKEN" \
go run ./scripts/oneoff/repair_sbti_profiles/ \
  --model-code SBTI_FUN \
  --backup-dir ./repair_backups \
  --apply
```

`QS_APISERVER_URL` 可以是服务 origin，也可以直接包含 `/api/v1`。修复完成后仍需在后台完成报告预览和人工发布。

## 验证

```bash
go test ./scripts/oneoff/repair_sbti_profiles/
```
