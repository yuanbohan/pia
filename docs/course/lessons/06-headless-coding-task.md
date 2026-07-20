# 第 06 课：Headless one-shot Coding Task

## 当前状态

Lesson 06 的原始实现、真实验收和学习者理解确认均已完成并提交。2026-07-20 为后续 Pi 横向评测重新对齐 coding prompt；随后只做业务职责命名重构，prompt、workflow 与任务文字均未改变。两次调整都按 D23 重置旧 streak，当前二进制已由两个连续的 fresh DeepSeek 进程从 untouched baseline 独立完成同一 bug 修复，验收状态为 `2/2`。

后续修正（2026-07-20）：D59 已把 Pia 确定为产品名，只有当前 CLI contract 仍不稳定；D60 已允许 `read` 使用 workspace 外 absolute host path。下文“临时命令名”和“所有文件工具 workspace-only”保留为 Lesson 06 当时的历史契约，不再描述当前 Pia。

## 学习目标

本课把前五课已经独立验证的 AI 协议、Agent loop、DeepSeek Provider 和四个 coding tools 组合成第一个真实可运行的 coding agent。重点不是增加新的 Agent 能力，而是确定应用层与进程边界：谁拥有 system prompt、workspace、Provider profile、OS signal、输出和调试 trace。

完成后，操作者可以在一个目标项目目录中运行：

```bash
pia "Inspect this Go project, fix the bug, add meaningful tests, and verify the result."
```

命令把启动时的当前目录作为唯一 workspace，把唯一的位置参数作为原始 task，运行完整 model/tool loop，并在成功时只把最终 assistant 文本写到 stdout。

## 冻结 Pi 源码证据

本课继续使用 commit `dcfe36c79702ec240b146c45f167ab75ecddd205`：

- `packages/coding-agent/src/core/system-prompt.ts`：Pi 的 coding 层拥有 agent identity、tool guidance、工作目录和 project context；这些不是 OpenAI-compatible Provider request 的独立业务字段。
- `packages/coding-agent/src/core/resource-loader.ts`：Pi 会加载项目级指令并把它们放入 system prompt。pi-go 只采用根目录文件这一小部分语义，不移植全局配置、ancestor 搜索、扩展、skills 或动态资源刷新。
- `packages/coding-agent/src/modes/print-mode.ts`：Pi 的 print text mode 在结束时投影最终 assistant 文本。它提供 final-only 行为证据，但 Pi 的完整 CLI 仍有更多模式和事件基础设施，不能机械复制为 pi-go 的第一版需求。

## 已确定的 pi-go 设计

### 分层与所有权

- `internal/ai` 继续只拥有模型协议和 Provider boundary。
- `internal/agent` 继续拥有完整有序 transcript 与 model/tool loop，不加入 coding prompt、DeepSeek 配置或 CLI 输出。
- `internal/coding` 拥有 workspace、coding system prompt、四个 tools 和 one-shot application composition。
- `cmd/pia` 只拥有参数、父进程环境、OS signal、stdout/stderr、退出码和可选 trace 文件。

`pia` 是学习阶段的临时命令名，不建立稳定品牌、参数协议、公共 SDK 或配置兼容承诺。

### System prompt 与项目指令

system prompt 是每个 Agent 实例的稳定输入。为了给后续 Pi 横向评测保留可解释基线，它尽量沿用冻结 Pi 默认 prompt 的段落顺序和可复用措辞，包含：

- 只把 Pi coding-agent identity 中的产品名替换为 pia，并在 Pi 默认 body 的 append seam 加入 pia 已有的自主完成任务要求；
- 冻结 Pi 的四工具 snippets 和 tool-specific guidelines；真实工具 description/schema 仍通过 Provider tool schemas 独立发送；
- workspace 的 canonical path；
- 测试、验证、错误恢复和简洁最终回答的通用要求；
- workspace 根目录中第一个存在的 project instruction 文件。

project instruction 候选顺序固定为 `AGENTS.md`、`AGENTS.MD`、`CLAUDE.md`、`CLAUDE.MD`。只检查 workspace 根目录；第一个存在的候选若不是可安全打开的 UTF-8 regular file、超过 50 KiB 或读取失败，则直接报错，不静默回退到后续名字。选中内容使用 Pi 的 `<project_context>` / `<project_instructions path="...">` 结构放在 cwd 之前，并保留 Pi template 在文件内容之后无条件追加的 framing newline。该规则防止配置错误被隐藏，也避免为了尚未出现的多层配置需求提前实现 ancestor/global discovery。

