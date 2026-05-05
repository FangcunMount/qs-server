# ModuleGraph 与 PostWire 边界

**本文回答**：apiserver 为什么保留 `moduleGraph` / post-wire hook；哪些依赖应构造期注入，哪些依赖可以 late-bound；当前 hooks 为什么很多已经退化成显式阶段标记；后续如何避免滥用 post-wire。

---

## 30 秒结论

| 维度 | 结论 |
| ---- | ---- |
| moduleGraph 定位 | 管理 apiserver 跨模块 post-wiring 的显式阶段对象 |
| 首选方式 | 构造函数依赖仍是首选 |
| 允许 post-wire | init order 或 optional infrastructure 造成构造期循环时，才可使用 |
| 当前状态 | 多个 hook 只保留阶段标记，依赖已经转为 constructor dependencies |
| 风险 | 滥用 post-wire 会隐藏依赖方向、制造运行期 nil、破坏架构边界 |

一句话概括：

> **PostWire 是少数 late-bound 依赖的安全阀，不是绕过构造函数依赖的捷径。**

---

## 1. moduleGraph 当前定义

`module_graph.go` 注释明确：

```text
Constructor dependencies remain the preferred path.
This graph exists for late-bound dependencies where init order or optional infrastructure would otherwise force module constructors into cycles.
```

它持有：

```go
type moduleGraph struct {
  container *Container
}
```

---

## 2. 当前 hooks

| Hook | 当前状态 |
| ---- | -------- |
| postWireCacheGovernanceDependencies | 依赖已通过 REST deps / StatisticsHandler 构造时接入 |
| postWireProtectedScopeDependencies | Protected-scope 依赖已变成构造期依赖，hook 保留阶段标记 |
| postWireQRCodeService | QRCode 依赖已变成构造期依赖 |

这说明当前工程正在从 post-wire 向 constructor injection 收敛。

---

## 3. 为什么仍保留 hook

保留 hook 的价值：

- 显式标出曾经的 late-bound 依赖阶段。
- 给后续重构提供稳定位置。
- 避免直接删除导致文档/测试/读者无法理解历史边界。
- 允许少数 optional infrastructure 做后置绑定。

---

## 4. 什么时候可以 post-wire

只有满足以下条件才考虑：

1. 构造期依赖会造成循环。
2. 依赖是 optional infrastructure。
3. 依赖不是领域不变量。
4. 有明确 nil/degraded 语义。
5. 有测试覆盖。
6. 文档说明为什么不能 constructor injection。

---

## 5. 什么时候禁止 post-wire

禁止用于：

- domain dependency。
- application 必需 port。
- repository 必需依赖。
- 权限/安全关键依赖。
- 数据一致性关键依赖。
- 为了省事绕过构造函数。
- handler 临时补依赖。

---

## 6. 依赖注入优先级

推荐顺序：

```text
constructor dependency
  -> module assembler dependency
  -> ContainerOptions
  -> explicit runtime deps
  -> post-wire hook
```

post-wire 永远排在最后。

---

## 7. 风险

| 风险 | 后果 |
| ---- | ---- |
| 依赖方向隐藏 | 架构图与代码不一致 |
| 初始化顺序脆弱 | 某些字段在使用时 nil |
| 测试困难 | 构造器无法暴露缺失依赖 |
| 可选依赖被误当必需 | 运行期才失败 |
| 循环依赖扩大 | 模块边界失控 |

---

## 8. 修改指南

新增 post-wire 前必须写：

```text
为什么不能 constructor injection？
依赖是否 optional？
nil 时行为是什么？
失败是否阻断启动？
如何测试？
何时可以移回 constructor？
```

如果答不出来，不允许新增 post-wire。

---

## 9. Verify

```bash
go test ./internal/apiserver/container
go test ./internal/apiserver/process
```
