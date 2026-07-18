# 第 05 课：Coding Tools 与 Workspace 边界

## 当前状态

待理解确认；`read`、`write` 与 `edit` 子阶段已经完成并提交，`bash` 也已完成冻结 Pi 源码核对、corner-case 讨论、测试先行实现和全仓验证。第 05 课代码已完成但尚未 commit，等待学习者 review 实现并确认理解。

开课记录：第 04 课已经完成、提交并 push 到 `main`。学习者确认第 05 课主要进入具体 Tool 实现，并要求按 `read`、`write`、`edit`、`bash` 一个个讲解和推进；本课先对齐共同 Tool 契约与 workspace 所有权，再逐个完成解释、讨论、实现和测试。

## 本课目标

完成本课后，学习者应能解释并从测试中验证：

1. 为什么通用 `agent.Tool` 契约留在 `internal/agent`，具体文件和进程实现进入 `internal/coding/tools`。
2. `os.Root` 能保证什么、不能保证什么，以及文件工具为什么仍要拒绝非 regular-file 目标。
3. `read`、`write`、`edit` 如何限制模型可见内容并保持同一个 workspace 内的可观察副作用顺序。
4. `bash` 为什么只是固定初始 cwd、继承完整 CLI 环境并管理可取消进程组，而不是 sandbox。
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

当前 Go 实现依据本仓库的 Go 1.26 toolchain 与标准库 `os.Root` 文档。Pi 源码说明上游工具的可观察能力；`docs/plans/2026-07-15-001-pi-core-go-learning-port-plan.md` 中的 U9、R9 和 KTD7/KTD8/KTD16/KTD20/KTD23/KTD26/KTD27/KTD35 是 pi-go 的范围与安全契约。候选设计在讨论确认前不记作既定架构。

## 推进顺序

1. 共同契约与 workspace 所有权。
2. `read`：root-relative 读取、regular-file 校验、分页和输出上限。
3. `write`：创建/覆盖、父目录和替换提交。
4. `edit`：唯一精确替换、诊断和替换提交。
5. `bash`：cwd、完整父环境、无交互 shell、可选超时、进程组取消、后台进程、drain/reap 和尾部截断。
6. 四工具组合与完整质量门禁。

## 已验证的共同契约

`internal/agent` 已经拥有 `Tool`：

- `Definition()` 冻结模型可见 JSON Schema 和调度能力；
- `Execute(ctx, rawArguments)` 负责具体工具的解码、语义校验和副作用；
- 普通工具错误成为 call-local error result，不直接终止整个 Run；
- `CanRunParallel` 默认 false，声明为 true 的同一工具实例必须支持并发调用。

因此第 05 课不会再设计第二套 Tool 接口。`internal/coding` 拥有 workspace 生命周期，文件工具共享同一个已打开的 `*os.Root`；工具不关闭 root，创建它的组合层负责最终关闭。`bash` 的 cwd 如何从同一个 Workspace 派生留到其小节确认，不把“持有 root”误写成命令 sandbox。`read` 可以并发使用该 root，`write`、`edit` 和 `bash` 不声明并行安全。

