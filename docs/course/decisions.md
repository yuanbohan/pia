# 课程与架构决策

这个文件记录跨课程有效的当前决定。新证据推翻旧决定时，直接更新对应决定，并在“变更记录”说明原因；Git 历史保存旧版本。

## 当前决定

### D1. 做语义移植，不做逐文件翻译

- 日期：2026-07-15
- 决定：Go 实现对齐 Pi 的可观察行为与设计约束，允许采用 Go 原生接口、goroutine、channel 和 `context.Context`。
- 原因：TypeScript 的类、Promise 和异步迭代器不是产品契约；机械复制会掩盖 Go 的并发与取消语义。

### D2. 第一期只实现模型与工具循环

- 日期：2026-07-15
- 决定：`internal/agent` 负责通用 Provider turn、transcript 和 tool loop；第一期不实现结构化 plan/progress/replan/done/blocked Goal Runtime。模型停止调用工具时只表示 loop 正常结束，coding task 是否成功由本地、独立的真实项目验收判断。
- 原因：当前目标是证明最小 coding loop 能完成真实任务。提前增加 Goal 状态机会把 Agent Loop 能力和上层目标策略混在一起。

### D3. DeepSeek-first，但 Provider 边界可替换

- 日期：2026-07-15
- 修正日期：2026-07-19
- 决定：第一个真实 Provider 只实现 DeepSeek；底层 Provider 保留显式模型、base URL 和密钥配置，非本地 base URL 默认必须使用 HTTPS，测试服务器需要显式开发覆盖。Lesson 06 的产品命令不暴露配置矩阵，固定使用 `deepseek-v4-pro`、thinking mode 和 high reasoning effort；只有 Provider package tests 继续使用其他模型或 endpoint。
- 原因：先减少兼容矩阵，同时保留未来扩展其他 Provider 的边界。

### D4. Faux Provider 是 Agent Loop 的首要验证设施

- 日期：2026-07-15
- 决定：先完成可脚本化的 Faux Provider，再接真实模型。
- 原因：事件顺序、工具错误、取消和 transcript 必须能确定性复现，不能依赖付费 API 或模型随机性。

### D5. 当前交付 headless Agent Runtime，不做 SDK-first

- 日期：2026-07-15
- 修正日期：2026-07-19
- 决定：临时入口位于 `cmd/pia`，实现包位于 `internal/`；该命令只用于本地运行和验收，名称、参数和输出都不向其他项目承诺稳定协议。公共 Go SDK、gRPC、其他网络 RPC 和 IM 适配均推迟设计。
- 原因：核心 API 尚处于学习和校正阶段，过早公开会固化错误抽象。出现真实外部调用方后，再根据同进程嵌入或跨进程服务选择接口。

### D6. 第一期不移植 TUI；后续 TUI 保持为外层投影

- 日期：2026-07-15
- 修正日期：2026-07-19
- 决定：第一期不实现主题、按键、命令提示、交互式选择器和 slash command UI。后续 TUI 进入独立阶段，消费 application/session 的事件和状态，不拥有 Agent Loop、权威 history、compaction 或持久化；其具体课次和拆法等前置契约稳定后再决定。
- 原因：这些能力不决定第一期 coding loop 是否能完成任务。TUI 整体是 XLarge 方向，必须建立在已经由非 TUI consumer 验证的事件、交互和 Session 契约之上；现在固定两课或更多实现细节都缺少真实证据，提前把状态放进 UI 还会破坏核心与展示层边界。

### D7. 第一期是单目录、单 active Run，不建立持久化 Session

- 日期：2026-07-16
- 修正日期：2026-07-19
- 决定：`cmd/pia` 把启动时的当前工作目录作为唯一 workspace，只接收一条位置参数形式的初始 task prompt。核心 `Agent` 仍拥有当前进程内的完整有序 transcript，并可按顺序接收后续 user input；同一时刻只允许一个 active Run。Session 创建、持久化、恢复、分支、多用户并发和 Agent Manager 延后。
- 原因：内存 transcript 是 Agent 保持目标和工具上下文的核心执行状态，不是可删减的 Session 附加能力；推迟的是持久化与管理层，而不是正确的消息所有权。

### D8. 第一期没有审批或 trust/yolo 策略系统

- 日期：2026-07-15
- 修正日期：2026-07-18
- 决定：模型选择的工具直接执行，不逐次请求批准，也不提供 trust/yolo 配置矩阵。`read`、`write`、`edit` 强制 workspace 文件边界；bash 只固定每次调用的初始 cwd，不是 sandbox，Provider 生成的命令拥有启动用户的完整主机和网络权限，可以访问 workspace 外资源并产生不可逆副作用。bash 与冻结 Pi 一样继承 CLI 的完整进程环境，因此已经存在于环境中的 Provider 凭据也对命令可见；只存在于 Provider 内存配置中的凭据不会被额外注入 argv、tool config、transcript、trace、日志或错误。若命令主动读取或打印父环境中的 secret，第一期也没有 redactor 阻止它进入 tool result 或 trace。
- 原因：学习者在 bash 子阶段确认本地 CLI 信任当前 workspace、模型和命令，选择冻结 Pi 的直接执行与完整环境语义，而不是此前未经讨论就写入文档的最小 allowlist。逐工具审批会阻断自动 coding loop；完整环境还能保留从 Ghostty/zsh 启动 CLI 时已经导出的 PATH、代理和语言工具链配置。这不是 credential isolation，操作者仍需理解命令拥有当前用户权限。

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
- 修正日期：2026-07-19
- 决定：Provider 对 Agent 暴露可取消的 pull/receive 流抽象。Lesson 06 的 one-shot CLI 只在 Run settlement 后投影最终 assistant 文本；bash 增量 drain 只负责避免 pipe deadlock 并构造有界 tool result，不新增通用 Tool/Agent progress 接口或 event sink。实时展示继续推迟到未来 TUI 或 interactive consumer。
- 原因：pull boundary 直接参与模型调用、结束、错误和取消，属于基础执行数据流；实时 presentation 则没有第一期消费者。把两者分开可以保留正确执行语义，而不为未知 call identity、投递顺序和 UI 状态提前固化 callback。

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
- 决定：Run 取消停止新的 Provider 调用和工具副作用，取消所有已启动 child contexts，等待 stream、tool workers 和 bash process group 收敛，然后返回明确的 canceled 非成功结果。工具阶段已经完成的调用保留实际结果，执行中调用记录 canceled，未启动调用不执行但记录明确的 not-executed settlement result；若 Provider aborted terminal 自身保留了已经组装完成、尚未进入工具阶段的 calls，这些调用全部不执行并追加同 ID not-executed results。未来只有真实交互式消费者引入 event sink 时，才扩展观察通道的 settlement 契约。
- 原因：取消请求不是完成信号。过早返回会遗留 goroutine、进程或晚到事件，并允许新旧副作用重叠；漏掉未启动调用的 settlement result 又会让持久 transcript 出现 orphaned tool calls，阻断同一 Agent 后续 Run。

### D22. 文件工具使用 os.Root；bash 不宣称 workspace containment

- 日期：2026-07-15
- 决定：Run 开始时用 `os.Root` 打开固定 workspace；`read`、`write`、`edit` 只执行 root-relative I/O 并拒绝非 regular-file 目标，不使用“先检查绝对路径再普通 open”的易竞态模式。bash 只从 workspace 启动，仍可访问当前用户资源。
- 原因：文件工具需要抵抗路径穿越和 symlink TOCTOU；cwd 不是命令 sandbox，不能把 fixture diff 或 canary 误写成主机级隔离。

### D23. Lesson 06 最终验收使用固定 bug-fix fixture，不在仓库内实现 Pi 比较

- 日期：2026-07-15
- 修正日期：2026-07-19
- 决定：真实验收只在被忽略的 `tmp/pia-acceptance/` 下保存一个能构建但 Fibonacci 实现错误、且没有测试的本地 Go baseline。每次从 untouched baseline 建立新副本并启动新 `pia` 进程，要求 Agent 修复函数、增加有意义的测试并验证；导师独立检查测试、程序输出以及新增测试在原始错误实现上确实失败。必须连续两次成功，任何产品代码、system prompt 或任务文字调整都会重置计数。仓库不提交 fixture、prompt、harness、trace 或模型修改结果。
- 原因：当前里程碑需要专家反复诊断 prompt、模型、loop 与 tools 的首个真实闭环，但还没有维护长期 benchmark 基础设施的需求；保留本地证据即可让学习者检查，同时不把一次验收题固化进产品树。

### D24. DeepSeek 数据外发与本地命令权限必须明确

- 日期：2026-07-15
- 修正日期：2026-07-19
- 决定：README 和 Lesson 06 明确披露 system prompt、task、模型选择的文件内容、命令和 tool output 会发送给 DeepSeek；操作者自行选择可披露 workspace。one-shot 命令不显示启动 warning，也不增加确认或审批交互。Provider 生成的 bash 命令继承完整父环境并拥有启动用户的主机和网络权限；第一期没有 sandbox、secret detector 或通用 redactor。read/bash 内容仍在进入 transcript 和 Provider request 前执行既有大小限制。
- 原因：final-only 命令不应混入未被用户要求的过程输出；披露责任仍必须持久存在于操作者文档。内容限制减少单次模型输入，却不是 secret isolation，也不能约束 unsandboxed bash 的主机副作用。

