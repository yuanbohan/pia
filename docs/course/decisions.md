# 课程与架构决策

这个文件记录跨课程有效的当前决定。新证据推翻旧决定时，直接更新对应决定，并在“变更记录”说明原因；Git 历史保存旧版本。

## 当前决定

### D1. 做语义移植，不做逐文件翻译

- 日期：2026-07-15
- 决定：Go 实现对齐 Pi 的可观察行为与设计约束，允许采用 Go 原生接口、goroutine、channel 和 `context.Context`。
- 原因：TypeScript 的类、Promise 和异步迭代器不是产品契约；机械复制会掩盖 Go 的并发与取消语义。

### D2. 第一期只实现模型与工具循环

- 日期：2026-07-15
- 决定：`internal/agent` 负责通用 Provider turn、transcript 和 tool loop；第一期不实现结构化 plan/progress/replan/done/blocked Goal Runtime。模型停止调用工具时只表示 loop 正常结束，coding task 是否成功由固定外部验收独立判断。
- 原因：当前目标是证明最小 coding loop 能完成真实任务。提前增加 Goal 状态机会把 Agent Loop 能力和上层目标策略混在一起。

### D3. DeepSeek-first，但 Provider 边界可替换

- 日期：2026-07-15
- 决定：第一个真实 Provider 只实现 DeepSeek；模型 ID、base URL 和密钥来自运行配置。非本地 base URL 默认必须使用 HTTPS，测试服务器需要显式开发覆盖。实现 Provider 课程时重新核对官方模型和协议，不把当前模型别名写成课程契约。
- 原因：先减少兼容矩阵，同时保留未来扩展其他 Provider 的边界。

### D4. Faux Provider 是 Agent Loop 的首要验证设施

- 日期：2026-07-15
- 决定：先完成可脚本化的 Faux Provider，再接真实模型。
- 原因：事件顺序、工具错误、取消和 transcript 必须能确定性复现，不能依赖付费 API 或模型随机性。

### D5. 当前交付 headless Agent Runtime，不做 SDK-first

- 日期：2026-07-15
- 决定：入口位于 `cmd/pi-go`，实现包位于 `internal/`；该命令只用于本地运行和验收，当前不向其他项目承诺调用协议。公共 Go SDK、gRPC、其他网络 RPC 和 IM 适配均推迟设计。
- 原因：核心 API 尚处于学习和校正阶段，过早公开会固化错误抽象。出现真实外部调用方后，再根据同进程嵌入或跨进程服务选择接口。

### D6. 不移植 TUI

- 日期：2026-07-15
- 决定：不实现主题、按键、命令提示、交互式选择器和 slash command UI。
- 原因：它们不决定 coding loop 是否能完成任务，会稀释当前课程对核心语义的关注。

### D7. 第一期是单目录、单 active Run，不建立持久化 Session

- 日期：2026-07-16
- 决定：`cmd/pi-go` 的第一期验收仍只接收一个 workspace 和一条初始 task prompt，但核心 `Agent` 拥有当前进程内的完整有序 transcript，并可按顺序接收后续 user input；同一时刻只允许一个 active Run。Session 创建、持久化、恢复、分支、多用户并发和 Agent Manager 延后。
- 原因：内存 transcript 是 Agent 保持目标和工具上下文的核心执行状态，不是可删减的 Session 附加能力；推迟的是持久化与管理层，而不是正确的消息所有权。

### D8. 第一期没有审批或 trust/yolo 策略系统

- 日期：2026-07-15
- 决定：模型选择的工具直接执行，不逐次请求批准，也不提供 trust/yolo 配置矩阵。`read`、`write`、`edit` 强制 workspace 文件边界；bash 只固定初始 cwd，不是 sandbox，可以访问当前用户有权访问的 workspace 外资源。bash 使用最小 allowlist 环境加显式非敏感配置；Provider 凭据不能进入子进程、argv、tool config、transcript、事件、日志或错误。参数校验、超时、取消和输出截断始终生效。
- 原因：逐工具审批会阻断长任务；第一期也没有足够场景证明需要策略抽象。不审批不等于隐藏权限或取消安全不变量。

### D9. 课程与代码按同一个提交节奏推进

- 日期：2026-07-15
- 决定：每课维护讲义、代码、测试和讨论记录；只有学习者明确说“开始第 NN 课”后才进入学习状态，理解确认前不进入下一课，只有学习者明确要求时才 commit。第 00 课是只建立 module、文档和验证证据的无 Runtime 代码例外。
- 原因：仓库要能还原完整理解过程，而不只是最终代码；占位 package 不能替代已经理解的行为实现。

### D10. 冻结上游参考基线

- 日期：2026-07-15
- 决定：第一轮课程固定参考 Pi commit `dcfe36c79702ec240b146c45f167ab75ecddd205`、agent package version `0.80.7`。
- 原因：固定源码坐标后，课程中的行为结论才能追溯到同一份实现；上游变化需要显式重新阅读和决策。

### D11. Provider stream 使用 pull boundary；Agent event sink 延后设计

- 日期：2026-07-16
- 决定：Provider 对 Agent 暴露可取消的 pull/receive 流抽象。第 02 课不设计或实现 Agent event sink；等本地 headless 入口真正需要展示运行过程时，再根据终端消费者核对事件形状、串行投递、错误和 settlement 语义。
- 原因：pull boundary 直接参与模型调用、结束、错误和取消，属于基础执行数据流；event sink 服务运行观察和展示，当前 transcript loop 没有真实消费者，不应为了未来 TUI 提前固化接口。

### D12. 工具 schema 与 Go 参数校验分离

