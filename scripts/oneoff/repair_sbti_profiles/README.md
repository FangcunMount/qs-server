# SBTI 结果 Profile 定点修复

`repair_sbti_profiles` 修复历史 SBTI DefinitionV2 中缺失或错误的结果执行配置：

- 为 25 个普通结果写入对应的 15 位 L/M/H `Pattern`；
- 将普通结果保持为 `IsSpecial=false` 并移除错误的 `Trigger`；
- 将 `HHHH`、`DRUNK` 设置为 `IsSpecial=true`、移除 `Pattern`，并写入对应 `Trigger`；
- 将已废弃的 `sbti` outcome/report adapter 迁移为通用 `personality_type` adapter；
- 保留 DefinitionV2 中题目贡献、结果文案、其他报告配置及未知扩展字段；
- 只保存草稿，不自动发布。

## 配置来源与许可

执行配置来自 [serenakeyitan/sbti-wiki](https://github.com/serenakeyitan/sbti-wiki) 的固定提交：

- revision：`6fbd41d63c60b322bb695e92457baa1b72fc3917`；
- 维度来源：`data/dimensions.json`；
- 模式及特殊结果来源：`data/patterns.json`；
- license：CC BY-NC-SA 4.0；
- attribution：SBTI 原始文案版权归 B 站 up 主“蛆肉儿串儿”（UID 417038183），配置整理归 `serenakeyitan/sbti-wiki`。

脚本目录下的 `data/` 是上述提交的最小执行投影，只保留维度顺序、维度元数据、普通 Pattern 和特殊 Trigger。脚本不会导入或覆盖以下 wiki 内容：

- 结果完整文案与图片：受非商业、署名及相同方式共享条款约束；
- `rarity.json`：它是均匀随机采样得到的理论分布，不是真实用户统计，也不是常模。

因此，更新 wiki 快照时必须人工审查上游提交、许可和数据语义，不能直接跟随 `main`。

## 更新快照

1. 只读拉取上游并记录完整 commit SHA；
2. 对比上游 `data/dimensions.json`、`data/patterns.json` 与当前 `data/`；
3. 仅将脚本实际使用的字段投影到本目录，更新 `sbtiWikiRevision`；
4. 运行测试，确认上游配置仍为 15 个维度、25 个普通结果、2 个特殊结果，并与服务内嵌 legacy seed 的执行字段一致；
5. 在真实环境先 dry-run，审核逐字段变更后再使用 `--apply`。

## 安全约束

脚本写入前必须满足以下条件，否则立即停止：

1. `Measure.FactorGraph.Roots` 与固定 wiki 快照中的 SBTI 因子顺序完全一致；
2. 顶层 `Outcomes` 和 type conclusion 的 `Profiles` Code 集合与快照完全一致；
3. Code 不得为空或重复；
4. 每个普通 Pattern 去掉连字符后必须恰好包含 15 个级别，且只能使用 `L/M/H`；
5. 特殊结果必须有 Trigger 且不能有 Pattern；
6. `OutcomeMapping.DetailKind` 和首个报告 section 的 Kind 必须为 `personality_type`；
7. adapter 只允许从空值或 `sbti` 迁移为 `personality_type`，不会覆盖 `trait_profile` 等无关配置；
8. 修复结果必须通过 DefinitionV2 领域结构校验。

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

确认变更后写入草稿：

```bash
QS_APISERVER_URL=https://qs.example.com \
QS_OPERATOR_TOKEN="$QS_OPERATOR_TOKEN" \
go run ./scripts/oneoff/repair_sbti_profiles/ \
  --model-code SBTI_FUN \
  --backup-dir ./repair_backups \
  --apply
```

`QS_APISERVER_URL` 可以是服务 origin，也可以直接包含 `/api/v1`。修复完成后仍需在后台完成报告预览和人工发布。

如果之前已经保存 Pattern/Trigger，但因 `sbti` adapter 废弃而校验失败，更新脚本后重新 dry-run，期望只出现：

```text
Planned changes: patterns=0 special_flags=0 triggers=0 adapters=2 total=2
```

确认两项均为 `sbti -> personality_type` 后，使用新的备份目录再次执行 `--apply`。旧版隐式 `scoring_mode` 信息属于兼容 warning，不阻塞校验；任何 `validation error` 都仍然禁止发布。

## 验证

```bash
go test ./scripts/oneoff/repair_sbti_profiles/
```
