# GitHub Actions 与交付契约

本页只维护 workflow 的稳定职责、输入、安全边界和本地验证入口。目标主机、实际副本数、runner 版本、某次部署 SHA、探针结果和数据扫描数量属于时点证据，统一记录在[基础设施生产证据台账](../../docs/00-总览/10-基础设施生产证据台账.md)。

## 1. Workflow 责任

| Workflow | 稳定职责 | 状态写入边界 |
| --- | --- | --- |
| `ci.yml` | 单测、静态/架构/配置契约、文档与 API 同步、构建 | 只证明该 run 的 checkout；不证明部署 |
| `sonar.yml` | SonarQube 分析 | 不替代 CI、部署或运行验收 |
| `cd.yml` | 计划服务、构建/交付镜像、生成部署包、按依赖顺序部署、逐实例验证 | 成功结果必须记录 exact SHA、image digest、effective config 与环境 |
| `ping-runner.yml` | 周期/手动执行目标环境连通与服务探针 | 只证明该时点、该检查项；不证明业务全链和数据一致性 |
| `db-ops.yml` | 受保护的备份、恢复与只读数据盘点入口 | 写操作/恢复需要独立授权、备份和复验 |
| `attention-reconcile-audit.yml` | 受控 Attention reconciliation 审计 | 不得把 dry-run 自动升级为 apply |
| `compatibility-observation.yml` | 只读兼容流量/指标观察 | 无指标、零值、无命中和证据缺失必须区分 |

删除、重命名或改变触发关系时，必须同步 `scripts/cd` 契约测试和文档门禁。历史 run 只进入证据台账，不回写本页。

## 2. 交付链

```text
CI for exact SHA
  -> plan-services
  -> build image + metadata/digest
  -> prepare-package (configs + env + scripts)
  -> target dependency/certificate preflight
  -> remote-deploy
  -> per-instance health/readiness/governance
  -> dated evidence ledger entry
```

配置、Secret、镜像、Compose 和网络 canonical 边界见[配置与部署](../../docs/03-基础设施/config-deployment/README.md)，部署执行步骤见[部署与端口 runbook](../../docs/04-接口与运维/06-部署与端口.md)。

## 3. 输入与 Secret

- workflow input/Variable 只表达服务计划、环境和非敏感参数；数据库、Redis、JWT、委托 key、registry token、SSH key 与 TLS private key 必须来自受保护 Secret/目标主机。
- `scripts/cd/prepare-package.sh` 在每个部署包内生成独立的 `config.prod.env`；日志只允许显示变量名和脱敏 endpoint，不得输出值。
- 自动部署必须校验目标 SHA 没有被更新提交取代；手动部署仍须记录调用者、输入、environment 和审批。
- self-hosted runner 的网络、SSH、Docker 权限和工具版本属于环境前置条件，不能写死为当前事实。

## 4. 本地非缓存验证

```bash
go test -count=1 ./internal/pkg/configcontract
bash scripts/cd/test-github-action-runtimes.sh
bash scripts/cd/test-plan-services.sh
bash scripts/cd/test-prepare-package.sh
bash scripts/cd/test-production-compose-network-exposure.sh
bash scripts/cd/test-wait-worker-readiness.sh
bash scripts/cd/test-verify-worker-dependencies.sh
bash scripts/cd/test-verify-worker-governance.sh
```

这些命令不读取生产 Secret，也不证明目标主机可达。真实部署必须另存 workflow URL/run ID、exact source/deployed SHA、image digest、effective-config hash、逐实例检查、limitations 和失效期。

## 5. 安全与恢复边界

- 不在 workflow 中无边界执行 `docker compose down`、全局 orphan 清理、volume prune 或自动重启所有容器。
- 只删除本 workflow/project 精确拥有且已被新实例替代的资源；先验证新实例，再切流，再清理旧资源。
- migration dirty、数据不兼容、证书身份错误、依赖不可达或 probe 语义不符时应停止部署，不用重试掩盖。
- 回滚旧镜像前先判断已落地 schema/event/config 是否向后兼容；one-off 写入不能靠镜像回滚恢复。
- `/healthz`、`/readyz`、`/serve-readyz` 和 governance 语义不同，验收按[健康探针 canonical 文档](../../docs/03-基础设施/observability/20-健康探针、治理与持久证据.md)解释。

## 6. 生产证据要求

workflow 成功后若未写入证据台账，只能称为“run 成功”，不能称为当前生产已签署。每条记录至少绑定：

- `observed_at`、environment、owner；
- source/deployed SHA 和 image digest；
- effective config hash 或具名 unknown limitation；
- workflow/命令、结果和原始 evidence reference；
- 探针/查询覆盖、limitations、`expires_on`、`supersedes`。