- 日期：2026-07-15
- 决定：Tool 向模型提供 JSON Schema，同时拥有 Go 侧参数解码和校验逻辑；初始版本不实现通用 JSON Schema 执行器。
- 原因：模型提示 schema 和运行时安全校验是两个责任。四个核心工具可以用强类型输入完成校验，不需要提前引入完整 schema framework。

### D13. 第一期只保留内存 transcript

- 日期：2026-07-16
- 决定：Agent 的第一次 Run 把原始 task 转换成 transcript 首条 user message；同一 Agent 后续每次 Run 都把新的 user input 追加到已有 transcript。每次 Provider 调用收到包含 workspace/cwd 说明的稳定 system prompt、截至当前的完整有序 transcript 和当前 tool schemas，不增加独立 `Task` 或 `WorkspaceContext` AI 协议字段。第一期不持久化 transcript，不实现 checkpoint、恢复、自动 compaction、摘要或跨 Session 上下文。
- 原因：基础多轮上下文已经足以验证 coding loop；持久化和压缩会引入版本、恢复和敏感数据存储契约，必须由二期真实需求驱动。

### D14. 第一轮 bash 进程树支持 macOS 和 Linux

- 日期：2026-07-15
- 决定：初始 bash executor 在 macOS 和 Linux 上实现进程组取消与测试；Windows 运行时支持在核心课程后单独设计。
- 原因：进程树控制是操作系统相关行为。当前开发与验收环境是 macOS，提前声称未经验证的跨平台等价会隐藏取消缺陷。

### D15. Agent Runtime 与 Agent Manager 分层

- 日期：2026-07-16
- 决定：第一期只构建单目录、单 active Run 的本地 Runtime；同一 Agent 可以按顺序接收 user input 并保留完整内存 transcript。后续 Agent Manager 才负责用户、仓库目录、持久化 Session、多 Agent/Run 并发、worktree、GitHub 和 IM 路由；这些职责不能进入通用 Agent Loop。
- 原因：headless 只描述无 UI 的运行形态，不等于当前已经提供外部服务或多租户调度。

### D16. 学习过程以证据驱动，不以对话共识驱动

- 日期：2026-07-15
- 决定：导师持续质疑学习者的设计假设，同时复查自己的既有判断。讨论必须区分学习者假设、已验证的 Pi 契约、候选 Go 机制和已确定的 pi-go 决策；冻结 Pi 的源码、文档、测试和可复现实验优先于对话中的赞同。每个新概念必须先完成术语、源码路径和例子讲解，再进行理解检查。
- 原因：未经挑战的共识会把遗漏固化进 Go 实现，降低长期可靠性。

### D17. 课程语义使用 RunStart 和 RunEnd

- 日期：2026-07-15
- 决定：课程和 Runtime Go 命名使用 `run_start/run_end` 描述一次 Run 的边界；引用 Pi 源码和原始事件轨迹时保留 `agent_start/agent_end`。这不预先确定未来 gRPC 或 SDK 的公开命名。
- 原因：结束的是一次 task Run，不是长期存在的 Agent 实例；引用源码时也不能篡改上游事件名。

### D18. 冻结基线不进入 Runtime package

- 日期：2026-07-15
- 决定：Pi commit 和 npm package version 只记录在课程文档，不生成 `internal/contract` 或其他课程专用 Runtime package。
- 原因：冻结上游是源码学习责任，不是 Agent Runtime 的产品能力。

### D19. 工具调用使用屏障式分段调度

- 日期：2026-07-15
- 决定：按模型源顺序线性扫描 tool calls；连续且显式 `CanRunParallel` 的调用组成一个并行阶段，其他调用各自形成串行屏障。能力默认 false，第一期只有 `read` 为 true，`write`、`edit`、`bash` 和未知工具均为 false。事件可按真实完成顺序出现，写入 transcript 的 tool results 必须恢复模型源顺序。
- 原因：保留无副作用 read 的简单并行能力，同时避免把整个混合批次并行造成读写和命令竞态。该策略是 pi-go 对 Pi 整批调度的有意差异。

### D20. 普通工具错误形成结果并继续

- 日期：2026-07-15
- 决定：未知工具、非法参数、执行错误、单工具 timeout 和非零 bash exit 都形成 call-local error tool result；同批后续阶段继续执行，下一轮模型收到全部有序结果。单工具 timeout 只取消自己的 child context；只有 Run context 取消才停止未开始工作。
- 原因：调度器不知道模型调用之间的语义依赖，不能通过一个局部失败替模型猜测是否跳过后续调用；把错误交回模型更利于恢复。

### D21. Run 取消必须等待所有已启动工作收敛

- 日期：2026-07-16；2026-07-17 修正未启动调用和 Provider-turn tool call 的 settlement 规则
- 决定：Run 取消停止新的 Provider 调用和工具副作用，取消所有已启动 child contexts，等待 stream、tool workers 和 bash process group 收敛，然后返回明确的 canceled 非成功结果。工具阶段已经完成的调用保留实际结果，执行中调用记录 canceled，未启动调用不执行但记录明确的 not-executed settlement result；若 Provider aborted terminal 自身保留了已经组装完成、尚未进入工具阶段的 calls，这些调用全部不执行并追加同 ID not-executed results。未来引入 event sink 时，再把观察通道的 settlement 纳入该取消契约。
- 原因：取消请求不是完成信号。过早返回会遗留 goroutine、进程或晚到事件，并允许新旧副作用重叠；漏掉未启动调用的 settlement result 又会让持久 transcript 出现 orphaned tool calls，阻断同一 Agent 后续 Run。

### D22. 文件工具使用 os.Root；bash 不宣称 workspace containment