### D25. 第一阶段 AssistantMessage 只保留 Loop 可观察字段

- 日期：2026-07-16
- 决定：`AssistantMessage` 第一阶段只保留有序 content blocks、token usage、stop reason 和 error message。暂不加入 api/provider/model、response ID/model、diagnostics、timestamp、cost 或未核验的 cache/reasoning usage 细分；stream delta 不重复携带完整 partial message，terminal event 携带权威 final/aborted message。
- 原因：当前 Agent Loop 只观察内容、用量和终态；其余字段属于 Pi 的多 Provider、路由、Session/UI、诊断或费用生态。第 02 课核对后没有发现 pi-go 第一阶段需要完整 partial snapshot 的消费者；第 04 课若核对 DeepSeek 后出现新的 usage/trace 需求，再用证据扩展内部协议。

### D26. Partial assistant 不属于正式 transcript

- 日期：2026-07-16
- 决定：第一阶段正式 transcript 对每个 Provider turn 只追加一条 terminal final/aborted assistant message，不保存、持久化或暴露可查询的完整 partial assistant view。第 03 课增加的 tool-result settlement 不属于 partial 或第二条 assistant；error/aborted terminal 中已完成但未执行的 tool calls 会在该 assistant 后得到同 ID not-executed results。formation events 与实时展示明确推迟到未来 TUI 或其他交互式消费者，不在第一阶段建立 event sink。
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
- 决定：具体工具分别位于 `internal/coding/tools/read`、`write`、`edit`，后续 `bash` 沿用独立子 package 布局而不提前创建空 package。文件工具对外提供自己的 `Tool` 与 `New(*os.Root)`；bash 不能从 `os.Root` 取得 `exec.Cmd` 所需的 host cwd，而且其命令本就不受 root containment，因此通过聚焦配置接收 canonical workspace path 与可选 shell path。包内参数、结果和 helper 使用短而准确的名称，不用工具名前缀解决同包冲突，也不为构造差异建立 base tool。
- 文件边界：模型协议与编排放在 `tool.go`；只有职责独立时才拆出分页、平台打开等实现文件。测试按模型协议、workspace boundary 和具体平台对象分组；同一工具可使用自己的 `helpers_test.go`，但测试 helper 不进入生产共享 package，也不跨工具 package 隐式共享。跨工具测试只通过公开构造器和可观察结果组合工具。
- 共享 package 边界：`internal/coding/tools/toolargs` 只保存 coding tools 已复用的严格 JSON 参数解码；`internal/coding/tools/fileutil` 保存 workspace-relative path、opened-handle regular-file 校验和普通文件替换提交。它们暂不上移 `internal/agent`：Agent 层只拥有 `Tool.Execute(ctx, json.RawMessage)` 契约，当前没有使用 decoder 或文件原语；只有出现非 coding 工具复用同一契约，并确认它是 Agent 工具实现的稳定公共责任时，才讨论上移。read 分页、edit 匹配策略、模型结果和测试 fixture 均保留在拥有它们的 package。
- 修正原因：只实现 `read` 时，扁平 package 尚未显露足够拆分证据；加入 `write` 后已经需要工具名前缀、混合不同工具测试，并出现 `write_test.go` 调用 `read_test.go` helper 的隐式依赖。此前“没有证据继续拆 package”的结论因此失效；按工具拆包让未来 `edit`、`bash` 加入时保持依赖和文件责任清晰，同时不引入 speculative interface 或空扩展点。

- 第二次修正：只有 `read` 使用 nonblocking candidate open 时，平台文件保留在 `read` package 符合最小共享原则；`edit` 加入后也必须在可能阻塞的 FIFO 打开之后校验实际 handle，第二个真实消费者已经成立，因此将聚焦的 `OpenRegularFile` 与互斥平台 open 文件迁入 `fileutil`。这不是建立通用 filesystem abstraction，具体分页和匹配仍不共享。

- 第三次修正：此前把未来 bash 也写成 `New(*os.Root)` 是从文件工具机械外推。`os.Root` 只服务 root-relative file I/O，不提供 host path，也不可能约束 shell；bash 使用 `Workspace.Path()`，文件工具使用 `Workspace.Root()`，由后续 composition root 分别注入。进程组、shell resolution 和 output accumulation 都属于 bash package，不进入 `fileutil` 或含义宽泛的 utils package。

### D40. edit 保留多段原文件匹配，但第一期只执行精确替换

- 日期：2026-07-18
- 模型协议：输入为 workspace-relative `path` 和非空 `edits[]`，每项使用 `oldText`/`newText`；`oldText` 非空且必须在原始文件中恰好出现一次，`newText` 可以为空。全部位置都基于同一原文件快照，重叠区域拒绝；只有所有项通过后才构造完整新内容，因此任一失败不会部分应用。目标必须是已经存在的 UTF-8 regular file，成功结果只返回规范化路径和 block count。
- 提交与调度：先固定 resolved parent，再用共享 `OpenRegularFile` 读取实际 handle，并通过 `ReplaceRegularFile` 在同一 parent root 内提交完整替换。最终 symlink/特殊文件、取消和 rename commit point 沿用 D38；`edit` 保持默认串行屏障，不复制冻结 Pi 的进程级 per-path mutation queue。
- Pi 证据与分歧：冻结 Pi commit 的主协议同样是 `edits[]`，所有项在原文件上匹配、要求唯一且不重叠，并在写入前完成整组校验；但其 exact 失败后会进行空白、Unicode 与行结束符 fuzzy normalization，还兼容旧单编辑参数和字符串化 `edits`，生成 TUI preview/diff/patch，支持可替换 operations，并由 mutation queue 后直接 `writeFile`。pi-go 第一期不移植这些扩展。
- fuzzy 延后原因：精确失败保留原文件并允许模型重新读取恢复，模糊误匹配却可能提交非预期修改。normalization 后唯一性、offset 映射、原字节保留、CRLF/BOM 和多编辑组合需要单独设计及大量测试；学习者确认它是出现真实需求后的一次独立工作，不能因为冻结 Pi 已实现就顺带加入当前 exact mutation contract。
- 选择原因：`edits[]` 是冻结源码已经证明的核心可观察协议，保留它避免先建立马上废弃的单编辑 schema；fuzzy、UI、remote operations 和额外队列则没有第一期消费者。这个边界同时满足 semantic port、当前计划的“精确且可验证”以及不为未发现需求过度设计的约束。

### D41. bash 沿用冻结 Pi 的受信任本地 CLI 语义

- 日期：2026-07-18
- 信任与进程环境：bash tool 暴露 `command` 和可选秒数 `timeout`，不逐次审批，也不提供 sandbox。每次调用从固定 workspace 启动一个新的非交互、无 PTY shell，stdin 为空，并完整继承启动 `pia` 的父进程环境；zsh 已导出的 PATH、代理和变量会传入 Bash，未导出的 shell variable、alias 和 function 不会。Provider 生成的命令拥有启动用户的主机与网络权限。shell 支持显式 path；默认按 `/bin/bash`、PATH 中的 `bash`、`sh` 回退。第一期不移植 Pi 的 remote operations、command prefix、spawn hook 或 Windows transport。
- 生命周期：macOS/Linux 为每次调用建立独立进程组。可选 timeout 没有默认值；timeout、Run cancellation 或 CLI cancellation 直接以 `SIGKILL` 终止原进程组并等待 shell 回收，不承诺运行 cleanup handler。正常 shell exit 不主动清理进程组，因此重定向输出的后台服务可以继续运行，后续失败不改变已经返回的 shell exit code，后台文件修改也可能与后续工具竞态；创建新 session 的 daemon 还可能逃出原进程组。每次调用都是新 shell，`cd`、`export` 和 virtualenv activation 不跨调用保留。
- 输出与错误：stdout/stderr 按到达顺序合并并在运行期间持续 drain；跨 pipe 的相对顺序不作确定性承诺。模型最终只看到最后 2000 行或 50 KiB，完整 raw output 在首次超限后写入无大小上限、不会自动删除的系统临时文件；单个超长行可能只保留尾部。正常 shell exit 后，继承 pipe 的 descendant 以冻结 Pi 的短 idle grace 收敛读取，持续输出的后台进程会让调用继续等待，除非 timeout 或取消。非零 exit 形成保留有界输出的 call-local error，Agent 继续运行；shell pipeline 和命令列表仍服从自身的最后命令 exit status。
- CLI 展示边界：Lesson 06 的 one-shot CLI 只输出 Run settlement 后的最终 assistant 文本，不实时展示 bash output。第 05 课的增量 drain、tail accumulator 和完整临时文件继续用于避免 `CombinedOutput` 式结束后读取和 pipe deadlock，并向模型提供有界结果；它不再被解释为第一版必须接入 event sink 的承诺。
- Pi 证据与修正：冻结 `bash.ts` 使用完整 `process.env`、可选无默认 timeout、detached process group、取消/超时 hard kill、合并输出、tail truncation 和完整临时文件；`waitForChildProcess` 用 output-idle grace 避免 descendant 持有 pipe 时永久等待。此前课程计划中的最小 allowlist、Provider key 对命令不可见和正常退出清理全部 descendants 没有经过学习者讨论，也不符合该源码，现由本决定取代。

