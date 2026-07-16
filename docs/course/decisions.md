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

- 日期：2026-07-16
- 决定：Run 取消停止新的 Provider 调用和工具阶段，取消所有已启动 child contexts，等待 stream、tool workers 和 bash process group 收敛，然后返回明确的 canceled 非成功结果。已启动调用保留 completed 或 canceled outcome；未启动调用不制造 synthetic result。未来引入 event sink 时，再把观察通道的 settlement 纳入该取消契约。
- 原因：取消请求不是完成信号。过早返回会遗留 goroutine、进程或晚到事件，并允许新旧副作用重叠。

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
- 决定：第一阶段正式 transcript 只追加 terminal final/aborted assistant message，不保存、持久化或暴露可查询的完整 partial assistant view。如何把 formation events 提供给本地终端、未来 TUI 或日志，等课程处理真实展示过程时再讨论，不在第 02 课建立 event sink。
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
- 决定：`Agent.Run(ctx, userInput)` 在同一个锁内先拒绝已有 active Run，再检查 `context.Cause(ctx)`；若 context 此时已经取消，Run 未被接受，不追加 user 或 aborted assistant，不调用 Provider，并返回当前稳定 transcript snapshot 与 context cause。若检查通过，设置 active 并追加 user 构成 Run acceptance point；此后到 terminal settlement 之前发生的取消属于已开始的 Run，保留 user，并最终只追加一条 aborted assistant。
- 原因：Pi 高层 `Agent.prompt()` 不接收外部 signal，而是在 prompt 被接受后创建内部 `AbortController`，所以没有完全对应的预取消调用；Pi 低层则会在 Provider 观察 signal 前把 prompt 加入 context。pi-go 需要同时遵守 Go 的预取消 context 不启动副作用惯例，以及 Pi 对已接受 prompt 保留 user 与 aborted assistant 的行为。

### D31. 每个 Provider turn 只在一个位置追加 terminal assistant

- 日期：2026-07-16
- 决定：每次 Provider call 的 stream consumer 在正常 terminal、Provider error/aborted terminal、terminal 前 EOF 和非 EOF receive error 的所有路径上只计算一对 `(terminal AssistantMessage, error)`，不直接修改 Agent；Turn/Run coordinator 在该次调用的唯一位置深复制并追加该 assistant。第一条有效 `DoneEvent` 或 `ErrorEvent` 构成该 Provider turn 的 terminal settlement point，之后发生的取消不追溯修改这条已经完成的 assistant；若同一次 `Receive()` 同时返回有效 terminal event 与 error，terminal event 优先。没有有效 terminal 时，raw stream failure 在 context 已取消时合成 aborted，否则合成 error；nil stream 作为协议错误合成 error assistant。第 02 课一个 Run 只有一个 Turn，所以暂时表现为每个 Run 追加一次；第 03 课一个 Run 可有多个 Turn，每个 Turn 各追加一次 terminal assistant。
- 原因：把 transcript 写入分散在 terminal、error、cancel 和 deferred cleanup 分支会产生缺失或重复 assistant。Provider stream 已绑定 context，Agent 等待 `Receive()` 真正收敛，不用额外 goroutine 与 `ctx.Done()` 竞争后提前返回，以免遗留仍在读取网络的工作。

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