- 日期：2026-07-15
- 决定：Run 开始时用 `os.Root` 打开固定 workspace；`read`、`write`、`edit` 只执行 root-relative I/O 并拒绝非 regular-file 目标，不使用“先检查绝对路径再普通 open”的易竞态模式。bash 只从 workspace 启动，仍可访问当前用户资源。
- 原因：文件工具需要抵抗路径穿越和 symlink TOCTOU；cwd 不是命令 sandbox，不能把 fixture diff 或 canary 误写成主机级隔离。

### D23. 最终验收使用固定 bug-fix fixture，不在仓库内实现 Pi 比较

- 日期：2026-07-15
- 决定：最终验收使用 checked-in prompt 和一个初始测试失败的固定小型 Go fixture。Agent 必须自主读取、修改并运行测试；独立 harness 检查测试成功、不可修改文件哈希、生产文件 allowlist、相邻 canary 和遗留进程。Pi 与 pi-go 的效果比较由学习者在仓库外手动执行，不创建 benchmark、eval package、进程比较协议或评分工具。
- 原因：固定 fixture 同时提供可重复的 Runtime 验收和人工对比材料，而不会把一次主观效果比较污染成产品代码。

### D24. DeepSeek 数据外发必须明确，默认事件必须有界

- 日期：2026-07-15
- 决定：运行前明确提示 system prompt、task、模型选择的文件内容、命令和 tool output 会发送给 DeepSeek；操作者自行选择可披露 workspace，Runtime 不增加确认或审批交互。read/bash 内容在进入 transcript、Provider request 或 event preview 前完成大小限制；默认事件只呈现 metadata、状态和有界 preview。第一期没有通用 secret detector 或 redactor。
- 原因：操作者必须理解真实数据边界；减少重复输出能降低额外泄漏，但不能被描述为阻止模型看到完成任务所需的 tool results。

### D25. 第一阶段 AssistantMessage 只保留 Loop 可观察字段

- 日期：2026-07-16
- 决定：`AssistantMessage` 第一阶段只保留有序 content blocks、token usage、stop reason 和 error message。暂不加入 api/provider/model、response ID/model、diagnostics、timestamp、cost 或未核验的 cache/reasoning usage 细分；stream delta 不重复携带完整 partial message，terminal event 携带权威 final/aborted message。
- 原因：当前 Agent Loop 只观察内容、用量和终态；其余字段属于 Pi 的多 Provider、路由、Session/UI、诊断或费用生态。第 02 课核对后没有发现 pi-go 第一阶段需要完整 partial snapshot 的消费者；第 04 课若核对 DeepSeek 后出现新的 usage/trace 需求，再用证据扩展内部协议。

### D26. Partial assistant 不属于正式 transcript

- 日期：2026-07-16
- 决定：第一阶段正式 transcript 对每个 Provider turn 只追加一条 terminal final/aborted assistant message，不保存、持久化或暴露可查询的完整 partial assistant view。第 03 课增加的 tool-result settlement 不属于 partial 或第二条 assistant；error/aborted terminal 中已完成但未执行的 tool calls 会在该 assistant 后得到同 ID not-executed results。如何把 formation events 提供给本地终端、未来 TUI 或日志，等课程处理真实展示过程时再讨论，不在第 02 课建立 event sink。
- 原因：冻结 Pi 的低层 loop 会把 partial 临时放进私有 working context，但高层 `Agent.state.messages` 只在 `message_end` 追加 terminal message，并用独立 `streamingMessage` 服务 UI。pi-go 第一期是 headless Runtime，没有必须查询完整 partial snapshot 的消费者；把权威 transcript 与临时展示状态分开可以避免半成品 text、thinking 或 tool-call 参数成为下一轮上下文事实。

### D27. RunResult 携带数据，Go error 表达调用终态

- 日期：2026-07-16
- 决定：第 02 课使用 `Agent.Run(ctx, userInput) (RunResult, error)`；`RunResult` 第一阶段携带 Agent 当前完整 transcript 的快照，不增加 `Outcome` 或内嵌 `Error` 字段。正常完成返回 nil error；Provider/stream failure 返回非 nil error；取消返回可用 `errors.Is` 识别的 context cause。所有路径都尽可能把 terminal 或合成的 final/error/aborted assistant message 追加到 Agent transcript 后再返回。
- 原因：`AssistantMessage.StopReason` 已保存模型响应事实，Go error 负责调用者控制流；再增加 Run outcome 会制造第三份可能漂移的终态分类。失败时同时返回 result 和 error 可以保留 transcript 现场，又符合 Go 调用方对错误与取消的处理习惯。

### D28. Agent 保留同一对话的全部消息

- 日期：2026-07-16
- 决定：只要仍是同一个 Agent 对话，每条新 user message、每条 terminal assistant message 和每条 tool result 都按顺序追加，任何后续 Provider call 都收到完整 transcript；收到新 user message 不会清空旧消息。开始新对话必须显式创建新 Agent 或使用未来明确设计的 reset/session boundary。Agent 拒绝并发 Run，避免共享 transcript 发生交错写入。
- 原因：原始目标、模型已采取的动作和工具观察共同构成后续推理上下文；丢弃前文会让复杂任务失去目标连续性。冻结 Pi 也由高层 `Agent._state.messages` 跨 prompt 保存历史，低层 loop 再把新 prompt、assistant 和 tool results 追加到完整 context。

### D29. Transcript snapshot 必须深复制