### D42. Lesson 06 由 coding application 拥有稳定 prompt 和 one-shot composition

- 日期：2026-07-19
- 修正日期：2026-07-20
- 决定：`internal/coding` 构建稳定 system prompt，持有 canonical workspace、四个 tools、固定 DeepSeek product profile 和 Agent composition；`cmd/pia` 只处理进程边界。Prompt 以冻结 Pi 默认结构和可复用措辞为横向比较基线：identity 只把 `pi` 替换为 `pia`，保持四工具 snippets/guidelines 与两条全局 guidelines 的默认 body 连续，再在 Pi 的 `appendSystemPrompt` seam 加入现有 headless、安全修改、错误恢复和验证指导；project-context 保留 content 之后的 template framing newline 与 cwd 顺序。不声称支持 Pi docs、custom tools、Skills、extensions 或完整 resource loader。Provider schema 与 trace 仍来自真实 tool definitions。workspace 根目录 project instructions 按 `AGENTS.md`、`AGENTS.MD`、`CLAUDE.md`、`CLAUDE.MD` 顺序选择第一个存在的 UTF-8 regular file，不搜索 ancestor 或全局配置。可选 `PIA_TRACE_PATH` 只在 Run settlement 后 create-new 一个 `0600` 调试文件，保存完整 prompt、schema、Conversation History（JSON 字段名为 `transcript`）和顶层错误但不保存 API key；trace schema 不稳定且可能包含敏感内容。第一版不增加自动执行预算。
- 原因：Pi 的 coding system prompt 和 print mode 证明这些责任属于 coding/host 层，而不是模型协议；后续 Pi 横向评测还要求 Prompt 与 workflow 的非必要差异尽量小。root-only instructions、固定 profile 和 post-run trace 足以支持当前真实闭环与诊断，而 Pi 产品文档、动态工具、Skills、extensions、配置矩阵、event audit log 或 policy engine 仍没有对应能力，不能为了文本一致而虚假加入。

### D43. 后续课程采用滚动式大纲，只编号近期闭环

- 日期：2026-07-19
- 决定：目前只为二期前三课分配稳定编号：Lesson 07 建立完整 Conversation History、Core Agent Working Context 与 Provider Request Snapshot 的所有权边界；Lesson 08 完成 settled Run 后、下一次 Provider call 前的 threshold context-budget/compaction 核心闭环；Lesson 09 完成 Skills 渐进披露。Skills 保持为二期第 03 课。Runtime 韧性、事件与文本交互、Session 持久化/恢复、TUI 和稳定评测只保留为后续方向，等前三课产生真实证据后再拆分和编号。
- 范围：Lesson 07 不做 compaction、持久化或 Skills；Lesson 08 不同时吸收 context-overflow retry、其他 Provider retry、branch summary 或持久化；Lesson 09 不扩建完整 ResourceLoader、extensions 或 MCP。大纲不预先决定具体 Go API、package、算法和完整测试矩阵，开课时才根据冻结 Pi 与当前实现补齐。
- 原因：context ownership 到 compaction 仍是解锁长任务的主线，Skills 是已经确认的第三个独立闭环。更远能力的依赖和责任会被这些实现改变；现在给它们固定课次或详细设计，只会制造很快失效的计划。

### D44. 课程规模是拆分门槛，不是工期估算

- 日期：2026-07-19
- 决定：未来已编号课程表至少记录解锁能力、Pi 大致做法、pi-go 当前边界与非目标、结束信号、依赖、相对规模和状态。规模使用 Small、Medium、Large、XLarge；XLarge 不能直接开课，必须先讨论并拆成能独立讲解和验收的闭环。越远的阶段信息越粗，不因为表格列更多而提前设计实现细节。
- 原因：这些字段让后续实施者理解“为什么这样排”和“何时算完成”，同时保留根据真实源码与代码推翻早期假设的空间。把多个状态所有权、生命周期或消费者塞进一课，会让讲解、review 和验收同时失焦。

### D45. 以 Pi parity 为能力下限，并用受控评测追求稳定超过 Pi

- 日期：2026-07-19
- 决定：完成 coding-relevant Pi 能力覆盖后，建立稳定、可重复的 pi-go 与冻结 Pi 对照评测。Pi parity 是不可退让的能力下限，稳定超过是演进目标。公平比较固定模型/Provider profile、任务与初始仓库并做多次独立运行；以客观 resolve rate 为主要能力指标，同时记录成本、turn、时延、tool error、恢复和长上下文表现，并把协议完整性与安全回归作为门槛。其他优秀开源 coding agent 以及 Codex、Grok 的可获得证据只提供候选机制与工程经验，不替代 Pi 对照组。
- 范围：当前只确定目标和公平性原则，不提前确定 benchmark corpus、runner 架构、统计阈值、trace schema 或产物存储。Lesson 06 的本地 fixture 仍只是第一期验收；正式评测方向在前置能力稳定后另行拆课。
- 原因：项目目标是先确保不弱于 Pi，再通过可验证机制变得更强，而不是仅达到文件结构或功能清单相似。没有同模型、同任务、重复运行和客观判据，“达到或超过 Pi”都只能是主观印象，不能指导后续优化。

### D46. 完整 Conversation History 与 Core Agent Working Context 分属不同 owner

- 日期：2026-07-19
- 决定：Lesson 07 采用外层最小内存 Conversation Owner 保存完整、有序的 Conversation History；Core Agent 只拥有可替换的 Working Context，并继续负责 Provider/tool loop；每次 Provider 调用使用由 Working Context 构造的 ownership-independent Request Snapshot。这里的 Conversation Owner 是语义责任，不预先要求同名 Go 类型或独立 package。
- 范围：当前决定不引入持久化 Session、entry tree、branch、compaction 算法、事件订阅或 TUI。Conversation Owner 与 Core Agent 之间的同步、首份 package 归属、active 边界、消息复制和 Working Context replacement 分别采用 D47 至 D51。
- 原因：冻结 Pi 在 `message_end` 后先把 terminal assistant 保存进完整 Session entries，再由外层在 threshold/overflow compaction 后替换 `agent.state.messages`；overflow error 可以保留在完整历史中，却从 retry 的 Working Context 中移除。当前 pi-go 的单 `transcript` slice 在无 compaction 时成立，但继续合并会迫使 Lesson 08 在“丢失原始历史”和“压缩不减少模型输入”之间二选一。只确定最小内存 owner 边界可以解决这项真实张力，同时避免提前复制 Pi 的完整 Session 基础设施。

### D47. Core Agent 以同步 run-local message delta 提交一次 Run 的新增消息

- 日期：2026-07-19
- 决定：一次 accepted Core Agent Run 在 settlement 后同步返回该 Run 新增的完整有序 message delta，并同时返回独立的 Go error；Conversation Owner 无论 error 是否为 nil，都先把 ownership-independent delta 一次性追加到 Conversation History。acceptance point 前取消或 concurrent Run rejection 返回空 delta且不修改 History。当前同步路径使用普通函数返回，不引入 Go channel、message callback 或完整 Agent event stream。
- 范围：这里确定的是 settlement 语义，不预先锁定 Go struct/field 名；课程使用 `NewMessages` 表示该 delta。History 在当前最小内存实现中以 settled Run 为提交边界，不承诺 active Run 中途可观察或进程崩溃恢复。实时持久化、TUI progress、extensions、steering/follow-up 和多订阅者事件仍在后续真实消费者出现时设计。
- 原因：冻结 Pi 的低层 `runAgentLoop()` / `runAgentLoopContinue()` 已经独立维护并返回 `newMessages`，同时另行发出逐消息和 `agent_end` 事件；前者表达 Run 结果，后者服务 `AgentSession` persistence、extensions 与 UI。pi-go 当前只有 settlement 后的 Conversation History 消费者，直接返回 delta 足够；提前使用 channel/callback 会额外引入 close、backpressure、drain、panic、reentrancy 和 listener settlement 契约，却没有当前收益。

### D48. 首份 Conversation Owner 是 coding-owned 私有实现

- 日期：2026-07-19
- 决定：首份 Conversation Owner 实现在现有 `internal/coding` package 的独立 `conversation.go` 中，使用未导出的类型协调具体 Core Agent 与完整内存 Conversation History。当前不创建 `internal/conversation`、`internal/session` 或同名公共抽象，也不把完整 History 的所有权放回 `internal/agent`。
- 范围：这是第一个真实 Coding Agent 消费者的 package 归属，不宣称 research、OCR、web search 等未来应用必然复用同一种 history policy。出现第二个非 coding 消费者时，必须从双方已经证明相同的责任重新审查 package 布局，再决定是否提取共享实现或接口；概念上的通用性本身不是提前建包的证据。
- 原因：冻结 Pi 同样把持久 conversation/session 协调放在 coding-agent 的 `AgentSession`，而不是低层 agent loop。pi-go 当前只有 Coding Agent 需要该责任；将私有实现放在 composition 所在应用层，既保持 Core Agent 通用，也避免为尚不存在的消费者设计宽泛 API。独立文件让 history ownership 与 `runtime.go` 的一次性装配责任保持收敛。

### D49. Conversation Owner 以 fail-fast active guard 保证 Run 提交顺序