第一版注册顺序仍为 `read`、`bash`、`edit`、`write`，与冻结 Pi 的默认 coding tool 顺序一致；只有 `read` 保持 parallel-safe。Prompt 为这四个已知工具单独保留冻结 Pi 的短 snippets 和使用 guidelines，真实 definitions 继续决定 Provider schemas、trace 和执行行为。完整 prompt 测试固定这条适配基线，避免横向评测前发生无意措辞漂移。

### 固定产品 profile

第一版不暴露模型或 Provider flags。`internal/coding` 固定创建 DeepSeek Provider，使用 `deepseek-v4-pro`、thinking mode 和 high reasoning effort；测试通过 package-private seam 注入 Faux Provider，不把 Provider 选择扩展成产品功能。凭据只从启动进程继承的 `DEEPSEEK_API_KEY` 读取，产品代码不会解析 `.zshrc`。

### 输出、错误与取消

- 成功：stdout 只包含最后一条 terminal assistant 的文本和结尾换行。
- 配置、prompt、Provider、Run 或 trace 写入失败：错误写到 stderr，退出码非零，不在 stdout 重复一个失败回答。
- `SIGINT`/`SIGTERM` 取消同一个 Run context；Agent 和 bash 已有的 settlement/进程组契约保持不变。
- 第一版没有自动 wall-clock 或 model-turn budget。出现真实无限循环证据前，不增加难以选择默认值的策略；操作者用 signal/cancellation 终止。
- 第一版没有 reasoning、tool call 或 bash output 的实时展示，也不为此新增 Agent event sink。未来 TUI 出现真实消费者时再设计事件和 presentation contract。

### 安全与数据边界

文件工具仍通过 `os.Root` 限制 workspace-relative regular-file 操作；bash 完整继承 `pia` 的父进程环境，并不受该文件边界约束。Provider 生成的命令以启动用户的主机和网络权限执行，能够读取 workspace 外文件、访问外部服务、读取已导出的凭据并产生不可逆副作用。

第一版没有 sandbox、逐工具 approval、secret detector、通用 redactor 或启动警告。操作者必须主动选择可以发送给 DeepSeek 的 workspace，并信任模型在当前用户权限下执行命令。该披露属于文档契约，不伪装成运行时隔离。

### 可选调试 trace

设置 `PIA_TRACE_PATH` 时，CLI 在 Run 完全结束后创建一个新的 `0600` JSON 文件；相对路径从启动目录解析，不创建父目录，也不覆盖任何既有路径。trace 写入失败使命令失败，避免操作者误以为已经得到诊断证据。

trace 保存实际 model profile、system prompt、task、tool schemas、完整 transcript 和顶层错误，但不保存 API key。因为 transcript 和 tool results 本身可能包含源码、命令输出、环境值和其他敏感信息，该变量只用于本地调试，schema 也不承诺稳定。

## 与 Pi 的主要差别

- 采用 Go 的消费方接口和 `context.Context`，不复制 TypeScript class/event API。
- 只实现 DeepSeek-first one-shot composition，不移植 Pi 的 provider/model registry、interactive/TUI mode、extensions、skills、global settings 或 session system。
- 只加载 workspace 根目录的一个 instruction 文件；不实现 ancestor/global discovery。
- 省略 Pi 自身文档路径和 custom-tools 声明，并在冻结 Pi 默认 body 之后、project context 之前的 append seam 加入 pia 已有的自主执行、保护无关改动、错误恢复与验证要求；identity 除 `pi` 改为 `pia` 外不再改写。
- print behavior 对齐 final-only 结果，但第一版完全不建立实时 event presentation。
- 文件工具使用更强的 `os.Root` regular-file boundary；bash 则有意保留冻结 Pi 的完整环境和 unsandboxed 主机权限。

## 实现与测试范围

本课计划增加：

- `internal/coding/prompt.go` 与 prompt tests；
- `internal/coding/runtime.go`、显式 trace DTO 与 Faux-driven composition tests；
- `cmd/pia` 的 process host、trace writer 和离线 tests；
- README、课程索引、决策记录和本课记录。

离线测试覆盖指令文件优先级与拒绝条件、稳定 prompt、完整 request context、真实四工具 schema、final projection、Provider/error/cancellation settlement、CLI 参数/输出/退出码，以及 trace 的 create-new、权限和敏感结构边界。默认测试不读取 API key、不联网。

## 实现记录

