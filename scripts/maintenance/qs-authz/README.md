# AuthZ 维护命令族

源码位于 `scripts/maintenance/qs-authz/`，保留 Go 实现。使用 `make build-maintenance` 构建；两个子命令继续使用 QS_APISERVER_ 配置前缀，保持现有 IAM 服务身份和证据 JSON。

| 命令 | 能力 |
| --- | --- |
| `qs-authz verify --config=<QS配置>` | 只读选择 QS 主体、读取 IAM 快照并执行四类主体的六项 action 检查；不创建用户或赋权 |
| `qs-authz provision --config=<QS配置>` | 显式创建或校准两个隔离测试主体及其 managed assignments；会写 IAM |

provision 要求 QS_AUTHZ_MATRIX_PROVISION_CONFIRM=provision-isolated-authz-matrix-subjects-v2，命令在读取配置前核对确认词，应用层仍独立校验。不隐式准备测试主体，也不在 verify 失败时调用 provision。

镜像仅携带 qs-authz，不携带旧独立二进制。现有巡检及准备脚本先检查运行容器内的新入口：存在时执行明确子命令；不存在时仅为仍受支持的旧镜像/回滚镜像调用旧入口。探测错误直接停止；执行失败不切换入口或重复写入。旧入口选择应在旧镜像支持责任结束后退出。

脚本保留 exact SHA、证据内容和固定确认校验，普通服务默认入口不变。只读巡检与显式准备的工作流触发关系保持。