- 日期：2026-07-19
- 决定：同一 Conversation 同时最多接受一个 active Run。Conversation Owner 的 active 边界从 Run acceptance 持续到 Core Agent 返回、run-local message delta 提交进 Conversation History并生成返回 snapshot 之后；这段时间的 concurrent Run 立即返回 active-run error，不阻塞也不进入隐式队列。Core Agent 保留自己的 active guard，分别保护其 Working Context。
- 范围：该决定只串行化同一 Conversation 的 Run；不同 Conversation 可以并行。一个 accepted Run 内仍可按既有工具调度契约并行执行连续的 parallel-safe tool calls。steering、follow-up、显式输入队列、优先级和中途插话仍不属于本课。
- 原因：Core Agent 的 deferred active release 会在调用方收到 result 之前发生；如果 Conversation Owner 没有覆盖 History commit 的外层 guard，Run B 可能在 Run A 已更新 Working Context、但尚未提交完整 History 的间隙被接受，从而使 Core Agent 处理顺序与 Conversation History 提交顺序不一致。阻塞第二个调用则会隐式引入尚未设计生命周期的输入队列。

### D50. Run delta 只在真实所有权边界深复制

- 日期：2026-07-19
- 决定：Core Agent 的 `RunResult` 使用 `NewMessages` 表示本次 accepted Run 的 run-local message delta，不再把完整 Working Context 命名为 `Transcript` 返回。`NewMessages` 必须与 Core Agent Working Context 深度隔离；Conversation Owner 接收后取得这份 delta 的内部所有权并直接追加到完整 History，不做第二次无收益的 clone。Coding Agent 返回的 `RunResult.Transcript` 仍是完整 Conversation History 的 deep-cloned snapshot，调用者不能反向修改 owner state。
- 范围：ownership transfer 只发生在 Conversation Owner 调用 Core Agent 的内部同步路径；Conversation Owner 不把接管的 delta slice 或嵌套可变内容原样暴露出去。Provider Request Snapshot、Provider terminal message 与 D51 的 Working Context replacement 仍各自在自己的 owner 边界执行深复制。
- 原因：Core Agent 与 Conversation History 是不同 owner，因此必须隔离；Conversation Owner 接管一份已经独立的返回值后没有第二个共享 owner，再 clone 只会增加成本并模糊真正的边界。最终完整 History snapshot 会跨到调用者，因此那里仍必须 clone。

### D51. Working Context 只能在 Core Agent idle 时原子替换

- 日期：2026-07-19
- 决定：Core Agent 提供窄方法 `ReplaceWorkingContext([]ai.Message) error`。该方法只在没有 active Run 时深复制并原子替换当前 Working Context；active Run 期间立即返回 active-run error，不等待、不排队，也不在同一 Run 的 Turns 之间切换 context。替换不读取或修改 Conversation History。
- 范围：这是 Lesson 08 compaction 投影可接入的状态边界，不在本课实现摘要、token budget、overflow recovery 或通用 message-graph validator。方法是即时内存操作，不接收 `context.Context`，也不返回旧 context 或暴露可变 Agent state。
- Pi 差异：冻结 Pi 通过赋值 `agent.state.messages` 替换 working messages，setter 只复制顶层数组；coding-agent 的实际 compaction replacement 位于 `agent_end` 之后、下一次 prompt 之前。pi-go 保留这一 idle-time 使用时序，但用显式 Go 方法、active guard 和深复制收紧所有权，而不暴露可变 state object。
- 原因：Run 内后续 Turn 依赖前面 Turn 和 tool results；中途替换会让同一 Run 使用两套上下文，破坏 delta 与 Provider request 的可推理关系。等待式 API 又会隐式引入生命周期和排队语义；调用方本就应由 Conversation Owner 在 Run settlement 之后协调 replacement，因此 fail-fast 最清晰。

### D52. 长期产品是由 Orchestrator 驱动的多 Session coding-agent service

- 日期：2026-07-19
- 决定：长期产品允许用户通过 IM 创建和推进 coding task，由 Gateway 提供外部服务边界，由 Orchestrator 协调多个可持久化、可恢复且相互隔离的 Sessions。当前 one-shot CLI、单进程内存 Conversation 和单 active Run 是演进基础，不是最终产品边界。
- 范围：这项决定只固定长期责任方向，不预先确定 Gateway 协议、部署拓扑、Session 存储格式、任务状态机、队列策略、租户模型或 IM 平台。相关能力继续作为未编号方向，待 Core Agent、Session persistence/recovery、事件与任务生命周期分别出现真实证据后拆分。
- 原因：最终 coding capability 需要能被用户从日常聊天入口持续驱动，并在多个长任务之间保持独立历史、恢复能力和可观察状态；把这些责任塞回 Core Agent 会破坏已经建立的通用模型/tool loop 边界。

### D53. Threshold compaction 使用 protocol-safe message-level cut

- 日期：2026-07-20
- 决定：Lesson 08 的 threshold compaction 可以在一个已经 settled 的 Run 的消息序列内选择 retained suffix，而不把完整 Run 当作不可分割单元。第一条 retained 原始消息可以是 user 或 assistant，但不能是 tool result，也不能从一条 message 的 content blocks 内部切分；保留含 tool calls 的 assistant 时，必须同时保留后续所有 matching tool results。cut 之前的较旧 History 和当前 Run prefix 进入 summary，完整 Conversation History 不删除或改写原始消息。
- 范围：本决定只固定 compaction 对 settled messages 的 cut 语义，不表示 active Run 执行到该位置时触发 compaction。执行时机由 D54 独立规定。本决定也不定义具体 Go API、summary message 表示、cut estimator、projection metadata、overflow retry、branch summary 或 Session persistence。
- Pi 对齐：冻结 Pi 的 `findCutPoint()` 同样允许 user/assistant cut point、拒绝从 tool result 开始，并在 retained cut 落入 user-started turn 内部时为被移除的 turn prefix 生成补充 summary。
- 原因：单次 coding Run 可以因多轮 read/edit/bash 调用而独自占满 Working Context；只按 Run 边界压缩会出现“保留则仍超预算、删除则丢掉全部近期原文”的死角。message-level cut 既能继续前移 retained window，又能保持 tool-call/result 协议完整。

### D54. Threshold compaction 在下一次 Run 接受输入前 lazy 执行

- 日期：2026-07-20
- 决定：Conversation Owner 在下一次 `conversation.run()` 取得自己的 active guard 后、Core Agent 接受新 user input 之前，根据上一份 settled Working Context 检查 threshold 并按需执行 compaction。没有后续 Run 就不调用 summarizer；threshold compaction 成功后不自动调用 `continue`。Conversation guard 持续覆盖 compaction、后续 Core Run、History commit 与返回 snapshot。
- 失败语义：summary request 不经过 Core Agent loop且不提供 coding tools，其 request、terminal response 和 usage 不进入 Conversation History。summary Provider error、取消、空文本、意外 tool call、无效 candidate context 或 `ReplaceWorkingContext` failure 都不修改 History、Working Context 或 projection metadata，新 user input 保持未接受，并返回当前完整 History snapshot 与错误。candidate 验证及最后一次 cancellation check 通过后，Working Context replacement 与 projection metadata 发布构成同步 commit；commit 之后发生的取消不回滚合法的新 projection，但 Core Agent 仍可拒绝尚未接受的 input。
- 范围：本决定只覆盖 threshold compaction 的 between-Runs lifecycle；明确不在同一个 active Run 的相邻 Provider Turns 之间 compaction，也不增加后台/eager compaction、manual command、compaction event、overflow compact-and-retry 或输入队列。取得 Conversation active guard 只是 lifecycle 协调，不等于 Core Agent 已接受 user input。
- Pi 差异：冻结 Pi 通常在 `agent_end` 后立即检查 threshold，并在新 prompt 前补做检查。pi-go 当前 one-shot composition 没有独立 compaction status/event，采用 pre-next-Run lazy 时机可以避免无后续调用时的额外模型成本，也不会把上一次已经成功的 Run 因后处理失败改写成 error，同时仍保持“settled Run 后、下一次 Provider call 前”的外部语义。
- 原因：当前最小 API 用 `RunResult + error` 表达一次 user input 是否被接受并完成，没有另一个通道承载 eager compaction failure。lazy 检查让失败明确归属于尚未接受的新 Run，旧的 settled result 与完整 History 都保持稳定。

### D55. 首版 between-Runs compaction 使用 192K threshold 与 64K soft ceiling