- `internal/coding/prompt.go` 按实际注册工具顺序选择冻结 Pi 的四个 prompt snippets/guidelines，未知工具才回退到其真实 schema description；Provider request 与 trace 始终使用真实 definitions。冻结 Pi identity 到全局 guidelines 的默认 body 保持连续，pia-only guidance 统一在对应 `appendSystemPrompt` seam 追加。它只在 pinned `os.Root` 根目录读取一个 project instruction 文件，并无条件写入 Pi template 的 content framing newline。实现先枚举根目录的实际 entry spelling，再按候选顺序 `Lstat` 和打开文件；这个额外步骤来自测试证据：macOS 的大小写不敏感文件系统会让一次 `Lstat("CLAUDE.md")` 命中实际名为 `CLAUDE.MD` 的 entry，若不区分实际拼写就无法兑现跨平台一致的优先级。内部 symlink 可以指向 workspace 中的 regular file，逃逸、悬空、目录、FIFO、不可读、非法 UTF-8 和超过 50 KiB 的高优先级候选都失败且不 fallback。
- `internal/coding/runtime.go` 创建一个 Workspace 和 `read → bash → edit → write` 工具列表，构建一次 stable prompt，再创建已有 `agent.Agent`。固定产品 helper 选择 `deepseek-v4-pro` 与 high reasoning effort；DeepSeek profile 继续负责 thinking wire semantics。package-private Provider/workspace seam 只服务 Faux 与 lifecycle tests，没有把 Provider 选择暴露成产品配置。
- `RunResult` 保存 canonical workspace、prompt、无凭据 model/tool context 和 Agent 已深复制的完整 transcript；Run error 仍通过独立 Go error 返回。`FinalText` 只拼接最后一条 assistant 中的 text blocks，不复制 reasoning、tool results 或失败终态到 stdout。
- `internal/coding/trace.go` 把 interface-backed transcript 转为显式 role/content-type DTO。tool-call arguments 使用 string，因此 error transcript 中的 malformed JSON 仍能进入一个合法 trace；配置 key 根本不属于输入 DTO，但 bash 主动输出的 secret 会作为普通 tool result 保留。
- `cmd/pia` 只接受一个非空 raw task，读取继承的 `DEEPSEEK_API_KEY` 和启动 cwd，用 `signal.NotifyContext` 处理 `SIGINT`/`SIGTERM`，并在所有 requested trace 成功后才输出 final。Go 1.26 会把具体 signal cause 保留为类似 `interrupt signal received` 的错误；测试只要求非零终态和取消证据，不固化平台退出码。
- trace writer 先在内存编码，再用 `O_WRONLY|O_CREATE|O_EXCL` 和 `0600` 原子 create-new。它不先 `Lstat`、不创建 parent、不跟随已有 symlink，也不会打开已有 FIFO；只有本调用已经成功创建文件后发生 write/close failure，才删除 partial path。该逻辑属于 process-host 的本地诊断策略，不复用 workspace 内“替换已有 regular file”的 `fileutil`。

离线 multi-turn composition 已经用真实四工具依次执行 read、edit、write 和 bash，证明每次 Provider request 的 prompt/schema 稳定、transcript 递增、工具副作用互相可见，且 root instruction 被工具修改后不会改变本 Run 的 prompt snapshot。Provider error、取消和 prompt construction failure 都在 workspace 关闭前完成 settlement；close failure 与 primary cause 合并保留。

## 真实验收

仓库只忽略 `tmp/`，不提交 fixture、生成器、隐藏测试或 harness。本地 `tmp/pia-acceptance/baseline` 是一个能构建但 Fibonacci 实现错误、且没有测试的最小 Go 程序；每次验收都从未修改 baseline 复制一个全新 workspace，启动一个新的 `pia` 进程，并要求 Agent 修复公开函数、补充有意义的测试、运行测试和程序。

本轮固定任务文字是：

> Fix Fibonacci in this Go project for non-negative inputs so F(0)=0 and F(1)=1, while preserving the public function signature. Add meaningful automated tests that would catch the current bug. Run the tests and executable, and leave the workspace in a passing state.

它说明公开语义、签名、测试和验证责任，但不列出具体测试 case，也不要求某个特定工具。baseline 的 `go test ./...` 因没有测试而正常返回，`go run .` 输出错误值 `89`；验收因此同时检查程序输出和 test discrimination，不能把“测试命令退出零”误当作初始实现正确。

每次运行后由课程导师独立检查：

1. `go test ./...` 通过；
2. `go run .` 输出 `55`；
3. Agent 新增的测试复制回原始错误实现后至少一项失败，证明测试不是空壳；
4. 最终 diff、trace 和 assistant 回答与任务一致，没有 fixture-specific 产品代码。

只有两个从同一 untouched baseline 开始、使用不同新进程的连续真实 DeepSeek 运行都通过，才完成本课验收。任何产品代码、system prompt 或 task wording 调整都会把连续成功计数清零。

### 2026-07-19 验收结果

