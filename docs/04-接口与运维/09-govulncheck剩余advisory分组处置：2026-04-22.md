# govulncheck 剩余 advisory 分组处置：2026-04-22

**本文回答**：当前 [`tmp/security/govulncheck.json`](../../tmp/security/govulncheck.json) 里剩余的 advisory 应该如何分组解读，哪些是真正还在当前版本上成立的 finding，哪些只是 OSV catalog 中已被当前版本吸收的历史项，以及接下来该怎样处置。

## 30 秒结论

截至 2026-04-22，`qs-server` 当前 `govulncheck` 结果可以稳定拆成三层：

1. **当前 active finding：9 条**  
   全部来自 `stdlib@go1.24.13`，修复线在 `go1.25.8 / go1.25.9`；这部分是真正还存在的 toolchain 风险。
2. **catalog 中已被当前版本吸收的历史项：201 条**  
   其中 `133` 条已被当前 `Go 1.24.13` 吸收，`68` 条第三方 advisory 已被当前依赖版本吸收，不应再作为当前阻塞项重复开单。
3. **catalog 中仍需关注但没有 active finding 的观察项：6 条**  
   包括 `stdlib` 的未来线条目、`golang.org/x/net` 的一条新修复线，以及 `github.com/satori/go.uuid` 的历史条目；它们不是当前 module scan 的 active finding，但应该进入 watchlist。

当前应以 [`tmp/security/govulncheck-summary.md`](../../tmp/security/govulncheck-summary.md) 为准，不再人工解读旧的 `govulncheck-module.txt` 文本输出。

## 分组结果

### A. 当前 active finding：9 条 stdlib / toolchain 缺口

这 9 条是当前版本上真正成立的 finding：

- `GO-2026-4601`
- `GO-2026-4602`
- `GO-2026-4603`
- `GO-2026-4864`
- `GO-2026-4865`
- `GO-2026-4869`
- `GO-2026-4870`
- `GO-2026-4946`
- `GO-2026-4947`

共同特征：

- 当前扫描命中的是 `stdlib@go1.24.13`
- 修复版本都在 `go1.25.8` 或 `go1.25.9`
- 这意味着它们不能通过继续升级三方依赖来解决，只能通过升级 Go toolchain 消除

**处置**：

1. 保持 advisory 状态，作为当前剩余安全债的主清单。
2. 在下一次工具链升级窗口，优先评估从 `1.24.x` 升到带补丁的 `1.25.x`。
3. 在升级之前，不要把这些条目误归类成“某个业务模块代码问题”。

### B. catalog 中已被当前版本吸收的历史项：201 条

#### B1. 已被当前 `Go 1.24.13` 吸收：133 条

这部分是旧的 `stdlib` advisory。虽然 OSV catalog 里仍然存在，但当前仓库使用的 `Go 1.24.13` 已经高于对应修复线。

**处置**：

1. 不再作为当前待修漏洞重复立项。
2. 保留在扫描摘要里，作为“历史已覆盖”证据，而不是“当前阻塞”证据。

#### B2. 第三方已修复但仍出现在 OSV catalog：17 个模块 / 68 条

当前已经被仓库版本吸收的代表性模块包括：

- `google.golang.org/grpc`
- `github.com/quic-go/quic-go`
- `filippo.io/edwards25519`
- `github.com/go-viper/mapstructure/v2`
- `github.com/gin-gonic/gin`
- `golang.org/x/crypto`
- `golang.org/x/net`
- `google.golang.org/protobuf`

这说明此前做的依赖升级已经真正生效，`govulncheck` JSON 里保留这些 OSV 只是 catalog 事实，并不代表当前版本仍然 vulnerable。

**处置**：

1. 不要重新打开 `grpc / quic-go / edwards25519 / mapstructure` 这批已完成升级的工单。
2. 对三方库的安全判断，以“当前版本是否仍低于修复线”为准，而不是只看它是否出现在 OSV catalog 中。

### C. 观察项：3 个模块 / 6 条

当前还有 6 条属于“catalog 中需要保留关注，但没有 active finding”的观察项：

- `github.com/satori/go.uuid`：`GO-2022-0244`
- `golang.org/x/net`：`GO-2026-4559`
- `stdlib`：`GO-2025-3955`
- `stdlib`：`GO-2026-4599`
- `stdlib`：`GO-2026-4600`
- `stdlib`：`GO-2026-4866`

这些条目不在当前 module scan 的 active finding 里，但它们提示两件事：

1. 未来如果扫描口径变化，或者 toolchain / indirect dependency 更新，它们可能会转成 active finding。
2. 当前升级目标不应只盯 `1.25.x`，还要留意后续 `1.26.x` 线上的修复窗口。

**处置**：

1. 进入 watchlist，不作为当前 hard gate。
2. 每次 `Go` 或关键依赖升级后，重新跑 `make security-govulncheck-ci` 并核对这 6 条是否消失、转组或变成 active finding。

## 运行方式

现在执行下面这个命令会同时生成原始 JSON 和分组摘要：

```bash
make security-govulncheck-ci
```

产物：

- 原始扫描流：[`tmp/security/govulncheck.json`](../../tmp/security/govulncheck.json)
- 分组摘要：[`tmp/security/govulncheck-summary.md`](../../tmp/security/govulncheck-summary.md)

对应脚本：

- [`scripts/security/govulncheck_summary.go`](../../scripts/security/govulncheck_summary.go)

## 最终判断

`govulncheck` 当前不应该再被解读成“仓库里还有大量未处理三方依赖漏洞”。更准确的说法是：

1. **当前真正剩余的安全债是 9 条 stdlib / toolchain finding**。
2. **关键三方依赖升级已经生效，历史 advisory 不应再重复计入当前待办**。
3. **还有 6 条观察项需要继续 watch，但它们现在不是 active finding**。

因此，下一步安全治理的重点应从“继续扫一轮依赖升级”切到“规划下一次 Go toolchain 升级窗口”。  
