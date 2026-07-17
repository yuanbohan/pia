# 第 05 课：Coding Tools 与 Workspace 边界

## 当前状态

实现中；`read` 子阶段的代码、测试和文档已经完成并经学习者确认，下一步进入 `write` 的讲解与设计讨论。

开课记录：第 04 课已经完成、提交并 push 到 `main`。学习者确认第 05 课主要进入具体 Tool 实现，并要求按 `read`、`write`、`edit`、`bash` 一个个讲解和推进；本课先对齐共同 Tool 契约与 workspace 所有权，再逐个完成解释、讨论、实现和测试。

## 本课目标

完成本课后，学习者应能解释并从测试中验证：

1. 为什么通用 `agent.Tool` 契约留在 `internal/agent`，具体文件和进程实现进入 `internal/coding/tools`。
2. `os.Root` 能保证什么、不能保证什么，以及文件工具为什么仍要拒绝非 regular-file 目标。
3. `read`、`write`、`edit` 如何限制模型可见内容并保持同一个 workspace 内的可观察副作用顺序。
4. `bash` 为什么只是固定 cwd、最小环境和可取消进程组，而不是 sandbox。
5. 为什么只有 `read` 声明 `CanRunParallel=true`，其余工具保持串行屏障。

## 证据边界

冻结 Pi 基线：commit `dcfe36c79702ec240b146c45f167ab75ecddd205`，Pi Agent package version `0.80.7`。

冻结源码阅读路径：

- `packages/coding-agent/src/core/tools/read.ts`
- `packages/coding-agent/src/core/tools/write.ts`
- `packages/coding-agent/src/core/tools/edit.ts`
- `packages/coding-agent/src/core/tools/bash.ts`
- `packages/coding-agent/src/core/tools/truncate.ts`
- `packages/coding-agent/src/core/tools/path-utils.ts`

当前 Go 实现依据本仓库的 Go 1.26 toolchain 与标准库 `os.Root` 文档。Pi 源码说明上游工具的可观察能力；`docs/plans/2026-07-15-001-pi-core-go-learning-port-plan.md` 中的 U9、R9 和 KTD7/KTD8/KTD20/KTD23/KTD26/KTD27 是 pi-go 的范围与安全契约。候选设计在讨论确认前不记作既定架构。

## 推进顺序

1. 共同契约与 workspace 所有权。
2. `read`：root-relative 读取、regular-file 校验、分页和输出上限。
3. `write`：创建/覆盖、父目录和替换提交。
4. `edit`：唯一精确替换、诊断和替换提交。
5. `bash`：cwd、最小环境、超时、进程组取消、drain/reap 和尾部截断。
6. 四工具组合与完整质量门禁。

## 已验证的共同契约

`internal/agent` 已经拥有 `Tool`：

- `Definition()` 冻结模型可见 JSON Schema 和调度能力；
- `Execute(ctx, rawArguments)` 负责具体工具的解码、语义校验和副作用；
- 普通工具错误成为 call-local error result，不直接终止整个 Run；
- `CanRunParallel` 默认 false，声明为 true 的同一工具实例必须支持并发调用。

因此第 05 课不会再设计第二套 Tool 接口。`internal/coding` 拥有 workspace 生命周期，文件工具共享同一个已打开的 `*os.Root`；工具不关闭 root，创建它的组合层负责最终关闭。`bash` 的 cwd 如何从同一个 Workspace 派生留到其小节确认，不把“持有 root”误写成命令 sandbox。`read` 可以并发使用该 root，`write`、`edit` 和 `bash` 不声明并行安全。

已经确认会被多个文件工具复用的严格 JSON object 解码和 workspace-relative path 规范化放入 `internal/coding/tools/utils`。分页、完整行扫描、UTF-8 校验和结果格式仍留在 `read`，因为这些是读取协议而不是所有工具都共享的能力；非阻塞打开也留在 `read` 旁，因为它专门防止 FIFO 在完成 opened-handle 校验前阻塞。这样落实“通用能力进入 utils”的同时，不为尚未讨论的 `write`/`edit` 提前制造抽象。