最后一次产品代码、system prompt 和固定任务文字冻结后，验收二进制重新构建，并从同一个 untouched baseline 连续启动两个新的 `pia` 进程；两次之间没有修改产品代码、prompt 或任务：

1. `tmp/pia-acceptance/attempts/001/workspace` 把 base case 修为 `n <= 1` 时返回 `n`，新增覆盖 `F(0)` 至 `F(10)` 和 `F(20)` 的 12 个 table cases；独立 `go test ./...` 通过，`go run .` 输出 `55`。
2. `tmp/pia-acceptance/attempts/002/workspace` 把 base case 修为 `n < 2` 时返回 `n`，新增覆盖 `F(0)` 至 `F(10)` 的 table test；独立 `go test ./...` 通过，`go run .` 输出 `55`。

两次新增测试分别复制到对应的 `original-with-tests` 原始错误实现后都因 Fibonacci 结果错误而失败，证明测试能够识别原 bug。`tmp/pia-acceptance/evidence/001.json` 和 `002.json` 都记录了 `deepseek-v4-pro`、thinking、high reasoning effort、完整四工具 schema、正常 `stop` 终态和空 Run error；两次运行碰巧都使用了 `read`、`bash`、`edit`、`write`，但工具序列不是验收条件。所有 baseline、workspace、测试辨别副本和 trace 继续留在被忽略的 `tmp/`，没有进入 tracked tree。

### 2026-07-20 Pi prompt 对齐回归

第一次“冻结 Pi 基线加窄适配”在 `attempts/004` 中完成过一次回归，但后续逐段 review 发现 identity 增加了不必要的临时产品描述、pia-only guidance 打断了 Pi 的 identity-to-tools 顺序，而且 newline-terminated project instructions 少保留一个 Pi template framing newline；因此 `004` 只保留为中间证据，streak 再次清零。

最终 prompt 只把 identity 的 `pi` 替换为 `pia`，保持 Pi 默认 identity、tool list 和 guidelines body 连续，把 pia-only guidance 放在对应 `appendSystemPrompt` seam，并对 instruction content 无条件追加 template newline。重新构建同一个二进制后，在没有任何产品代码、prompt 或任务文字变化的情况下连续运行：

1. `tmp/pia-acceptance/attempts/005/workspace` 把 base case 修为 `n < 2` 时返回 `n`，增加覆盖基础值、`F(2)` 至 `F(10)` 和 recurrence 的 table test；独立 `go test ./...` 通过，`go run .` 输出 `55`。
2. `tmp/pia-acceptance/attempts/006/workspace` 把 base case 修为 `n <= 1` 时返回 `n`，增加覆盖 `F(0)` 至 `F(10)` 的 table test；独立 `go test ./...` 通过，`go run .` 输出 `55`。

两次新增测试分别复制到对应 `original-with-tests` 后都在原始错误实现上失败。`evidence/005.json` 与 `006.json` 均确认 `deepseek-v4-pro`、thinking、high reasoning effort、真实 `read` / `bash` / `edit` / `write` schemas、最终 `stop`、空 Run error，以及最终 identity、append-block 顺序和 unsupported-section omission。最终 prompt 的连续验收状态为 `2/2`。

### 2026-07-20 业务职责命名回归

后续代码审查明确了新的命名边界：文档、注释和断言可以在来源说明中提到 Pi，但 Go 标识符、测试函数和 subtest 必须按业务行为或契约职责命名。此次重构没有改变 prompt 文本、one-shot workflow 或任务文字；完整字符串测试继续固定同一 prompt。由于 D23 对任何产品代码调整都会清零，`005`、`006` 只保留为 prompt 对齐的历史证据。

当前二进制随后连续运行 fresh `attempts/007`、`008`。两次都把 base case 修为 `n < 2` 时返回 `n`，增加覆盖 `F(0)` 至 `F(10)` 的 table test，独立 `go test ./...` 通过且 `go run .` 输出 `55`；把各自新增测试复制到对应 `original-with-tests` 后，原始错误实现均失败。`evidence/007.json` 与 `008.json` 确认固定 DeepSeek profile、四工具 schema、最终 `stop` 和空 Run error；规范化 workspace 路径后，两次 system prompt 与 `006` 一致。当前连续验收状态重新达到 `2/2`。

## 明确不做

- TUI、interactive REPL、实时 reasoning/tool/bash 展示；
- Session 持久化、Goal Runtime、Agent Manager、公共 SDK 或 RPC；
- runtime model/provider/config matrix；
- 自动执行预算、sandbox、approval 或 rollback；
- 仓库内 benchmark/eval harness；
- 稳定 trace schema 或 JSON output mode。
