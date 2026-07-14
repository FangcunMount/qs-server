# BRIEF-2、感觉统合 SPM、SBTI 服务器维护手册

所有命令必须从 `qs-server` 仓库根目录运行。BRIEF-2/SPM 使用 MongoDB；SBTI 使用受保护的 REST API。三个脚本都默认先 dry-run。

BRIEF-2/SPM 的 `--apply` 会直接创建常模、模型草稿和正式发布快照，不需要再到 operating 后台发布；该过程不是跨集合事务。首次执行前必须备份，且先不要使用 `--force`。SBTI 的 `--apply` 只更新并校验草稿，仍需人工预览和发布。

## 1. 部署前检查

```bash
go test ./scripts/oneoff/seed_brief2/
go test ./scripts/oneoff/seed_spm_sensory/
go test ./scripts/oneoff/repair_sbti_profiles/
```

准备变量：

```bash
export MONGO_DB=qs
export MONGO_URL='mongodb://...'
export QS_APISERVER_URL='https://qs.example.com/api/v1'
export QS_OPERATOR_TOKEN='...'
```

本批次不使用 MySQL。不要把连接串和 token 写入仓库或执行日志。

如果服务器已有其他变量名，先在当前 shell 映射即可，例如：

```bash
export MONGO_URL="$mongodb_url"
```

## 2. BRIEF-2

```bash
# dry-run
go run ./scripts/oneoff/seed_brief2/ \
  --questionnaire-code gXkk9W --questionnaire-version 4.0.1 \
  --factor-map ./scripts/oneoff/seed_brief2/data/gXkk9W_4.0.1_factor_map.json

# apply
go run ./scripts/oneoff/seed_brief2/ \
  --mongo-uri "$MONGO_URL" --mongo-db "$MONGO_DB" \
  --questionnaire-code gXkk9W --questionnaire-version 4.0.1 \
  --factor-map ./scripts/oneoff/seed_brief2/data/gXkk9W_4.0.1_factor_map.json \
  --apply
```

期望 dry-run：13 个常模因子、6 个年龄/性别分层、60 个计分题、10 个排除题。

## 3. 感觉统合 SPM

```bash
# dry-run
go run ./scripts/oneoff/seed_spm_sensory/ \
  --questionnaire-code bJFKi3 --questionnaire-version 4.0.1 \
  --factor-map ./scripts/oneoff/seed_spm_sensory/data/bJFKi3_4.0.1_factor_map.json

# apply
go run ./scripts/oneoff/seed_spm_sensory/ \
  --mongo-uri "$MONGO_URL" --mongo-db "$MONGO_DB" \
  --questionnaire-code bJFKi3 --questionnaire-version 4.0.1 \
  --factor-map ./scripts/oneoff/seed_spm_sensory/data/bJFKi3_4.0.1_factor_map.json \
  --apply
```

期望 dry-run：8 个常模因子、201 个查表项、9 个顶端百分位回退、75 个映射题。

## 4. SBTI

```bash
# dry-run
go run ./scripts/oneoff/repair_sbti_profiles/ \
  --api-base "$QS_APISERVER_URL" --model-code SBTI_FUN

# apply，仅保存草稿并调用服务端校验，不发布
go run ./scripts/oneoff/repair_sbti_profiles/ \
  --api-base "$QS_APISERVER_URL" --model-code SBTI_FUN \
  --backup-dir ./repair_backups --apply
```

SBTI 成功后必须在 operating 后台预览报告并人工发布。

## 5. 强制覆盖规则

BRIEF-2/SPM 若发现已有同编码模型会停止。不要立即增加 `--force`。先完成：

1. 备份 `assessment_norms`、`assessment_models`、`published_assessment_models`；
2. 导出目标 code 的草稿和发布快照；
3. 确认问卷仍是指定版本；
4. 确认允许整体替换并安排缓存刷新或服务重启；
5. 才能在 apply 命令末尾增加 `--force`。

常模版本是不可变键：相同内容可重复执行，不同内容必须使用新版本，不能覆盖旧版本。
