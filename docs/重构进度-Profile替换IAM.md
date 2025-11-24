# Testee 模块重构：Profile 替换 IAM User/Child

## 重构目标
将 Testee 中的 `IAMUserID` 和 `IAMChildID` 统一为 `ProfileID`，为未来更通用的用户档案系统做准备。

## 已完成工作

### 1. 领域层 (Domain Layer) ✅
- **文件**: `internal/apiserver/domain/actor/testee/testee.go`
  - ✅ 将 `iamUserID` 和 `iamChildID` 字段替换为 `profileID *uint64`
  - ✅ 更新 `bindProfile()` 方法
  - ✅ 更新 `ProfileID()` 和 `IsBoundToProfile()` 方法
  - ✅ 更新 `RestoreFromRepository()` 方法签名

- **文件**: `internal/apiserver/domain/actor/testee/interfaces.go`
  - ✅ Repository 接口：`FindByIAMUser/FindByIAMChild` → `FindByProfile(orgID, profileID)`
  - ✅ Factory 接口：`GetOrCreateByIAMChild/GetOrCreateByIAMUser` → `GetOrCreateByProfile(orgID, profileID, ...)`

- **文件**: `internal/apiserver/domain/actor/testee/binder.go`
  - ✅ Binder 接口：`BindToIAMUser/BindToIAMChild` → `BindToProfile(profileID)`
  - ✅ 更新 binder 实现

- **文件**: `internal/apiserver/domain/actor/testee/factory.go`
  - ✅ 实现 `GetOrCreateByProfile()` 方法

### 2. 应用层 (Application Layer) 🔄
- **文件**: `internal/apiserver/application/actor/testee_management/service.go`
  - ✅ Service 接口：`FindByIAMChildID` → `FindByProfileID`
  - ✅ CreateTesteeDTO：`IAMUserID`/`IAMChildID` → `ProfileID *uint64`
  - ✅ TesteeResult：`IAMUserID`/`IAMChildID` → `ProfileID *uint64`

- **文件**: `internal/apiserver/application/actor/testee_management/interface.go`
  - ✅ TesteeProfileApplicationService：`BindIAMUser`/`BindIAMChild` → `BindProfile`
  - ✅ TesteeQueryApplicationService：`FindByIAMChild` → `FindByProfile`
  - ✅ TesteeManagementResult：`IAMUserID`/`IAMChildID` → `ProfileID *uint64`

- **文件**: `internal/apiserver/application/actor/testee_registration/interface.go`
  - ✅ RegisterTesteeDTO：`IAMUserID`/`IAMChildID` → `ProfileID *uint64`
  - ✅ EnsureTesteeDTO：`IAMUserID`/`IAMChildID` → `ProfileID *uint64`
  - ✅ TesteeResult：`IAMUserID`/`IAMChildID` → `ProfileID *uint64`

- **文件需要更新**:
  - ⏳ `composite_service.go` - 部分完成，需要修复 toTesteeResult
  - ⏳ `profile_service.go` - 需要实现 BindProfile 方法
  - ⏳ `query_service.go` - 需要实现 FindByProfile，更新 toManagementResult
  - ⏳ `testee_registration/service.go` - 需要更新所有使用 IAM 的地方
  - ⏳ `testee_registration/query_service.go` - 需要更新查询方法

## 待完成工作

### 3. 基础设施层 (Infrastructure Layer) ❌
需要更新的文件：
- `internal/apiserver/infra/mysql/actor/testee_repository.go`
  - 实现 `FindByProfile()` 方法
  - 删除 `FindByIAMUser()` 和 `FindByIAMChild()` 方法

- `internal/apiserver/infra/mysql/actor/testee_mapper.go`
  - 更新 PO → Domain 映射：`IAMUserID`/`IAMChildID` → `ProfileID`
  - 更新 Domain → PO 映射

- `internal/apiserver/infra/mysql/actor/testee_po.go` (可能需要)
  - 数据库表结构：`iam_user_id`/`iam_child_id` → `profile_id`

### 4. 接口层 (Interface Layer) ❌
需要更新的文件：

#### gRPC
- `internal/apiserver/interface/grpc/proto/actor/actor.proto`
  - CreateTesteeRequest：`iam_user_id`/`iam_child_id` → `profile_id`
  - TesteeResponse：`iam_user_id`/`iam_child_id` → `profile_id`
  - TesteeExistsRequest：`iam_child_id` → `profile_id`

- `internal/apiserver/interface/grpc/service/actor_service.go`
  - 更新所有使用 IAMUserID/IAMChildID 的地方
  - 更新 toTesteeProtoResponse 转换函数
  - 更新辅助转换函数

#### RESTful
- `internal/apiserver/interface/restful/handler/actor.go`
  - CreateTesteeRequest：`IAMUserID`/`IAMChildID` → `ProfileID`
  - TesteeResponse：`IAMUserID`/`IAMChildID` → `ProfileID`
  - 更新所有请求/响应转换

### 5. 数据库迁移 ❌
- 创建 migration 脚本
- 修改 testees 表结构：
  ```sql
  ALTER TABLE testees 
  DROP COLUMN iam_user_id,
  DROP COLUMN iam_child_id,
  ADD COLUMN profile_id BIGINT UNSIGNED NULL COMMENT '用户档案ID(当前对应IAM.Child.ID)';
  
  -- 数据迁移
  UPDATE testees SET profile_id = iam_child_id WHERE iam_child_id IS NOT NULL;
  ```

## 当前编译错误统计

根据最新检查，还有以下文件存在编译错误：

1. **应用层** (6个文件)
   - composite_service.go
   - profile_service.go  
   - query_service.go
   - testee_registration/service.go
   - testee_registration/query_service.go

2. **基础设施层** (2个文件)
   - testee_repository.go
   - testee_mapper.go

3. **接口层** (2个文件)
   - grpc/service/actor_service.go
   - restful/handler/actor.go

**总计**: 约 10 个文件需要修复编译错误

## 下一步行动计划

### 第一轮：修复应用层编译错误
1. 完成 `composite_service.go` 的 toTesteeResult 修复
2. 更新 `profile_service.go` 实现 BindProfile
3. 更新 `query_service.go` 实现 FindByProfile 和 toManagementResult
4. 更新 `testee_registration/service.go` 的所有方法
5. 更新 `testee_registration/query_service.go`

### 第二轮：修复基础设施层
1. 实现 `testee_repository.go` 的 FindByProfile
2. 更新 `testee_mapper.go` 的转换逻辑
3. 检查 PO 结构是否需要更新

### 第三轮：修复接口层
1. 更新 proto 文件定义
2. 重新生成 protobuf 代码
3. 更新 gRPC 服务实现
4. 更新 RESTful Handler

### 第四轮：测试验证
1. 编译通过
2. 单元测试
3. 集成测试
4. 启动服务验证

## 注意事项

1. **向后兼容性**: 当前 ProfileID 对应 IAM.Child.ID，未来可以扩展为更通用的档案系统
2. **数据迁移**: 需要将现有的 iam_child_id 数据迁移到 profile_id
3. **API 版本**: 可能需要保持 API 向后兼容或者升级 API 版本
4. **文档更新**: 完成后需要更新相关文档

## 时间估算

- 剩余应用层修复: ~30分钟
- 基础设施层: ~20分钟  
- 接口层: ~30分钟
- 测试验证: ~20分钟

**总计**: 约 1.5-2 小时完成所有重构工作