已经由 coding tools 实际复用的严格 JSON object 解码放入 `internal/coding/tools/toolargs`；workspace-relative path 规范化，以及 `write` 已建立且当前课程明确由 `edit` 复用的普通文件替换提交，放入 `internal/coding/tools/fileutil`。两者暂时都留在 `coding`：`agent.Tool` 只拥有原始参数的执行契约，Agent 循环本身并不使用具体解码 helper；在出现非 coding 工具复用同一严格解码契约前，不因为名字“通用”就提前上移。分页、完整行扫描、UTF-8 校验和结果格式仍留在 `read`，因为这些是读取协议；非阻塞打开也留在 `read` 旁，因为它专门防止 FIFO 在完成 opened-handle 校验前阻塞。每个模型工具使用独立子 package，避免为私有符号增加多余的 `read`/`write` 前缀；共享 package 也按参数协议与文件系统责任分开，而不是继续扩张含义模糊的 `utils`。

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
- `internal/coding/tools/toolargs/decode.go`：只接受一个 JSON object，拒绝未知字段和尾随 JSON，并限制可能包含模型输入的错误诊断。
- `internal/coding/tools/fileutil/path.go`：统一限制 4096-byte workspace-relative path，拒绝绝对路径、逃逸路径和任意 `..` component，返回 OS-native path 与 slash-normalized 模型路径。
- `internal/coding/tools/read/tool.go`：实现 `read` schema、参数语义、regular-file 校验、稳定输出与 `CanRunParallel=true`；`page.go` 保存流式分页和 UTF-8 文本边界，`open_nonblocking.go` 与 `open_portable.go` 明确区分已验证和可移植的打开策略。
- `internal/coding/tools/read/*_test.go` 与 workspace/toolargs/fileutil 测试：按模型协议、分页、workspace boundary 和平台特殊文件拆分，覆盖 macOS/Linux FIFO 非阻塞拒绝、symlink 边界和交换竞态、并发与取消。

关键实现理由保留在相邻代码注释中：

1. 先打开再对实际 handle 做 `Stat`，避免验证一个对象却在路径替换后读取另一个对象。
2. `//go:build darwin || linux` 只在已经测试 FIFO 语义的平台启用 `O_NONBLOCK`；其他平台通过互斥 build tag 使用可移植的普通 `Open` 保持包可构建，在补充平台证据前不宣称无 writer FIFO 一定不会阻塞，也不把可构建误写成受支持。
3. `utf8.Valid` 不做静默 replacement，因为模型看到的文本必须能被后续 exact edit 可靠匹配和 round-trip。
4. 选中行一旦超过 50 KiB 就立即停止，不 drain 剩余巨型行；被 offset 跳过的单行也遵守相同边界，避免“有界结果”仍产生无界 I/O。
5. `read` 参数体限制为 8192 bytes，路径限制为 4096 bytes；标准 JSON/path 错误可能回显模型输入，先限制输入才能保证错误结果也不会绕过 transcript 边界。

测试先证明不存在实现时构建失败；实现后覆盖 schema/并行能力、稳定输出、CRLF 与终止换行、1-based offset/limit、2000 行和 50 KiB 边界、空文件、EOF、非法 UTF-8、非法参数、超长诊断、missing/non-regular/FIFO、internal/escaping/dangling symlink、symlink swap outside canary、预取消和同实例并发。实现审查额外发现并修复了巨型单行无界 drain、offset 绕过单行上限、`filepath.Clean` 改变 symlink 前 `..` 语义，以及 decoder 接受 `null` 的问题。

验证结果：`go test ./...`、`go vet ./...`、`go test -race ./...` 和 `git diff --check` 全部通过。`read` 子阶段没有剩余 actionable review finding；学习者已经确认理解并明确要求形成独立本地 commit，本课整体继续进入 `write`。

## 05-B：`write` 设计讨论

冻结 Pi 的 `write` 接受 `path` 和 `content`，递归创建父目录后直接调用 Node `fsWriteFile`；已有测试只锁定写入内容和自动创建父目录。它允许相对/绝对路径，默认会跟随最终 symlink，覆盖时直接截断目标，并通过全局 file-mutation queue 串行化同一 resolved path。成功文本中的 `content.length` 是 JavaScript UTF-16 code-unit 数，却标记为 bytes，例如 `你好` 会报告 2 而不是 UTF-8 的 6 bytes。

学习者确认 pi-go 不把目录、symlink、FIFO、socket 或 device 的创建混进 `write`。本工具只负责工作区内普通文件的完整内容创建或覆盖：