- 日期：2026-07-16
- 决定：Agent 在接收 Provider terminal message、创建 Provider Request 和返回 `RunResult.Transcript` 这三个所有权边界深复制 AI message；调用方或 Provider 不能通过嵌套 `AssistantMessage.Content`、`ToolCall.Arguments` 或 tool schema JSON 反向修改 Agent 的权威 transcript。复制规则由拥有消息结构的 `internal/ai` 统一提供，Faux 与 Agent 不分别维护重复实现。
- 原因：只复制 `[]ai.Message` 外层仍会共享嵌套 slice 和 `json.RawMessage` backing array，不能兑现 snapshot 语义，还可能在并发观察时形成 data race。边界深复制让 ownership 明确；其成本相对完整模型请求的遍历、序列化和网络调用可忽略。

### D30. 预取消发生在 Run acceptance point 之前

- 日期：2026-07-16
- 决定：`Agent.Run(ctx, userInput)` 在同一个锁内先拒绝已有 active Run，再检查 `context.Cause(ctx)`；若 context 此时已经取消，Run 未被接受，不追加 user 或 aborted assistant，不调用 Provider，并返回当前稳定 transcript snapshot 与 context cause。若检查通过，设置 active 并追加 user 构成 Run acceptance point；此后到 terminal settlement 之前发生的取消属于已开始的 Run，保留 user，并最终只追加一条 aborted assistant。若该 assistant 含取消前已完成组装的 tool calls，可以在其后追加 non-executed settlement results；assistant exactly-once 不等于 transcript 只能再增加一条消息。
- 原因：Pi 高层 `Agent.prompt()` 不接收外部 signal，而是在 prompt 被接受后创建内部 `AbortController`，所以没有完全对应的预取消调用；Pi 低层则会在 Provider 观察 signal 前把 prompt 加入 context。pi-go 需要同时遵守 Go 的预取消 context 不启动副作用惯例，以及 Pi 对已接受 prompt 保留 user 与 aborted assistant 的行为。

### D31. 每个 Provider turn 只在一个位置追加 terminal assistant

- 日期：2026-07-16
- 决定：每次 Provider call 的 stream consumer 在正常 terminal、Provider error/aborted terminal、terminal 前 EOF 和非 EOF receive error 的所有路径上只计算一对 `(terminal AssistantMessage, error)`，不直接修改 Agent；Turn/Run coordinator 在该次调用的唯一位置深复制并追加该 assistant。第一条有效 `DoneEvent` 或 `ErrorEvent` 构成该 Provider turn 的 terminal settlement point，之后发生的取消不追溯修改这条已经完成的 assistant；若同一次 `Receive()` 同时返回有效 terminal event 与 error，terminal event 优先。没有有效 terminal 时，raw stream failure 在 context 已取消时合成 aborted，否则合成 error；nil stream 作为协议错误合成 error assistant。第 02 课一个 Run 只有一个 Turn，所以暂时表现为每个 Run 追加一次；第 03 课一个 Run 可有多个 Turn，每个 Turn 各追加一次 terminal assistant。assistant 的唯一写入与其后追加 tool-result settlement 不冲突。
- 原因：把 transcript 写入分散在 terminal、error、cancel 和 deferred cleanup 分支会产生缺失或重复 assistant。Provider stream 已绑定 context，Agent 等待 `Receive()` 真正收敛，不用额外 goroutine 与 `ctx.Done()` 竞争后提前返回，以免遗留仍在读取网络的工作。

### D32. Agent Runtime 不自动重试工具调用

- 日期：2026-07-17
- 决定：Agent 对每个 tool call 只调用一次对应 Tool；未知工具、参数错误、执行错误和 timeout 形成 tool result，由模型决定是否在下一 Turn 发起新调用。第一阶段本地文件工具不重试。未来若远程 I/O 后端能够明确识别瞬时错误，可在单次 `Execute` 内实现有界、遵守 context 的内部重试，但不得由通用 Agent Loop 按工具名或错误字符串猜测并重放调用。
- 原因：冻结 Pi 的 Agent Loop 和本地 read 都不自动重试工具；稳定错误重试没有价值，通用重试还会给 write/edit/bash 带来重复副作用。具体后端才拥有错误分类和安全重试所需的信息。

### D33. 取消时在 Agent transcript 中闭合全部 tool calls

- 日期：2026-07-17
- 决定：当 Run 在工具阶段取消时，Agent 等待已启动 worker 收敛，并为 terminal assistant 中每个 tool call 追加且只追加一个同 ID settlement result：completed 使用实际结果，执行中取消标为 canceled，尚未启动标为因 Run 取消而 not executed。若 Run 在 Provider stream 阶段取消，或 Provider 以 error 结束，error/aborted assistant 中已完成的 tool calls 全部不执行，但分别追加同 ID not-executed result。所有结果按模型源顺序写入 transcript，随后直接返回 Run/Turn error，不再调用 Provider。未启动 result 只陈述没有执行，不代表发生过工具副作用。
- Pi 分歧：冻结 Pi 的 Agent Loop 可能把未执行调用留成 orphaned tool calls，再由 `packages/ai/src/api/transform-messages.ts` 在下一次 Provider request 中临时插入 `No result provided`，不写回 Agent state。pi-go 不复制这层隐藏修复，而是在权威 transcript 中显式闭合调用。
- 原因：pi-go 已确定同一 Agent 跨顺序 Run 保存完整 transcript，并把它作为 Provider history 的单一事实源。显式 settlement 让 `RunResult`、Agent state 和下一次 Provider request 保持一致，也让取消后的对话可以直接继续。离线 Faux 测试和真实 DeepSeek compatibility smoke 都要专门验证这一分歧。

### D34. Tool-call terminal 协议按 call identity 和可执行性分层

