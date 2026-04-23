# govulncheck 剩余 advisory 分组处置：2026-04-23

**本文回答**：在仓库升级到 `Go 1.25.9` 之后，[`tmp/security/govulncheck.json`](../../tmp/security/govulncheck.json) 里还剩哪些需要处理的项、哪些已经被当前 toolchain 或依赖版本吸收，以及接下来该如何看待这些结果。

## 30 秒结论

截至 2026-04-23，`qs-server` 当前 `govulncheck` 结果可以稳定拆成三层：

1. **当前 active finding：0 条**  
   `Go 1.25.9` 已经吸收此前 `stdlib@go1.24.13` 上那 9 条真正成立的 toolchain finding。
2. **catalog 中已被当前版本吸收的历史项：211 条**  
   其中 `143` 条已被当前 `Go 1.25.9` 吸收，`68` 条第三方 advisory 已被当前依赖版本吸收，不应再作为当前阻塞项重复开单。
3. **catalog 中仍需关注但没有 active finding 的观察项：5 条**  
   包括 `stdlib` 的 `1.26.x` 修复线条目、一条 `golang.org/x/net` 新修复线，以及 `github.com/satori/go.uuid` 的历史条目；它们不是当前 module scan 的 active finding，但应进入 watchlist。

当前应以 [`tmp/security/govulncheck-summary.md`](../../tmp/security/govulncheck-summary.md) 为准，不再人工解读旧的 `govulncheck-module.txt` 文本输出。

## 分组结果

### A. 当前 active finding：0 条

升级到 `Go 1.25.9` 后，之前挂在 `stdlib@go1.24.13` 上的 9 条 active finding 已全部转入“当前版本已吸收”的历史项。当前摘要中的两项关键结论是：

- `Stdlib / Toolchain 缺口（0）`
- `第三方可行动 finding（0）`

这意味着当前 `govulncheck` 已不再给出需要立即修复的 module-scan finding。

**处置**：

1. 当前不再需要为 `govulncheck` 结果创建新的阻塞工单。
2. 后续若要把 `govulncheck` 从 advisory 推到 hard gate，应以“active finding 是否为 0”为准，而不是以 catalog 总量为准。

### B. catalog 中已被当前版本吸收的历史项：211 条

#### B1. 已被当前 `Go 1.25.9` 吸收：143 条

这部分是旧的 `stdlib` advisory。虽然 OSV catalog 里仍然存在，但当前仓库使用的 `Go 1.25.9` 已经高于对应修复线，其中也包括此前真实阻塞的：

- `GO-2026-4601`
- `GO-2026-4602`
- `GO-2026-4603`
- `GO-2026-4864`
- `GO-2026-4865`
- `GO-2026-4869`
- `GO-2026-4870`
- `GO-2026-4946`
- `GO-2026-4947`

**处置**：

1. 不再作为当前待修漏洞重复立项。
2. 保留在扫描摘要里，作为“升级已经吸收风险”的证据，而不是“当前阻塞”的证据。

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

这说明此前做的依赖升级仍然有效，`govulncheck` JSON 里保留这些 OSV 只是 catalog 事实，并不代表当前版本仍然 vulnerable。

**处置**：

1. 不要重新打开 `grpc / quic-go / edwards25519 / mapstructure` 这批已完成升级的工单。
2. 对三方库的安全判断，以“当前版本是否仍低于修复线”为准，而不是只看它是否出现在 OSV catalog 中。

### C. 观察项：3 个模块 / 5 条

当前还有 5 条属于“catalog 中需要保留关注，但没有 active finding”的观察项：

- `github.com/satori/go.uuid`：`GO-2022-0244`
- `golang.org/x/net`：`GO-2026-4559`
- `stdlib`：`GO-2026-4599`
- `stdlib`：`GO-2026-4600`
- `stdlib`：`GO-2026-4866`

这些条目不在当前 module scan 的 active finding 里，但它们提示两件事：

1. 如果扫描口径变化，或者 toolchain / indirect dependency 更新，它们可能会转成 active finding。
2. 当前升级目标已经从“修掉 `1.24.13` 上的真实缺口”转成“观察是否需要进入 `1.26.x` 升级窗口”。

**处置**：

1. 进入 watchlist，不作为当前 hard gate。
2. 每次 `Go` 或关键依赖升级后，重新跑 `make security-govulncheck-ci` 并核对这 5 条是否消失、转组或变成 active finding。

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

`govulncheck` 当前不应该再被解读成“仓库里还有未处理的 active 漏洞”。更准确的说法是：

1. **当前真正成立的 active finding 已经清零**。
2. **关键三方依赖升级与 Go toolchain 升级都已生效，历史 advisory 不应再重复计入当前待办**。
3. **后续安全升级焦点已从 `1.24.x -> 1.25.x` 切到 `1.26.x` watchlist 观察**。
3. **还有 6 条观察项需要继续 watch，但它们现在不是 active finding**。

因此，下一步安全治理的重点应从“继续扫一轮依赖升级”切到“规划下一次 Go toolchain 升级窗口”。  
