# 受试者详情页API接口实现总结

## 概述

本次更新根据《受试者详情页API接口文档.md》的要求，完成了以下接口的实现和更新。

## 完成的工作

### 1. ✅ 补充 GET /testees/{id} 的 guardians 字段

**文件修改：**
- `internal/apiserver/interface/restful/handler/actor.go`
  - 添加 `guardianshipService` 依赖注入
  - 在 `GetTestee` 方法中集成 IAM 的监护人查询功能
  - 从 IAM 的 `ListGuardians` 接口获取监护人信息并转换为响应格式

- `internal/apiserver/container/assembler/actor.go`
  - 更新 `NewActorHandler` 构造函数，传入 `guardianshipService` 参数

**实现细节：**
- 通过 IAM 的 GuardianshipService 查询监护人信息
- 从 GuardianshipEdge 中提取监护人姓名、关系和联系电话
- 监护人信息获取失败不影响主流程，仅记录日志
- 响应格式完全符合文档定义的 GuardianResponse 结构

**返回字段示例：**
```json
{
  "guardians": [
    {
      "name": "张大明",
      "relation": "父亲",
      "phone": "13800138000"
    }
  ]
}
```

### 2. ✅ 新增 GET /testees/{id}/scale-analysis 接口

**文件修改：**
- `internal/apiserver/interface/restful/handler/actor.go`
  - 实现 `GetScaleAnalysis` 方法
  - 验证受试者存在性
  - 返回符合文档定义的响应结构

- `internal/apiserver/interface/restful/response/scale_analysis.go` (新建)
  - 定义 `ScaleAnalysisResponse` 响应结构
  - 定义 `ScaleTrendResponse` 量表趋势结构
  - 定义 `ScaleTestResponse` 测评记录结构
  - 定义 `ScaleFactorResponse` 因子得分结构

**当前状态：**
- ✅ 接口路由已注册
- ✅ Handler 方法已实现
- ✅ 响应结构已定义
- ⚠️ 当前返回空数据结构（待 Evaluation 模块提供查询服务）

**返回格式：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "scales": []
  }
}
```

**TODO（后续扩展）：**
1. 在 Evaluation 模块添加按受试者查询测评记录的服务
2. 按量表分组聚合测评历史数据
3. 实现因子得分的查询和转换

### 3. ✅ 新增 GET /testees/{id}/periodic-stats 接口

**文件修改：**
- `internal/apiserver/interface/restful/handler/actor.go`
  - 实现 `GetPeriodicStats` 方法
  - 验证受试者存在性
  - 返回符合文档定义的响应结构

- `internal/apiserver/interface/restful/response/periodic_stats.go` (新建)
  - 定义 `PeriodicStatsResponse` 响应结构
  - 定义 `PeriodicProjectResponse` 项目响应结构
  - 定义 `PeriodicTaskResponse` 任务响应结构

**当前状态：**
- ✅ 接口路由已注册
- ✅ Handler 方法已实现
- ✅ 响应结构已定义
- ⚠️ 当前返回空数据结构（待 Plan 模块提供周期性测评项目查询服务）

**返回格式：**
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "projects": [],
    "total_projects": 0,
    "active_projects": 0
  }
}
```

**TODO（后续扩展）：**
1. 实现 Plan 模块的周期性测评项目管理
2. 查询受试者参与的周期性项目
3. 计算项目完成进度和任务状态

## 架构设计

### 依赖注入流程

```
Container.initActorModule()
  └─> ActorModule.Initialize(mysqlDB, guardianshipSvc, identitySvc)
      └─> NewActorHandler(..., guardianshipSvc)
          └─> ActorHandler.guardianshipService
```

### 接口调用流程

```
GET /api/v1/testees/{id}
  └─> ActorHandler.GetTestee()
      ├─> TesteeQueryService.GetByID()  // 查询受试者基本信息
      ├─> GuardianshipService.ListGuardians()  // 查询监护人（可选）
      └─> Response with guardians field
```

## 测试建议

### 1. 测试 Guardians 字段

```bash
# 有 profile_id 的受试者
curl -X GET "http://localhost:8080/api/v1/testees/{id}" \
  -H "Authorization: Bearer {token}"

# 预期：如果 profile_id 存在且 IAM 启用，返回 guardians 数组
```

### 2. 测试 Scale Analysis 接口

```bash
curl -X GET "http://localhost:8080/api/v1/testees/{id}/scale-analysis" \
  -H "Authorization: Bearer {token}"

# 预期：返回 scales 空数组（当前版本）
```

### 3. 测试 Periodic Stats 接口

```bash
curl -X GET "http://localhost:8080/api/v1/testees/{id}/periodic-stats" \
  -H "Authorization: Bearer {token}"

# 预期：返回 projects 空数组（当前版本）
```

## 兼容性说明

1. **向后兼容**：所有更新都是增量式的，不影响现有接口
2. **IAM 可选**：如果 IAM 服务未启用，guardians 字段为空，不影响正常使用
3. **渐进式实现**：新接口返回空数据结构，待相关模块完善后逐步填充数据

## 后续工作

### 高优先级（P1）
- [ ] 实现 Evaluation 模块的量表趋势分析查询服务
- [ ] 完善 GET /testees/{id}/scale-analysis 接口的数据查询逻辑

### 中优先级（P2）
- [ ] 设计并实现 Plan 模块的周期性测评项目管理
- [ ] 完善 GET /testees/{id}/periodic-stats 接口的数据查询逻辑

### 优化建议
- [ ] 添加监护人信息缓存（减少 IAM 调用）
- [ ] 为 scale-analysis 和 periodic-stats 添加分页支持
- [ ] 添加数据统计的增量更新机制

## 文件清单

### 新建文件
- `internal/apiserver/interface/restful/response/scale_analysis.go`
- `internal/apiserver/interface/restful/response/periodic_stats.go`

### 修改文件
- `internal/apiserver/interface/restful/handler/actor.go`
- `internal/apiserver/container/assembler/actor.go`

### 已有文件（确认兼容）
- `internal/apiserver/routers.go` - 路由已注册
- `internal/apiserver/container/container.go` - 依赖注入已配置
- `internal/apiserver/interface/restful/response/actor.go` - GuardianResponse 已定义

## 版本信息

- **更新日期**: 2025-12-16
- **文档版本**: v1.0
- **涉及模块**: Actor, IAM, Response
- **API 版本**: v1
