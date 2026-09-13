# 未归属受试者随机初配门店

仅通过 QS 总部首次配置接口，将预演时“未归属清单”中的受试者随机打散、尽量均分至**武汉店、西安店、南京店**，三店新增人数最多相差 1。目标店按登录公司中的精确名称唯一匹配，必须启用。不调整其他门店已有人员。

这属于总部人工分配，不是从历史医生关系推导的真实服务事实。分配会使目标门店获得这些受试者及历史测评的业务读取范围；不会回填历史测评开展门店、修改医生关系或历史统计。

## 准备

使用同一位具有公司全部门店范围的总部管理员执行预演、应用及重跑。令牌放入权限为 `600` 的本地文本文件，不放入命令行、仓库或清单。脚本仅使用 Python 3 标准库，支持 Linux/macOS。

```sh
export QS_TOKEN_FILE=/absolute/private/headquarters-access-token.txt
```

以下示例的 API 地址、公司 ID、清单路径需按实际环境填写。`--base-url` 必须是 QS 的 `/api/v1` 根路径。

## 1. 预演（只读）

```sh
python3 scripts/oneoff/assign_unassigned_testees.py preflight \
  --base-url https://YOUR-QS-HOST/api/v1 \
  --org-id 1 \
  --manifest /absolute/private/random-store-assignment.json
```

完整翻页两次复核 ID 与归属版本，保存不可覆盖的原始随机结果、种子、目标店、预期版本、逐人请求标识和校验和。清单不保存姓名、答卷或令牌；文件默认 `600`。日志只展示数量摘要。可用 `--seed 20260913` 指定随机种子。

分页不提供数据库快照隔离。应在受试者新增、删除和归属变更暂停时生成清单；两次扫描检测漂移，服务端版本检查仍是最终写入保护。预演后新增的未归属人员不包含在该清单中，不会自动扩大执行范围。

## 2. 应用（写入）

核对清单中的目标门店及人数后执行：

```sh
python3 scripts/oneoff/assign_unassigned_testees.py apply \
  --base-url https://YOUR-QS-HOST/api/v1 \
  --org-id 1 \
  --manifest /absolute/private/random-store-assignment.json
```

只调用 `PUT /testees/:id/store`，不调用转店接口。每位受试者独立事务；记录原始 `initial` 归属审计。串行执行，默认写入间隔 0.2 秒，避免集中请求。约三万人可能执行数小时；请求超时、权限失效、版本冲突或配置变化均立即停止，已有成功项保留。

每笔回执追加到 `*.receipts.jsonl` 并同步落盘。遇到超时或进程中断，不另建清单、不重新随机：排查后以**同一管理员、同一清单**重跑 `apply`。所有请求重复使用原请求 ID，服务端返回原审计，不重复写归属。令牌过期可更新令牌文件后重跑，不必长期令牌。重放从清单开头开始，可能耗时；不将本地“成功”标记当作当前数据库事实。

## 3. 核验（只读）

```sh
python3 scripts/oneoff/assign_unassigned_testees.py verify \
  --base-url https://YOUR-QS-HOST/api/v1 \
  --org-id 1 \
  --manifest /absolute/private/random-store-assignment.json
```

核对清单每人的实时公司、当前门店和归属版本。任何后续转店或未完成项导致非零退出，不自动覆盖。成功只表示本清单人员一致，不表示执行后新增的未归属人员为零。

脚本不提供自动清空归属或回滚；错误配置应通过总部已审计的转店入口纠正，保留原始证据。不要并发执行不同清单处理同批人员。

## 验证脚本

```sh
python3 -m unittest discover -s scripts/oneoff -p 'test_assign_unassigned_testees.py' -v
```

自动化覆盖均分、随机可复现、长 ID、漂移、名单重复、公司和门店验证、请求重放与冲突停止。以上验证不代表已执行生产分配。