## 与冻结 Pi 的明确差别

冻结 Pi 的 `read` 支持相对/绝对路径、图片、UI 渲染、可替换远程 operations、`offset`/`limit`，并用 2000 行与 50 KiB 双上限截断文本。它通过普通 cwd/path resolution 访问文件，本身不是 workspace containment primitive。冻结实现的默认 operations 先调用 `fsAccess(path, R_OK)`，再用 `fsReadFile(path)` 读取整个 `Buffer`；`read.ts` 没有 regular-file/FIFO 检查、`O_NONBLOCK` 或按平台选择的打开实现，相应工具测试也没有建立 FIFO 契约。因此 Pi 并不是用 pi-go 当前方式处理 FIFO：本地 FIFO 没有 writer 时可能停在底层打开/读取；它的 abort wrapper 会拒绝外层 Promise，但没有把 `AbortSignal` 传给 `fsReadFile`，不等同于将底层操作改成 nonblocking。

pi-go 第一期不移植图片、TUI、远程 operations 或绝对路径访问。文件工具只接受 root-relative 路径并通过共享 `os.Root` 操作。`os.Root` 会阻止 `..` 或 symlink 指向 root 外部，但不会阻止 mount、设备文件或 root 内特殊文件，因此打开目标后仍要基于实际 handle 校验 regular-file，不能把“位于 root 内”误写成“内容一定安全”。

`read` 还有五个经过测试或冻结源码核对的有意差异：

- 冻结 Pi 对默认 `fsReadFile` 目标不做 regular-file/FIFO 区分。pi-go 先取得实际 handle 再校验 regular-file，并在 macOS/Linux 用 `O_NONBLOCK` 避免无 writer 的 FIFO 在校验前阻塞；其他平台暂用普通 `Open` 保持包可构建，尚不承诺这一项非阻塞保证，也不因此扩展第一阶段只支持 macOS/Linux 的平台范围。
- 不先把整文件读入内存，也不为了计算总行数扫描未返回内容；它流式读取当前页并只探测是否还有后续字节。
- 任何 `..` path component 都直接拒绝，而不先 `filepath.Clean`。如果较早的 component 是 symlink，先 clean `alias/../file` 会改变真正应访问的对象；拒绝该形状才能让模型看到的规范化路径和交给 `os.Root` 的路径保持一致。
- 完整行包含磁盘上的终止换行，50 KiB 按实际返回字节计算。冻结 Pi 的 `split/join` 会在用户 `limit` 恰好停在文件末尾换行前时提示一个空的尾页；pi-go 不制造这个无内容页。
- 冻结 Pi 通过 Node 的 UTF-8 解码替换非法字节；pi-go 在本次选中页包含非法 UTF-8 时返回 call-local error。未返回的后续字节不会仅为整文件编码判定而被扫描，读取到对应页时再报错。

## 05-A：`read` 设计讨论

已经确定保留 Pi 对 coding loop 有直接价值的三个输入：`path`、可选的 1-based `offset` 和可选 `limit`。读取只返回完整 UTF-8 行，默认最多 2000 行或 50 KiB，先达到哪个上限就停止，并给模型明确的下一次 `offset`。这样输出在进入 transcript 前已经有界，同时模型仍能不用 `bash` 分页读取大文件。

已确认：

- `read` 保留可选 `offset`/`limit`。没有安全分页能力时，大文件会迫使模型绕过文件工具改用 `bash`；保留分页可以同时满足 transcript 输出上限和后续内容读取。
- 本次返回页含非法 UTF-8 时形成 call-local error，不静默替换无效字节。模型必须看到与磁盘字节一致的文本视图，否则替换字符会让后续 `edit` 的精确匹配失败，写回时还可能破坏原文件。实现必须在 `utf8.Valid` 判断旁保留这条原因注释，方便后续 review 区分有意的文本边界与随意收紧输入；不增加基于扩展名或内容猜测文件类型的启发式逻辑。
- 模型可见结果固定包含规范化的 workspace-relative path、实际返回行范围、内容和结束状态。存在后续内容时给出下一次 1-based `offset`；已经到达 EOF 时明确标记。结果不暴露 host absolute path，也不为了报告总行数而继续扫描未返回的整份文件。

