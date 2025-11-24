package service

// TODO: ActorService gRPC 实现需要重构以适配新的按行为者组织的服务接口
// 暂时禁用此文件，直到完成 gRPC 层的重构
//
// 重构要点：
// 1. 使用 TesteeRegistrationService, TesteeManagementService, TesteeQueryService
// 2. 根据 gRPC 方法的语义，调用对应的应用服务
// 3. 更新 DTO 转换逻辑
