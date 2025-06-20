# 开发环境指南

## 热更新开发

本项目使用 [Air](https://github.com/air-verse/air) 实现开发环境的热更新功能。

### 快速开始

1. **安装 Air**（如果还没有安装）：
   ```bash
   make install-air
   # 或者
   go install github.com/air-verse/air@latest
   ```

2. **启动开发环境**：
   ```bash
   make dev
   # 或者
   ./script/dev.sh
   # 或者直接使用 air
   air
   ```

### 配置说明

#### Air 配置文件 (.air.toml)

- **监听文件类型**：`.go`, `.yaml`, `.yml`, `.json`, `.html`, `.tpl`, `.tmpl`
- **排除目录**：`assets`, `tmp`, `vendor`, `testdata`, `docs`, `script`
- **排除文件**：`*_test.go`
- **构建命令**：`go build -o ./tmp/main ./cmd/qs-apiserver`
- **启动参数**：`--config=configs/qs-apiserver.yaml`

#### 工作流程

1. Air 监听项目文件变化
2. 当检测到变化时，自动重新构建应用
3. 停止旧进程，启动新进程
4. 显示构建和运行日志

### 常用命令

```bash
# 查看所有可用命令
make help

# 启动开发环境（热更新）
make dev

# 构建应用
make build

# 运行应用（无热更新）
make run

# 运行测试
make test

# 清理构建文件
make clean

# 安装依赖
make deps
```

### 文件结构

```
.
├── .air.toml              # Air 配置文件
├── Makefile               # 构建脚本
├── script/
│   └── dev.sh            # 开发环境启动脚本
├── tmp/                  # Air 临时文件目录
├── bin/                  # 构建输出目录
└── configs/
    └── qs-apiserver.yaml # 应用配置文件
```

### 注意事项

1. **配置文件**：确保 `configs/qs-apiserver.yaml` 文件存在
2. **端口冲突**：如果应用使用固定端口，确保端口未被占用
3. **权限问题**：确保脚本有执行权限 `chmod +x script/dev.sh`
4. **依赖管理**：首次运行前执行 `make deps` 安装依赖

### 故障排除

#### 问题：Air 未找到
```bash
# 解决方案：重新安装 Air
go install github.com/air-verse/air@latest
```

#### 问题：配置文件未找到
```bash
# 解决方案：检查配置文件路径
ls -la configs/qs-apiserver.yaml
```

#### 问题：构建失败
```bash
# 解决方案：检查 Go 模块
go mod tidy
go mod download
```

#### 问题：权限被拒绝
```bash
# 解决方案：添加执行权限
chmod +x script/dev.sh
```

### 自定义配置

如果需要修改 Air 配置，可以编辑 `.air.toml` 文件：

- **修改监听文件类型**：调整 `include_ext` 数组
- **修改排除目录**：调整 `exclude_dir` 数组
- **修改构建命令**：调整 `cmd` 字段
- **修改启动参数**：调整 `args_bin` 数组 