- 目标不存在时创建普通文件，父目录不存在时递归创建；空内容合法，不增加没有计划证据的文件大小上限。
- 已存在的普通文件通过同目录临时文件和 rename 替换，不直接截断模型当前可见的目标。
- 最终 component 是 symlink、目录或其他特殊文件时拒绝；即使 symlink 已由 bash 创建，后续 `write` 也不会跟随它。创建或操作这些对象需要由 unsandboxed bash 显式完成。
- 指向 workspace 内目录的 ancestor symlink 可以使用；逃逸或 dangling ancestor 仍由 `os.Root` 拒绝。模型结果展示输入规范化后的相对路径，不声称它是 host realpath。
- `write` 继续保持 `CanRunParallel=false`，依赖 Agent 的串行屏障，不复制 Pi 的进程级 per-path mutation queue。

替换提交只承诺运行期间的原子可见性：读者看到旧文件或完整新文件，不看到最终路径上的半写内容。它没有调用 file/directory `fsync`，因此不承诺断电或内核崩溃后的 durability。rename 前取消或失败会关闭并删除临时文件，原目标保持不变；已经创建的父目录不会回滚，因为删除目录会与 workspace 中的其他参与者竞争。rename 是 commit point，提交后的取消不再把已完成副作用报告成失败并诱导模型重试。

新文件使用 `0666`、新目录使用 `0777`，两者都服从进程 umask；覆盖普通文件时保留已有的九个 permission bits。临时文件替换会产生新 inode，第一阶段不保留原 hard-link relationship，也不复制原文件 owner、ACL、extended attributes 或特殊 mode bits。成功结果不回显文件内容，只返回规范化路径和 `len(content)` 的实际 UTF-8 bytes：

```text
Successfully wrote 7 bytes to nested/hello.txt
```

## 05-B：`write` 实现记录

本子阶段新增：

- `internal/coding/tools/write/tool.go`：实现 schema、必填但可为空的 `content`、父目录创建、固定 parent root、普通文件替换和稳定成功文本。
- `internal/coding/tools/fileutil/replace.go`：提供已经由 `write` 建立、且后续 `edit` 也需要的同目录替换提交；对最终 component 做 `Lstat`，用随机 `O_EXCL` 临时文件写完整内容，并在 context 仍有效时 rename。
- `internal/coding/tools/write/*_test.go` 与共享 helper 测试：按模型协议、workspace boundary 和平台特殊文件拆分，覆盖新建/覆盖、空内容、大于 read 参数上限的内容、UTF-8 byte count、permission bits、参数错误、目录/symlink/FIFO、内部/逃逸 ancestor symlink、final/ancestor swap outside canary、取消清理以及 `write → read` 可见性。

实现把 resolved parent 先通过 `root.OpenRoot` 固定下来，再只用 basename 创建临时文件和 rename。这样 ancestor symlink 在校验后被交换时，临时文件、目标和失败清理仍指向同一个已打开目录；最终 symlink 由 `Lstat` 拒绝，若它在检查后才出现，rename 替换的是 link directory entry，不会跟随 referent。共享 JSON decoder 同时把可能回显超长模型字段名的错误截为 512 bytes；因此 `write` 可以接受完整大内容，又不会让 malformed arguments 通过 error result 制造无界模型输出。

最初只实现 `read` 时，扁平 `tools` package 尚未形成明确冲突；加入 `write` 后重新按完整课程结构审查，已经出现三个具体信号：工具私有符号需要 `read`/`write` 前缀才能共存、不同工具的协议与平台测试混在同一 package、`write_test.go` 还隐式调用 `read_test.go` 的 helper。学习者确认后将工具拆为 `tools/read` 与 `tools/write` 子 package，构造 API 收敛为各包的 `New`，私有 helper 去掉工具名前缀；测试 helper 只在所属工具 package 内共享，`write → read` 只通过两个工具的公开行为做集成验证。后续 `edit`、`bash` 遵循同一布局，但不提前创建空 package。