- 日期：2026-07-17
- 决定：`stop`、`length`、`toolUse` 是合法 Done reason；content 中存在完整 calls 才进入后续分支，不能只看 reason。`stop + calls` 可执行，`length + calls` 全部生成 truncation error results 而不执行，`toolUse + no calls` 是 Provider protocol error。空或重复 tool-call ID 会让原 terminal 被 synthetic error assistant 替代；ID 有效时，空 tool name、未知工具、非法 JSON、参数语义错误和 Tool execution error 都是 call-local error result。Provider error/aborted terminal 中的 calls 不执行，只生成 not-executed settlement results。
- 原因：call ID 决定 transcript 是否能可靠配对，属于 Provider 协议完整性；工具名、arguments 和执行失败属于单次调用可由模型观察和恢复的错误。stop reason 可能与 content 不完全一致，content 的完整结构才是能否执行或闭合的直接事实。

### D35. DeepSeek 使用薄 Provider 层复用 OpenAI-compatible Chat Completions 协议

- 日期：2026-07-17
- 决定：具体 Provider 实现统一归类到 `internal/ai/provider/`；其中完整包名 `provider/openaicompatible` 负责当前 DeepSeek 所需的 Chat Completions request/message 编码、HTTP/SSE、reasoning、tool-call 分片、usage 和 finish-reason 映射，`provider/deepseek` 只负责 DeepSeek 配置、认证、endpoint 安全约束和兼容 profile，`provider/faux` 保存确定性实现。consumer-owned `ai.Provider` 接口仍与 `Request`、`Stream` 一起留在模型无关 `internal/ai`，Agent 只依赖该端口。第一期不移植 Pi 的 Provider registry、模型目录、动态刷新、凭据存储、lazy module 或多 API dispatch，也不宣称兼容所有 OpenAI-compatible 服务。线协议层使用标准库 `net/http` 和可注入 client/transport，不引入 OpenAI SDK。
- Pi 证据：冻结 Pi 的 `deepseekProvider()` 只向 `createProvider()` 注册 DeepSeek 的 base URL、认证、模型元数据与共享 `openAICompletionsApi()`；消息转换和流解析位于 `packages/ai/src/api/openai-completions.ts`。
- 原因：DeepSeek 当前使用 OpenAI-compatible Chat Completions 线协议，把协议转换全部写进 DeepSeek 包会混合可复用 wire semantics 与厂商配置；用目录归类具体实现能让依赖方向可见。把小接口移进 `provider` 子包会迫使 Agent 为同一个 AI 协议同时依赖两个包，而复制 Pi 的完整 Provider 生态则超出单 Provider 第一期范围。

### D36. OpenAI-compatible stream 由 pull parser 显式完成 settlement

- 日期：2026-07-17
- 决定：`Provider.Stream()` 只创建绑定 context 的 stream 状态，首次 `Receive()` 启动 HTTP 调用，后续 `Receive()` 按需解析 SSE，不启动后台 goroutine，也不实现 Provider-level retry。第一阶段不增加 3xx 特殊逻辑，完全服从注入 `http.Client` 的标准 redirect policy；redirect 对请求体、凭据传播和 loop 的实际影响留待出现证据后单独验证。成功 terminal 同时要求有效 `finish_reason` 和 `[DONE]`；usage chunk 可在 finish reason 后到达，因此只在 `[DONE]` 后产生 `DoneEvent`。tool calls 按 wire `index` 聚合；最终 arguments 即使不是合法 JSON，也作为 raw call 交给 Agent 形成 call-local error，只有无法可靠配对的 index/ID/name 才是 Provider protocol error。
- 取消语义：finish reason 前取消只保留已形成的 text/reasoning，不保留没有 wire completion 证据的 tool call；finish reason 后、`[DONE]` 前取消可以保留完整 calls，Agent 再按既有规则追加 not-executed settlements。HTTP 错误体最多读取 64 KiB，单个 SSE event 最多 1 MiB；API key、headers 和完整 request 不进入错误。第一阶段 `Usage` 继续只保存 input/output，DeepSeek completion tokens 已包含 reasoning，当前没有 cache/reasoning 分项消费者。
- 原因：直接 pull parsing 与现有 `ai.Stream` 一致，避免隐藏 goroutine、SDK retry 和第二套终态；finish reason 表达模型停止原因，`[DONE]` 表达传输闭合，两者不能互相替代。raw invalid arguments 属于模型可观察的单次调用错误，不应升级为整个 Provider turn 失败。

### D37. 文件工具共享 Workspace root；read 使用有界完整行分页

