# BRIEF-2、感觉统合 SPM、SBTI 服务器运行指导

本文用于在服务器上维护以下测评配置：

| 测评 | 问卷 | 维护方式 | `--apply` 的结果 |
| --- | --- | --- | --- |
| BRIEF-2 家长版 | `gXkk9W@4.0.1` | Go 脚本直连 MongoDB | 写入常模、模型并创建正式发布快照 |
| 感觉统合 SPM | `bJFKi3@4.0.1` | Go 脚本直连 MongoDB | 写入常模、模型并创建正式发布快照 |
| SBTI | 模型 `SBTI_FUN` | Go 脚本调用 qs-server REST API | 备份并更新草稿，服务端校验，不发布 |

所有命令都必须从 `qs-server` 仓库根目录运行。本批次不使用 MySQL，因此不需要配置 `MYSQL_URL`。

## 1. 重要安全说明

1. BRIEF-2/SPM 的常模已内嵌在 Go 脚本中，服务器不需要 PHP、本地附件或 `--norm-source`。
2. BRIEF-2/SPM 的 dry-run 只校验内嵌常模和题目映射，不连接 MongoDB。
3. BRIEF-2/SPM 的 `--apply` 会直接发布，不需要再到 operating 后台点击发布。
4. BRIEF-2/SPM 会在一个 MongoDB 多文档事务中写入常模、草稿和发布快照；目标 MongoDB 必须是副本集或分片集群。
5. 首次执行禁止使用 `--force`。如果发现同编码模型，停止操作并先审阅现有数据。
6. SBTI 的 `--apply` 不发布；执行后必须在 operating 后台预览并人工发布。
7. 不要将 MongoDB URI、API token 或备份文件提交到 Git。

## 2. 服务器前置条件

服务器需要：

- 当前版本的 `qs-server` 代码；
- Go `1.25.12` 或与仓库 `go.mod` 兼容的版本；
- 能连接目标 MongoDB；
- MongoDB 支持多文档事务（副本集或分片集群）；
- 执行备份时需要 `mongodump`；
- SBTI 需要能访问 qs-server API，并持有模型定义读取、编辑和校验权限的 token。

进入仓库并检查工具：

```bash
cd /path/to/qs-server

go version
command -v mongodump
git status --short
```

确认正在使用包含以下目录的代码版本：

```bash
test -d ./scripts/oneoff/seed_brief2
test -d ./scripts/oneoff/seed_spm_sensory
test -d ./scripts/oneoff/repair_sbti_profiles
```

## 3. 准备环境变量

建议先关闭 shell 命令回显，避免凭据进入日志：

```bash
set +x

export MONGO_DB='qs'
export MONGO_URL='mongodb://user:password@host:27017/?authSource=admin'
export QS_APISERVER_URL='https://qs.example.com/api/v1'
export QS_OPERATOR_TOKEN="$(tr -d '\r\n' < /secure/path/qs-operating-api-token)"
```

如果服务器已经使用小写变量，可以在当前 shell 映射：

```bash
export MONGO_URL="$mongodb_url"
```

只检查变量是否存在，不输出变量内容：

```bash
test -n "$MONGO_URL" && echo 'MONGO_URL: set'
test -n "$MONGO_DB" && echo 'MONGO_DB: set'
test -n "$QS_APISERVER_URL" && echo 'QS_APISERVER_URL: set'
test -n "$QS_OPERATOR_TOKEN" && echo 'QS_OPERATOR_TOKEN: set'
```

确认 MongoDB 支持事务：

```bash
mongosh "$MONGO_URL" --quiet --eval '
const hello = db.getSiblingDB("admin").runCommand({hello: 1});
printjson({setName: hello.setName, msg: hello.msg});
if (!hello.setName && hello.msg !== "isdbgrid") {
  print("ERROR: MongoDB does not support multi-document transactions");
  quit(2);
}
'
```

副本集应输出非空 `setName`；分片集群应输出 `msg: "isdbgrid"`。检查失败时禁止执行 `--apply`。

## 4. 编译与单元测试

```bash
go test ./scripts/oneoff/internal/modelseed/ \
  ./scripts/oneoff/seed_brief2/ \
  ./scripts/oneoff/seed_spm_sensory/ \
  ./scripts/oneoff/repair_sbti_profiles/
```

三个包都必须显示 `ok`。失败时不要继续执行 `--apply`。

## 5. MongoDB 只读预检

先确认 BRIEF-2/SPM 模型是否已经存在。下面的命令只读数据库：

```bash
mongosh "$MONGO_URL" --quiet --eval '
const target = db.getSiblingDB(process.env.MONGO_DB);
for (const code of ["gXkk9W", "bJFKi3"]) {
  printjson({
    code,
    drafts: target.assessment_models.countDocuments({code, deleted_at: null}),
    published: target.published_assessment_models.countDocuments({model_code: code, deleted_at: null})
  });
}
'
```

首次初始化时，两个 code 的 `drafts` 和 `published` 都应为 `0`。如果任何值大于 `0`：

- 不要执行普通 `--apply`；
- 不要立即追加 `--force`；
- 先导出现有草稿、发布快照和关联常模，确认是修复还是整体替换任务。

