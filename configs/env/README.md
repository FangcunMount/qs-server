# 环境配置目录

> 状态：当前有效
>
> 适用范围：本地开发、基础设施连通性检查、部署参数整理
>
> 事实来源：应用 YAML、`pkg/app/config.go`、`Makefile`、`scripts/check-infra.sh`

## 结论

`configs/env/` 中的 `.env.dev` 和 `.env.prod` 是基础设施参数目录与部署模板，不是三个应用的自动加载配置文件。

qs-server 的应用配置由以下两层组成：

1. `configs/*.dev.yaml` 或 `configs/*.prod.yaml` 提供基线；
2. 带应用前缀的环境变量覆盖 YAML 字段。

`Makefile` 不会自动读取本目录下的 `.env` 文件；仅执行 `source configs/env/.env.dev` 也不会自动覆盖应用 YAML 中的同名字段。

## 文件职责

| 文件 | 当前职责 | 是否由应用自动加载 |
|---|---|---|
| `.env.dev` | 本地基础设施检查参数的开发目录；只允许保存可公开的本地开发值 | 否 |
| `.env.prod` | 生产部署参数模板；占位符不代表线上运行值 | 否 |
| `README.md` | 配置边界与变量映射说明 | 不适用 |
| `QUICK_START.md` | 最短本地启动路径 | 不适用 |

## 应用配置加载规则

`ENV` 只影响 `Makefile` 选择哪组 YAML：

| 进程 | `ENV=dev` | `ENV=prod` | 环境变量前缀 |
|---|---|---|---|
| qs-apiserver | `configs/apiserver.dev.yaml` | `configs/apiserver.prod.yaml` | `QS_APISERVER_` |
| collection-server | `configs/collection-server.dev.yaml` | `configs/collection-server.prod.yaml` | `COLLECTION_SERVER_` |
| qs-worker | `configs/worker.dev.yaml` | `configs/worker.prod.yaml` | `QS_WORKER_` |

环境变量名由“前缀 + YAML 路径”组成，点号和连字符统一转换为下划线。例如：

```bash
QS_APISERVER_MYSQL_HOST=127.0.0.1 make run-apiserver
COLLECTION_SERVER_GRPC_CLIENT_ENDPOINT=127.0.0.1:9090 make run-collection
QS_WORKER_MESSAGING_NSQ_ADDR=127.0.0.1:4150 make run-worker
```

其中：

- `MYSQL_HOST` 供 `make check-mysql` 等基础设施检查读取；
- `QS_APISERVER_MYSQL_HOST` 才会覆盖 apiserver 的 `mysql.host`；
- 两者职责不同，不应依赖隐式传递。

## 本地开发

完整启动：

```bash
make quick-start
make health
```

只做基础设施检查：

```bash
make check-infra
make check-mysql
make check-redis
make check-mongodb
make check-nsq
```

为检查命令临时覆盖基础设施连接参数：

```bash
MYSQL_HOST=127.0.0.1 MYSQL_PORT=3306 make check-mysql
```

直接运行二进制时，应显式传入配置文件：

```bash
./bin/qs-apiserver --config=configs/apiserver.dev.yaml
./bin/collection-server --config=configs/collection-server.dev.yaml
./bin/qs-worker --config=configs/worker.dev.yaml
```

`ENV=prod` 只表示选择生产 YAML，不等于已经具备生产部署条件，也不会注入密钥、证书或 systemd 环境。

## 安全边界

- 不要在仓库文档、脚本输出、提交记录或 CI 日志中写入生产密钥。
- `.env.prod` 只保留变量名或安全占位符，真实值由部署环境管理。
- `.env.dev` 只能保存本地开发值；任何疑似真实凭据都应立即轮换并从版本历史治理。
- 应用启动失败时，先确认实际选中的 YAML，再检查对应的前缀环境变量；不要把基础设施检查成功当成应用配置生效的证据。
- 修改配置字段时，应同步三个环境 YAML、配置结构和相关文档，并运行对应测试。

## 排查顺序

1. 确认命令使用的 `ENV` 与 YAML 文件。
2. 检查是否存在对应应用前缀的环境变量覆盖。
3. 使用 `make check-infra` 验证依赖连通性。
4. 查看进程启动日志和健康检查结果。
5. 对生产问题，以当前部署单元和安全脱敏后的运行证据为准，不以本目录模板推断线上值。

## 相关文档

- [本地开发与配置约定](../../docs/00-总览/06-本地开发与配置约定.md)
- [配置与环境变量](../../docs/04-接口与运维/05-配置与环境变量.md)
- [部署与端口](../../docs/04-接口与运维/06-部署与端口.md)
- [快速开始](QUICK_START.md)