- 日期：2026-07-17
- 共同边界：`internal/coding.Workspace` 拥有规范化 host path 和一个共享 `*os.Root`；文件工具只借用 root，组合层在全部调用收敛后关闭。严格 JSON object 解码归入 `internal/coding/tools/toolargs`，workspace-relative path 规范化归入 `internal/coding/tools/fileutil`，具体分页、编码和输出语义留在 `internal/coding/tools/read`。路径最多 4096 bytes，拒绝绝对/逃逸路径以及任何 `..` component；不能先 clean `alias/../file`，因为较早 component 是 symlink 时会改变实际目标。
- `read` 契约：参数体最多 8192 bytes，输入为 `path`、可选 1-based `offset` 和可选 `limit`；同一页最多 2000 个完整行或 50 KiB 实际内容。完整行保留终止换行，尾部换行不产生空 continuation page。结果固定返回规范化相对路径、实际行范围、内容与 EOF/next-offset 状态，不暴露 host absolute path，也不为总行数继续扫描整文件。
- 文件与编码：所有 I/O 通过 root-relative handle；`fileutil.OpenRegularFile` 在 macOS/Linux 以 nonblocking read-only 方式取得 candidate，再对实际 opened handle 校验 regular-file，因此 FIFO 不会在校验前等待 writer，path replacement 也不能造成“检查 A、读取 B”。该行为由互斥 `go:build` 文件限定到已验证的平台；其他平台暂用普通 `Open` 保持包可构建，仍在打开成功后校验 regular-file，但不承诺无 writer FIFO 的非阻塞打开，也不扩展第一阶段的平台支持范围。本次返回页包含非法 UTF-8 时形成 call-local error，不使用 replacement character；未返回内容不为整文件编码判断而额外扫描。
- 有界性：选中单行超过 50 KiB 时立即报错并停止读取，不 drain 剩余行；offset 跳过的任一单行也受相同限制，不能用 seek-by-line 绕过边界。参数/path 上限同时约束标准库错误可能回显的模型输入，使失败结果也保持有界。`read` 无可变 call state，依赖 `os.Root` 的并发安全并声明 `CanRunParallel=true`。
- Pi 分歧：冻结 Pi 的默认 operations 使用 `fsAccess(R_OK)` 后直接 `fsReadFile`，没有 regular-file/FIFO 校验、nonblocking open 或相应 FIFO 测试；因此它不是用 pi-go 的 opened-handle 方案解决该问题。冻结 Pi 还允许绝对路径和可清理的 `..`、整文件读入后 `split/join`、非法 UTF-8 replacement、总行数提示、图片/UI/remote operations，并在首行超限时提示 bash fallback。pi-go 第一期选择 root containment、实际 handle 校验、macOS/Linux FIFO 非阻塞拒绝、精确文本视图和流式有界输出，不移植这些能力，也不建议通过 bash 绕过文件工具边界。
- 原因：Agent 后续会把 read result 直接写入 transcript，并用其中的原文驱动 exact edit；路径、内容、错误和并发行为都必须在进入模型前可预测、有界且与磁盘对象一致。只抽取已有真实消费者的聚焦 helper，避免为尚未讨论的工具建立猜测性接口。

### D38. write 只替换普通文件，并在固定父目录内原子提交

- 日期：2026-07-18
- 能力边界：`write(path, content)` 创建或完整覆盖普通文件并递归创建父目录；空内容合法，不增加计划未要求的 content 上限。目录、最终 symlink、FIFO、socket 和 device 都拒绝，创建或直接操作这些对象属于 bash 的显式能力。workspace 内 ancestor symlink 可以使用，逃逸或 dangling ancestor 由 `os.Root` 拒绝。
- 提交边界：先以 `root.OpenRoot(parent)` 固定实际父目录，再在该 root 内 `Lstat` 最终 component、创建随机 `O_EXCL` 临时文件、写完并关闭，最后以同目录 rename 提交。这样 ancestor swap 不会让临时文件、目标和清理落入不同目录；最终 symlink 在检查时直接拒绝，在检查后才出现则由 rename 替换 link entry 而不是跟随 referent。rename 前失败或取消删除临时文件并保留原目标，父目录创建不回滚；rename 后不再因 context 取消反报失败。
- 可观察语义：替换保证运行时读者只看到旧内容或完整新内容，不通过 `fsync` 承诺 crash durability。新文件 `0666`、父目录 `0777` 并服从 umask；覆盖保留九个 permission bits，但新 inode 不保留原 hard-link relationship、owner、ACL、extended attributes 或特殊 mode bits。成功文本只返回规范化相对路径和 Go string 的实际 UTF-8 byte length，不回显 content。
- Pi 分歧：冻结 Pi 递归建目录后直接 `fsWriteFile`，会跟随最终 symlink、直接截断目标，并用全局 per-path mutation queue；其成功文本把 JavaScript UTF-16 `content.length` 标作 bytes。pi-go 使用 Agent serial barrier、`os.Root`、普通文件限制和替换提交，不复制额外 mutation queue，并修正 byte count。
- 共享能力：同目录替换提交进入 `internal/coding/tools/fileutil`，因为计划已经明确 `write` 与后续 `edit` 共用该责任；模型协议、父目录策略和结果格式仍留在 `internal/coding/tools/write`。`internal/coding/tools/toolargs` 中的 decoder 只截断可能回显模型输入的超长诊断，不限制合法 write content。
- 原因：完整覆盖是模型需要的原子文件能力，不应隐式获得 symlink/特殊文件语义；固定 parent root 和 rename 同时满足 workspace boundary、取消前不暴露半写目标，以及后续 edit 可复用的提交契约。

### D39. 每个 coding tool 使用独立子 package

- 日期：2026-07-18
- 决定：具体工具分别位于 `internal/coding/tools/read`、`write`，后续已经确认的 `edit`、`bash` 实现时沿用同一布局，但不提前创建空 package。每个 package 对外提供自己的 `Tool` 与 `New(*os.Root)`；包内参数、结果和 helper 使用短而准确的名称，不再用工具名前缀解决同包冲突。
- 文件边界：模型协议与编排放在 `tool.go`；只有职责独立时才拆出分页、平台打开等实现文件。测试按模型协议、workspace boundary 和具体平台对象分组；同一工具可使用自己的 `helpers_test.go`，但测试 helper 不进入生产共享 package，也不跨工具 package 隐式共享。跨工具测试只通过公开构造器和可观察结果组合工具。
- 共享 package 边界：`internal/coding/tools/toolargs` 只保存 coding tools 已复用的严格 JSON 参数解码；`internal/coding/tools/fileutil` 保存 workspace-relative path、opened-handle regular-file 校验和普通文件替换提交。它们暂不上移 `internal/agent`：Agent 层只拥有 `Tool.Execute(ctx, json.RawMessage)` 契约，当前没有使用 decoder 或文件原语；只有出现非 coding 工具复用同一契约，并确认它是 Agent 工具实现的稳定公共责任时，才讨论上移。read 分页、edit 匹配策略、模型结果和测试 fixture 均保留在拥有它们的 package。
- 修正原因：只实现 `read` 时，扁平 package 尚未显露足够拆分证据；加入 `write` 后已经需要工具名前缀、混合不同工具测试，并出现 `write_test.go` 调用 `read_test.go` helper 的隐式依赖。此前“没有证据继续拆 package”的结论因此失效；按工具拆包让未来 `edit`、`bash` 加入时保持依赖和文件责任清晰，同时不引入 speculative interface 或空扩展点。

