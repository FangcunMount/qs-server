# 人格测评历史 adapter 归一化

本脚本只处理以下四份已发布的 `DefinitionV2`，并拒绝版本漂移：

- `MBTI_OEJTS@v25`: `mbti` → `personality_type`
- `MBTI_FC_93@v15`: `mbti` → `personality_type`
- `SBTI_FUN@v29`: `sbti` → `personality_type`
- `BIG5_IPIP_50@v9`: `bigfive` → `trait_profile`

它不会处理九型人格。更新经由受保护的 DefinitionV2 编辑接口和 assessment-release 发布接口完成；不会直接写 MongoDB。

先运行 dry-run：

```bash
QS_APISERVER_URL=https://qs.example.com QS_OPERATOR_TOKEN=... \
  go run ./scripts/oneoff/migrate_personality_runtime_adapters/
```

保存草稿并让服务端校验：

```bash
QS_APISERVER_URL=https://qs.example.com QS_OPERATOR_TOKEN=... \
  go run ./scripts/oneoff/migrate_personality_runtime_adapters/ --apply
```

保存、校验并重新发布：

```bash
QS_APISERVER_URL=https://qs.example.com QS_OPERATOR_TOKEN=... \
  go run ./scripts/oneoff/migrate_personality_runtime_adapters/ --apply --publish
```

`--apply` 会在 `migration_backups/`（可用 `--backup-dir` 覆盖）保存每一份写入前的 DefinitionV2，权限为仅当前用户可读。`--publish` 必须和 `--apply` 一起使用。每个目标会先确认当前模型没有未发布草稿；仍包含旧 adapter 的目标还必须处于脚本锁定版本，任一检查失败即停止，避免覆盖人工编辑或回退新版本。已经归一化的目标即使因本次或此前发布产生了新版本，也会安全跳过。

运行令牌需要读取模型、编辑 DefinitionV2 与发布 assessment release 的权限。发布后脚本会重新读取已发布快照，确认它已是不同于发布前的新版本，且两个 adapter 都已归一化。不能假定版本只加一：保存草稿和发布都会推进模型配置修订，历史快照版本也可能与当前修订不连续。
