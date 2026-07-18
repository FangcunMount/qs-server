# collection REST

## 1. 事实源

[`api/rest/collection.yaml`](../../api/rest/collection.yaml) 是小程序/前台接口契约。

## 2. 服务责任

collection REST 提供前台模型目录、问卷/答卷、提交状态、报告等待与报告查询等 BFF 接口，并组合身份投影、限流、排队、防重和下游背压。

## 3. 接入原则

- 客户端必须区分成功、已接收、限流、排队/下游过载和业务校验失败；
- 提交后使用服务返回的 request/assessment/report 标识继续查询；
- WebSocket 或长轮询唤醒失败时回退到事实查询；
- 不根据旧 prose 文档硬编码枚举，使用当前 OpenAPI 和后端返回。

## 4. 深入阅读

见 [小程序接入文档](./15-小程序接入文档.md) 与 [报告等待指南](./12-小程序报告等待接入指南.md)。