这次修正不是按行数机械拆分：在 `write` 子阶段当时，`read/page.go`、按平台行为命名的 `read/open_nonblocking.go`/`open_portable.go`、模型协议 `tool.go` 各自承担独立责任；FIFO 专属测试也使用 `fifo_test.go`，而不是笼统的 `nonregular_test.go`。当时依赖保持为 `read/write -> toolargs + fileutil`，工具 package 之间没有生产依赖；`toolargs` 只保存 coding tools 当前共用的严格参数解码，`fileutil` 只保存 workspace path 与普通文件替换原语。加入 `edit` 后 opened-handle regular-file 打开形成第二个真实消费者，因此平台 open 文件随后按 05-C 的记录迁入 `fileutil`；read 分页、结果格式和测试 helper 仍未进入共享 package，也没有引入 filesystem interface、base tool、registry 或尚未需要的 `bash` 空 package。

按工具拆包并将共享责任收敛为 `toolargs`/`fileutil` 后，`gofmt`、`go test ./...`、`go vet ./...`、`go test -race ./...` 与 `git diff --check` 全部通过；read/final/ancestor symlink swap 测试分别重复 20 次、取消清理测试重复 100 次也通过。简化与代码审查没有剩余 actionable finding。`write` 子阶段实现已经完成，仍需学习者确认后才进入 `edit`。

## 05-C：`edit` 设计讨论

最初的课程预览把冻结 Pi 的 `edit` 简化成了单个唯一精确替换；重新核对固定 commit 后确认该表述不完整。冻结实现的模型协议是 `path + edits[]`，每项包含 `oldText` 与 `newText`。所有编辑都在同一份原文件上定位，每个 `oldText` 必须唯一，区域不能重叠；任一项失败都不会写入，成功时再统一应用。冻结 Pi 的运行时还会在 exact match 失败后进行 fuzzy normalization，包括行结束符、尾部空格、Unicode compatibility form、智能引号、破折号和特殊空格，并保留 BOM/原行结束符。它通过 per-path mutation queue 串行化同一文件的自身写操作，最后直接调用 `writeFile`，另为 TUI 生成 preview、display diff 和 unified patch。

学习者选择保留已经由冻结源码证明的多段协议，但把 mutation 语义收敛为精确且可验证：

- 输入使用 `path + edits[]`，每项保持 Pi 的模型字段 `oldText`/`newText`；`edits` 至少一项，`oldText` 非空，`newText` 可以为空以删除目标块。严格 JSON object、未知字段拒绝和 workspace-relative path 继续复用 `toolargs`/`fileutil`。
- 文件必须已经存在且是 UTF-8 regular file；`edit` 不创建父目录或新文件。所有 `oldText` 按原始 UTF-8 内容逐字节匹配，包含空白和行结束符，并分别要求恰好出现一次。先完成零/多匹配与 overlap 校验，再构造整份新内容，因此失败不会产生部分编辑。
- fuzzy matching 明确不在本子阶段实现。精确匹配失败是文件不变且可由模型重新 `read` 后恢复的 false negative；模糊误匹配则可能把猜测结果真正提交。冻结 Pi 的 fuzzy 算法还涉及 normalization 后的唯一性、offset 映射、未修改字节保留、CRLF/BOM 和多编辑组合测试，学习者确认它应在出现真实需求后作为一次独立且工作量较大的任务设计、实现和验证，不能通过普通重构顺带加入。
- `edit` 先固定 resolved parent，读取并校验实际 opened handle，再复用 `ReplaceRegularFile` 在同一 parent root 中提交完整结果；因此拥有与 `write` 相同的最终 symlink、临时文件、取消和 rename commit point 语义。成功只返回替换块数和规范化相对路径，不回显文件内容，也不移植 TUI preview/diff metadata。
- `edit` 保持 `CanRunParallel=false`。第一期只有一个 active task，Agent 已把 `write`、`edit`、`bash` 作为串行屏障，因此不复制冻结 Pi 的进程级 mutation queue；这不声称能检测 workspace 外部参与者的并发修改。