- 日期：2026-07-20
- 决定：Lesson 08 以 projected Provider input `192_000` tokens 作为首版 between-Runs quality-oriented compaction threshold；`64_000` tokens 是 compaction 后完整 projected Provider input 的普通情况 soft ceiling，不是 summary size、必须填满的目标或成功所需的硬上限。projected input 包含稳定 system prompt、tool schemas、synthetic summary、retained raw suffix 与尚未接受的新 user input。`1_000_000` model context 仍只表示 Provider hard capacity，不作为日常 coding target。
- 证据与折中：DeepSeek V4 官方 MRCR 证据显示到 `128K` 基本稳定、之后开始可见退化，而 `256K` 已明显下降；它证明不能把 1M 容量当质量区间，但不足以把 128K 认定为 coding 精确最优点。OpenAI 对 Codex agent loop 的说明和其链接源码显示 Codex 默认以 model context 的 `90%` 触发 auto-compaction，`272K` coding context 对应约 `244.8K`；GPT-5.3-Codex system card 中一项以最大化长任务表现为目标的评测则每 `100K` 触发。pi-go 的普通文本 summary 又弱于 OpenAI 模型原生 opaque compaction item，因此 `192K` 在容量、压缩频率与质量风险之间取中间偏保守位置；达到 `64K` ceiling 时提供约 `128K` 的再增长空间。
- Soft-ceiling 语义：cut selection 应尽量使 candidate 不超过 `64K`，但不得为了凑该数值丢弃没有进入 summary 的消息。不可压缩的新 input、固定 prompt/tools、单条大 message、实际 summary 或 protocol-safe granularity 使 `64K` 无法达到时，允许以高于 `64K` 但低于 `192K` 的 candidate 成功；连 threshold 都无法降到时按 D54 原子失败，新 input 不被接受。
- 范围：这是 D54 lifecycle 下的首版产品 policy，不是 active Run hard ceiling，也不改变本课不做 run 内 compaction、overflow compact-and-retry 或模型 registry 的范围。它不宣称一次 active Run 不会从低于 threshold 增长到更高区间。
- 复评义务：真实 DeepSeek coding traces 可用后，必须按 `<128K`、`128K-192K`、`192K-256K` 与 `>256K` 分桶比较任务成功率、重复 compaction 后的信息损失、成本和频率。`128K-192K` 已显著退化时下调；只有 `192K-256K` 质量稳定且 compaction 过频时才上调。更换模型、reasoning mode、tool schema、Skills 暴露、summary prompt/表示或 tool-result bounds 时必须重新校准。

### D56. 首版 summary prompt 与 model-visible 表达沿用冻结 Pi

- 日期：2026-07-20
- 决定：没有 DeepSeek 或 pi-go 证据要求偏离时，Lesson 08 原样沿用冻结 Pi commit `dcfe36c79702ec240b146c45f167ab75ecddd205` 的 summarization system prompt、首次 structured checkpoint prompt、已有 summary 时的 update prompt、split-turn prefix prompt，以及 `<conversation>` / `<previous-summary>` 输入组织。待摘要消息按 Pi 规则序列化为带 role labels 的纯文本，每个 tool result 在 summary input 中最多保留 `2000` characters；从 tool calls 确定性提取的 read/modified file lists 追加到模型 summary。
- Model-visible 表达：成功 summary 在 Working Context 中投影成普通 synthetic user message，沿用 Pi 的 `The conversation history before this point was compacted into the following summary:` preamble 与 `<summary>` tags，随后保留 D53 的 protocol-valid raw suffix。它不进入完整 Conversation History，也不冒充真实 user input；Conversation Owner 私有 projection metadata 分别保存 summary 与 cut boundary。重复 compaction 只把新丢弃的原始消息放入 `<conversation>`，把前次 summary 放入 `<previous-summary>` 交给 update prompt，不把 synthetic summary 当普通对话再次摘要。
- 范围：本决定不引入新的 `ai.Message` role、custom summary instructions、branch summary、extensions/hooks、持久化 compaction entry 或 Session entry tree。具体预算由 D57 规定；Go constants 与文件拆分在实现设计中继续收敛。D54 对空 summary、意外 tool call、Provider failure 与 cancellation 的严格原子失败语义保持不变。
- 原因：prompt 直接影响模型保留哪些工作状态，属于需要证据才能改动的行为契约；当前没有真实 DeepSeek 结果证明 Pi prompt 不适用。精确复用还能先建立可比较基线，后续只有结构遵循率、事实保真度或 coding continuation 评测显示具体问题时才做有针对性的修改。

### D57. 首版 budget allocation 沿用 Pi 并对大项目与 Skills 强制复评

- 日期：2026-07-20
- 决定：固定 DeepSeek coding product profile 拥有 `contextCapacity: 1_000_000` 与 `modelMaxOutput: 384_000`；coding-owned Conversation compaction policy 拥有 D55 的 `threshold: 192_000`、`softCeiling: 64_000`，以及按冻结 Pi 映射的 `retainedRawTarget: 20_000`、`summaryMaxOutput: floor(0.8 * 16_384) = 13_107`、`splitTurnPrefixMaxOutput: 8_192` 和 `providerContextSafety: 4_096`。这些 output 数字都是 request caps，不要求模型填满。
- Request 边界：`ai.Request` 增加窄的 request-local output limit，由 OpenAI-compatible payload 映射为 `max_tokens`；它只承载调用方已经计算的单次限制，不拥有模型 catalog 或 compaction policy。正常 coding call 参考 Pi 使用 `min(384_000, max(1, 1_000_000 - projectedInput - 4_096))`，summary calls 分别使用 `13_107` 或 `8_192` 并受同一 hard-cap clamp 约束。
- 预算关系：普通 summary 加 `20K` retained raw 的上限约 `33.1K`；split-turn 两份 summary 加 retained raw 的上限约 `41.3K`。`64K` soft ceiling 的其余空间用于 system prompt、tool schemas、新 input、summary envelope 和确定性 file lists。cut finder 可以为完整 projected budget 减少 retained suffix，但不能丢弃未进入 summary 的消息；D55 规定无法达到 soft ceiling 时的降级与失败语义。
- 未验证风险：这组数值尚未在真实大型项目或 Skills-enabled context 中证明充分。大项目可能增加 project instructions、相关文件、tool turns 与跨 Run 状态；未来 Skills metadata、按需正文和由 Skills 引导出的 tool results 也会改变静态与动态 context。`20K` retained target、Pi summary 容量和 `64K` soft ceiling 都可能不足，当前不得宣称它们适用于所有项目规模。
- 延后与复评：Lesson 08 不提高这些值，不实现按仓库大小自动调参，也不提前设计 Skills。至少在 Lesson 09 引入 Skills 时、以及产品声称支持真实长项目任务前，必须用真实仓库和任务测量各 context 来源、compaction 前后 token 分布、soft-ceiling miss rate、compaction 间隔、分桶 coding 完成率、summary continuation 成功率、重复摘要的信息损失和重复读取成本。证据可以要求修改 threshold、soft ceiling、retained target、summary budgets 或 prompt。

### D58. Projected input 以有效 usage 为锚并使 pre-compaction usage 显式失效

- 日期：2026-07-20
- 决定：`internal/ai` 提供 model-neutral 的 request estimator：优先采用最后一条非 error/aborted 且非零 terminal assistant usage 的 `input + output` 作为已知 prefix，只以 `ceil(characters / 4)` 估算其后的 messages；无有效 usage 时，估算 system prompt、JSON tool schemas、全部 messages 与新 input。coding Conversation 对该完整 projected request 使用 D55 的 `>= 192_000` threshold；Core Agent 对每个正常 request 使用同一 estimator 和 D57 clamp 计算 request-local output limit。
- Compaction boundary：成功 replacement 会清空 candidate retained assistant 的 usage，并在 private projection metadata 保存 `UsageValidFrom` absolute History index。Conversation 从完整 History 重建 Working Context 时，只有该 boundary 之后的新 assistant usage 可以恢复为锚点；第一次 post-compaction response 之前走完整 fallback 估算。这样无需给公开 message protocol 增加 timestamp 或 compaction role，也不会用压缩前的大 usage 立即重新触发 compaction。
- 所有权：fixed request limits 由 coding product profile 提供；`ai.RequestLimits` 只实现 model-neutral clamp，`ai.Request.MaxOutputTokens` 只承载一次调用的值；threshold、soft ceiling、retained target、summary prompts、cut、projection 与 failure semantics 仍由 coding-owned Conversation 负责。Provider 只映射 `max_tokens`，不拥有估算或 compaction policy。
- 精度边界：字符近似是冻结 Pi 基线，不是 tokenizer 等价物。真实 traces 必须比较 estimate 与 Provider-reported input；大型项目、Skills、模型/reasoning/tool schema 或 prompt 变化仍按 D55/D57 强制复评。

### D59. 产品统一命名为 Pia

- 日期：2026-07-20
- 决定：产品、架构和后续课程讨论统一使用 **Pia**。`pi-go` 只保留为当前 repository、workspace directory、Go module/import path 与历史记录中的技术标识；是否重命名 repository/module 是另一项有迁移影响的工作，未经明确批准不在本课机械修改。
- CLI 边界：`pia` 不再被描述为临时产品名；`cmd/pia` 是当前本地入口，但其参数、输出协议、部署形态与公共 SDK 承诺仍不稳定。
- 原因：产品身份与现存技术路径是两个维度。现在统一术语可以避免把 Pi 基线、pi-go repository 和最终产品混成同一个概念，同时不为命名讨论引入无关 module migration。

### D60. read 支持 workspace-relative 与 absolute host path

