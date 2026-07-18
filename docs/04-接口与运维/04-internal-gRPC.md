# internal gRPC

## 1. 结论

internal gRPC 是 worker/collection 调用 apiserver 用例的进程边界，不是绕过权限、事务或领域校验的内部后门。

## 2. 责任链

```text
client
  -> service authentication / metadata
  -> gRPC service adapter
  -> application service
  -> domain + repository/UoW/outbox
```

## 3. 核对入口

- proto：`api/grpc/proto/internalapi`；
- server：`internal/apiserver/transport/grpc/service`；
- worker handler/client：`internal/worker`；
- collection client：`internal/collection-server`。

接口是否“internal”不改变其兼容性和可观测要求。
