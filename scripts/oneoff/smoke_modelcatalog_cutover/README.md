# ModelCatalog cutover smoke

这个工具通过 collection-server 的真实公开入口串行验证：

```text
Published Model
→ frozen Questionnaire / Typology Session
→ POST AnswerSheet (202 accepted)
→ Assessment readiness
→ Evaluation / Outcome
→ Report status interpreted
→ Report fetch and contract checks
```

工具会根据模型 `kind` 自动选择报告入口：

- `scale`：`/api/v1/assessments/*`
- `typology`：`/api/v1/typology-assessments/*`
- `behavioral_rating`、`cognitive`：`/api/v1/behavior-assessments/*`

它不会直接连接或修改数据库，但会为每个 model code 创建一份真实答卷、测评、Outcome 和报告。只能使用专用 smoke 受试者执行。

## 构建

```bash
go build -o /tmp/smoke-modelcatalog-cutover \
  ./scripts/oneoff/smoke_modelcatalog_cutover/
```

## 执行

可以直接复用 perf token 文件；支持纯文本 token、JSON token 数组以及 `{"tokens":[...]}`：

```bash
set -o pipefail

/tmp/smoke-modelcatalog-cutover \
  --collection-base-url 'https://collect.fangcunmount.cn' \
  --token-file ./tmp/perf/tokens.json \
  --testee-id '替换为专用测试受试者ID' \
  --model-codes 'ISI7,MBTI_OEJTS,gXkk9W,bJFKi3,替换为CognitiveSPM编码' \
  --timeout 10m \
  --output /tmp/modelcatalog-smoke-result.json \
  | tee /tmp/modelcatalog-smoke.log

QS_SMOKE_RC=$?
echo "ModelCatalog smoke 退出码=$QS_SMOKE_RC"
```

也可以全部通过环境变量配置：

```bash
export QS_MODELCATALOG_SMOKE_COLLECTION_URL='https://collect.fangcunmount.cn'
export QS_MODELCATALOG_SMOKE_TOKEN_FILE='./tmp/perf/tokens.json'
export QS_MODELCATALOG_SMOKE_TESTEE_ID='替换为专用测试受试者ID'
export QS_MODELCATALOG_SMOKE_MODEL_CODES='ISI7,MBTI_OEJTS,gXkk9W,bJFKi3,替换为CognitiveSPM编码'
export QS_MODELCATALOG_SMOKE_TIMEOUT='10m'
export QS_MODELCATALOG_SMOKE_OUTPUT='/tmp/modelcatalog-smoke-result.json'

/tmp/smoke-modelcatalog-cutover
```

不建议把 token 放在命令行参数中，因为它可能出现在进程列表和 shell history；优先使用 `--token-file` 或 `QS_MODELCATALOG_SMOKE_TOKEN`。

## 判定

- `0`：所有配置模型均完成报告并通过身份、OutcomeCode/Label、常模引用检查。
- `1`：参数、token、collection readiness 或证据文件不可用。
- `2`：至少一个模型的业务链路失败；其他模型仍会继续执行并写入汇总结果。

行为评定和 Cognitive SPM 报告必须至少包含一个带具体 `table_version` 的 `norm_reference`。所有报告必须返回与发布目录精确一致的 `kind/sub_kind/algorithm/code/version`，并包含独立的短 `level.code` 和非空 `level.label`。

smoke 通过后仍需重新执行 `verify_definition_v2_cutover`；HTTP smoke 不替代 Mongo/MySQL schema 与存量数据审计。