固定文本形状为：

```text
Path: internal/example.go
Lines: 101-180
Content:
<原始 UTF-8 内容，保留行结束符>

[More content available. Continue with offset=181.]
```

完整读到 EOF 时最后一行改为 `[End of file.]`；空文件成功返回 `Lines: 0`、`[empty file]` 和 EOF 标记。`offset` 超过 EOF 是 call-local error。路径字段表示清理后实际交给 `os.Root` 的相对路径，不声称解析成 host realpath；这样既让模型获得稳定定位信息，也不绕过 root 去做另一套易竞态的绝对路径解析。

以上设计已经由学习者确认，`read` 进入测试先行实现。

## 05-A：`read` 实现记录

本子阶段新增：

- `internal/coding/workspace.go`：规范化操作者选择的 workspace，并拥有一个共享 `*os.Root` 的生命周期；文件工具只借用 root，不负责关闭。
- `internal/coding/tools/utils/arguments.go`：只接受一个 JSON object，拒绝未知字段和尾随 JSON。
- `internal/coding/tools/utils/path.go`：统一限制 4096-byte workspace-relative path，拒绝绝对路径、逃逸路径和任意 `..` component，返回 OS-native path 与 slash-normalized 模型路径。
- `internal/coding/tools/read.go`：实现 `read` schema、参数语义、regular-file 校验、流式分页、UTF-8 文本边界、稳定输出与 `CanRunParallel=true`。
- 平台打开 helper 与 workspace/read/utils 测试：覆盖 macOS/Linux FIFO 非阻塞拒绝、symlink 边界和交换竞态、并发与取消。

关键实现理由保留在相邻代码注释中：

1. 先打开再对实际 handle 做 `Stat`，避免验证一个对象却在路径替换后读取另一个对象。
2. `//go:build darwin || linux` 只在已经测试 FIFO 语义的平台启用 `O_NONBLOCK`；其他平台通过互斥 build tag 使用可移植的普通 `Open` 保持包可构建，在补充平台证据前不宣称无 writer FIFO 一定不会阻塞，也不把可构建误写成受支持。
3. `utf8.Valid` 不做静默 replacement，因为模型看到的文本必须能被后续 exact edit 可靠匹配和 round-trip。
4. 选中行一旦超过 50 KiB 就立即停止，不 drain 剩余巨型行；被 offset 跳过的单行也遵守相同边界，避免“有界结果”仍产生无界 I/O。
5. `read` 参数体限制为 8192 bytes，路径限制为 4096 bytes；标准 JSON/path 错误可能回显模型输入，先限制输入才能保证错误结果也不会绕过 transcript 边界。

测试先证明不存在实现时构建失败；实现后覆盖 schema/并行能力、稳定输出、CRLF 与终止换行、1-based offset/limit、2000 行和 50 KiB 边界、空文件、EOF、非法 UTF-8、非法参数、超长诊断、missing/non-regular/FIFO、internal/escaping/dangling symlink、symlink swap outside canary、预取消和同实例并发。实现审查额外发现并修复了巨型单行无界 drain、offset 绕过单行上限、`filepath.Clean` 改变 symlink 前 `..` 语义，以及 decoder 接受 `null` 的问题。

验证结果：`go test ./...`、`go vet ./...`、`go test -race ./...` 和 `git diff --check` 全部通过。`read` 子阶段没有剩余 actionable review finding；学习者已经确认理解并明确要求形成独立本地 commit，本课整体继续进入 `write`。
