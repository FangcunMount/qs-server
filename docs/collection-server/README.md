# Collection Server 文档

## 概述

Collection Server 是问卷系统中的轻量级数据收集服务，负责接收、验证和转发问卷答卷数据。

## 文档目录

### [📖 01-整体设计与实现](./01-整体设计与实现.md)

- 服务概述和核心职责
- 整体架构设计
- 核心模块介绍
- 数据流向和API设计
- 技术栈和部署配置
- 性能优化和最佳实践

### [🔍 02-校验模块设计与实现](./02-校验模块设计与实现.md)  

- 验证规则系统架构
- 串行验证实现机制
- 并发验证实现机制
- 验证策略选择和配置
- 性能优化和扩展机制
- 监控和质量保证

## 快速开始

### 启动服务

```bash
# 启动 collection-server
make run-collection-server
```

### API 示例

```bash
# 提交答卷
curl -X POST http://localhost:8080/api/v1/answersheets \
  -H "Content-Type: application/json" \
  -d '{
    "questionnaire_code": "sample-quiz",
    "testee_info": {
      "name": "张三",
      "age": 25
    },
    "answers": [...]
  }'
```

## 架构特点

- 🏗️ **六边形架构** - 清晰的分层设计
- ⚡ **高性能** - 支持并发验证，高吞吐量
- 🔍 **动态验证** - 基于问题验证规则的智能校验
- 📊 **可观测性** - 详细的日志记录和监控
- 🔧 **易扩展** - 灵活的验证策略和规则扩展

## 相关链接

- [项目总体文档](../README.md)
- [API 文档](../api/)
- [部署指南](../deployment/)
