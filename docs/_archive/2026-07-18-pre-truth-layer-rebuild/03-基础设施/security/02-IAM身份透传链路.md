# IAM 身份透传链路

## 1. 解决什么问题

IAM 身份透传解决外部请求进入 qs-server 后如何保留用户、组织和权限上下文的问题。

## 2. 所在位置

身份透传位于小程序、后台、collection-server、qs-apiserver 和 IAM bridge 之间。

## 3. 设计目标

不复制认证中心；只消费 IAM 身份；跨进程调用携带必要身份上下文；业务层不直接解析 token。

## 4. 正常流程

入口解析 token，生成 Principal；调用 IAM 或本地投影获取组织范围和权限；业务服务只消费标准上下文。

## 5. 异常流程

IAM 超时或返回错误时，读接口可按配置降级，写接口默认保守失败；权限不明确时拒绝。

## 6. 观测指标

IAM call latency、IAM error rate、principal missing、scope missing、authz denied。

## 7. 代码事实源

- [../../../internal/apiserver/infra/iam](../../../internal/apiserver/infra/iam)
- [../../../internal/apiserver/port/iambridge](../../../internal/apiserver/port/iambridge)
- [../../../internal/collection-server/infra/iam](../../../internal/collection-server/infra/iam)
- [../../../internal/collection-server/port/iamport](../../../internal/collection-server/port/iamport)