- 第二次修正：只有 `read` 使用 nonblocking candidate open 时，平台文件保留在 `read` package 符合最小共享原则；`edit` 加入后也必须在可能阻塞的 FIFO 打开之后校验实际 handle，第二个真实消费者已经成立，因此将聚焦的 `OpenRegularFile` 与互斥平台 open 文件迁入 `fileutil`。这不是建立通用 filesystem abstraction，具体分页和匹配仍不共享。

### D40. edit 保留多段原文件匹配，但第一期只执行精确替换

- 日期：2026-07-18
- 模型协议：输入为 workspace-relative `path` 和非空 `edits[]`，每项使用 `oldText`/`newText`；`oldText` 非空且必须在原始文件中恰好出现一次，`newText` 可以为空。全部位置都基于同一原文件快照，重叠区域拒绝；只有所有项通过后才构造完整新内容，因此任一失败不会部分应用。目标必须是已经存在的 UTF-8 regular file，成功结果只返回规范化路径和 block count。
- 提交与调度：先固定 resolved parent，再用共享 `OpenRegularFile` 读取实际 handle，并通过 `ReplaceRegularFile` 在同一 parent root 内提交完整替换。最终 symlink/特殊文件、取消和 rename commit point 沿用 D38；`edit` 保持默认串行屏障，不复制冻结 Pi 的进程级 per-path mutation queue。
- Pi 证据与分歧：冻结 Pi commit 的主协议同样是 `edits[]`，所有项在原文件上匹配、要求唯一且不重叠，并在写入前完成整组校验；但其 exact 失败后会进行空白、Unicode 与行结束符 fuzzy normalization，还兼容旧单编辑参数和字符串化 `edits`，生成 TUI preview/diff/patch，支持可替换 operations，并由 mutation queue 后直接 `writeFile`。pi-go 第一期不移植这些扩展。
- fuzzy 延后原因：精确失败保留原文件并允许模型重新读取恢复，模糊误匹配却可能提交非预期修改。normalization 后唯一性、offset 映射、原字节保留、CRLF/BOM 和多编辑组合需要单独设计及大量测试；学习者确认它是出现真实需求后的一次独立工作，不能因为冻结 Pi 已实现就顺带加入当前 exact mutation contract。
- 选择原因：`edits[]` 是冻结源码已经证明的核心可观察协议，保留它避免先建立马上废弃的单编辑 schema；fuzzy、UI、remote operations 和额外队列则没有第一期消费者。这个边界同时满足 semantic port、当前计划的“精确且可验证”以及不为未发现需求过度设计的约束。

## 变更记录

