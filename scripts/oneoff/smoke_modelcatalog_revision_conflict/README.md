# ModelCatalog REST revision-conflict smoke

这个工具验证已部署 qs-apiserver 的两个乐观锁冲突是否真正映射为 HTTP 409：

```text
Assessment Model draft basic-info concurrent PUT → HTTP 200 + HTTP 409
Questionnaire draft basic-info concurrent PUT    → HTTP 200 + HTTP 409
```

REST 请求没有暴露 revision，因此无法从客户端精确指定两个写者读取同一个 revision。工具使用同步起跑、并发放大和有限轮次来捕获 409；G4 Replica Set 集成测试仍是“两个写者恰好一人成功”的确定性 CAS 证据，本脚本只补部署环境的 REST 409 映射证据。

## 安全边界

- 只能操作专门创建、从未发布的 draft Model 和 Questionnaire。
- Model 必须绑定到指定 Questionnaire；apply 时还必须用 `--confirm-targets` 再次精确确认两个 code。
- 目标只要存在 active version、在线状态或不是 `draft`，脚本立即拒绝。
- 默认仅做 preflight；只有显式 `--apply` 才会并发写入。
- 每个目标执行后会恢复原始 basic-info 并再次 GET 校验；恢复失败时退出 2，并停止操作下一个目标。
- 即使恢复成功，revision/working version 也会递增，所以不能使用真实业务 Model、已发布 Model 或准备发布的草稿。

Token 需要同时拥有 ModelCatalog、Questionnaire 的管理和读取权限。不要把 token 写到命令行历史，优先使用 `--token-file`。

## 构建

```bash
go build -o /tmp/smoke-modelcatalog-revision-conflict \
  ./scripts/oneoff/smoke_modelcatalog_revision_conflict/
```

## 先做只读 preflight

```bash
set -o pipefail

/tmp/smoke-modelcatalog-revision-conflict \
  --api-base-url "$QS_APISERVER_URL" \
  --token-file "$QS_MODELCATALOG_ADMIN_TOKEN_FILE" \
  --model-code 'SMOKE_CAS_MODEL' \
  --questionnaire-code 'SMOKE_CAS_QUESTIONNAIRE' \
  --output /tmp/modelcatalog-revision-conflict-preflight.json \
  | tee /tmp/modelcatalog-revision-conflict-preflight.log

QS_PREFLIGHT_RC=$?
echo "preflight 退出码=$QS_PREFLIGHT_RC"
```

输出 `REVISION_CONFLICT_SMOKE_PREFLIGHT_OK` 后，再执行写 smoke：

```bash
set -o pipefail

/tmp/smoke-modelcatalog-revision-conflict \
  --api-base-url "$QS_APISERVER_URL" \
  --token-file "$QS_MODELCATALOG_ADMIN_TOKEN_FILE" \
  --model-code 'SMOKE_CAS_MODEL' \
  --questionnaire-code 'SMOKE_CAS_QUESTIONNAIRE' \
  --confirm-targets 'SMOKE_CAS_MODEL,SMOKE_CAS_QUESTIONNAIRE' \
  --concurrency 16 \
  --rounds 5 \
  --timeout 2m \
  --output /tmp/modelcatalog-revision-conflict.json \
  --apply \
  | tee /tmp/modelcatalog-revision-conflict.log

QS_CONFLICT_SMOKE_RC=$?
echo "revision-conflict smoke 退出码=$QS_CONFLICT_SMOKE_RC"
```

`--confirm-targets` 的顺序固定为 `<model-code>,<questionnaire-code>`，内容必须与前两个参数逐字一致。

## 判定

- `0`：preflight 通过；或 apply 模式下两个目标均观察到至少一次 200、至少一次带稳定 revision-conflict 消息的 409，且 basic-info 恢复校验通过。
- `1`：参数、token、readiness、目标读取、安全 guard 或证据文件不可用。
- `2`：未观察到契约 409、出现其他 HTTP 结果或恢复失败。

并发 REST smoke 具有概率性。如果只出现 200 且恢复成功，可以在确认服务实例和目标仍安全后提高 `--concurrency` 或 `--rounds` 重试；不能把“全是 200”记录为通过。

JSON 证据不会记录 token 或原始标题/描述，只记录目标 code、release state、各轮 HTTP 分类和恢复结果。
