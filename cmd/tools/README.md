# QS Tooling

`seeddata` 已从 `qs-server` 主模块迁移到仓库内独立子模块：

- [tools/seeddata-runner](../../tools/seeddata-runner)

推荐入口：

```bash
./scripts/run_seeddata_daemon.sh
```

上面的根脚本只是兼容 wrapper。权威运行方式是：

```bash
cd tools/seeddata-runner
./scripts/run_seeddata_daemon.sh
```

或：

```bash
cd tools/seeddata-runner
go run ./cmd/seeddata --config ./configs/seeddata.yaml
```

详细说明见：

- [tools/seeddata-runner/README.md](../../tools/seeddata-runner/README.md)
- [tools/seeddata-runner/GUIDE.md](../../tools/seeddata-runner/GUIDE.md)