- 日期：2026-07-20
- 决定：Pia 的 model-facing `read` 同时接受 workspace-relative path 和 absolute host path。relative path 继续经 workspace `os.Root` 打开，拒绝 `..` 与逃逸 symlink；absolute path 由 host filesystem 直接解析，可以读取 workspace 外的 regular file。成功结果对 relative input 保留规范化 relative path；absolute input 在 host open 和结果显示中保留原始路径语义（显示时只转换分隔符），不能在 symlink-aware host resolution 前用 lexical `filepath.Clean` 改写 `..`。
- 保留契约：目标仍必须是 opened-handle 验证过的 regular file 和合法 UTF-8；arguments、单页行数/bytes、errors、cancellation 与 concurrency 仍保持有界。`write` 与 `edit` 继续受 workspace `os.Root` 限制，bash 的既有 host authority 不变。
- 安全与数据外发：absolute `read` 明确授予模型调用用户的 host read authority，不是 sandbox 或 trusted-root allowlist。模型选择的外部文件内容和 tool result 可能发送给 Provider；operator documentation 必须直说这项边界。此扩权支持显式 absolute reads 与外部 references，但不再被解释为 Skill discovery 扫描或 external-symlink 支持的理由，也不是宣称读取外部文件天然安全。

### D61. Skills 是 Pia 核心能力，首版从 Pia Skill v1 开始

- 日期：2026-07-20
- 决定：Pia 直接拥有 Skills 的 discovery、bounded disclosure、activation 和 context-lifecycle 责任。首版只实现 project-local Pia Skill v1：selected workspace 根目录下 `.pia/skills/<direct-child>/SKILL.md` 的 required name、description 和 Markdown instructions。plugin、extension 或 package 可以在以后分发 Skills，但不是启用核心 Pia lifecycle 的前置层。
- 当前范围：workspace 是 project scope boundary；不向上寻找 repository root，不递归发现 nested Skills，不扫描 `.agents/skills`、`.claude/skills`、user、admin、system 或其他 global locations，也不通过 external symlink 引入 Skill。catalog location 使用 workspace-relative path。Pia Skill v1 只加载 `SKILL.md`；`scripts/`、`references/`、`assets/` 和其他 supporting files 不被发现、解析、列出、注入或赋予执行语义，是否支持全部留给后续决定。Coding Agent 的通用 tools 仍可处理普通项目文件，但这不构成 Skill resource support。
- Validation：缺少 name、缺少 description 或 YAML 完全无法解析的 Skill 跳过。Agent Skills strict name constraints 仍产生 diagnostics，但 name 与父目录不一致、超过 64 字符或字符形式不合规不单独导致拒绝；Pia 继续加载 bounded non-empty frontmatter name。同名 entries 按 stable lexical path 选择一个并 warning。规范合规性与 runtime acceptance 是两个不同事实。
- 长期方向：Pia Skill v1 有意沿用 Agent Skills 的基础目录、frontmatter 和渐进披露形状，避免形成不可迁移的私有格式。完整 Agent Skills compatibility、Claude Code/Codex community roots、global scopes、optional metadata、supporting resources 与 vendor runtime semantics 必须在后续按真实需求逐项校准、拆课和声明，当前不得提前宣称支持。
- 上下文义务：Pia Skill v1 先实现 metadata catalog 与普通 `read` 按需正文，不预载 supporting files。Lesson 08 已有 compaction，因此未来 managed activation 仍需要稳定身份、去重和 compaction protection；catalog 必须有独立预算并触发 D55/D57 的 Skills-enabled 真实项目复评。Lesson 09 决定 validation、diagnostics 与 catalog budget；Lesson 10 在重新开课校准后决定 structured activation 与 compaction projection。

### D62. Skills 核心生命周期拆为 discovery/catalog 与 activation/continuity 两课

- 日期：2026-07-20
- 决定：原 Lesson 09 在加入多来源 compatibility、bounded catalog、dedicated activation、resource lifecycle、dedupe 和 compaction protection 后成为 XLarge，不再作为一个可进入课程。本决定以开课后的新证据修正 D43 的三课假设：Lesson 09 收敛为 Medium 的 Pia Skill v1 discovery、bounded catalog 与普通 `read` 基础使用闭环；新增 Lesson 10 作为 Large 的 managed activation 与 Context Continuity，且保持未开始直到学习者明确进入。
- Lesson 09：只解析 `.pia/skills` 直接子目录的 bounded name/description，构建有独立预算的 catalog，证明正文不进入 initial request，并复用普通 `read` 在匹配后加载 instructions。D60 absolute-read 是已完成的独立能力，Pia Skill v1 location 只需 workspace-relative path。
- Lesson 10：只在 Lesson 09 完成后消费稳定 catalog，把普通 read result 升级为有 stable identity、dedupe 和 compaction 后 durable model-visible projection 的 structured activation。community/global discovery、完整 supporting-resource engine、installer/plugin、vendor-specific runtime 和完整交互式 invocation 仍不自动并入。
- 原因：discovery/catalog 与 activation/continuity 有不同 owner、失败语义和完成信号；前者属于 filesystem-to-prompt projection，后者跨 tool execution 与 Conversation compaction lifecycle。分别完成可以逐课解释和验证，也避免用“最小实现”牺牲 Skills 作为 Pia 核心能力所需的 continuity。

### D63. 项目指令兼容与 Skills 分属不同课程边界

- 日期：2026-07-20
- 当前事实：Lesson 06 已实现 selected workspace 根目录的最小 project instructions 支持，候选顺序是 `AGENTS.md`、`AGENTS.MD`、`CLAUDE.md`、`CLAUDE.MD`，只加载第一个有效候选。冻结 Pi 同样把 context files 与 Skills 分开，但会加入 global agent-dir 文件并遍历 ancestors；Codex 按 project root 到 CWD 每层选择一个 `AGENTS.override.md`/`AGENTS.md`/fallback，Claude 则累加 `CLAUDE.md`/`CLAUDE.local.md` 并对 descendants 做按需加载。三者不是一套可机械合并的规则。
- 课程边界：Lesson 09/10 只负责 project Skills catalog 与 activation，不发现或解释 `AGENTS.md`、`CLAUDE.md`。Lesson 06 的 workspace-root first-match 行为继续有效；完整 project-only instruction chain 作为尚未编号的独立 prompt/context 方向，等 project root、launch CWD、nested scope 与 conflict semantics 有足够证据后再拆课。即使以后增强，也不自动引入 user/global Codex 或 Claude instructions。
- 原因：project instructions 是每次任务都应可见的 standing context；Skills 是 metadata-first、按任务选择后才加载正文的渐进披露资源。把两者放进同一 loader 或同一课程会混淆触发时机、预算、冲突和 compaction continuity。

### D64. Agent Skills 与 Claude/Codex compatibility 分阶段补充

- 日期：2026-07-20
- 决定：Lesson 09 不实现或声明完整 Agent Skills、Claude Code Skills 或 Codex Skills compatibility，只实现 D61 的 Pia Skill v1。`.agents/skills`、`.claude/skills`、ancestor/nested roots、user/global scopes、symlinked Skills、optional resources、explicit invocation 和 vendor runtime fields 都进入 Lesson 10 之后的未编号兼容方向。
- 原因：一个现实 Skill 可以同时携带长 instructions、scripts、references、assets、tool grants、dynamic preprocessing、subagents、hooks 和 client-specific metadata。一次性“兼容”会把 discovery、execution、permissions、context lifecycle 与 distribution 混成无法准确验收的 Skill engine。先建立 Pia 自己的最小可靠闭环，既保留未来迁移形状，也让每项兼容能力以后有独立证据、失败语义和完成信号。

### D65. Pia Skill v1 采用一次性有界 snapshot 和非阻塞 diagnostics