加入 `edit` 后，nonblocking candidate open 加 opened-handle regular-file 校验第一次出现第二个真实消费者。此前只有 `read` 时将平台文件留在 `read` 包内是合理的；现在该能力迁入 `internal/coding/tools/fileutil`，由 `read` 与 `edit` 共用。这个修正只共享已经相同的 workspace 文件原语，不建立 filesystem interface、远程 operations 或含义宽泛的 `utils` package。

## 05-C：`edit` 实现记录

本子阶段新增 `internal/coding/tools/edit/tool.go` 负责 schema、参数与文件编排，`apply.go` 负责基于原文件的唯一匹配、overlap 检查和一次性内容构造；模型协议和匹配算法没有堆入同一个大文件。测试按公开协议、workspace/symlink boundary 和 macOS/Linux FIFO 行为拆分，覆盖多段成功、原文件匹配、删除、permission 保留、零/多匹配、overlap、无变化、全有或全无、严格参数、非法 UTF-8、预取消、read 可见性、特殊文件和 outside-canary 交换。

`apply.go` 在 exact matcher 旁保留英文原因注释：冻结 Pi 虽有 fuzzy fallback，pi-go Phase 1 有意让不完全匹配安全失败；后续只有经过独立设计和测试才能扩展该 mutation contract。`fileutil/open_*.go` 也保留英文平台注释，说明 `O_NONBLOCK` 只在已有 FIFO 测试的 Darwin/Linux 生效，portable fallback 不扩大第一期平台承诺。

实现审查发现最初为诊断精确重复次数而扫描全部重叠 occurrence，会在高度重复的大文件上产生不必要的放大工作；唯一性只需要确认第二个位置，最终实现因此在首个 match 后最多再搜索一次，并用 `aaa`/`aa` 测试锁定重叠 occurrence 也必须判为不唯一。这个收敛保留零/多匹配诊断，却不为模型无用的精确计数持续扫描整份重复内容。

最终验证中，`make check` 完成 `go fmt ./...`、`go vet ./...`、`go test ./...` 和 `golangci-lint run ./...`，lint 为 0 issues；`make race` 全部通过。`edit` package 连续运行 20 次通过，Windows cross-build 也验证 portable open fallback 仍可构建。简化和代码审查修复了上述重复扫描问题，没有剩余 actionable finding；`edit` 子阶段完成，仍需学习者确认理解后才进入 `bash`。

## 05-D：`bash` 源码纠正与设计讨论

此前课程材料在尚未进入 `bash` 子阶段、也未与学习者讨论时，就写入了“最小 allowlist 环境”“Provider 凭据不能进入子进程”和“正常 shell exit 后清理全部 descendants”。进入本子阶段重新核对冻结 Pi 后确认，这些不是上游行为：`bash.ts` 的本地 operations 使用 `getShellEnv()` 复制完整 `process.env`；每次调用启动 detached shell，只有 abort 或可选 timeout 调用 `killProcessTree`；正常 shell exit 后不会主动 kill 该进程组。`waitForChildProcess` 在 shell 已 exit 但 descendant 仍持有 stdout/stderr 时等待 pipe output idle，而不是等待所有 descendants 退出。学习者也明确指出此前安全边界从未讨论，因此旧结论不能继续作为实现依据。

讨论从最终产品形态出发：pi-go 是由用户在 Ghostty/zsh 等终端中启动的本地 coding-agent CLI。导出的环境变量属于进程环境，会按 `zsh -> pi-go -> bash` 继承；Bash 无需再次读取 `.zshrc`。未导出的 shell variable、alias、function 和 zsh option 不会继承。基于这个区别，学习者逐项确认下面的第一期契约：

