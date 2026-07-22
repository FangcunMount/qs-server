# repair_enneagram_report_template

生产 smoke 后的外科式 ModelCatalog 修复工具。它只处理已确认的
`ENNEAGRAM_45@v16` 活动发布快照，把错误的 `trait_profile + mbti`
路由改为 `trait_profile + enneagram`，随后重新物化 DefinitionV2 派生层并更新
canonical definition hash。

该工具不连接 MySQL，不读取或修改 Assessment、AnswerSheet、Outcome、Report、
Checkpoint、Norm、Questionnaire 或其他 Model。apply 使用 Mongo transaction 和
`_id + updated_at + old template_id` CAS；完整 runtime identity、唯一 type
conclusion、outcome-free trait profile、唯一 report section 或已知前值任一不匹配时
均退出 2，且不写数据。

## 执行前提

- repair binary 与待部署服务来自同一提交；
- 暂停 ModelCatalog 发布写流量，并在 apply 到新版本服务全部就绪期间暂停九型人格新测评；
- `MONGO_URI` 指向授权生产 Replica Set，当前节点可访问 writable primary；
- 已保留本次 ModelCatalog 资产备份和恢复校验结果。

## Dry-run

```bash
go build -o /tmp/repair-enneagram-report-template \
  ./scripts/oneoff/repair_enneagram_report_template/

set -o pipefail

/tmp/repair-enneagram-report-template \
  --mongo-db qs \
  --timeout 2m \
  | tee /tmp/enneagram-template-repair-dry-run.txt

QS_ENNEAGRAM_DRY_RUN_RC=$?
echo "dry-run 退出码=$QS_ENNEAGRAM_DRY_RUN_RC"
```

只有退出码为 0，且输出 `action=update` 和
`PASS: exact ENNEAGRAM_45@v16 repair is apply-safe` 时才允许 apply。

## Apply

```bash
set -o pipefail

/tmp/repair-enneagram-report-template \
  --mongo-db qs \
  --timeout 2m \
  --apply \
  | tee /tmp/enneagram-template-repair-apply.txt

QS_ENNEAGRAM_REPAIR_RC=$?
echo "apply 退出码=$QS_ENNEAGRAM_REPAIR_RC"
```

成功必须同时包含退出码 0 和 `ENNEAGRAM_REPORT_TEMPLATE_REPAIR_OK`。随后重新运行
`verify_definition_v2_cutover`，并完成 qs-apiserver、Worker、Collection 的新版本滚动/重启，
避免继续使用旧 catalog cache；audit 退出 0 后，用新答卷执行九型人格 smoke。