- 日期：2026-07-20
- Discovery 与解析：每个 Conversation 只在创建 Core Agent 前读取一次 selected workspace 的 `.pia/skills`。source 本身通过 supported-platform nonblocking open 和 opened-handle directory validation 打开；source enumeration 在输入侧最多读取 257 个 direct entries，超过 256 时整个可选 Skill source 以一条 warning 忽略。未超限时先按目录名 lexical sort，再最多检查 64 个直接、非 symlink Skill directories。没有 watcher 或 Run 中途 reload。每个入口必须是 `os.Root` 内可安全打开的 regular `SKILL.md`。discovery 只从最多 16 KiB 的 prefix 提取 YAML frontmatter，使用 `go.yaml.in/yaml/v3`，只消费 string `name` 与 `description`；其他字段 warning 后忽略，正文及 supporting files 不被解析、验证或注入。
- Validation：缺少/空 name、缺少/空 description、无法解析的 YAML、重复 mapping key、非 string 必需字段、不安全目标或超限 frontmatter 跳过。Agent Skills 的 1–64 个小写字母/数字/连字符、无首尾/连续连字符并与目录名一致仍是规范诊断；Pia 对 65–256 characters、字符形式或目录 mismatch 只 warning 并加载 frontmatter name，超过 256 characters 才按 hard safety 跳过。同名 Skill 选择 workspace-relative location lexical 较小者。description 超过 1024 characters 时只在 catalog 中截断并 warning。
- Catalog budget：稳定 system prompt 只加入 XML-escaped name、description 和 workspace-relative `SKILL.md` location，并指导模型匹配时用现有 `read` 获取完整文件；没有有效 Skill 时整个 section 省略。catalog 使用 D58 的 `ceil(characters / 4)` 估算，最多 4096 estimated tokens；超限时先统一缩短 descriptions，仍放不下才省略 lexical tail entries，所有实际裁剪都有 warning。该值只是首版 ceiling，必须随真实大型项目和 Skills-enabled context 分桶继续复评。
- Diagnostics 与 trust：单个 Skill 或整个可选 source 的发现失败不阻塞普通 coding task。最多保留 64 条有界 `SkillDiagnostic`，进入内部 `RunResult` 和可选 trace；`cmd/pia` 只在 Run 和 trace 都成功后把它们作为简短 warning 写入 stderr，并在唯一输出边界同时 quote untrusted path 与 message，避免任何 producer 遗漏的控制字符伪造日志或操纵终端。首版没有 trust UI 或逐 Skill approval；选择 workspace 是操作者的 trust decision，metadata 可能自动发送给 Provider，完整 `SKILL.md` 在模型选择普通 `read` 后可能发送，Skill instructions 可以进一步引导现有高权限 tools。

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
- 2026-07-18：进入 `bash` 子阶段后纠正未经讨论的最小环境与 descendant cleanup 结论；学习者逐项确认沿用冻结 Pi 的直接执行、完整父环境、新 shell、无 stdin/PTY、可选无默认 timeout、正常退出保留后台进程、取消/超时 hard-kill 进程组、合并 tail 加完整临时文件、非零 call-local error 和最终 CLI 实时展示语义。
- 2026-07-18：`bash` 完成测试先行实现：独立 package 接收 canonical workspace host path，Darwin/Linux 进程组负责 timeout/cancel，正常退出使用 output-idle grace 且保留后台进程，增量 accumulator 提供 2000 行/50 KiB tail 与完整 raw 临时文件；CLI live event 接口仍按决定延后到第 06 课的真实消费者。
- 2026-07-19：Lesson 06 以新的 one-shot 实施计划修正旧设定：临时入口改为 `cmd/pia`，只输出最终 assistant 文本，不增加 event sink 或启动 warning；真实 Fibonacci 验收只保存在被忽略的 `tmp/`，连续两次 fresh 运行通过后完成。
- 2026-07-19：Lesson 06 在最终产品代码、system prompt 和任务文字冻结后完成两次连续真实 DeepSeek 验收；两个新进程都从 untouched baseline 修复通用 Fibonacci base case、增加能让原错误实现失败的测试，并通过独立测试与程序输出 `55`。本地 workspace 和 trace 保留在 ignored `tmp/`，课程进入待理解确认且尚未提交。
- 2026-07-19：学习者独立运行第三个 `pia` acceptance workspace，复核了最终修改、测试与 trace，并确认理解 stable system prompt、每轮完整 transcript 重放、结构化 tool schemas、thinking replay 与 final-only 输出的组合流程；随后明确要求提交并推送第 06 课。
- 2026-07-20：为后续 Pi 横向评测复核 Lesson 06 system prompt 后，学习者明确要求以 pi-go 能力为边界但尽量保留冻结 Pi 默认 Prompt，且不改 one-shot workflow。D42 因此从自由 pi-go wording 修正为“冻结 Pi 基线加窄适配”，并以完整字符串测试固定 identity、四工具 snippets/guidelines、project-context 与 cwd 顺序。
- 2026-07-20：Prompt 修正使此前真实验收 streak 按 D23 清零。当前分支构建的 `pia` 在 fresh `attempts/004` 中仍 one-shot 修复 Fibonacci、添加能让原实现失败的测试，并通过独立测试与程序输出 `55`；`evidence/004.json` 确认实际使用新 prompt 和四工具 schema。当前新基线计数为 `1/2`，不与旧 prompt 的历史运行合并。
- 2026-07-20：进一步逐段 review 发现 `004` 使用的 prompt 仍有三项可避免的 Pi 差异：identity 额外描述、pia guidance 插入默认 body、instruction trailing-newline framing 不同。修正后再次清零，并以同一最终二进制和固定任务连续运行 fresh `attempts/005`、`006`；两次都修复通用 Fibonacci base case、增加能让原错误实现失败的测试、通过独立 `go test ./...` 且程序输出 `55`。`evidence/005.json`、`006.json` 确认最终 prompt、四工具 schema、正常 `stop` 和空 Run error，新基线完成 `2/2`。
- 2026-07-20：代码命名进一步收敛为业务行为和契约职责；文档、注释和断言可保留 Pi 来源说明，但 Go 标识符、测试函数与 subtest 不使用参考实现名称。此次纯命名重构未改变 prompt、workflow 或任务文字，但仍按 D23 清零；当前二进制在 fresh `attempts/007`、`008` 中再次连续完成通用 Fibonacci 修复、测试判别、独立测试和程序输出验证，规范化路径后的 prompt 与 `006` 一致，新基线重新达到 `2/2`。
- 2026-07-19：完成第一期后重新核对 Pi、Codex 和 Grok Build 的长任务与交互边界；学习者确认后续课程采用滚动式大纲，只固定最近三个闭环，Skills 位于二期第 03 课。所有 XLarge 方向必须在开课前拆分，远期 TUI 与稳定 Pi 对照评测暂不绑定课次和实现细节。
- 2026-07-19：学习者明确开始 Lesson 07，并要求把“每课开课先重读对应 Pi 源码、再修正甚至推翻大纲”的流程写入 `AGENTS.md`。冻结源码校准确认 Pi 实际区分完整 session entries、Agent working messages 与 request-local transformed messages，Lesson 07 因此从含糊的两层 active-context 表述修正为三种角色的所有权课程，但仍不引入 compaction 或持久化。
- 2026-07-19：学习者接受 Lesson 07 的 owner 分离方向：外层最小内存 Conversation Owner 保存完整 Conversation History，Core Agent 保存可替换 Working Context，Provider Request Snapshot 保持 request-local；同步机制留待本课下一步讨论。仓库新增 `CONCEPTS.md`，为 Agent、Core Agent、Coding Agent、Conversation、Working Context、Session 等跨课程词汇记录明确包含项和排除项。
- 2026-07-19：学习者确认一次 Core Agent Run 可以包含多个 Turns 和 Messages，并接受 settlement 后同步返回 run-local `NewMessages` delta、由 Conversation Owner 批量提交 History 的方向；当前不使用 Go channel、callback 或完整事件流。`CONCEPTS.md` 同步增加 Run、Turn 与 Run Message Delta 的有界定义。
- 2026-07-19：学习者确认首份 Conversation Owner 作为 Coding Agent 的私有应用层实现放在 `internal/coding/conversation.go`；当前不建立通用 conversation/session package，也不把完整 History 放回 Core Agent。第二个非 coding 消费者出现时再依据已证明的共同责任重新审查并提取。
- 2026-07-19：学习者选择 Conversation Owner 自有的 fail-fast active guard，覆盖 Core Agent Run 到 History commit 和返回 snapshot 的完整边界；同一 Conversation 的 accepted Runs 保持顺序，不建立隐式等待队列，Core Agent guard 继续独立保护 Working Context。
- 2026-07-19：学习者接受最小深复制契约：Core Agent 以 ownership-independent `RunResult.NewMessages` 返回 run-local delta，Conversation Owner 直接接管并提交，Coding Agent 的完整 `RunResult.Transcript` 继续作为 deep-cloned History snapshot；不在同一 owner 内重复 clone。
- 2026-07-19：学习者确认 Core Agent 使用 idle-only `ReplaceWorkingContext`：输入深复制、替换原子化、active 时立即失败，不允许一个 Run 的不同 Turns 使用两套 context；Pi 的可变 state setter 不机械移植到 Go。
- 2026-07-19：Lesson 07 按 D46–D51 完成实现：Core Agent 改为 Working Context 加 run-local `NewMessages`，coding-owned 私有 Conversation Owner 提交完整 History，idle-only replacement 提供 Lesson 08 接入点。完整 tests、vet 和 race 通过，课程进入待理解确认且尚未提交。
- 2026-07-19：学习者确认理解 Lesson 07 的 Core Agent delta、Conversation History commit 与 idle-only replacement 主线，并明确要求提交；课程状态更新为已提交，不夹带 Lesson 08 工作。
- 2026-07-19：学习者确认长期产品方向为 Orchestrator 驱动的多 Session coding-agent service：通过 Gateway 和 IM 创建、推进任务，并要求 Session 持久化、恢复、并发隔离；当前只记录策略与责任方向，不提前设计实现。
- 2026-07-20：学习者明确开始 Lesson 08。开课源码校准确认 threshold compaction 仍是一个可进入的 Large 闭环：Pi 以有效 Provider usage 为主要预算事实、近似估算 usage 后的尾部，在 coding-owned `AgentSession` 边界生成 summary、保留 protocol-valid suffix 并替换 working messages；当前 pi-go 已有 usage、独立 History owner 与 idle-only Working Context replacement，因此本课不需要先引入 tokenizer、Session persistence、branch 或事件系统。具体 budget owner、cut granularity、summary 表达和失败语义留待本课讨论后形成新决定。
- 2026-07-20：学习者确认 Lesson 08 采用 protocol-safe message-level cut：可以在一个长 Run 内从 user 或 assistant message 开始保留，绝不从 tool result 或 message 内部切分；若切进 Run，中间被移除的 prefix 进入 summary，完整 History 保持原样。该语义记录为 D53。
- 2026-07-20：学习者确认 Lesson 08 在下一次 Run 接受新 input 前执行 lazy threshold compaction；summary 或 replacement 失败时旧 History、Working Context 和 projection metadata 全部不变，新 input 未接受，commit 后的取消不回滚合法 projection。该 lifecycle 与失败语义记录为 D54。
- 2026-07-20：针对 1M window 是否会降低 coding 质量继续核对产品和研究证据。DeepSeek V4 官方 MRCR 曲线在 128K 后出现可见退化，coding 长上下文研究也显示未过滤长输入可能降低真实仓库修复表现；因此冻结 Pi 在 1M model 上约 983616 tokens 才触发的 capacity-oriented 默认值不能直接成为 pi-go 的 quality policy。当前提出 projected input `128K` between-Runs quality-oriented threshold、`32K–64K` 正常高信号区间作为待验证假设，并明确它不是 active Run 内每次 Provider call 的 hard ceiling；尚未形成新 durable decision。
- 2026-07-20：学习者要求明确保留 ceiling 复评义务，并指出“在同一 Run 内选择 cut point”容易被误读为“active Run 内触发 compaction”。课程记录已澄清：D53 只允许事后切入一个 settled Run 的消息序列，D54 的执行时机严格位于两个 Runs 之间；`128K` 与 `32K–64K` 仅为首版可测参数，必须依据后续 DeepSeek coding 分桶评测以及模型、reasoning mode、tools 或 summary policy 的变化重新校准。
- 2026-07-20：重新对照当前代码后保持 D54 的 between-Runs 范围。Conversation Owner 只能在 `core.Run()` 返回后取得完整 delta，而 Core Agent 明确拒绝 active-time Working Context replacement；因此 run 内 compaction 不是同级参数选择，而会要求新的增量状态所有权和 safe point。当前 read/bash 结果已有单次 50 KiB 模型可见上限，且尚无单个 Run 经常超过候选阈值的 trace 证据；该风险保留为显式缺口，出现真实越界或 coding 质量分桶证据后再拆分能力。
- 2026-07-20：参考 Codex 通用客户端约 `244.8K` 默认 auto-compaction 点与高强度长任务评测每 `100K` compaction 的两端，并结合 DeepSeek 在 128K 后开始退化及 pi-go 仅有普通文本 summary 的差异，学习者委托确定首版折中值：projected Provider input `192K` 触发、普通情况以 `64K` 为 post-compaction soft ceiling。该 policy 记录为 D55，并保留按真实 coding 长度分桶强制复评的义务。
- 2026-07-20：学习者说明尚未逐字 review summary prompt，并要求没有特殊理由时继续沿用 Pi。重新核对冻结源码后没有发现 DeepSeek 或当前内存边界要求改写 prompt；因此首次/update/split-turn 三套 prompt、对话序列化、tool-result summary truncation、file-operation tags 与 synthetic user-message projection 按 Pi 建立首版基线，排除 branch/extensions/persistence，并记录为 D56。
- 2026-07-20：学习者确认首版 budget allocation 参考 Pi，并要求把 `64K` 澄清为不要求填满的 soft ceiling。课程采用 `20K` retained raw、`13,107` initial/update summary、`8,192` split-turn prefix 和 `4,096` Provider safety；同时明确记录这组值可能不足以支撑真实大型项目，尤其未来 Skills 会改变 context 组成。当前不解决或自动调参，待 Skills 引入及真实长项目验证时强制复评。该决定记录为 D57。
- 2026-07-20：学习者要求开始实现，并在实现后展开说明无精确 tokenizer 时如何判断下一次请求达到 `192K`。实现采用最后有效 Provider usage 加尾部 `ceil(characters / 4)`、无 usage 时完整 request fallback，并在 compaction 后使旧 usage 显式失效；所有权和精度边界记录为 D58。
- 2026-07-20：Lesson 08 首版实现和最终审查完成。request-local output clamp、between-Runs lazy compaction、Pi prompts、message-level cut、重复 summary、完整 History/Working Context 分离以及失败、取消、并发和 protocol 校验均有确定性测试；审查把连续 cut point 前移的重复线性扫描收敛为二分查找，并补齐 projected input 恰好等于 threshold 的测试。`make check` 与 `go test -race ./...` 全部通过，课程进入待理解确认且尚未提交。
- 2026-07-20：学习者要求把 Lesson 08 直接提交并推送到 `main`；提交 `c967027` 已推送，未创建 feature branch 或 PR。学习者随后明确要求开始 Lesson 09。
- 2026-07-20：Lesson 09 开课源码校准确认渐进披露主线：冻结 Pi 启动时只把 Skill name、description 与 location 放入 system prompt，模型匹配后通过普通 `read` 获取 `SKILL.md` 正文。旧提纲同时被收紧：完整来源发现属于 ResourceLoader/package/settings/trust 组合，而 Pi 的 `read` 可读绝对路径、pi-go 的 `read` 严格限制在 workspace；因此全局 Skill 不能在不改变读取安全边界的情况下机械移植。课程先讨论 project-only Skills 与外部 trusted roots 的边界，再形成实现决定。
- 2026-07-20：学习者明确推翻 Lesson 09 的 project-only 候选并统一产品名为 Pia：`read` 必须接受 workspace-relative 与 workspace 外 absolute paths，Skills 是 Pia 核心需求而不是 Pi 式 extension 附属能力。对应产品、读取和 Skills 决定记录为 D59–D61。
- 2026-07-20：继续核对 Agent Skills specification/client guide、Claude Code 与 Codex 官方文档后，Lesson 09 从 Medium 修正为 Large 的核心生命周期闭环：直接兼容 `.agents/skills` 与 `.claude/skills` 的 portable 内容，同时保留 `.pia/skills` native root；厂商私有 runtime extensions 明确不伪装成 portable semantics。普通 read 与 dedicated activation、同名 precedence 和 catalog budget 留待实现前讨论。
- 2026-07-20：D59/D60 前置改造完成：system prompt identity 统一为 Pia；`read` 对 relative path 保留 `os.Root` containment，对 absolute path 使用 host nonblocking open 并校验实际 regular-file handle。项目内/外 absolute file、absolute symlink、relative escaping symlink、absolute directory/FIFO、分页、UTF-8、取消和 concurrency 契约均有测试；`make check` 与 `go test -race ./...` 通过。
- 2026-07-20：学习者指出沟通后的 Skills 课程可能已经过大，并同意需要时逐课拆分。课程按 D62 将当前 Lesson 09 收敛为 discovery/bounded catalog（Medium），把 activation、dedupe 与 compaction continuity 移到未开始的 Lesson 10（Large）；absolute-read 只作为已完成前置，不另占课次。
- 2026-07-20：学习者正式重新开始拆分后的 Lesson 09，并澄清 Claude/Codex compatibility 只面向当前项目，不发现全局 Codex/Claude Skills。重新核对官方语义和当前 Pia 后，D61/D62 与课程大纲改为 selected-workspace-only；`AGENTS.md`/`CLAUDE.md` 被确认是 Lesson 06 已有最小支持、未来独立增强的 project-instructions 能力，而不是 Skill discovery 的一部分，记录为 D63。
- 2026-07-20：学习者进一步收紧 Lesson 09：当前只做 Pia 自己的 project-local Skills，不做 `.agents`/`.claude` community compatibility，也不因 Skill 可能包含大量 resources 与厂商 runtime 而一次性扩建完整引擎。课程改为 Pia Skill v1 discovery/catalog 加普通 `read` 基础使用闭环；managed activation 留在 Lesson 10，完整 Agent Skills、community/global scopes 与 resources 进入后续未编号方向，记录为 D64。
- 2026-07-20：Lesson 09 按 D65 完成实现与最终审查：project-local snapshot、YAML metadata、宽松 name diagnostics、重复名 lexical winner、catalog/diagnostic ceilings、普通 `read` 基础使用、trace 和 success-time stderr warnings 均有确定性测试。审查额外拒绝重复 YAML mapping key 并收紧 unknown-field warning；`make check` 与 `go test -race ./...` 全部通过，课程进入待理解确认且尚未提交。
- 2026-07-21：学习者确认理解 Lesson 09，并明确要求通过 feature branch 提交 PR；Lesson 09 至此结束，Lesson 10 仍未开始。
- 2026-07-21：PR review 发现两个边界实现仍会在限制生效前改变或放大输入：absolute `read` 在 host open 前 lexical-clean path 会改变 symlink parent 后的 `..` 解析目标，`fs.ReadDir` 则会在 64-candidate ceiling 前物化整个 Skill directory。实现改为保留 absolute input 的 host path 语义，并为 Skill source 增加 256-entry input ceiling；两项均补充回归测试。
- 2026-07-21：复审发现输入侧枚举改造直接 `Open` Skill source 时，source 自身若为无 writer FIFO 仍可能在类型诊断前阻塞。Skill source 改为复用 supported-platform nonblocking open 并通过 opened handle 验证 directory 类型，新增 source-FIFO 回归测试。
- 2026-07-21：后续复审发现 CLI 以 `%s` 输出来自 untrusted directory name 的 diagnostic path，控制字符可伪造额外日志行或操纵终端。stderr warning 改为 quoted path，并加入 newline、carriage-return 与 ANSI escape 回归测试。
- 2026-07-21：再一轮复审指出同类控制字符也可能经 filesystem error 进入 diagnostic message。修复提升为输出边界的类修复：CLI 同时 quote path 与 message，回归测试覆盖两个字段，因此不再依赖每个上游 producer 单独完成 sanitizer。