- bash tool 启用后直接执行模型命令，不增加逐次审批、trust/yolo matrix 或 sandbox。文件工具仍受 `os.Root` 限制，bash 只保证每次调用从 workspace 开始，可以访问当前用户有权访问的 workspace 外文件、网络和其他资源。
- 每次调用使用新建的非交互、非 login shell，stdin 为空且不分配 PTY；`cd`、`export`、virtualenv activation 和 alias 不跨调用保留，文件、Git 状态、外部服务和后台进程等真实副作用会保留。shell 支持显式 path，默认按 `/bin/bash`、PATH 中的 `bash`、`sh` 回退。
- 子进程完整继承 pi-go 的进程环境。环境来源可以是启动 zsh、配置工具或上层 launcher；Provider 只需在内存中持有 key 并为 HTTP request 设置 Authorization，并不要求 key 一定存在环境中。反过来，如果 key 已在父环境中，bash 就能看到；如果 Pi-style auth storage 把 key 存在文件中，unsandboxed bash 也可能直接读取该文件。本阶段不声称 credential isolation。
- 输入为 `command` 与可选秒数 `timeout`，没有默认 timeout。timeout、Run cancellation 或 CLI cancellation 直接 hard-kill 原进程组；正常 exit 不清理后台进程。`server >server.log 2>&1 &` 可以继续运行，而 `go test &` 的后续失败不会改变 shell 已返回的成功；后台文件修改可能与后续工具竞态，`setsid`/daemonize 还可能逃出原进程组。
- stdout/stderr 合并并持续 drain。非零 shell exit 保留输出并形成 call-local error，Agent 继续；`grep` 的 no-match、pipeline 的最后命令以及 `cmd1; cmd2` 的最终 exit status 都服从 shell 自身语义，不由工具猜测业务成功。
- 模型 final result 只保留最后 2000 行或 50 KiB，完整 raw output 从首次超限开始写入系统临时文件并回填此前 chunks。该文件没有大小上限、不会自动删除，也可能包含命令打印的敏感内容；在无默认 timeout 的选择下，`yes` 等命令可以持续占用临时磁盘。学习者在看到该 corner case 后仍选择沿用 Pi。
- 最终 CLI 要像 Pi 一样节流展示实时 bash output，但第 05 课不提前增加 Agent event sink 或 Tool callback。当前实现必须在进程运行期间增量读取 pipe，使用独立 accumulator 形成有界 tail 与完整临时文件，从数据流上保留实时展示能力；第 06 课有真实 CLI consumer 时再决定 call identity、投递顺序和展示接口。

Go 机制不会机械复制 TypeScript API：macOS/Linux 通过 `exec.Cmd.SysProcAttr.Setpgid` 建立独立进程组，取消时对负 PID 发送 `SIGKILL`，并始终调用 `Wait` 回收直接 shell。只调用 `exec.CommandContext` 不足以满足该契约，因为它不能保证终止 shell 启动的整个进程组。stdout 与 stderr pipe 必须并发 drain 到同步 accumulator；正常 shell exit 后使用短 output-idle grace 处理仍持有 pipe 的 descendant，不能因等待 pipe close 永久挂住，也不能在 output 仍持续到达时过早丢弃尾部。

本子阶段不移植 Pi 的 TUI renderer、remote `BashOperations`、command prefix、spawn hook 或 Windows Git Bash/taskkill 分支。第一期只在 macOS/Linux 验证本地 shell、进程组、取消、后台进程、输出 tail/临时文件和 tool-result 语义；这些范围已经由学习者确认，可以进入测试先行实现。

## 05-D：`bash` 实现记录

本子阶段新增独立的 `internal/coding/tools/bash` package，没有把进程或输出逻辑放入文件工具的 `fileutil`，也没有建立含义宽泛的 utils：