- 2026-07-15：建立初始课程和架构决策，并补充 stream、tool validation、Session storage、平台范围与 Runtime/Manager 边界。
- 2026-07-15：增加证据驱动的学习原则、先讲后问顺序和 `run_start/run_end` 课程术语。
- 2026-07-15：移除课程专用 `internal/contract`；冻结基线仅保存在文档。
- 2026-07-15：取消当前阶段的外部调用承诺；`cmd/pi-go` 仅用于本地运行与验收。
- 2026-07-15：第一期收敛为单目录、单 active task 的最小 coding loop；Goal Runtime、持久化 Session、完整 lifecycle/subscription、Manager 和外部集成移至二期。
- 2026-07-15：确定屏障式工具调度、错误继续、取消 settlement、`os.Root` 文件边界、DeepSeek 数据外发和固定 bug-fix 验收。
- 2026-07-16：根据冻结 Pi `Context` 与 coding-agent system prompt 证据，明确 workspace/cwd 说明进入稳定 system prompt，不建立独立 `WorkspaceContext` AI 协议字段。
- 2026-07-16：学习者确认原始 task 只作为 transcript 首条 user message，不在 Provider Request 中建立第二份 `Task`；第一阶段完整 transcript 是单一事实源。
- 2026-07-16：学习者确认第一阶段 `AssistantMessage` 的最小字段与删减范围；partial snapshot 和 Provider/模型诊断元数据按真实消费者延后。
- 2026-07-16：核对 Pi 的低层 working context、高层 Agent state 和 TUI 消费链后，确认 partial assistant 不进入正式 transcript；第 02 课不设计 event sink，流式观察和终端/TUI 展示留到出现真实消费者时讨论。
- 2026-07-16：学习者确认第 02 课使用 `RunResult + error`，不增加重复的 Run outcome；terminal assistant 留在 transcript，正常、失败和取消由 nil/non-nil error 与 context cause 区分。
- 2026-07-16：纠正“Phase 1 可使用 Run-local disposable transcript”的弱化推导；Agent 持有同一对话的完整内存 transcript，顺序 Run 的新 user input 继续追加，Provider 始终看到全部历史。
- 2026-07-16：学习者确认 Provider Request、Provider terminal message 和 `RunResult.Transcript` 在 Agent 所有权边界深复制，外部不能通过嵌套 slice 或 JSON bytes 反向修改权威 transcript。
- 2026-07-16：学习者确认预取消 context 在 Run acceptance point 前返回且不修改 transcript；acceptance point 后、terminal settlement 前取消则保留 user，并只产生一条 aborted assistant。
- 2026-07-16：学习者继续进入实现前收尾，确认已接受 Run 的全部退出路径统一返回一条 terminal assistant，由外层 Run 在唯一位置写入 transcript；第一条有效 terminal event 是 settlement point。
- 2026-07-16：第 02 课实现与测试完成；补充锁定同一次 Receive 同返 terminal event 与 error 时 terminal 优先，以及 nil stream 合成协议错误 assistant。
- 2026-07-16：学习者确认第 02 课无需继续讲解，要求完整性审查、补齐测试后提交并 push 到 `main`，随后开始第 03 课。
- 2026-07-16：第 02 课已推送到 `main`，学习者明确要求开始第 03 课；课程先核对冻结 Pi 的 Tool Loop 与整批调度，再讨论 pi-go 的 Tool contract 和屏障式分段实现。
- 2026-07-17：学习者接受第 03 课 Tool retry 结论；Agent 不自动重试 tool call，模型负责从 error tool result 恢复，后端内部重试必须有明确的瞬时错误证据。
- 2026-07-17：冻结 Pi 的 orphaned tool-call request 修复暴露出旧取消契约与完整 transcript 复用的冲突；学习者选择方案 2，pi-go 在 Agent transcript 中为 canceled/not-executed 调用追加明确 settlement results，并要求后续真实 DeepSeek 验证特别覆盖该分歧。
- 2026-07-17：第 03 课实现 review 发现 Provider aborted assistant 可以保留已完成 tool calls，而第 01/02 课的旧表述只约束了 assistant exactly-once。学习者确认扩展方案 2：失败 Turn 不执行这些 calls，但在唯一 terminal assistant 后追加同 ID not-executed results，并同步修正旧课程记录。
- 2026-07-17：学习者明确开始继续课程并进入第 04 课；确认参考冻结 Pi 的 Provider/API 分层，以薄 DeepSeek 层复用最小 OpenAI-compatible Chat Completions 协议层，不扩建多 Provider registry。
- 2026-07-17：学习者提出统一 Provider 目录；确定 `internal/ai/provider/` 只归类 Faux、OpenAI-compatible 与 DeepSeek 实现，consumer-owned `ai.Provider` 仍留在 `internal/ai`，并记录这与 Pi 大 Provider runtime abstraction 的差别。
- 2026-07-17：学习者确认用标准库 `net/http` 实现已讲解的完整消息映射，并将共享线协议包定名为 Go 惯例下的完整名称 `provider/openaicompatible`；不要求沿用 Pi 的 `openai-completions` 命名。
- 2026-07-17：学习者确认 pull-based SSE parser、finish reason 加 `[DONE]` 双 settlement、tool-call 分片边界、零自动重试、64 KiB HTTP 错误体、1 MiB SSE event 和最小 usage 语义；第 04 课进入实现。
- 2026-07-17：学习者明确要求第 04 课不为没有真实 DeepSeek/Pi 证据的 3xx 增加特殊逻辑或专门测试；Provider 保持标准 `http.Client` 行为，redirect 影响记录为后续独立验证项，“零自动重试”只表示 Provider 不自行 retry。
- 2026-07-17：学习者开始第 05 课并确认 `read` 的 offset/limit、非法 UTF-8 error 与固定模型输出；共享 JSON/path 能力先进入 coding tools 的集中辅助 package，具体读取语义保留在 `read`。
- 2026-07-17：`read` 子阶段完成测试先行实现和审查修复；workspace root、non-regular/FIFO、symlink/`..`、分页/双上限、错误有界、取消和并发测试通过。学习者随后确认理解并要求本地提交，下一步进入 `write`。
- 2026-07-17：核对冻结 Pi 后确认其默认 `read` 没有 regular-file/FIFO 特殊处理；pi-go 的 `O_NONBLOCK` 加 opened-handle 校验是有意增强。课程同时修正 `go:build` 理由：当前只将该保证开放给已有测试的 macOS/Linux，而不是声称其他 Go 平台一定缺少该常量。代码注释统一使用英文，课程和其他文档语言不作限制。
- 2026-07-18：学习者确认 `write` 只负责普通文件完整内容，其他文件系统对象由 bash 显式处理；完成固定 parent root、同目录临时文件替换、最终 symlink/特殊文件拒绝、取消清理、UTF-8 byte count 和 outside-canary 竞态测试。
- 2026-07-18：加入 `write` 后重新审查 coding tools 结构，学习者确认按工具拆为 `read`/`write` 子 package；私有 helper 去掉多余工具前缀，测试 helper 不再跨工具隐式共享，并记录共享 package 只接纳已证明复用的责任。
- 2026-07-18：学习者重新审查参数解码归属，确认暂不因为潜在通用性上移到 `internal/agent`；集中 `utils` 拆为 coding-owned `toolargs` 与 `fileutil`，待出现真实跨领域消费者后再讨论上移。
- 2026-07-18：重新核对冻结 Pi 后纠正“单个精确替换”的课程预览；学习者选择保留 `edits[]`、原文件匹配、唯一性、overlap 与全有或全无，但第一期不引入 fuzzy fallback。
- 2026-07-18：学习者确认 fuzzy matching 涉及 normalization、位置映射和大量组合测试，应在真实需求出现后作为独立工作推进；代码和文档必须保留该延后原因。加入 `edit` 同时让 opened-handle regular-file 校验形成第二个真实消费者，因此从 `read` 迁入聚焦的 `fileutil` 原语。
- 2026-07-18：`edit` 完成测试先行实现和审查；多段原文件匹配、唯一性、overlap、失败不落盘、普通文件替换、workspace/symlink/FIFO 边界、取消和 read 可见性测试通过。审查将重复 occurrence 检测收敛为最多寻找两个位置，避免为非唯一诊断扫描全部高度重复内容。
