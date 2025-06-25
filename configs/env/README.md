# 环境配置文件

## 概述

此目录包含问卷收集&量表测评系统的数据库环境变量配置文件。

## 文件说明

| 文件 | 说明 | 用途 |
|------|------|------|
| `config.env` | 开发环境配置 | 本地开发和测试 |
| `config.prod.env` | 生产环境配置模板 | 生产环境部署模板 |

## 使用方法

### 开发环境

开发环境直接使用 `config.env` 文件：

```bash
cd build/docker/infra
./deploy.sh deploy
```

### 生产环境

1. 复制生产环境模板：
```bash
cp configs/env/config.prod.env configs/env/config.env
```

2. 修改配置文件中的敏感信息：
```bash
nano configs/env/config.env
```

3. 部署服务：
```bash
cd build/docker/infra
./deploy.sh deploy
```

## 配置类别

### 数据库连接配置

- **MySQL**: 主机、端口、数据库名、用户名、密码
- **Redis**: 主机、端口、密码、数据库编号
- **MongoDB**: 主机、端口、数据库名、用户名、密码

### Docker 配置

- 容器名称
- 镜像名称
- 网络配置

### 数据持久化配置

- 数据目录路径
- 日志目录路径
- 备份目录路径

### 应用配置

- 环境类型（development/production）
- 调试模式
- 时区设置

## 安全建议

### 开发环境

- 使用简单密码便于开发调试
- 可以提交到版本控制系统

### 生产环境

1. **强密码**: 使用复杂密码
   ```bash
   # 生成32位随机密码
   openssl rand -base64 32
   ```

2. **文件权限**: 限制配置文件访问权限
   ```bash
   chmod 600 configs/env/config.env
   ```

3. **版本控制**: 不要提交生产配置到代码仓库
   ```bash
   # 添加到 .gitignore
   echo "configs/env/config.env" >> .gitignore
   ```

4. **环境分离**: 为不同环境使用不同的配置文件
   ```bash
   # 开发环境
   configs/env/config.dev.env
   
   # 测试环境
   configs/env/config.test.env
   
   # 生产环境
   configs/env/config.prod.env
   ```

## 配置示例

### MySQL 配置示例

```bash
# 开发环境
MYSQL_ROOT_PASSWORD=simple_dev_password
MYSQL_PASSWORD=dev_password

# 生产环境
MYSQL_ROOT_PASSWORD=Xy9@kN2$mP8#vR5!
MYSQL_PASSWORD=qW3$eR7&tY9*uI1@
```

### 路径配置示例

```bash
# 开发环境（相对路径）
MYSQL_DATA_PATH=./data/mysql
MYSQL_LOGS_PATH=./logs/mysql

# 生产环境（绝对路径）
MYSQL_DATA_PATH=/data/mysql/qs-prod/data
MYSQL_LOGS_PATH=/data/logs/qs-prod/mysql
```

## 故障排除

### 配置文件不生效

1. 检查文件路径是否正确
2. 确认文件权限可读
3. 验证语法格式正确

### 权限错误

```bash
# 检查文件权限
ls -la configs/env/

# 修复权限
chmod 644 configs/env/config.env
```

### 路径错误

```bash
# 检查数据目录是否存在
ls -la /data/mysql/qs/

# 创建缺失目录
mkdir -p /data/mysql/qs/data
mkdir -p /data/logs/qs/mysql
```

## 最佳实践

1. **环境分离**: 为每个环境维护独立的配置文件
2. **密码管理**: 使用密码管理工具生成和存储密码
3. **权限控制**: 严格控制配置文件的访问权限
4. **版本控制**: 只提交模板文件，不提交实际使用的配置
5. **定期更新**: 定期更换生产环境密码
6. **备份配置**: 安全备份生产环境配置文件 