脚本在真正写入时还会读取已发布问卷，并严格检查问卷版本是否分别为 `gXkk9W@4.0.1` 和 `bJFKi3@4.0.1`。

## 6. 备份 MongoDB

BRIEF-2/SPM 写入前备份目标集合：

```bash
export BACKUP_DIR="./repair_backups/mongo-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR"

mongodump --uri="$MONGO_URL" --db="$MONGO_DB" \
  --collection=assessment_norms --out="$BACKUP_DIR"
mongodump --uri="$MONGO_URL" --db="$MONGO_DB" \
  --collection=assessment_models --out="$BACKUP_DIR"
mongodump --uri="$MONGO_URL" --db="$MONGO_DB" \
  --collection=published_assessment_models --out="$BACKUP_DIR"

find "$BACKUP_DIR" -maxdepth 3 -type f -print
```

确认三个集合都有 `.bson` 文件后再继续。备份目录可能包含敏感数据，应限制权限并按服务器规范转存：

```bash
chmod -R go-rwx "$BACKUP_DIR"
```

### 6.1 当前 BRIEF-2 部分写入现场的恢复执行

如果数据库处于以下已确认状态：

- active 发布快照仍是 `scale/scale_default`；
- 草稿已经是 `behavioral_rating/brief2`、状态为 `draft`；
- `brief2-parent-cn-legacy-gXkk9W-v1` 常模已经存在；

部署修复后的脚本并确认现场备份非空，然后使用第 7.2 节命令加 `--force`。新脚本会在同一个事务中：复用相同常模、删除当前草稿、按 `model_code + questionnaire_code/version` 停用历史 scale 快照、创建 BRIEF-2 发布快照和 published 草稿。任一步失败都会回滚整个事务。

当前现场使用：

```bash
go run ./scripts/oneoff/seed_brief2/ \
  --mongo-uri "$MONGO_URL" \
  --mongo-db "$MONGO_DB" \
  --questionnaire-code gXkk9W \
  --questionnaire-version 4.0.1 \
  --factor-map ./scripts/oneoff/seed_brief2/data/gXkk9W_4.0.1_factor_map.json \
  --apply \
  --force
```

命令成功后必须先执行第 9 节验收；确认 BRIEF-2 的 active snapshot 已变为 `behavioral_rating/brief2` 后，才能继续 SPM。

## 7. BRIEF-2

### 7.1 dry-run

```bash
go run ./scripts/oneoff/seed_brief2/ \
  --questionnaire-code gXkk9W \
  --questionnaire-version 4.0.1 \
  --factor-map ./scripts/oneoff/seed_brief2/data/gXkk9W_4.0.1_factor_map.json
```

期望输出包含：

```text
model=gXkk9W
questionnaire=gXkk9W@4.0.1
norm=brief2-parent-cn-legacy-gXkk9W-v1
factors=13
strata=6
mapped_questions=60
excluded_questions=10
```

### 7.2 正式写入

```bash
go run ./scripts/oneoff/seed_brief2/ \
  --mongo-uri "$MONGO_URL" \
  --mongo-db "$MONGO_DB" \
  --questionnaire-code gXkk9W \
  --questionnaire-version 4.0.1 \
  --factor-map ./scripts/oneoff/seed_brief2/data/gXkk9W_4.0.1_factor_map.json \
  --apply
```

成功输出应包含：

```text
seeded BRIEF-2 model gXkk9W -> questionnaire gXkk9W@4.0.1
```

执行成功后立即完成第 9 节的 MongoDB 验收，再继续 SPM。

## 8. 感觉统合 SPM

这里的 SPM 是 Sensory Processing Measure，不是 Raven SPM。

### 8.1 dry-run

```bash
go run ./scripts/oneoff/seed_spm_sensory/ \
  --questionnaire-code bJFKi3 \
  --questionnaire-version 4.0.1 \
  --factor-map ./scripts/oneoff/seed_spm_sensory/data/bJFKi3_4.0.1_factor_map.json
```

期望输出包含：

```text
model=bJFKi3
questionnaire=bJFKi3@4.0.1
norm=spm-sensory-cn-legacy-bJFKi3-v1
factors=8
lookups=201
percentile_fallbacks=9
mapped_questions=75
```

### 8.2 正式写入

```bash
go run ./scripts/oneoff/seed_spm_sensory/ \
  --mongo-uri "$MONGO_URL" \
  --mongo-db "$MONGO_DB" \
  --questionnaire-code bJFKi3 \
  --questionnaire-version 4.0.1 \
  --factor-map ./scripts/oneoff/seed_spm_sensory/data/bJFKi3_4.0.1_factor_map.json \
  --apply
```

成功输出应包含：

```text
seeded SPM sensory model bJFKi3 -> questionnaire bJFKi3@4.0.1
```

## 9. BRIEF-2/SPM 写入后验收

运行只读验收命令：

