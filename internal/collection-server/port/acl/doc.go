// Package acl 适配 application DTO 与 grpcbridge/infra 形状不一致的 BFF 边界。
//
// 边界约定：
//   - catalog / evaluation 读路径：grpcbridge 直接产出 application DTO，不经 acl；
//   - answersheet / testee：REST 与 gRPC 字段语义差异大，保留 acl 做双向映射。
package acl
