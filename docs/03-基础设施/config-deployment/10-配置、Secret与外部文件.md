# 配置、Secret 与外部文件

## 1. 当前结论

三个进程均以主 YAML 为结构基线，并由各自前缀的环境变量注入环境相关值。生产包还依赖缓存策略、Event Catalog、gRPC ACL 和证书等外部文件。缺少被启用能力所需的文件时，应在启动或部署预检阶段失败，不能静默回退成宽松策略。

本页是 `config/secrets` 主题的 canonical 文档。生产中某次注入了什么值，必须以脱敏的 effective-config 证据记录，不能从仓库默认值反推。

## 2. 三个主配置与环境前缀

| 进程 | 主配置 | 容器启动参数 | 环境变量前缀 |
| --- | --- | --- | --- |
| qs-apiserver | `configs/apiserver.prod.yaml` | `--config=/app/configs/apiserver.prod.yaml` | `QS_APISERVER_` |
| collection-server | `configs/collection-server.prod.yaml` | `--config=/app/configs/collection-server.prod.yaml` | `COLLECTION_SERVER_` |
| qs-worker | `configs/worker.prod.yaml` | `--config=/app/configs/worker.prod.yaml` | `QS_WORKER_` |

`scripts/cd/prepare-package.sh` 在每个部署包内生成独立的 `config.prod.env`。MySQL、MongoDB、Redis、NSQ、JWT、委托签名等环境相关值从 CI Secret/Variable 进入该文件；脚本的契约测试必须同时验证所需键存在、废弃键缺失，并禁止把密码打印到日志。

## 3. 外部文件清单

| 文件或目录 | 消费方 | 失败语义 |
| --- | --- | --- |
| `configs/events.yaml` | apiserver、worker | 事件注册与 handler 不一致时启动/测试失败；它是 durable/best-effort 事件目录，不是 Secret |
| `configs/signals.yaml` | 文档门禁与 Signal 契约 | 只描述 Redis ephemeral Signal 拓扑；当前进程不会在启动时动态加载它 |
| `configs/cache/apiserver.prod.yaml` | apiserver | 相对路径按主配置所在目录解析；策略加载或校验失败应阻断对应配置装配 |
| `configs/cache/collection-server.prod.yaml` | collection-server | 同上 |
| `configs/grpc-acl.prod.yaml` | apiserver gRPC | `grpc.acl.enabled=true` 时文件缺失或无效会使 gRPC server 构造失败；不能回退到 default allow |
| `/etc/qs-server/ssl/certs`、`/etc/qs-server/ssl/private` | apiserver HTTPS | 由宿主机只读挂载；私钥权限需允许容器运行用户读取 |
| `/etc/qs-server/ssl/grpc` | 三个进程 | apiserver 使用服务端证书与 CA，collection/worker 使用客户端证书与 CA；身份必须通过部署预检 |

Event、Signal 与缓存策略的业务语义分别由 Event、Cache 文档维护；本页只维护交付和加载边界。

## 4. Secret 边界

以下内容不得提交到 YAML、Markdown、测试快照或命令输出：数据库密码、Redis 密码、JWT Secret、委托签名 key、OSS Secret、TLS 私钥、registry token、SSH key。

仓库允许保留空值、示例名和变量名。有效配置证据只能记录：

- 部署 SHA 与镜像 digest；
- 主配置和非敏感外部文件的内容哈希；
- Secret 是否存在、来源类别和轮换版本，不记录值；
- 脱敏后的 endpoint、端口、开关和资源预算；
- 验证时间、环境、执行命令、结果、失效期。

`scripts/cd/remote-deploy.sh` 会设置 env 文件、registry token 和证书权限；任何新 Secret 都必须同步加入打包、权限、脱敏和契约测试，不能只在 workflow 中临时注入。

## 5. Fail-closed 条件

- 生产 gRPC ACL 已启用、默认策略为 deny；ACL 文件无法读取或规则无效时不得启动 gRPC server。
- 生产 mTLS 所需 CA、证书、私钥或预期证书身份不满足时，部署预检应失败。
- 主配置引用的 cache policy 不可解析时，不得悄悄使用另一环境的策略。
- 必填环境变量缺失时，`prepare-package.sh` 必须在生成部署包前失败。
- Event Catalog 与代码注册表不一致时，catalog/registry 契约测试必须失败。

## 6. 验证层级

| 层级 | 入口 | 能证明什么 | 不能证明什么 |
| --- | --- | --- | --- |
| 源码/配置 | `configs/`、三个 `options` 包、`prepare-package.sh` | 配置字段、文件路径和打包规则 | 目标环境实际值 |
| 非缓存测试 | `go test -count=1 ./internal/pkg/configcontract ./internal/pkg/grpc ./internal/apiserver/options ./internal/collection-server/options ./internal/worker/options` | loader、ACL、配置契约在当前 checkout 成立 | Secret 可读、远端依赖可连 |
| 脚本测试 | `bash scripts/cd/test-prepare-package.sh` | 生成包包含期望文件且不输出测试 Secret | 真实 CI Secret 完整 |
| 环境预检 | 对部署包做文件、权限、证书身份、endpoint 连通检查 | 本次包在目标机具备启动前提 | 服务已正确处理业务流量 |
| 生产证据 | exact SHA + effective-config hash + 探针/查询 | 指定时点的有效配置与运行结果 | 之后持续不变 |

## 7. 未闭合 gap

1. 当前 checkout 没有一份绑定 exact SHA 的脱敏 effective-config 快照。
2. 静态测试不能证明生产 Secret 已轮换、权限正确或证书未过期。
3. `signals.yaml` 是机器契约/文档事实源，但不是运行时加载文件；不得把它列为容器启动依赖。
4. 主配置仍包含历史更新时间、容量注释等易漂移文本；这些注释不具备生产证据等级。