```bash
mongosh "$MONGO_URL" --quiet --eval '
const target = db.getSiblingDB(process.env.MONGO_DB);
for (const spec of [
  {code: "gXkk9W", norm: "brief2-parent-cn-legacy-gXkk9W-v1", algorithm: "brief2"},
  {code: "bJFKi3", norm: "spm-sensory-cn-legacy-bJFKi3-v1", algorithm: "spm_sensory"}
]) {
  const norm = target.assessment_norms.findOne(
    {table_version: spec.norm, deleted_at: null},
    {table_version: 1, kind: 1, algorithm: 1, form_variant: 1}
  );
  const draft = target.assessment_models.findOne(
    {code: spec.code, deleted_at: null},
    {code: 1, kind: 1, algorithm: 1, questionnaire_code: 1, questionnaire_version: 1, status: 1}
  );
  const published = target.published_assessment_models.findOne(
    {model_code: spec.code, deleted_at: null},
    {model_code: 1, model_algorithm: 1, questionnaire_code: 1, questionnaire_version: 1}
  );
  printjson({expected: spec, norm, draft, published});
}
'
```

必须确认：

- BRIEF-2 的算法为 `brief2`，问卷为 `gXkk9W@4.0.1`；
- SPM 的算法为 `spm_sensory`，问卷为 `bJFKi3@4.0.1`；
- 两份常模、模型记录和发布快照都存在；
- SPM 没有使用 Raven 的 `spm` 算法。

然后在 operating 后台分别打开两个模型，检查因子、结果类型和报告预览。数据库已经存在发布快照，不要重复发布。

## 10. SBTI

SBTI 使用 API token，不需要 MongoDB URI。token 只能通过环境变量传递，不支持命令行参数。

### 10.1 dry-run

```bash
go run ./scripts/oneoff/repair_sbti_profiles/ \
  --api-base "$QS_APISERVER_URL" \
  --model-code SBTI_FUN
```

检查输出中的每一项变化。脚本应修复普通结果的 Pattern，以及 `HHHH`、`DRUNK` 的特殊 Trigger；dry-run 不写入草稿。

### 10.2 正式更新草稿

```bash
export SBTI_BACKUP_DIR="./repair_backups/sbti-$(date +%Y%m%d-%H%M%S)"

go run ./scripts/oneoff/repair_sbti_profiles/ \
  --api-base "$QS_APISERVER_URL" \
  --model-code SBTI_FUN \
  --backup-dir "$SBTI_BACKUP_DIR" \
  --apply
```

脚本会以 `0600` 权限保存原始 DefinitionV2，然后写入并调用服务端校验。执行完成后：

1. 检查备份文件存在且非空；
2. 在 operating 后台打开 `SBTI_FUN`；
3. 重新执行“本地校验”；
4. 预览普通结果和 `HHHH`、`DRUNK` 报告；
5. 人工确认后发布。

## 11. 常见失败处理

### `draft model ... already exists` 或 `published model ... already exists`

说明目标模型已经存在。停止执行，不要直接增加 `--force`。先比较现有草稿、发布快照、问卷版本和常模版本。

### `questionnaire ... is published by ... other model(s)`

目标问卷版本被不同 model code 占用。脚本不会替换无关模型；必须人工确认模型归属，不得绕过。

### `model ... bound to another questionnaire version`

同 model code 还存在绑定其他问卷版本的 active 快照。脚本会停止，避免一次迁移误删多个版本。

### `Transaction numbers are only allowed ...`

MongoDB 不支持多文档事务，或连接没有进入副本集/分片集群。不要降级成非事务写入；修正连接拓扑后再执行。

### `active version is ..., want 4.0.1`

说明当前发布问卷版本与脚本绑定不一致。不要修改命令绕过；应先确认问卷是否重新发布，以及题目 code、选项分值和 factor-map 是否仍兼容。

### `norm content conflicts` 或相同常模版本内容不同

常模版本是不可变键。不能覆盖旧版本；需要审阅差异并使用新的常模版本，同时更新模型引用。

### SBTI 返回 `401` 或 `403`

确认 token 未过期，并具有模型定义读取、编辑和校验权限。不要把 token 打印到终端或通过命令行参数传递。

### SBTI 写入后服务端校验失败

不要发布。保留脚本生成的备份和错误输出，回到 operating 后台检查 DefinitionV2；必要时使用备份内容恢复草稿。

### 进程中断或 MongoDB 网络错误

修复后的 BRIEF-2/SPM 使用 MongoDB 事务，命令返回失败时事务应整体回滚。仍需先执行第 9 节的只读查询确认状态，不要把 `--force` 当作普通重试参数。

## 12. `--force` 使用条件

只有同时满足以下条件，才可以评估使用 `--force`：

1. 三个 MongoDB 集合已有可恢复备份；
2. 已导出目标模型的草稿和发布快照；
3. 已确认问卷仍为指定版本，题目和选项计分没有漂移；
4. 明确批准整体替换同编码模型；
5. 已安排写入后的缓存刷新或服务重启；
6. 已准备失败后的恢复步骤。

`--force` 会在事务中替换同编码草稿，并停用具有相同 model code 和问卷 code/version 的历史发布快照；它允许历史 `scale` 迁移到 `behavioral_rating`，但不会覆盖其他 model code 或其他问卷版本。它不是普通的幂等重试参数。