- `tool.go` 保存模型 schema、`Config`、严格参数解码和 Agent-facing 结果。`Config.WorkingDirectory` 接收 composition root 已规范化的 `Workspace.Path()`；文件工具继续借用 `Workspace.Root()`，两者不会被虚构成相同的 base tool。
- `shell.go` 保存显式 shell path 与 `/bin/bash -> PATH bash -> sh` 的冻结 Pi 回退顺序，不读取 `$SHELL`，因为交互 zsh 与模型命令所需的 Bash-compatible shell 不是同一个责任。
- `process.go` 使用两个独立 pipe goroutine 持续读取 stdout/stderr，再由单一 event loop 顺序更新 accumulator。它同时使用 `exec.CommandContext` 作为 direct-shell fallback，并在 macOS/Linux 通过 `Setpgid` 与负 PID `SIGKILL` 处理整个原进程组；只杀 `exec.Cmd.Process` 会遗漏 shell 启动的 child/grandchild。每个已启动 shell 都进入 `Wait`。
- `process_group_unix.go` 保存已经验证的 Darwin/Linux 进程组机制；互斥的 portable 文件只让其他平台明确返回 Phase 1 unsupported error，不假装已经移植 Windows taskkill/Git Bash 语义。
- `output.go` 增量解码跨 chunk UTF-8，统计全局行数和 decoded bytes，只在内存保留有界 tail。阈值前 raw chunks 暂存在小型 `bytes.Buffer`；首次超过 2000 行或 50 KiB 时创建 `0600` 系统临时文件、先回填旧 chunks，再持续写入后续原始 bytes。产品有意不删除该文件。

正常 shell exit 后不能直接等待 pipe EOF：安静的后台进程可能长期继承 handle。实现因此沿用冻结 Pi 的 100ms output-idle grace；每个新 chunk 都重新计时，持续输出的 descendant 继续被 drain，安静 handle 到期后由本调用关闭 reader。这个路径不 kill 正常退出留下的后台进程。相反，timeout 或 context cancellation 会 hard-kill 原进程组；即使 descendant 已把 stdout/stderr 重定向、导致 pipe 提前关闭，返回前的 context 检查仍会补做 group kill，避免它借此逃出取消。

测试先证明 package 缺少生产实现时构建失败；实现后按责任拆分覆盖：

- schema、严格 JSON、timeout 边界、explicit/default shell、workspace cwd、完整父环境、fresh shell、stdin EOF、空命令、stdout/stderr 与非零 exit；
- 预取消无副作用、运行中取消保留既有输出和自定义 cause、timeout 杀 child/grandchild、正常 exit 后 quiet background 存活、shell exit 后 active descendant 输出完整 drain；
- 跨 chunk UTF-8、无效尾部 replacement、2000 行/50 KiB tail、单个超长 UTF-8 行、阈值前 chunks 回填、raw temp file 完整一致和 footer；
- `bash-created file -> read` 与 `edit -> bash` 通过公开构造器验证同一 Workspace 的真实副作用可见性。

冻结 Pi 在 TypeScript tool 内把 timeout、abort 和非零 exit 组装成 thrown `Error` 文本；pi-go 保持已经确定的通用 Go Tool 契约，由 `Execute` 分别返回有界 output 与 idiomatic error，再由 Agent 统一形成 model-visible call-local error result。因此保留信息和循环继续语义一致，但错误文案不机械复制 TypeScript。

简化审查将阈值前 raw chunks 收敛为 `bytes.Buffer`，移除重复的 stream-error 状态，并避免为字符串 byte length 创建临时 `[]byte`；复用审查确认现有 `toolargs` 已用于真正共享的参数边界，而 shell resolution、进程组与 accumulator 没有其他生产消费者，不应提前上移。可靠性审查补上了已重定向 descendant 的取消路径和 threshold-crossing 多 chunk 测试；还锁定完整输出临时文件首次创建失败后不能重试生成缺失 chunks 的误导文件，footer 也不声称存在完整路径。修复后没有剩余 actionable finding。

最终 `make check` 完成 `go fmt ./...`、`go vet ./...`、`go test ./...` 与 `golangci-lint run ./...`，lint 为 0 issues；`make race` 全部通过，bash package 重复运行通过，Windows cross-build 也确认 portable unsupported 分支保持可构建。Windows 执行语义仍明确不属于第一期支持范围。第 05 课现在停在待理解确认，只有学习者明确要求后才 commit。
