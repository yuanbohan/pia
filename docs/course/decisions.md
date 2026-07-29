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
- 决定：导师持续质疑学习者的设计假设，同时复查自己的既有判断。讨论必须区分学习者假设、已验证的 Pi 契约、候选 Go 机制和已确定的 Pia 决策；冻结 Pi 的源码、文档、测试和可复现实验优先于对话中的赞同。每个新概念必须先完成术语、源码路径和例子讲解，再进行理解检查。
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
- 原因：保留无副作用 read 的简单并行能力，同时避免把整个混合批次并行造成读写和命令竞态。该策略是 Pia 对 Pi 整批调度的有意差异。

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
- 原因：当前 Agent Loop 只观察内容、用量和终态；其余字段属于 Pi 的多 Provider、路由、Session/UI、诊断或费用生态。第 02 课核对后没有发现 Pia 第一阶段需要完整 partial snapshot 的消费者；第 04 课若核对 DeepSeek 后出现新的 usage/trace 需求，再用证据扩展内部协议。

### D26. Partial assistant 不属于正式 transcript

- 日期：2026-07-16
- 决定：第一阶段正式 transcript 对每个 Provider turn 只追加一条 terminal final/aborted assistant message，不保存、持久化或暴露可查询的完整 partial assistant view。第 03 课增加的 tool-result settlement 不属于 partial 或第二条 assistant；error/aborted terminal 中已完成但未执行的 tool calls 会在该 assistant 后得到同 ID not-executed results。formation events 与实时展示明确推迟到未来 TUI 或其他交互式消费者，不在第一阶段建立 event sink。
- 原因：冻结 Pi 的低层 loop 会把 partial 临时放进私有 working context，但高层 `Agent.state.messages` 只在 `message_end` 追加 terminal message，并用独立 `streamingMessage` 服务 UI。Pia 第一期是 headless Runtime，没有必须查询完整 partial snapshot 的消费者；把权威 transcript 与临时展示状态分开可以避免半成品 text、thinking 或 tool-call 参数成为下一轮上下文事实。

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
- 原因：Pi 高层 `Agent.prompt()` 不接收外部 signal，而是在 prompt 被接受后创建内部 `AbortController`，所以没有完全对应的预取消调用；Pi 低层则会在 Provider 观察 signal 前把 prompt 加入 context。Pia 需要同时遵守 Go 的预取消 context 不启动副作用惯例，以及 Pi 对已接受 prompt 保留 user 与 aborted assistant 的行为。

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
- Pi 分歧：冻结 Pi 的 Agent Loop 可能把未执行调用留成 orphaned tool calls，再由 `packages/ai/src/api/transform-messages.ts` 在下一次 Provider request 中临时插入 `No result provided`，不写回 Agent state。Pia 不复制这层隐藏修复，而是在权威 transcript 中显式闭合调用。
- 原因：Pia 已确定同一 Agent 跨顺序 Run 保存完整 transcript，并把它作为 Provider history 的单一事实源。显式 settlement 让 `RunResult`、Agent state 和下一次 Provider request 保持一致，也让取消后的对话可以直接继续。离线 Faux 测试和真实 DeepSeek compatibility smoke 都要专门验证这一分歧。

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
- Pi 分歧：冻结 Pi 的默认 operations 使用 `fsAccess(R_OK)` 后直接 `fsReadFile`，没有 regular-file/FIFO 校验、nonblocking open 或相应 FIFO 测试；因此它不是用 Pia 的 opened-handle 方案解决该问题。冻结 Pi 还允许绝对路径和可清理的 `..`、整文件读入后 `split/join`、非法 UTF-8 replacement、总行数提示、图片/UI/remote operations，并在首行超限时提示 bash fallback。Pia 第一期选择 root containment、实际 handle 校验、macOS/Linux FIFO 非阻塞拒绝、精确文本视图和流式有界输出，不移植这些能力，也不建议通过 bash 绕过文件工具边界。
- 原因：Agent 后续会把 read result 直接写入 transcript，并用其中的原文驱动 exact edit；路径、内容、错误和并发行为都必须在进入模型前可预测、有界且与磁盘对象一致。只抽取已有真实消费者的聚焦 helper，避免为尚未讨论的工具建立猜测性接口。

### D38. write 只替换普通文件，并在固定父目录内原子提交

- 日期：2026-07-18
- 能力边界：`write(path, content)` 创建或完整覆盖普通文件并递归创建父目录；空内容合法，不增加计划未要求的 content 上限。目录、最终 symlink、FIFO、socket 和 device 都拒绝，创建或直接操作这些对象属于 bash 的显式能力。workspace 内 ancestor symlink 可以使用，逃逸或 dangling ancestor 由 `os.Root` 拒绝。
- 提交边界：先以 `root.OpenRoot(parent)` 固定实际父目录，再在该 root 内 `Lstat` 最终 component、创建随机 `O_EXCL` 临时文件、写完并关闭，最后以同目录 rename 提交。这样 ancestor swap 不会让临时文件、目标和清理落入不同目录；最终 symlink 在检查时直接拒绝，在检查后才出现则由 rename 替换 link entry 而不是跟随 referent。rename 前失败或取消删除临时文件并保留原目标，父目录创建不回滚；rename 后不再因 context 取消反报失败。
- 可观察语义：替换保证运行时读者只看到旧内容或完整新内容，不通过 `fsync` 承诺 crash durability。新文件 `0666`、父目录 `0777` 并服从 umask；覆盖保留九个 permission bits，但新 inode 不保留原 hard-link relationship、owner、ACL、extended attributes 或特殊 mode bits。成功文本只返回规范化相对路径和 Go string 的实际 UTF-8 byte length，不回显 content。
- Pi 分歧：冻结 Pi 递归建目录后直接 `fsWriteFile`，会跟随最终 symlink、直接截断目标，并用全局 per-path mutation queue；其成功文本把 JavaScript UTF-16 `content.length` 标作 bytes。Pia 使用 Agent serial barrier、`os.Root`、普通文件限制和替换提交，不复制额外 mutation queue，并修正 byte count。
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
- Pi 证据与分歧：冻结 Pi commit 的主协议同样是 `edits[]`，所有项在原文件上匹配、要求唯一且不重叠，并在写入前完成整组校验；但其 exact 失败后会进行空白、Unicode 与行结束符 fuzzy normalization，还兼容旧单编辑参数和字符串化 `edits`，生成 TUI preview/diff/patch，支持可替换 operations，并由 mutation queue 后直接 `writeFile`。Pia 第一期不移植这些扩展。
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
- 决定：未来已编号课程表至少记录解锁能力、Pi 大致做法、Pia 当前边界与非目标、结束信号、依赖、相对规模和状态。规模使用 Small、Medium、Large、XLarge；XLarge 不能直接开课，必须先讨论并拆成能独立讲解和验收的闭环。越远的阶段信息越粗，不因为表格列更多而提前设计实现细节。
- 原因：这些字段让后续实施者理解“为什么这样排”和“何时算完成”，同时保留根据真实源码与代码推翻早期假设的空间。把多个状态所有权、生命周期或消费者塞进一课，会让讲解、review 和验收同时失焦。

### D45. 以 Pi parity 为能力下限，并用受控评测追求稳定超过 Pi

- 日期：2026-07-19
- 决定：完成 coding-relevant Pi 能力覆盖后，建立稳定、可重复的 Pia 与冻结 Pi 对照评测。Pi parity 是不可退让的能力下限，稳定超过是演进目标。公平比较固定模型/Provider profile、任务与初始仓库并做多次独立运行；以客观 resolve rate 为主要能力指标，同时记录成本、turn、时延、tool error、恢复和长上下文表现，并把协议完整性与安全回归作为门槛。其他优秀开源 coding agent 以及 Codex、Grok 的可获得证据只提供候选机制与工程经验，不替代 Pi 对照组。
- 范围：当前只确定目标和公平性原则，不提前确定 benchmark corpus、runner 架构、统计阈值、trace schema 或产物存储。Lesson 06 的本地 fixture 仍只是第一期验收；正式评测方向在前置能力稳定后另行拆课。
- 原因：项目目标是先确保不弱于 Pi，再通过可验证机制变得更强，而不是仅达到文件结构或功能清单相似。没有同模型、同任务、重复运行和客观判据，“达到或超过 Pi”都只能是主观印象，不能指导后续优化。

### D46. 完整 Conversation History 与 Core Agent Working Context 分属不同 owner

- 日期：2026-07-19
- 决定：Lesson 07 采用外层最小内存 Conversation Owner 保存完整、有序的 Conversation History；Core Agent 只拥有可替换的 Working Context，并继续负责 Provider/tool loop；每次 Provider 调用使用由 Working Context 构造的 ownership-independent Request Snapshot。这里的 Conversation Owner 是语义责任，不预先要求同名 Go 类型或独立 package。
- 范围：当前决定不引入持久化 Session、entry tree、branch、compaction 算法、事件订阅或 TUI。Conversation Owner 与 Core Agent 之间的同步、首份 package 归属、active 边界、消息复制和 Working Context replacement 分别采用 D47 至 D51。
- 原因：冻结 Pi 在 `message_end` 后先把 terminal assistant 保存进完整 Session entries，再由外层在 threshold/overflow compaction 后替换 `agent.state.messages`；overflow error 可以保留在完整历史中，却从 retry 的 Working Context 中移除。当前 Pia 的单 `transcript` slice 在无 compaction 时成立，但继续合并会迫使 Lesson 08 在“丢失原始历史”和“压缩不减少模型输入”之间二选一。只确定最小内存 owner 边界可以解决这项真实张力，同时避免提前复制 Pi 的完整 Session 基础设施。

### D47. Core Agent 以同步 run-local message delta 提交一次 Run 的新增消息

- 日期：2026-07-19
- 决定：一次 accepted Core Agent Run 在 settlement 后同步返回该 Run 新增的完整有序 message delta，并同时返回独立的 Go error；Conversation Owner 无论 error 是否为 nil，都先把 ownership-independent delta 一次性追加到 Conversation History。acceptance point 前取消或 concurrent Run rejection 返回空 delta且不修改 History。当前同步路径使用普通函数返回，不引入 Go channel、message callback 或完整 Agent event stream。
- 范围：这里确定的是 settlement 语义，不预先锁定 Go struct/field 名；课程使用 `NewMessages` 表示该 delta。History 在当前最小内存实现中以 settled Run 为提交边界，不承诺 active Run 中途可观察或进程崩溃恢复。实时持久化、TUI progress、extensions、steering/follow-up 和多订阅者事件仍在后续真实消费者出现时设计。
- 原因：冻结 Pi 的低层 `runAgentLoop()` / `runAgentLoopContinue()` 已经独立维护并返回 `newMessages`，同时另行发出逐消息和 `agent_end` 事件；前者表达 Run 结果，后者服务 `AgentSession` persistence、extensions 与 UI。Pia 当前只有 settlement 后的 Conversation History 消费者，直接返回 delta 足够；提前使用 channel/callback 会额外引入 close、backpressure、drain、panic、reentrancy 和 listener settlement 契约，却没有当前收益。

### D48. 首份 Conversation Owner 是 coding-owned 私有实现

- 日期：2026-07-19
- 决定：首份 Conversation Owner 实现在现有 `internal/coding` package 的独立 `conversation.go` 中，使用未导出的类型协调具体 Core Agent 与完整内存 Conversation History。当前不创建 `internal/conversation`、`internal/session` 或同名公共抽象，也不把完整 History 的所有权放回 `internal/agent`。
- 范围：这是第一个真实 Coding Agent 消费者的 package 归属，不宣称 research、OCR、web search 等未来应用必然复用同一种 history policy。出现第二个非 coding 消费者时，必须从双方已经证明相同的责任重新审查 package 布局，再决定是否提取共享实现或接口；概念上的通用性本身不是提前建包的证据。
- 原因：冻结 Pi 同样把持久 conversation/session 协调放在 coding-agent 的 `AgentSession`，而不是低层 agent loop。Pia 当前只有 Coding Agent 需要该责任；将私有实现放在 composition 所在应用层，既保持 Core Agent 通用，也避免为尚不存在的消费者设计宽泛 API。独立文件让 history ownership 与 `runtime.go` 的一次性装配责任保持收敛。

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
- Pi 差异：冻结 Pi 通过赋值 `agent.state.messages` 替换 working messages，setter 只复制顶层数组；coding-agent 的实际 compaction replacement 位于 `agent_end` 之后、下一次 prompt 之前。Pia 保留这一 idle-time 使用时序，但用显式 Go 方法、active guard 和深复制收紧所有权，而不暴露可变 state object。
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
- Pi 差异：冻结 Pi 通常在 `agent_end` 后立即检查 threshold，并在新 prompt 前补做检查。Pia 当前 one-shot composition 没有独立 compaction status/event，采用 pre-next-Run lazy 时机可以避免无后续调用时的额外模型成本，也不会把上一次已经成功的 Run 因后处理失败改写成 error，同时仍保持“settled Run 后、下一次 Provider call 前”的外部语义。
- 原因：当前最小 API 用 `RunResult + error` 表达一次 user input 是否被接受并完成，没有另一个通道承载 eager compaction failure。lazy 检查让失败明确归属于尚未接受的新 Run，旧的 settled result 与完整 History 都保持稳定。

### D55. 首版 between-Runs compaction 使用 192K threshold 与 64K soft ceiling

- 日期：2026-07-20
- 决定：Lesson 08 以 projected Provider input `192_000` tokens 作为首版 between-Runs quality-oriented compaction threshold；`64_000` tokens 是 compaction 后完整 projected Provider input 的普通情况 soft ceiling，不是 summary size、必须填满的目标或成功所需的硬上限。projected input 包含稳定 system prompt、tool schemas、synthetic summary、retained raw suffix 与尚未接受的新 user input。`1_000_000` model context 仍只表示 Provider hard capacity，不作为日常 coding target。
- 证据与折中：DeepSeek V4 官方 MRCR 证据显示到 `128K` 基本稳定、之后开始可见退化，而 `256K` 已明显下降；它证明不能把 1M 容量当质量区间，但不足以把 128K 认定为 coding 精确最优点。OpenAI 对 Codex agent loop 的说明和其链接源码显示 Codex 默认以 model context 的 `90%` 触发 auto-compaction，`272K` coding context 对应约 `244.8K`；GPT-5.3-Codex system card 中一项以最大化长任务表现为目标的评测则每 `100K` 触发。Pia 的普通文本 summary 又弱于 OpenAI 模型原生 opaque compaction item，因此 `192K` 在容量、压缩频率与质量风险之间取中间偏保守位置；达到 `64K` ceiling 时提供约 `128K` 的再增长空间。
- Soft-ceiling 语义：cut selection 应尽量使 candidate 不超过 `64K`，但不得为了凑该数值丢弃没有进入 summary 的消息。不可压缩的新 input、固定 prompt/tools、单条大 message、实际 summary 或 protocol-safe granularity 使 `64K` 无法达到时，允许以高于 `64K` 但低于 `192K` 的 candidate 成功；连 threshold 都无法降到时按 D54 原子失败，新 input 不被接受。
- 范围：这是 D54 lifecycle 下的首版产品 policy，不是 active Run hard ceiling，也不改变本课不做 run 内 compaction、overflow compact-and-retry 或模型 registry 的范围。它不宣称一次 active Run 不会从低于 threshold 增长到更高区间。
- 复评义务：真实 DeepSeek coding traces 可用后，必须按 `<128K`、`128K-192K`、`192K-256K` 与 `>256K` 分桶比较任务成功率、重复 compaction 后的信息损失、成本和频率。`128K-192K` 已显著退化时下调；只有 `192K-256K` 质量稳定且 compaction 过频时才上调。更换模型、reasoning mode、tool schema、Skills 暴露、summary prompt/表示或 tool-result bounds 时必须重新校准。

### D56. 首版 summary prompt 与 model-visible 表达沿用冻结 Pi

- 日期：2026-07-20
- 决定：没有 DeepSeek 或 Pia 证据要求偏离时，Lesson 08 原样沿用冻结 Pi commit `dcfe36c79702ec240b146c45f167ab75ecddd205` 的 summarization system prompt、首次 structured checkpoint prompt、已有 summary 时的 update prompt、split-turn prefix prompt，以及 `<conversation>` / `<previous-summary>` 输入组织。待摘要消息按 Pi 规则序列化为带 role labels 的纯文本，每个 tool result 在 summary input 中最多保留 `2000` characters；从 tool calls 确定性提取的 read/modified file lists 追加到模型 summary。
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

### D59. 产品、仓库与 Go module 统一命名为 Pia

- 日期：2026-07-20
- 修正日期：2026-07-21
- 决定：产品、repository、workspace directory 约定和后续课程讨论统一使用 **Pia**；repository 为 `yuanbohan/pia`，Go module/import path 为 `github.com/yuanbohan/pia`。2026-07-21 的明确迁移指令取代此前暂缓 technical-path migration 的边界。
- CLI 边界：`pia` 不再被描述为临时产品名；`cmd/pia` 是当前本地入口，但其参数、输出协议、部署形态与公共 SDK 承诺仍不稳定。
- 迁移边界：内部 imports、operator-facing environment variables、临时文件前缀以及现行课程、计划和引用同步使用 Pia 命名；不保留旧项目名的兼容 alias。带日期的历史实现记录可保留迁移前 literal，但必须同时标明迁移日期与当前值，且不构成现行契约。冻结 Pi 的源码、链接和来源说明仍使用上游名称。
- 原因：repository 与远端已使用 Pia；统一 module、imports 和文档可以消除同一项目的双重身份，同时继续明确区分上游 Pi 基线与本项目 Pia。

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
- Discovery 与解析：每个 Conversation 只在创建 Core Agent 前读取一次 selected workspace 的 `.pia/skills`。source 必须是直接 directory entry，不能是 symlink：先经 `os.Root.Lstat` 检查，再以 supported-platform nonblocking policy 打开 directory handle，最后以第二次 non-following lookup 和 `os.SameFile` 验证 handle/entry identity，避免 check/open 竞态把 symlink target 激活。该 verified source handle 保持到 metadata snapshot 完成；每个 direct Skill directory 与其 `SKILL.md` 都通过 Darwin/Linux `openat` 相对父 handle 打开，并以 `O_NOFOLLOW`、`O_NONBLOCK` 与 opened-handle type validation 固定整条读取链，source path 后续被 rename、replacement 或 symlink 替换都不会混入另一 source 的 metadata。source enumeration 在输入侧最多读取 257 个 direct entries，超过 256 时整个可选 Skill source 以一条 warning 忽略。未超限时先按目录名 lexical sort，再最多检查 64 个直接、非 symlink Skill directories。没有 watcher 或 Run 中途 reload。discovery 只从最多 16 KiB 的 prefix 提取 YAML frontmatter，使用 `go.yaml.in/yaml/v3`，只消费 string `name` 与 `description`；其他字段 warning 后忽略，正文及 supporting files 不被解析、验证或注入。
- Validation：缺少/空 name、缺少/空 description、无法解析的 YAML、重复 mapping key、非 string 必需字段、不安全目标、非 UTF-8 direct Skill entry name 或超限 frontmatter 跳过。Agent Skills 的 1–64 个小写字母/数字/连字符、无首尾/连续连字符并与目录名一致仍是规范诊断；Pia 对 65–256 characters、字符形式或目录 mismatch 只 warning 并加载 frontmatter name，超过 256 characters 才按 hard safety 跳过。同名 Skill 选择 workspace-relative location lexical 较小者。description 超过 1024 characters 时只在 catalog 中截断并 warning。
- Catalog budget：稳定 system prompt 只加入 XML-escaped name、description 和 workspace-relative `SKILL.md` location，并指导模型匹配时用现有 `read` 获取完整文件；没有有效 Skill 时整个 section 省略。catalog 使用 D58 的 `ceil(characters / 4)` 估算，最多 4096 estimated tokens；超限时先统一缩短 descriptions，仍放不下才省略 lexical tail entries，所有实际裁剪都有 warning。该值只是首版 ceiling，必须随真实大型项目和 Skills-enabled context 分桶继续复评。
- Diagnostics 与 trust：单个 Skill 或整个可选 source 的发现失败不阻塞普通 coding task。最多保留 64 条有界 `SkillDiagnostic`，进入内部 `RunResult` 和可选 trace；`cmd/pia` 只在 Run 和 trace 都成功后把它们作为简短 warning 写入 stderr，并在唯一输出边界同时 quote untrusted path 与 message，避免任何 producer 遗漏的控制字符伪造日志或操纵终端。首版没有 trust UI 或逐 Skill approval；选择 workspace 是操作者的 trust decision，metadata 可能自动发送给 Provider，完整 `SKILL.md` 在模型选择普通 `read` 后可能发送，Skill instructions 可以进一步引导现有高权限 tools。

### D66. Lesson 10 延后重复 Skill activation 去重

- 日期：2026-07-21
- 决定：Lesson 10 继续实现 stable Skill identity、structured activation、bounded instructions 与 compaction 后的 durable model-visible continuity，但不实现或验收重复 activation 的短路、`already active` 结果或重复原始 tool result 抑制。该决定收紧 D61/D62 中把 dedupe 与 activation/continuity 同课完成的旧范围；完整 activation dedupe 进入 Lesson 10 之后重新编号的后续能力。
- 保留约束：dedupe 延后不代表 durable projection 可以随重复调用无限复制。重复 activation 如何更新受保护状态、projection 是否按 stable identity 保留一个当前 snapshot，以及再次读取是否允许更新正文，仍是本课实现前必须讲清并形成决定的 bounded-state 问题，但不扩成用户可见的完整 dedupe lifecycle。
- 预算义务：system prompt、Skill metadata、tool schemas、synthetic summary、retained suffix、新 user input 与任何 compaction-protected Skill instructions 都属于 projected Provider input。Lesson 10 必须先确定单 Skill 与全部 active instructions 的预算及超限失败语义；不能把 durable instructions 当作 compaction 之外的免费状态。
- 原因：本课的首要教学和实现目标是先建立 activation identity 与 Working Context replacement 的 continuity。把重复调用行为、重新加载策略与完整去重验收同时加入会掩盖 compaction 所有权主线；预算和有界 projection 则是状态安全约束，不能随 dedupe 一并延后。

### D67. Lesson 10 保留重复 Skill activation 去重

- 日期：2026-07-21
- 决定：D67 supersedes D66 的临时范围收缩。Lesson 10 同时完成 stable Skill identity、structured activation、bounded instructions、重复 activation 去重与 compaction 后的 durable model-visible continuity。同一 Conversation 内首次成功 activation 建立一份 durable snapshot；重复 activation 不再次注入完整正文，compaction projection 对同一 stable identity 最多保留一份。第二次调用返回何种小型结果、是否支持显式 refresh 以及文件变化语义仍需在实现前教学和讨论，不由本决定预先指定 API。
- 范围：dedupe 只针对当前 Conversation 的 Skill activation 与 protected projection；不引入跨 Conversation cache、持久化 activation、全局 Skill 状态、manual deactivate、版本管理或 watcher。新 Conversation 重新开始 activation lifecycle。
- 原因：一旦 activated instructions 被保护并跨 compaction 持续占据 Provider input，重复正文就会成为无法由普通 compaction 回收的固定成本。Stable identity、dedupe 和 continuity 因而共享同一有界状态不变量；把去重延后会让本课完成一个可无限放大的 durable projection，违背 context budget 与最小可靠闭环。
- 后续教学纠偏：D67 的“一份 durable snapshot / projection”不应被解释为完整正文永久 model-visible。学习者指出多个历史 Skill 永久 pinning 会造成不可回收的 token 成本和过期相关性；当前候选改为区分 Conversation-scoped snapshot 与 Working Context residency，并在正文被 compaction 切掉后按需精确 rehydrate。逐 Skill dormant receipt 与通用 reactivation 提示之间尚未形成新决定。

### D68. Lesson 10 采用 Grok Build 风格的无状态按需 Skill activation

- 日期：2026-07-21
- 决定：D68 supersedes D62 中要求 durable model-visible activation projection 的 Lesson 10 条款，并 supersedes D67。Pia 增加 project-local `skill(name)` dedicated tool；每次调用按 Conversation 启动时的 bounded catalog snapshot 解析 name/location，读取调用时的当前 `SKILL.md`，并返回有界、结构化的完整 instructions。一次 activation 只是一次 tool invocation，不建立长期 active 状态。
- Compaction：Skill result 是普通 tool result。它落在 retained suffix 时保留原文，落入被压缩 prefix 时只由现有 summary 表达；不增加 protected Skill block、dormant receipt、activation registry projection、frozen body snapshot、自动 reactivation 或 Skill-specific compaction policy。稳定 catalog 已位于 system prompt，compaction 后无需按“曾激活”集合增加第二份 model-visible metadata。
- 重复调用与 freshness：不做跨调用 activation dedupe、`already active` 短路或 hidden residency tracking。重复 `skill(name)` 重新读取当前文件并再次返回正文；文件在两次调用间变化时，第二次调用观察新内容。删除、替换、非 regular file、非 UTF-8、超限和读取失败必须产生有界且保因的明确 tool failure，而不是回退到旧 cache。
- 采用依据：冻结 Pi、OpenCode、Codex open source 和 Grok Build 均未把所有曾加载 Skill 正文永久投影到 full-compaction 后的 model context。Grok Build 的 dedicated tool + current-file read + ordinary compaction 与 Pia 当前 headless tool loop、Lesson 09 metadata-only catalog 和 project-local safety boundary 最吻合；OpenCode 的 instance content cache 会重新打开 Lesson 09 已关闭的全量正文预取/缓存问题，Codex 的显式 mention 与 Pi 的 slash/read 路径则会引入当前没有的 user invocation surface，或继续缺少 first-class Skill tool identity。
- 范围与规模：Lesson 10 从 Large 收敛为 Medium，只完成 `skill` tool、bounded body/result、当前文件读取、composition 接入和普通 compaction 验证。不加入 slash syntax、community/global roots、supporting resources runtime、plugins、MCP、watcher、manual refresh/deactivate、跨 Conversation cache 或 Session persistence。以后只有真实 trace 证明重复调用造成质量或预算问题时，才重新评估 dedupe；不为假设性问题预建 lifecycle。
- 学习者确认与验证：学习者于 2026-07-21 确认该方向符合其对 compaction 的理解，并接受先实现、后验证效果。离线 Faux tests 只证明 D68 契约；真实效果必须在固定任务和模型配置下，对相关、无关及跨 compaction 再使用场景做多次对照运行，记录 Skill 选择、instruction adherence、任务完成、调用次数和输入成本。单个 fixture、单次模型运行或主观 review 不能证明设计更好，也不能据此增加 dedupe 或 durable Skill state。

### D69. Skill activation 使用 50 KiB 完整结果上限和可恢复单调用失败

- 日期：2026-07-21
- 决定：一次成功的最终 model-visible `skill` structured result 以 UTF-8 计最多 `50 * 1024` bytes，预算覆盖 result envelope 与完整 instructions。成功必须表示正文完整；不得把截断内容包装成 activation success。超过上限时返回有界、保因且 `IsError=true` 的 call-local tool result，明确说明未激活、实际大小、上限和稳定 workspace-relative location。D20 继续适用：该错误不结束 Run 或 Conversation，同批后续 tool stages 和下一轮 Provider call 继续。
- 恢复语义：超限文件不进入永久 blacklist 或失败 cache。模型可以通过现有 `read` 按 offset 分页查看原始 `SKILL.md`，作者可以把核心 instructions 留在主文件并把条件性细节移到普通 reference files；文件缩小或重构后，下一次 `skill(name)` 重新读取当前文件。分页只是一条明确降级路径，不能被表述为已经完成完整 activation。Lesson 10 不为已经存在且可分页读取的 source 再创建 OpenCode 风格临时副本，也不增加 paged activation API。
- 作者指导：主 `SKILL.md` 以少于约 5000 estimated tokens 和不超过 500 行为非强制目标；500 行不是 validity hard limit，也不另设运行时行数 ceiling。Pia Skill v1 仍不建立 supporting-resource engine，references 只是通用 tools 可按 Skill 明确引用读取的普通项目文件。
- 阈值复评：`50 KiB` 是首版 safety ceiling，不是 Agent Skills 标准或永久常量。实现稳定后按真实 Skill 大小分布、超限率、恢复成功率、Provider input 成本、instruction adherence 和任务完成质量复评；调整必须有多次受控运行或真实 trace 证据，不能只因为 Provider context capacity 更大就上调。
- 学习者确认：学习者于 2026-07-21 接受该首版语义，并明确要求以后根据效果再调整阈值。

### D70. Skill catalog identity 按 Conversation 冻结，activation 正文按调用读取当前位置

- 日期：2026-07-22
- 决定：Conversation 启动 snapshot 只冻结 model-visible catalog name 到 direct Skill directory/location 的 lookup mapping，不冻结正文 bytes 或长期 file handle。每次 `skill(name)` 都从当前 `.pia/skills` source 重新走 direct-directory/final-file handle chain，并读取该 location 当时的正文；同路径的 body edit 或 regular-file replacement 在下一次调用生效。catalog name、description、winner 或 source topology 的重新发现只发生在新 Conversation，Lesson 10 不加入 watcher、reload 或 per-call full discovery。
- Current metadata：activation 只重新识别有界 frontmatter delimiters 以分离正文，不重新解析 current name/description，也不要求 current frontmatter name 等于 frozen catalog name。此前课程记录中的 identity-drift failure 候选撤回。文件或目录消失、类型不允许、delimiter 无法识别、正文非 UTF-8、result 超限或实际 I/O 失败仍按 D68/D69 形成 call-local failure，不回退 discovery 时正文或任何 stale cache。
- 依据：学习者希望本地修改的 Skill 在下一次调用立即生效；Agent Skills client guide 明确把 activation-time body read 作为观察两次 activation 之间文件变化的实现选项。核对的冻结 Pi、当前 Codex 与 Grok Build path-based reread 均未普遍增加 current-frontmatter-name equality check，OpenCode 则明确选择 instance body cache。为 Pia 单独加入 metadata authentication 会把稳定 lookup 误解为第二次 discovery，并在没有横向证据的情况下拒绝同路径最新正文。
- 并发边界：handle-relative no-follow open 固定一次 path lookup 得到的对象并抵抗 path replacement/symlink 竞态，但不承诺另一个进程对同一 inode 原地改写时的事务性 byte snapshot。原子替换会让一次调用读取旧文件或新文件之一；Lesson 10 不增加跨进程锁、版本号或文件 watcher。

### D71. Lesson 10 明确不支持 activation dedupe

- 日期：2026-07-22
- 决定：Lesson 10 的同名重复 `skill(name)` 调用不会短路、合并或返回 `already active`；每次调用都重新读取 current file，并各自形成完整或失败的普通 tool result。不维护 Conversation active set、content hash、body/failure cache 或 transcript/projection dedupe。Lesson 09 对 duplicate catalog names 的 lexical winner selection 继续保留，但它是 discovery conflict resolution，不是 activation dedupe；通用 compaction 对旧重复结果的 summary 也不被称为 dedupe。
- 纠偏：此前只有在 durable protected body 设计下，dedupe 才是防止不可回收重复 projection 的必要有界状态约束。D68 已取消 protected body，普通 compaction 可以回收旧 result；继续保留 activation dedupe 反而需要追踪正文 freshness、Working Context residency 和 compaction 后 rehydration，重新引入本课已经删除的 lifecycle state。D71 因而不是再次延期 D66，而是确认当前无状态语义根本不包含该能力。
- 演进门槛：package boundary 可以容纳未来单独评估的 dedupe，但只有多次真实 trace 证明正文仍在当前 model input 时出现高频无意义重复调用，并且其输入成本显著影响任务结果，才重新编号、设计和评测。未来可扩展不等于当前支持，也不得倒推回永久 protected body。
- 学习者确认：学习者于 2026-07-22 明确认可当前方案足够简单、便于维护，并要求把不支持 activation dedupe 的边界记录清楚。

### D72. Skill 单调用上限不扩成 Skill-specific mid-Run aggregate policy

- 日期：2026-07-22
- 决定：D69 的 `50 * 1024` bytes 只约束一次最终 model-visible `skill` result，不承诺同一 assistant batch 或 Run 内全部 Skill/tool results 的总和永不超过 Provider context。Lesson 10 不增加 Skill-specific aggregate counter、每批调用数量限制、半批跳过、hidden reservation 或特殊 compaction trigger。
- 所有权边界：当前 compaction 只在下一次 accepted Run 开始前由 Conversation Owner 执行；Core Agent 在同一 Run 的 tool result 后直接继续下一次 Provider turn。要在这条路径增加 aggregate overflow prevention、mid-Run compaction 或 context-overflow retry，会改变通用 Agent Loop、tool settlement、Working Context replacement 与 Provider error recovery，而问题同样适用于多个有界 `read`、`bash` 和其他 tool results，不属于 Skill tool 的独立责任。
- 当前保证：每个 `skill` result full-or-error 且有界，所有成功/失败结果都进入现有 request estimation、Working Context 和普通 compaction 语义。Runtime 必须继续把单调用 ceiling 与完整 projected request capacity 分开；真实 trace 若出现 aggregate overflow，应在当时尚未编号、现已进入 Lesson 11 的通用 Runtime recovery 路径统一设计和验证，不能只在 Skill 层静默丢调用或正文。
- 学习者确认：在该边界被明确提出后，学习者于 2026-07-22 要求继续实现；Lesson 10 按此范围完成。

### D73. Lesson 11 收尾第二阶段，第三阶段按 Session Runtime 系列滚动展开

- 日期：2026-07-22
- 第二阶段边界：学习者明确要求开始 Lesson 11，并决定其实现完成后进入第三阶段。Lesson 11 只完成 explicit context-overflow recovery 的 closed loop：保留失败的 complete History，构造不含 eligible overflow error assistant 的可继续 Working Context，forced compact 后做一次 input-free Core continuation。它不吸收 generic Provider retry、semantic events、steering/follow-up、Session persistence 或 Orchestration；具体 projection representation、classifier placement 与 Run terminology 仍需按 Lesson 11 文档在实现前讨论确认。Recovery eligibility 由 D74 进一步收窄。
- 第三阶段主题：以“可观察、可控制、可持久化、可恢复、实例间隔离的 Session Runtime”为阶段目标，预先分为 semantic events、单 Session lifecycle、Provider retry、steering、follow-up、durable journal、clean restore、interrupted recovery 与 concurrent-instance isolation 九个阶段内 capability slots。Session Runtime 是未来 Orchestrator/Gateway/IM 的前置层，不在本阶段提前建设这些外层系统。
- 编号规则：只给当前最近且依赖边界已足够清楚的 semantic-events 课程分配全局 Lesson 12；后续 slots 暂不分配全局编号。每一课开课仍必须重新核对冻结 Pi 与当时 Pia 路径，并允许证据推翻、拆分、合并或换序；任何校准后成为 XLarge 的行都不能直接进入实现。
- 明确延后：Goal Runtime、公共 SDK、Gateway、gRPC、IM、Agent Manager/scheduler、TUI、worktree/GitHub、多用户/多仓库、完整 Agent Skills/community compatibility 与正式 Pi 对照 benchmark 都不因第三阶段大纲存在而视为已设计或已开始。
- 原因：Lesson 11 先补齐第二阶段内存 Conversation/Working Context/compaction 面对真实 overflow 时的恢复缺口，第三阶段再从 events 到 persistence/recovery 逐层建立长期 Session owner。这样既让每课有独立完成信号，也避免从 one-shot command 直接跳到网络 Orchestrator，或用一个 XLarge “Session 课”混合观察、控制、队列、存储、恢复和并发。

### D74. Lesson 11 只自动恢复不含 completed tool calls 的明确 overflow error

- 日期：2026-07-24
- 校准纠正：开课首轮把 Lesson 03 已建立的通用失败结算形状——error/aborted assistant 加 same-ID not-executed tool results——与 context-overflow 主路径合并，进而把 `E2 + N2` 描述成 Lesson 11 必须恢复的 failed terminal settlement group。源码复核与课堂追问确认，这不是当前 DeepSeek overflow 的常态，也没有真实 Provider 证据证明两者会相交。
- 决定：Lesson 11 的自动 recovery eligibility 除了要求有充分的 explicit context-overflow error evidence，还要求 terminal assistant 不含 completed tool calls。符合条件时，complete History 原样保留该 error assistant，retry projection 只排除该 assistant，再 forced compact 并 input-free continue 一次。如果一条 error terminal 一边命中 overflow 文本、一边含 completed tool calls 及其 not-executed settlements，本课不猜测性删除或重试，而是保留完整失败并返回错误。
- 既有契约：D33 的通用 tool-call settlement 不变。Provider 已完成 tool-call formation 后发生 transport failure、cancellation 或其他 terminal error 时，Agent 仍不执行这些 calls，并追加 same-ID not-executed results，使 Working Context 与 History 可直接复用。该能力解决 orphaned tool calls，不是 Lesson 11 的 overflow recovery 能力。
- 证据：当前 OpenAI-compatible Provider 在请求建立或 HTTP status error 时还没有 finish reason，因此形成不带 tool calls 的 error assistant；只有已经收到 finish reason 后的失败才可能保留完整 calls。冻结 Pi 的 overflow recovery 同样从 active state 移除单个尾部 error assistant。对当前产品路径，优先保留可证明的窄恢复边界，而不是为未观察到的 Provider 交集扩建 projection group 语义。
- 后续扩展：若真实 DeepSeek 或新增 Provider trace 证明明确 overflow terminal 可以合法携带 completed tool calls，再以该 wire evidence 重新讨论 recovery eligibility 与原子排除范围；不得只凭通用类型上“可能出现”提前扩大本课。

### D75. 正式 Session owner 必须重新分配而不是叠加 lifecycle 职责

- 日期：2026-07-26
- 当前边界：Lesson 11 的 Conversation guard 以 fail-fast 方式覆盖同一个 accepted user advance 的 Core Run、overflow recovery、History commit 与最终 snapshot；Core Agent guard 只覆盖一次 `Run`/`Continue` execution，并禁止 active 时替换 Working Context。两者保护同一 Conversation/Core 实例，不是同一台机器的全局 Session 锁，也不负责不同 Session 共享 workspace 时的文件副作用冲突。
- 第三阶段约束：进入单 Session lifecycle 课程时，必须重新阅读当时源码并追踪所有入口。如果 Session 独占 Conversation 与 Core Agent，且所有 user advance 都只能经过 Session，Session 应成为唯一外层 lifecycle authority，并优先评估吸收 Conversation 的 `active` 职责。Conversation 默认只保留 complete History/projection 所有权；Core Agent 默认只保留 Working Context 与一次模型/工具 execution。Core 本地 guard 是否继续存在，必须由独立 package contract 或具体并发 invariant 证明。
- 禁止事项：不得仅为了“多一层保险”让 Session、Conversation 与 Core 同时维护语义重叠的 `active`、`busy`、queue、wait、cancel 或 close 状态。保留多个局部 guard 时，每个 guard 必须有不同的 owner、acceptance point、release point 和失败语义，并用对应并发测试证明不会形成状态分歧或嵌套锁。
- 并发语义：对同一个逻辑 Session 的第二次推进应由该 Session 的 policy 拒绝或排队；不同 Session instances 应能隔离并发。若两个 Session 操作同一 workspace，所需的 worktree、文件冲突或外部副作用协调属于未来 Orchestrator/workspace owner，不应回填给 Conversation 或 Core Agent。
- 当前非目标：本决定不在 Lesson 11 引入 Session 类型、持久化 identity、queue、wait/close API、跨进程 lease、全局锁或 workspace manager。它只要求第三阶段不要把当前 guard 位置误当成不可改变的长期架构。

### D76. Recovery projection 以 absolute History exclusions 保留失败事实并过滤模型视图

- 日期：2026-07-26
- 决定：Lesson 11 继续以 append-only complete Conversation History 作为唯一完整事实源，并在 coding-owned compaction projection 中记录 eligible overflow error assistant 的 absolute History position。`projectedMessages()`、token estimation、compaction summary input、retained suffix planning、Working Context replacement 和 Provider request snapshot 必须消费同一过滤后的 model source；不得只在 Core Agent state 临时删尾部，也不得按错误文本或“当前最后一条”在以后重建时重新猜测。
- 生命周期边界：第一次 failed Core Run 的 delta 先原样提交 History，随后才按该 assistant 的 absolute position 构造 candidate exclusion。D76 只要求已发布的 projection metadata 与 Core Working Context 始终表达同一个过滤结果；forced compaction 失败时是完全保留旧 model view，还是先提交独立有效的 exclusion 再保留未压缩 filtered context，留在后续失败语义讨论中确认。后续 continuation delta 使用新的 absolute History positions 正常追加；新的 cut 已越过旧 exclusion 后可丢弃对应 metadata。
- 验证义务：至少覆盖两个不同 accepted user advances 的 overflow，并在其间执行普通 threshold compaction，证明旧 error assistant 不会重新进入 summary 或 Provider request。只验证当次 `slice(0, -1)` 不足以证明该 representation 正确。
- 范围：本决定固定可重建性与原子发布语义，不固定 Go field、slice/map representation 或 package split；实现前仍按当前文件 cohesion 决定最小清晰结构。

### D77. Lesson 11 只限制自动 recovery，未来用户显式 retry 是新的控制操作

- 日期：2026-07-26
- 校准纠正：冻结 Pi 的 `_overflowRecoveryAttempted` 在新 user message 或 non-error assistant 后重置，因此它限制的是未取得中间进展的连续 overflow recovery chain，不是严格的“每个 user prompt 一次”。OpenCode 与当前 Codex 同样允许成功 compaction 后返回执行循环，并各自提供独立的手动 compact surface。
- 当前决定：Pia Lesson 11 采用更严格且易于验证的 one-shot policy：每个 accepted user advance 最多自动发起一个 forced-compaction-plus-input-free-continuation cycle。Compaction 失败、continuation 再次 overflow 或遇到其他失败后，本次调用 settled 并返回错误，不进行第二个隐藏的 recovery cycle；新的 user advance 重新获得自己的 automatic budget。
- 未来交互：交互式 Session/TUI 可以在失败 settled、阶段和原因已经展示后，让用户显式发起新的 compact/retry control operation。该操作具有独立的用户来源和 attempt lifecycle，不追加假的 Conversation user message，也不通过偷偷重置当前自动 attempt flag 实现。具体 retry、换模型、减少 context 或新 Session 选项必须在对应课程基于当时产品证据设计。
- 原因与范围：自动上限解决当前无 UI、无 Session、无 generic retry 和无 cost/turn budget 时的终止性、隐藏成本与可解释性，不是死锁机制，也不是永久禁止再次 compaction。Lesson 11 不增加手动命令、事件、pending-task 状态或恢复 UI。

### D78. Forced compaction 普通失败保留已提交 History，但不发布 recovery model view

- 日期：2026-07-26
- 两段提交：第一次 Core Run settled 后，Conversation 先把原 user、已经完成的 Provider/tool turns 与 eligible overflow error assistant 原样提交 complete History；该事实提交和已经发生的工具副作用不因后续恢复失败回滚。Exclusion、summary、cut、usage boundary 与 replacement Working Context 则共同组成第二个 candidate model-view commit。
- 失败语义：compaction planning、summary Provider、空/无效 summary、unexpected tool call、candidate capacity/protocol validation 或 atomic `ReplaceWorkingContext` 任一普通失败时，complete History 保留第一次 failed Run 的完整 delta，Core Working Context 与旧 projection 保持 recovery 前状态，不发布 exclusion 或任何半成品 compaction metadata，不调用 `Continue`。外层返回当前 History snapshot，以及带 recovery/compaction operation context并保留最终 compaction cause 的 error。
- 非对话副作用：Summary request 不经过 Core Agent loop、不提供 coding tools，其 request、terminal、局部 summary 与 usage 不进入 complete History。已经发生的 Provider 数据外发、计费或某个 split-summary 子请求无法回滚；后续子步骤失败时只丢弃本次局部 candidate，不能用已发生成本作为发布不完整 Agent state 的理由。
- Pi 分歧与未来 retry：冻结 Pi 在 auto-compaction 前先从 mutable agent messages 删除 overflow error，compaction 失败后不恢复，而完整 Session entries 仍保留错误。Pia 需要显式、可重建 projection，当前选择沿用 Lesson 08 的 candidate-then-commit model-view 原子语义。未来用户显式 retry 按 D77 作为新控制操作，从已提交 History 和失败事实重新构造 candidate，不依赖上一次失败留下半发布 projection。
- 范围：本决定只覆盖非 cancellation 的 forced-compaction failure；取消发生在 model-view commit 前后时的语义单独确认。

### D79. Recovery model-view commit section 之后的取消不回滚已发布 projection

- 日期：2026-07-26
- 当前取消来源：Lesson 11 继续使用调用方传入的 Go `context.Context`；当前 one-shot CLI 由 `signal.NotifyContext` 把 `SIGINT`/`SIGTERM` 转成取消。这里没有新增 TUI cancel command、Session control state 或 event protocol。
- Commit 前：summary、planning、candidate validation 或最后一次 cancellation check 观察到取消时，返回 `context.Cause(ctx)`；第一次 failed Core Run 已提交的 complete History 不回滚，未发布的 recovery candidate 被丢弃，旧 Working Context 与旧 projection 保持不变，也不调用 `Continue`。
- Commit section：最后一次 cancellation check 通过后，idle-only Working Context replacement 与 projection metadata publish 构成一个短小、同步、无异步等待的 commit section。进入该 section 后新到达的取消不触发回滚；即使随后 `Continue` 因预取消而未被接受，已合法发布的 projection 仍保留，且没有新的 continuation delta。
- Continue 边界：若取消发生在 `Continue` acceptance 之后，Core Agent 按既有 cancellation/terminal/tool-call settlement 契约收敛，Conversation 再提交其 ownership-independent delta。Lesson 11 不为 recovery 另建第二套取消状态机。
- 原因：外部取消可以阻止尚未提交的工作，却不能安全地追溯撤销已经同步发布且内部一致的 model view。把可取消的 summary 阶段与短 commit section 分开，也避免在锁内等待 Provider 或把“取消到达时刻”误当成跨多个字段的回滚协议。

### D80. Compaction 使用独立的 settled Session journal record 持久化

- 日期：2026-07-26
- 阶段归属：Lesson 12 是第三阶段第一课，不是第二阶段补课。它可以为 compaction/recovery 定义实时 semantic events，但 events 只服务观察者；Session persistence 已确定属于第三阶段的 durable journal 与 restore 能力，当前只有阶段内 slot，尚未获得稳定全局课号。
- 持久化位置：未来每次已经结算的 compaction attempt 都在 versioned append-only Session journal 中追加一条独立 typed record，与 message/history entries 并列。它不是 `ai.Message`，不进入 complete Conversation History，也不进入 Working Context；trace 可以投影或展示它，但 trace 与 live event stream 都不是恢复的权威数据源。
- 最小事实：一条 settled record 保存 attempt 的开始和结算时间、trigger/reason，以及 committed、failed 或 canceled 的结果语义。Committed record 还保存重建 recovery projection 所需的 summary、retained/cut boundary、exclusions 与 usage-validity facts；failed/canceled record 只保存有界、脱敏的原因，不发布或替换 projection。确切字段名、wire schema、文件名、Go package 与 durable-write/in-memory-publish 顺序留到 journal 课程按当时源码确定。
- 不保存的内容：内部 summary Provider request、原始 API/HTTP payload、完整失败响应、局部 summary candidate 与逐 token/delta 不默认进入 Session journal。成功 summary 已由 committed projection 保存；诊断若以后需要更多细节，应通过有界 trace policy 单独设计，不能把 Conversation History 变成内部调用日志。
- 中途进程崩溃：这里指 compaction 尚未 settled 时，Pia 进程因 `SIGKILL`、未恢复的 panic、OOM、机器断电或同类原因突然终止；普通 Provider error、可处理的 `SIGINT`/`SIGTERM` cancellation 或 summary validation failure 只要代码能够正常返回并结算，就不是这个特例。首版只追加 settled record，因此突然终止的 attempt 不留下新 CompactionRecord；恢复时继续以更早的 latest committed projection 为准，不自动重放 continuation。Summary call 不拥有 coding tools，所以这不会制造未知 workspace tool 副作用，但已经发生的 Provider 数据外发和计费仍不可撤销。
- 简化边界：若真实审计或 crash-recovery 证据以后要求识别“开始过但没有结算”的 compaction，可把 journal 扩展为 durable Started + Settled entries，并把 unmatched Started 解释为 interrupted attempt。当前不为纯诊断需求提前引入配对 ID、未完成状态与清理规则。
- 对照证据：冻结 Pi `dcfe36c` 把成功结果保存为特殊 [`CompactionEntry`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/session-manager.ts#L69-L78)，失败主要停留在运行事件；Codex `61a4488` 以独立 [`CompactedItem`](https://github.com/openai/codex/blob/61a44880a85d2fd0d8770908dea5733495e571c8/codex-rs/protocol/src/protocol.rs#L3184-L3249) 保存恢复检查点，并把 trigger/status 放在事件或 analytics；OpenCode V2 `7534d23` 使用 durable [compaction lifecycle events](https://github.com/anomalyco/opencode/blob/7534d23551f665e65080809975b4ca5c7d63807b/packages/schema/src/session-event.ts#L398-L431)，并只让已完成 compaction 进入历史投影。Pia 采用三者共有的“特殊 control/checkpoint record 不等于普通对话”边界，同时补足最小 settled failure/cancellation 可追溯性。
- 当前课程影响：Lesson 11 仍只实现内存 candidate-then-commit 与 D79 的同步 commit section，不增加 Session journal、持久化 CompactionRecord 或 event API。第三阶段 journal 课程引入 durability 时，必须重新检查 crash-safe append 如何成为 model-view commit protocol 的一部分，不能机械照搬当前纯内存写入顺序。

### D81. Overflow classifier 暂时属于 coding private policy

- 日期：2026-07-26
- 当前决定：Lesson 11 在 `internal/coding` 保留一个 private overflow predicate，只消费已经 bounded、credential-redacted 的 terminal `ai.AssistantMessage`。它只接受 `stopReason=error`、不含 completed tool calls 且错误文本明确表达 context length/window exceeded 的 terminal；rate limit、too many requests、throttling、server overload、普通 400/413/422、5xx、`length` 和 aborted 均不能单独触发 recovery。
- 阶段性归属：这个位置只适用于当前“一个 coding recovery consumer、一个 DeepSeek/OpenAI-compatible 产品路径、没有稳定结构化 overflow code”的阶段，不是长期层级承诺。实现必须留下英文 `TODO` 并引用本决定的触发条件：Provider 提供稳定结构化 error code、第二个 Provider 需要相同分类，或 generic retry 成为第二个 consumer 时，重新评估把 evidence preservation/classification 下沉到 `internal/ai` 或具体 Provider profile；不得把当前文本 matcher 永久固化在 coding。
- 协议边界：当前不向 `ai.AssistantMessage` 增加 failure kind，也不把启发式结果伪装成 Provider wire contract。Classifier 只回答是否进入当前 overflow path；一次性 recovery budget、compaction、backoff、continuation 与用户显式 retry 都不是它的责任。
- 证据与验证：DeepSeek 官方只说明 context limit 和一般 400/422 语义，仓库尚无可证明稳定 code/message 的真实 overflow trace。首版 matcher 因而只覆盖窄的 context-length phrases，并用 negative cases 锁定误判边界；local HTTP fixture 只能证明现有 bounded/redacted terminal path 能把 error text 交给 classifier，不能宣称为 DeepSeek 官方 live contract。

### D82. Core Run 表示一次 accepted Agent Loop execution

- 日期：2026-07-26
- 决定：Core Agent `Run(ctx, userInput)` 与 `Continue(ctx)` 是同一种 Agent Loop execution 的两个 acceptance surface。前者在 acceptance 时追加一条 user message；后者要求当前 Working Context 非空且以 user 或 paired tool result 结尾，不追加输入。两者共享 Provider/tool loop、active guard、cancellation 与 terminal/tool-call settlement。
- Delta：两者都返回 ownership-independent run-local `NewMessages`。Input-started Run 的 delta 从本次 user message开始；continuation 的 delta 从 acceptance 时的 Working Context 尾部开始，只包含随后新产生的 assistant/tool-result messages。
- 外层术语：一次 coding user advance 通常协调一个 Core Run；Lesson 11 overflow recovery 可以在同一个 Conversation guard 内顺序协调一个 input-started Core Run 和一个 input-free Core Continue。这里不引入 Session、queue 或第二套 outcome type。

### D83. 第三阶段首版 Session 固定绑定创建时的 workspace

- 日期：2026-07-26
- 当前判断：一个 Session 在创建时绑定一个 workspace root，并把该 binding 作为 durable Session metadata 保存。第三阶段的 `resume` 只表示重新加载已经 clean settled 的 Session：它从任何调用者 cwd 都重新打开记录的 workspace，再重建 History、compaction projection、Working Context 与可继续状态。调用者的进程 cwd 不是 Conversation 内容，也不能静默覆盖 Session workspace。
- 不可用与改绑：记录的 workspace 不存在或不可访问时，resume 明确失败，不在当前目录或另一个项目中猜测性继续。首版不提供 relocate/rebind；未来本地 TUI 可以展示选择或说明，但 Session Runtime 不负责交互。仓库搬迁若成为真实需求，应先讨论显式 relocation、fork/new Session、journal transition 与安全校验，不能把“从另一个目录启动”偷偷解释为改绑。
- 判断依据：workspace 决定 `read`/`write`/`edit` containment、bash 起始目录、相对路径解释、稳定 system prompt、project instructions、project-local Skills 和 Provider 数据外发边界。同一 History 若在另一个 workspace 静默恢复，可能读取或修改错误项目，并让 journal 无法唯一重建当时的运行环境。Pia 的长期入口是 Orchestrator/Gateway/IM，远程调用不存在可作为权威 Session binding 的本地启动 cwd；固定 binding 也符合当前 `internal/coding/runtime.go` 先打开一个 workspace，再组装 tools、prompt、Core Agent 与 Conversation 的所有权路径。
- 对照证据：冻结 Pi 的 [`SessionHeader`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/session-manager.ts#L32-L39) 保存 `cwd`，其默认 Session storage 也由 cwd 派生。当前本机 Codex CLI `0.145.0` 采用更偏交互产品的策略：默认 picker 按当前 cwd [过滤](https://github.com/openai/codex/blob/25af12f7e61572b0bc18ddb1008be543b91519b0/codex-rs/tui/src/resume_picker.rs#L538-L550)，选中异目录 Session 且未配置偏好时再让用户在 recorded Session cwd 与 current cwd 之间[选择](https://github.com/openai/codex/blob/25af12f7e61572b0bc18ddb1008be543b91519b0/codex-rs/tui/src/session_resume.rs#L107-L165)。Pia 当前吸收“Session 必须记录 workspace、差异不能静默处理”的共同约束，但不把 Codex 的 TUI 选择策略下沉为 headless Session Runtime 契约。
- 明确非目标：journal 不保存 Provider credentials，也不快照整个 workspace 内容；fixed binding 不承诺恢复时文件内容、Git branch 或未提交改动保持不变。本决定不设计 path wire schema、symlink normalization、repository fingerprint、worktree manager、跨主机映射、公共 resume API 或 UI。它是第三阶段规划期的可修订判断：durable journal/restore 开课时仍需重新核对冻结 Pi、当时 Codex/Pia 路径和真实 consumer；repo relocation、worktree、container/remote workspace 或 Orchestrator mapping 证据可以触发调整。

### D84. 第三阶段优先完成单 Session 日常使用与 safe resume 主路径

- 日期：2026-07-26
- 路线纠正：D73 的九个 Session Runtime capability slots 是 Lesson 11 开始时的早期滚动假设。学习者随后明确要求先覆盖日常大部分工作，不让低概率边缘场景阻塞 Pia 进入真实 coding 使用。当前第三阶段因此收敛为七个阶段内 slots：semantic events、单 Session lifecycle、follow-up、steering、最小交互终端、durable journal、safe resume；只有最近的 semantic-events 课程继续固定为全局 Lesson 12，其余只固定阶段顺序，开课后再编号。
- 教学顺序：Follow-up 当前排在 steering 前，先用 Session settlement boundary 建立 queue、pending 与 quiescence，再修改 active Core loop 的 post-tool safe boundary。这个顺序是可校准假设，不是要求实现提前共享 queue API；对应课程开课的冻结 Pi 与当前 Pia 路径若证明依赖相反或不可分割，必须在写代码前修订课程表。
- 最小交互终端：第三阶段增加一个 non-full-screen local terminal host，消费 semantic events 并调用 Session controls，不把 UI 状态下沉给 Core Agent、Conversation 或 journal。Idle input 启动普通 user advance；active `Enter` 表示 steering，`Tab` 表示当前 execution 结束后的 follow-up，`Esc` 取消当前 execution 但保留 Session，`/exit` 在 active 时请求取消、等待既有 settlement 与 commit section 收敛、丢弃未消费的 steering/follow-up，再关闭 Session。Ctrl-C/Ctrl-D 不作为首版主产品控制；操作系统 signal/caller cancellation 仍可作为外部执行边界。
- 可用性里程碑：完成最小交互终端后，Pia 应已经可以在一个进程中的一个 Session 内承担日常交互式 coding；durable journal 与 safe resume 再补上正常关闭后的精确跨进程连续性，以及异常终止时回退到较早健康 checkpoint 的有限连续性。完整 TUI 的主题、复杂布局、picker/history browser、完整 slash commands 与 polish 继续延后。
- 持久化与恢复边界：Session journal 只保存 committed Session facts，并继续遵守 D80 的 event/history/compaction-record 分离和 D83 的 fixed workspace binding。D96 修订原 strict clean-only 边界：第三阶段 resume 对 clean settled/closed Session 精确恢复；对 unclean termination 只应用到 last committed Advance 的完整前缀，忽略其后不完整 tail，向 model projection 加入 workspace 可能含部分副作用的 bounded warning，并等待新用户输入。两条路径都不自动重放、不按当前 cwd rebind，也不声称已经恢复 interrupted work。
- 明确延后：automatic Provider retry 等真实 Provider/transport failure distribution 出现后再单独设计；保留并继续未完成 Advance 的 in-place interrupted execution recovery、multi-Session instance isolation 与阶段并发验收移出第三阶段主路径。它们仍是长期产品所需候选能力，但不再是单 Session 日常使用和 safe resume 的前置条件。Goal Runtime、Orchestrator/Gateway/IM、公共 SDK、worktree/GitHub 与完整 TUI 继续保持外层后续方向。
- 既有决定关系：本决定取代 D73 当前路线中的 Provider retry、interrupted recovery 与 concurrent-instance isolation 阶段归属，并把 D6/D73 的“完整 TUI 延后”细化为“第三阶段先做最小交互 host，完整 TUI 继续延后”。D6 的 UI 只做外层投影、D75 的 Session 候选唯一 lifecycle authority、D80 的 journal 归属和 D83 的 workspace binding 均保持不变。

### D85. Lesson 12 使用单个同步只读 observer

- 日期：2026-07-26
- 决定：Lesson 12 由 composition 为当前 coding execution 安装至多一个同步只读 observer，不建设通用多订阅者 event bus、后台 event goroutine 或异步 queue。产生事实的 Core/coding owner 交付 ownership-independent、bounded semantic event；observer 只做 line projection，不拥有或修改 Agent Loop、Conversation History、Working Context、compaction、recovery 或 lifecycle state。
- Settlement：事件交付按定义顺序发生，最后一条 event 的 observer 调用返回后，对应 Core Run 或 outer coding advance 才向调用方返回。这个边界只防止 return 之后出现 late events，不把 observer 变成 authoritative commit participant；具体 line-write failure 如何在 execution settle 后由 host 报告，继续在 Lesson 12 单独确认。
- 并行与锁：observer 只从 coordinator 路径顺序调用，parallel tool workers 不并发进入 observer。Tool-settled events 可以按 coordinator 观察到的实际完成顺序交付，而 tool-result Messages 继续按模型 source order 提交。调用 observer 时不得持有 Agent/Conversation state mutex，也不向 observer 暴露 reentrant Run、wait、cancel 或 state mutation surface；首版因此不增加新的业务锁或 lifecycle state machine。
- 取舍：同步 observer 若很慢或永不返回，会延迟 settlement。当前唯一真实 consumer 是进程内、有界输出的 line observer，这项已知限制比提前设计 queue capacity、backpressure/drop、close/drain、writer failure propagation 和 goroutine settlement 更小。未来出现网络 consumer、多个独立 subscribers、明确不可阻塞 producer 的 telemetry 或真实吞吐证据时，再重新评估异步 fan-out；不得仅因“事件通常是异步的”预建它。

### D86. Observer 输出失败不改变 coding execution settlement

- 日期：2026-07-26
- 决定：首个 line-observer write failure 由 observer 本地记录，随后停止向同一失败 writer 渲染；已经接受的 Core Run 与 outer coding advance 继续按既有 Provider、tool、cancellation、History commit 和 final snapshot 契约完整结算。Observer failure 不回滚已发生事实、不删除 History、不把已执行 tool 伪装成失败，也不自动发起 cancellation。
- Host 结果：execution settlement 后，host 单独报告 observer projection error；若 coding execution 同时失败，两项错误都必须保留而不是互相覆盖。Observer 的 first-write-error 只是 output adapter 的有界诊断状态，不是 Agent/Conversation lifecycle state，也不授权 observer 决定 recovery、retry 或下一步工作。默认 one-shot 的具体输出归属与 final rendering 顺序由 D87 补充。
- 阻塞与 panic：同步 writer 永不返回仍按 D85 作为首版已知限制，不为它增加 timeout、后台 goroutine 或异步 queue。Observer 是仓库内已知实现；其 panic 视为程序错误，本课不增加通用 `recover` 层或把 panic 静默转换成普通 event failure。未来真实 plugin/extension boundary 若允许不可信 observer，再在该边界单独设计隔离。

### D87. 默认 one-shot 将实时投影与最终结果分流

- 日期：2026-07-26
- 决定：Lesson 12 的默认 human-readable one-shot line observer 只向 `stderr` 写实时 semantic-event 状态；`stdout` 继续只承载成功 coding advance 的最终 assistant 文本。Observer 可以报告 terminal/advance 已 settled，但不得复制完整 final text。最终文本只能在 advance 与同步 observer 都 settlement 后由 `cmd/pia` 输出一次。
- 失败顺序：observer 的 `stderr` writer 失败不阻止成功 advance 的 final text 继续尝试写入 `stdout`，随后 host 按 D86 报告 projection error；final-output write failure 也是独立 host output error，不能覆盖已有 coding 或 observer error。Coding advance 自身失败时保持当前命令语义：错误写入 `stderr`、退出非零，不把 error terminal 伪装成成功 stdout result。两个 OS streams 合并后的跨流相对顺序不作契约承诺。
- 模式边界：该选择属于最外层 renderer，不改变 Core Agent、Conversation History、Working Context 或 semantic-event ownership。未来真实机器消费者可以引入显式 JSONL event mode，TUI 可以直接消费同一语义事件并自行管理终端；它们不与当前 human-readable stdout 契约混用。本课不增加 JSONL/TUI renderer，也不复制 Pi 的 process-wide stdout takeover；出现真实输出污染证据后再评估那层防护。
- 依据：Codex `exec` 默认把 progress 写入 `stderr`、只把 final message 写入 `stdout`，`--json` 则把 `stdout` 切换为 JSONL events；冻结 Pi 的 text/json print modes 采用同样的 result/event 分离并额外接管意外 stdout；OpenCode `run` 的默认 projection 也把 UI/tool 状态写入 `stderr`、在 non-TTY 下把 final text 写入 `stdout`，显式 JSON format 则输出 raw events。Pia 采用该共同主路径，但不复制 OpenCode `--thinking` 可混入 stdout 的例外。

### D88. 事实由对应责任产生并通过同一 observer 串行交付

- 日期：2026-07-27
- 概念边界：Session、Conversation、Core 与 user advance 不再表述为四个并列控制器。Session 是长期 outer lifecycle object，Conversation 是其交互数据，Core Agent 是执行引擎，user advance 是 Session 处理一次用户提交的短期操作。当前 coding-owned `conversation` 同时持有 History/projection 并临时协调 user advance；它是正式 Session 出现前的过渡性 owner，不成为事件 API 中的永久层级。D75 对未来 Session 吸收重叠 outer `active` 职责的要求保持不变。
- 事实 owner：Core execution engine 在事实成立处产生 Core Run、Turn、terminal Message 与 tool execution observations；outer user-operation coordinator 产生 compaction、overflow recovery 与 user-advance settlement observations，并且是任何 History-commit observation 的唯一合法事实源。当前 outer producer 是 coding-owned `conversation`，未来换成 Session 时保留这些语义。外层不得在 Core 返回后从 `NewMessages`、History 或 trace 猜测并重建实时 Core 过程；D89 进一步决定当前不暴露独立 History-commit event。
- 串行交付：两个 producer 使用 composition 安装的同一个 D85 同步 observer。Outer coordinator 同步调用 Core 的当前路径自然形成嵌套顺序，不增加中央 event controller、后台 serializer goroutine、跨运行 queue 或第二套 lifecycle。Observer 调用不得持有 owner state mutex，也不能 re-enter control APIs。
- Parallel tools：parallel workers 不直接调用 observer。每个 worker 只向当前 stage 的 Core coordinator 交回一次 outcome；coordinator 按实际观察到的完成顺序交付 tool-settled observation，同时按模型 source order 保存并提交 tool-result Messages。该 completion handoff 有界于当前 stage、在 stage 返回前完全消费，没有独立 drop/backpressure/close policy、长期 goroutine 或 return 后的 late events，因此不构成 D85 所拒绝的异步 observer queue。
- 结构复评：Lesson 12 不借事件实现重命名或重构当前 Go ownership。正式 Session lifecycle 开课时必须在增加 queue/cancel/close 前重新审查当前 `conversation` 与 Core guards；不得在两者外面机械叠加第三层同义 `busy`。Semantic event names 按稳定责任命名，使这次未来吸收不要求改写事件含义。

### D89. 不暴露独立的 Conversation History commit event

- 日期：2026-07-27
- 内部语义：Core terminal Message settlement 与 Conversation History commit 仍是两个事实。前者在 Core 接受 terminal assistant 或 ordered tool-result Message 并更新 Working Context 时成立；后者只在 Core execution 返回后，由 outer user-operation coordinator 接管并追加完整 Run Message Delta 时成立。Terminal observation 不得声称 Message 已持久化或已经进入 complete History。
- Live event 边界：Lesson 12 不为每次 Run delta 增加独立 History-commit event。Observer 可以看到 terminal Message、Core Run settlement、compaction/recovery 与最终 user-advance settlement；最终 advance-settled observation 保证本次操作需要的所有 History commits 和 final snapshot 已经完成。Overflow recovery 中，初次失败 Run 的 delta 必须先提交，随后才能观察到其 recovery compaction start，但中间不额外渲染 commit line。
- 理由：当前 line observer、未来最小交互终端和本课测试都不需要在每个中间 Core execution commit 后启动外部动作。增加 commit event 会把内部 ownership handoff 提升成用户必须理解的 lifecycle，并与未来 durable journal append 容易混淆，却不增加当前控制能力。若以后出现明确需要逐 Run commit notification 的 plugin、replication 或 diagnostics consumer，再由 outer owner 增加有界 observation。
- Durability：live terminal、Run 或 advance events 都不是 Session Journal 的恢复证据。未来 journal 课程必须以 crash-safe committed record 定义 durability，不能把已发出的 ephemeral event 当作 durable commit；该持久化事实也不要求回填为普通 Conversation Message。

### D90. Core semantic events 收敛为 Run、Turn、Message 与 Tool 四族

- 日期：2026-07-27
- 事件集合：Lesson 12 的 Core execution engine 只产生 `run_started/run_settled`、`turn_started/turn_settled`、`message_accepted` 与 `tool_started/tool_settled` 四个 semantic families。这里的名称先固定业务语义；具体 Go representation 可以用一个 closed event union 或等价窄类型实现，不因此为每个名称建立 controller、package 或状态机。
- Run 与 Turn：`run_started` 表示 input-started Run 或 input-free continuation 已通过 Core acceptance；input-started acceptance 同时已经接纳 user Message，随后可按逻辑顺序观察相应 `message_accepted`。`run_settled` 只在该 execution 启动的 Provider/tool work 和 terminal settlement 全部结束后交付。每次 Provider terminal assistant 及其引发的 tool executions/ordered results 构成一个 Turn；`turn_started` 位于 Provider request 前，`turn_settled` 位于该 terminal 与所有本 Turn tool-result Messages 接受后。一条 Run 可以包含多个 Turns。
- Message：`message_accepted` 只报告完整 user、terminal assistant 或 tool-result Message 已进入 Core Working Context，不暴露 token/thinking/tool-call formation delta，也不声称 complete History 或 durable journal 已提交。Assistant observation 发生在 tools 启动前；parallel tool-result Message observations 在 stage outcomes 全部收集后继续按模型 source order 交付。
- Tool：`tool_started` 与 `tool_settled` 观察一次模型请求的工具执行；不增加 partial-progress/tool-update event。Parallel completion 继续遵守 D88：settled observations 反映 coordinator 实际观察到的完成顺序，随后 tool-result Messages 恢复 source order。
- 删减：不增加独立 Provider-call、message-start/update、tool-update、History-commit 或 generic error event。Provider request boundary由 Turn 表达；本课所有非成功 settlement 统一使用 `error` outcome，cancellation-specific observation 按 D94 延后。Semantic event 是否投影成 stderr line 与 event 是否存在分开决定，避免为了人类输出删掉后续真实 observer 需要的顺序事实，也避免 line renderer 重复 final text。

### D91. Outer semantic events 只保留 Advance 与 Compaction 两族

- 日期：2026-07-27
- 事件集合：Lesson 12 的 outer user-operation coordinator 只产生 `advance_started/advance_settled` 与 `compaction_started/compaction_settled` 两个 semantic families。结合 D90，当前完整集合是 Advance、Compaction、Run、Turn、Message 与 Tool 六族；不再为 overflow recovery 建立第七个事件族或独立状态机。
- Advance acceptance 与 settlement：concurrent-advance rejection 发生在 acceptance point 之前，因此不产生 `advance_started`。每个已接受 advance 在所有 Core executions、必要的 History commits、compaction/recovery work 与 final History snapshot 完成后恰好产生一次 `advance_settled`；其 `success/error` outcome 表示整个 user operation 的最终结论，不等同于其中任一 Core Run 的结论。既有 caller-context cancellation 若发生，在本课观察面暂时归入 generic `error`，不增加第三种 outcome。
- Compaction：只有真正开始一次 compaction attempt 才产生 `compaction_started`，因此不增加 `compaction_skipped`。Start payload 以 `threshold` 或 `overflow` reason 区分正常阈值整理与错误后的恢复整理。`compaction_settled(success)` 只在新的 model-view projection 已通过既有 commit section 发布后成立；commit 前的任何非成功结算统一为 `error` 并保留旧 projection。本课不为取消到达 commit 前后的差异增加专用 event 状态；D79 的底层原子语义保持不变。
- Recovery 表达：threshold 路径是 `advance_started -> compaction_started(reason=threshold) -> compaction_settled -> Core Run -> advance_settled`。Overflow recovery 路径是初次 Core `run_settled(error)`、随后 `compaction_started(reason=overflow)`、成功后 input-free continuation `run_started`，最后才 `advance_settled`。Core 不拥有 context-overflow classifier，因此不把 `context_overflow` 作为 Core Run outcome；outer compaction reason 与后续 continuation Run 的组合已经完整表达 recovery。第二次 overflow、recovery exhaustion 或 continuation failure 继续由该 Run 与最终 advance outcomes 表达。
- 失败边界：observer write failure 继续遵守 D86，只影响 projection error 报告，不改变 advance、compaction 或 recovery 的业务结论。Bounded payload 与 error-text 边界由 D92 补充；具体 Go representation 与 line rendering 在本课后续讨论中确定。

### D92. Semantic event payload 只携带有界状态与 tool-owned safe summary

- 日期：2026-07-27
- 最小字段：`advance_started` 与 `turn_started` 不需要 payload；对应 settled events 只携带 `success/error` outcome。`compaction_started/settled` 携带 `threshold/overflow` reason，settled 再携带 outcome。`run_started` 携带 `input/continuation` mode，`run_settled` 携带 outcome。`message_accepted` 只携带 role，以及 assistant 的固定 stop reason 或 tool result 的 `is_error` 等有界 role-specific state。`tool_started/settled` 携带本 Turn 内的 source-order index、复制且有界的 tool display name、tool-owned bounded safe summary，settled 再携带 outcome。既有 assistant `aborted` stop reason 仍可作为 Message 已接受的协议事实出现，但它不是本课新增的 settlement outcome。
- Tool safe summary：当前 line observer 是已经成立的真实 consumer，因此不再把所有 tool detail 延后。每个 tool 只投影完成 operator progress line 所需的安全摘要：`read` 显示 path，`write`/`edit` 显示 path，`bash` 显示有界 command，`skill` 显示 skill name；不得把 write content、edit replacement、read result、bash output 或整份 raw JSON arguments 交给 observer。具体 Go hook 仍需在实现前的 package/structure review 中选择，但 summary ownership 必须留在理解该 tool schema 的责任一侧，不能让 generic line renderer 按 tool name 猜测并解码 raw arguments。
- 不进入 event 的内容：不携带 user/assistant 正文、compaction summary、raw tool arguments/output、原始 model-generated ToolCall ID、原始 error text/chain、workspace identity、Provider/model identity、token usage，也不暴露 `ai.Message`、`json.RawMessage`、slice、map、`context.Context` 或其他权威/可变对象。Tool summary 中有意显示的相对/绝对 target path 或 bounded bash command 属于当前 operator-visible action，不因此授权 event 暴露文件内容、环境值或 Provider credentials。Observer 不能把 event 当成第二份 History、trace、Provider log 或 state handle。
- 关联与 bounds：同一 terminal assistant 中的 tool calls 以小整数 source-order index 区分，因此两个同名 parallel tools 仍可配对 start/settled，而无需传播不可信且可能很长的 model call ID。Tool name 与 safe summary 都必须在 event construction 时独立复制并应用固定上限；renderer 还要转义换行、ANSI/control characters，保证一个 event 最多形成有界的一行。
- 错误可见性：省略 raw error 不会删除权威错误。Run/advance error 仍通过原调用结果交给 host；Provider terminal error 仍保存在 assistant Message；tool call-local error 仍保存在 ToolResult Message；事后 trace 继续按自身策略投影诊断。默认 live line 只需报告哪项操作以 `error` 结算，不重复可能包含正文、主机细节或敏感数据的 error 内容。
- 未来边界：Lesson 12 的 line observer 负责实时进度，one-shot final assistant 正文仍来自 settled advance result。若未来交互 TUI、JSONL 或外部 consumer 证明需要 terminal content、tool result previews、timestamps、durations、usage 或 stable cross-Session IDs，应在该真实 consumer 课程中增加相应有界 projection；不得预先把当前 internal event 变成公共或 durable wire schema。

### D93. 默认 line observer 显示动作开始，只为异常追加 settlement line

- 日期：2026-07-27
- 源码校准：本地 Codex `0fb559f0` 的 TUI 把相邻 read-shaped commands 合并、去重为一个可更新的 `Exploring/Explored` cell，不为每个 read 追加 completed；其非交互 `codex exec` 则是另一套 append-only start/completion renderer。本地 OpenCode `cb562b2c` TUI 为每个 read 保留一项 spinner/完成态而不写 completed，`run` 对普通 read 主要在完成时输出单行。本课冻结 Pi `dcfe36c7` 也以每个 tool-call component 的 pending/success/error visual state 表达结算，成功 read 默认不展开结果；text print mode 只输出 final response。这些证据共同表明 semantic start/end events 不应机械变成两条永久文本。
- 默认 projection：Lesson 12 的 human-readable append-only observer 在 `tool_started` 时输出一条 `pia: <safe summary>`，成功 `tool_settled` 不再输出；tool error 输出一条同 summary 的 `failed` line。Compaction 同样只在 start 时报告 threshold/overflow action，成功 settlement 保持安静，error 则追加 failed line。本课不增加 cancellation-specific line。Run、Turn、Message 与正常 Advance events 仍交付 observer，但默认不渲染；coding advance 自身 error 继续由 settled host error path 报告。
- 噪声边界：不输出通用 `pia: working` 或 `pia: completed`，不增加时间戳、duration、颜色、spinner、carriage-return overwrite 或其他 terminal control。没有 tool/compaction 的普通成功 one-shot 可以在 final stdout 前完全没有 progress stderr；这不是缺失 event，而是 renderer 选择。
- 聚合边界：Lesson 12 不复制 Codex 的 read coalescing。Codex 的效果依赖可变 TUI cell 与 display-grouping state；在当前 append-only line renderer 中实现等价效果，需要等待一组 calls、识别 group boundary 或把 scheduler-stage metadata 提升进 payload。当前没有足够收益引入这些状态。未来最小交互终端可以直接消费既有 per-tool events，动态合并相邻 reads、更新 `Exploring/Explored` 状态，而不改变 Core tool settlement contract。

### D94. Lesson 12 不新增 cancellation observation

- 日期：2026-07-27
- 当前收窄：Lesson 12 不增加 `cancel_requested` event、`aborted` settlement outcome、cancellation-specific line、监听 `ctx.Done()` 的 observer goroutine 或 cancellation-specific event tests。所有新 settled events 只有 `success/error`；既有 caller-context cancellation 若进入当前观察路径，临时按 generic `error` 投影。
- 保留语义：这项删减不修改 D21、D30、D33 与 D79 已实现的 cancellation、tool-call closure 和 compaction commit semantics。Core 仍返回可识别的 context cause，accepted Run 仍保留既有 aborted assistant 与必要的 not-executed results，已启动工作仍必须收敛，commit 后已经发布的合法 projection 仍不回滚。`message_accepted` 可以报告既有 assistant `stop_reason=aborted`，因为这是 Message 协议事实；它不等同于为 Run、Turn、Tool、Compaction 或 Advance 新增 aborted outcome。
- 延后理由：当前 one-shot caller cancellation 只有外部 context/signal，没有 Session-owned `Cancel()` 或 TUI `Esc` 这个真实控制面。现在区分“取消请求已接收”“正在收敛”“最终已取消”会迫使本课预建 request event、监听 lifecycle 与 UI 文案，却没有当前 consumer 使用。正式 Session 与最小交互终端出现后，再共同定义 `Esc` request、`Canceling` 与真正 `Canceled` settlement、follow-up/steering/close 交互及相应测试。

### D95. Semantic events 使用 internal value union 与 tool-owned description hook

- 日期：2026-07-27
- Representation 与 package：Lesson 12 在 `internal/observation` 定义一个由 Advance、Compaction、Run、Turn、Message 和 Tool 六种 value types 组成的 closed event union，以及 nil-safe 同步 `Observer func(Event)`。该 package 是内部 execution observation vocabulary，不是公共 SDK、durable journal schema 或 network wire contract；它只依赖既有 `ai.StopReason` 这一项固定协议枚举，不拥有 Agent、Conversation 或 Session state。
- Payload bounds：没有正文的事件只携带固定 enum、bool 和 source index。Tool display name 与 safe summary 在 event construction 时独立复制并分别限制为 64/512 UTF-8 bytes，截断带可见 marker；line renderer 继续转义控制字符。当前 `success/error` 映射由 observation package 统一提供，D94 延后的 cancellation distinction 不散落到 Core 和 coding producers。
- Tool hook：`agent.Tool` 增加同步、无副作用的 `DescribeInvocation(json.RawMessage) string`。它由理解 schema 和敏感字段的 concrete tool 实现，只返回 operator-safe identity projection；generic Agent/renderer 不按 tool name 解码 raw JSON。没有 observer 时 Agent 不调用该 hook，parallel stage 的 descriptions 也由 coordinator 串行取得，workers 只执行 `Execute`。
- Wiring 与 ownership：`agent.Config` 和 internal `coding.RunInput` 接受同一种 observer；composition 把同一实例交给 Core 与 outer coordinator。Core settlement 与 outer settlement 的 observer 调用都发生在各自 active guard 释放前且不持有 state mutex。`cmd/pia` 安装一个已知 line observer，observer error 留在 host output path，不进入 event、History、Working Context 或 trace。

### D96. Safe resume 支持回退到 last committed Advance

- 日期：2026-07-28
- 路线修正：学习者确认 hard-killed process 不应让整个 Session 永久不可恢复，并接受丢失最新 Run、若干 Turns 或整个未完成 Advance 来换取简单可靠的恢复。D96 supersedes D84 中“没有 clean close 就拒绝整个 Session”的 strict clean-only 部分；第三阶段同时支持 exact clean resume 和 checkpoint fallback resume。
- Checkpoint 边界：一个完整 settled Advance 是首版 crash recovery 的最小业务提交单元。Journal 课程必须让恢复方识别 last committed Advance 及其完整有效前缀；unclean termination 时，恢复只应用到这个 checkpoint，忽略其后的 message、Run、compaction 或其他不完整 tail。确切 transaction/framing、checksum、segment 和 durable-write 顺序仍留到 journal 课程，不在 Lesson 13 预设计 wire schema。
- 不做逐条修复：首版不按单条 assistant/tool result 猜测“健康度”，也不保留并继续未完成 Advance。Tool result 尚未持久化不证明副作用没有发生；逐条删除还可能破坏 tool-call 配对、History/Working Context、compaction projection 与 Advance final snapshot 的一致性。Provider in-flight、tool pre-start、tool running 和 result commit 的精细恢复继续属于延后的 in-place interrupted execution recovery。
- 重建与提示：authoritative Conversation History 和 Working Context 只从 checkpoint 前缀重建。Unclean-recovery fact 属于 Session Journal/control state，不伪装成普通 Conversation Message；恢复方从它向 model-visible projection 派生一条有界 warning，说明 Conversation 已回退而 workspace 可能包含后来中断工作产生的部分修改，要求重复动作前检查现状。Projection 不是判断 journal 健康度的 authority。
- 无重放与副作用：恢复后等待新的用户输入；Runtime 不恢复半条 LLM stream，不重新发送旧 Provider request，也不自动执行被丢弃 tail 中的 tool call。Checkpoint fallback 只回退 Session state，不回滚 workspace、Git、网络请求、外部服务或其他副作用，因此不能承诺 exactly-once。
- 边界情况：idle hard kill 通常恢复到最新 Advance，只增加保守 warning；首个 Advance checkpoint 前被杀时，可以在有效 Session identity/workspace binding 上恢复为空 Conversation 加 warning。若 journal header/version 或任何可用 checkpoint prefix 无法验证，或记录的 workspace 不可访问，则明确失败。是否保留丢弃 tail 供 operator 事后查看，不影响首版恢复 authority，并留到 journal/terminal consumer 校准。
- 当前课程影响：Lesson 13 只实现同进程 cancellation settlement、History protocol completeness 与 lifecycle controls，不提前增加 journal 或 resume。第三阶段 journal/resume 课程必须落实 exact clean path 和 last-committed-Advance fallback；完整保留 interrupted execution、逐调用修复或自动继续仍不作为第三阶段退出条件。

### D97. Session 成为唯一长期 Conversation owner，Agent execution 改为 run-local

- 日期：2026-07-28
- 路线纠偏：学习者确认 Lesson 13 采用“一个 authoritative Session 加 run-local Agent Execution Engine”的结构，并明确 Pia 当前没有需要保护的外部 API 或类型兼容包袱。已有模块、owner 和课程结论是学习证据，不是永久约束；新能力证明旧边界会形成重复 state 或长期负担时，应在该课程中替换、合并或删除，而不是增加 compatibility wrapper。较大结构调整可以是课程能力本身；只有包含多个可独立讲解和验收的能力时才按 XLarge 规则拆课。
- 单一长期 owner：一个 `coding.Session` 独占一个 Conversation 的 Workspace/resources、完整 Conversation History、compaction/recovery projection，以及 busy/cancel/wait/close lifecycle。Conversation 只保留为逻辑交互数据，不建立并列 controller、active guard 或独立 lifecycle。Session 是唯一 user-advance 入口，同一 Session 至多接受一个 active Advance；该 guard 覆盖 preflight、所有内部 Runs、overflow recovery、History/projection commits、observations、final snapshot 与 settlement。
- 派生 Working Context：Session 在每次 execution 边界从权威 History 与已提交 projection 派生 ownership-independent Working Context snapshot。Compaction 只原子发布新的 Session projection，后续 execution 自然从它派生；不再维护由另一 owner 长期持有、需要同步替换的 Working Context。
- Run-local execution：`internal/agent` 保留 Provider/tool loop、Run/Turn ordering、terminal assistant exactly-once、tool-call closure、cancellation settlement、semantic events 和 ownership-independent message delta，但不拥有某个 Conversation 的长期消息、active guard 或 replacement API。它接收 Working Context snapshot，只有 invocation-local context，并在 settlement 时返回 delta。具体 Go type 名与方法 surface 在 Lesson 13 实现设计中确定，D97 固定责任而不预设计公共 API。
- one-shot host 与资源：删除 product-level `coding.Run`、`runWithProvider`、`runWithWorkspaceOperations` 和 coding-owned `conversation` controller，不保留同义 wrapper。`cmd/pia` 是进程 host：创建 Session、Advance 一次、Close，并合并 Advance 与 Close errors；未来 TUI 或 Orchestrator 创建同一种 Session 并多次 Advance。当前 final stdout、semantic-event stderr、trace、Skill diagnostics、terminal/error/cancellation 与 tool ordering 是要保留的 observable behavior，旧函数、类型和 package shape 不是。
- 构造所有权：Session constructor 打开 Workspace 后若后续构造失败，立即关闭并以 `errors.Join` 保留 construction 与 close errors；构造成功后 Workspace 所有权转交 Session，只有 Session Close 释放。当前只有 Workspace 是已证明的 owned closeable resource，不预建通用 cleanup registry。
- 对旧决定的影响：D97 保留 D46 的 complete History 与 model view 语义分离，但取代“Core 长期拥有 Working Context”的 owner 形状；保留 D47 与 D50 的 settled run-local delta 和真实所有权边界深复制，并把接收者改为 Session；取代 D48 的临时 coding-owned `conversation` type、D49 的 Conversation/Core 双 guard，以及 D51 的 idle-only `ReplaceWorkingContext`。D54、D58 与 D73–D82 中 compaction、projection、overflow recovery 和 commit-section 的可观察/原子语义继续有效，其 Conversation Owner/Core replacement 实现机制按本决定迁入 Session 与派生 snapshot。
- 参考证据：冻结 Pi `dcfe36c79702ec240b146c45f167ab75ecddd205` 证明 stateful core `Agent` 加 coding `AgentSession` 可以工作，但也显示双层 lifecycle 随 retry、compaction、queue 和 Runtime 增长后的复杂度；Codex `0fb559f0f6e231a88ac02ea002d3ecd248e2b515` 把 history、configuration/services、active turn 与 input queue 内聚在内部 Session；OpenCode `cb562b2c6289c2eee707078f9ab644cbe1d3d8a9` 的 durable Session、workspace Instance 与 run registry 更适合 multi-Session server，当前复制会过早分散 Pia 的单 Session ownership。Pia 取三者的责任证据，不机械复制类型图。
- 课程影响：Lesson 13 从“新增 Session 并移动 outer guard”修正为“先完成 ownership refactor，再建立 lifecycle controls”，规模仍为 Large，因为它是一个单一、可独立验证的 Session ownership/lifecycle capability。Follow-up、steering、terminal、journal、resume、Manager 与 multi-Session isolation 仍不进入本课。

### D98. Close 立即取消并有界等待，host 可以在 grace period 后 hard exit

- 日期：2026-07-28
- Session 契约：Close acceptance 立即永久停止新 Advance admission，并立即向 active Advance 传播 cancellation；取消不能等到 timeout 才发送。Session 随后在 caller context 内等待 terminal/tool closure、History/projection commit、observations、Advance settlement 与 owned-resource close。Clean settlement 完成前的状态是 closing，不是 idle 或 closed。
- 等待上限：caller context 到期时，Close 停止阻塞调用者并返回其 context cause；Session 不重新开放 admission、不强关仍被 active work 借用的 Workspace，也不写成 clean close。若进程继续运行，现有 active settlement path 在真正收敛后完成一次 resource close；重复 Close 只观察同一个 shutdown，不重复取消或关闭。
- 能力边界：Go 不能安全强杀任意 goroutine，context 也不能保证 Provider、tool、filesystem operation、同步 observer 或资源关闭一定在固定时长内响应。因此 Session Runtime 保证“立即请求取消 + caller 可有界停止等待”，不虚构“任意工作都能在固定时长内 clean closed”。实现必须用不配合 cancellation 的测试 double 验证 Close caller 可以退出等待，同时 Session 保持永久不可复用。
- Host policy：未来最小交互终端的 `/exit` 在调用 Close 时使用短暂 host-owned grace period；窗口耗尽后可以直接结束整个 Pia 进程，由操作系统回收进程资源。真正的 hard upper bound 位于 process host，而不是 Session 假装杀死其内部 goroutine。学习者举例的 `3 秒`是首版候选，不在 Lesson 13 固化；终端课程根据真实 Provider、bash、filesystem 与 observer cancellation latency 决定默认值。
- 恢复语义：grace period 内 clean close 时未来可以 exact resume；hard exit 不产生 clean-close fact，journal/resume 按 D96 只恢复 last committed Advance，忽略不完整 tail，提示 workspace 可能含部分副作用，并且不自动重放旧 Provider/tool 调用。多数退出不会 resume 不改变 state correctness，但也不应迫使日常退出为小概率恢复无限等待。
- 错误所有权：active Advance 的 Provider/tool/cancellation outcome 仍返回给 Advance 调用者；Close 只负责 shutdown wait 与 owned-resource close，不重复返回同一 Advance business error。One-shot host 在 Advance 已返回后继续合并 `advanceErr` 与 `closeErr`；终端 host 可以把用户主动退出导致的预期 cancellation 按自身 UI policy 处理。

### D99. Session control 使用 `Cancel` 与 `Wait`，不增加 `Current`

- 日期：2026-07-28
- 命名：Lesson 13 的最小 control surface 使用 `Cancel()`、`Wait(ctx)` 与 D98 的 `Close(ctx)`。一个 Session 同时至多有一个 active Advance，receiver 已经限定作用域；`CancelCurrent`/`WaitCurrent` 不增加信息，反而为尚未实现的 queue 提前固化含义。
- Cancel：`Cancel()` 是立即、idempotent、无返回值的 request。busy 时向唯一 active Advance 传播 cancellation；idle、重复 cancel、closing 或 closed 时 no-op。“当前没有工作”不是异常，尤其不能让未来 TUI 的 idle `Esc` 变成 error path。Cancel 不等待 settlement，也不提前发布 idle。
- Wait：`Wait(ctx)` 只观察 Session 从当前 busy Advance 收敛到不再 busy；它不发送 cancellation。idle 或 closed 时立即返回 nil；caller context 到期返回 `context.Cause(ctx)`，但不修改 active Advance。Advance 的 Provider/tool/cancellation error 仍由原始 `Advance` 调用返回，Wait 不复制该 business outcome。
- Future boundary：Lesson 13 没有 pending input queue，故“不再 busy”当前只涉及一个 accepted Advance。Follow-up 课程必须根据 queue/quiescence 的真实 owner 重新确认 Wait 是否覆盖 pending input；当前不增加 `Current`、`WaitIdle`、queue token 或 state-query API 预先解决未来语义。

### D100. Session 使用窄的 Go object API，Engine 只执行 run-local loop

- 日期：2026-07-28
- 横向命名证据：冻结 Pi 的 `createAgentSession()` 返回长期 `AgentSession`，其主要入口是 `prompt()`、`abort()`、`waitForIdle()` 与 `dispose()`；Codex 由 `ThreadManager.start_thread()` 返回 `CodexThread`，通过 `submit(Op)` 区分 `UserInput`、`Interrupt` 与 `Shutdown`；OpenCode 的 `SessionPrompt.prompt(PromptInput)` 和 `SessionRunState.cancel/ensureRunning(sessionID)` 是面向多 Session service/registry 的 ID-based API。Pia 采用三者共同证明的“长期 handle、输入推进、取消与 lifetime shutdown 分离”，不复制 Pi 的大 options/result、Codex 的通用协议命令入口或 OpenCode 的 server registry。
- Session surface：`internal/coding` 定义 `NewSession(SessionConfig) (*Session, error)`，以及 `(*Session).Info() SessionInfo`、`Advance(ctx, input string) (AdvanceResult, error)`、`Cancel()`、`Wait(ctx) error` 和 `Close(ctx) error`。package 已经限定 coding 语义，因此不命名为 `AgentSession`；Pia 的已有课程词汇把一次完整用户操作定义为 Advance，因此不使用只暗示模型输入的 `Prompt`，也不使用会把未来 protocol operations 提前塞进 Runtime 的 `Submit`。当前只有 text input，故不增加只有一个字段的 `AdvanceInput`；images、attachments 或其他真实输入出现后再升级。
- 构造配置与静态信息：`SessionConfig` 当前只有 `WorkspacePath string`、`DeepSeekAPIKey string` 与可选 `Observer observation.Observer`。凭据字段明确写出当前固定 Provider，不用含义虚假的通用 `APIKey`；不把 `ai.Provider`、Workspace opener、resource registry 或未来 persistence/queue options 暴露给 application caller。`SessionInfo` 保存 canonical `WorkspacePath`、`SystemPrompt`、`Model ModelInfo`、`Tools []ai.ToolSchema` 与 `SkillDiagnostics []SkillDiagnostic`；`Info()` 在任何 lifecycle 状态都返回 ownership-independent snapshot，绝不包含 credential。
- Advance result：`AdvanceResult` 只含 `History []ai.Message`，并保留 `FinalText()` 这一 coding-owned projection。字段使用权威 domain 名 `History`，不再把 Go application state 命名为诊断展示词 `Transcript`。一个 accepted Advance 无论成功、Provider/tool failure 或 cancellation 都返回结算后的 complete History snapshot 与独立 Go error；blank input、pre-canceled context、busy 或 closed 在 acceptance 前拒绝，不发 Advance events、不修改 History，并返回当时的 History snapshot 与对应 error。并发拒绝使用 `ErrSessionBusy`，closing 与 closed admission 都使用 `ErrSessionClosed`；不公开 `closing` error、state enum、`IsBusy` 或 status polling API。
- Engine surface：`internal/agent.Agent` 改名为 `Engine`，package-qualified constructor 仍使用简洁的 `agent.New(Config) (*Engine, error)`。它提供 `Run(ctx, workingContext []ai.Message, userInput string) (RunResult, error)` 与 `Continue(ctx, workingContext []ai.Message) (RunResult, error)`；每次调用深复制输入并只维护 invocation-local messages，`RunResult.NewMessages` 契约保持。Engine 不再有 long-lived Working Context、`ReplaceWorkingContext`、`ErrRunActive`、active mutex 或 Session lifecycle；Session 是当前唯一 caller 并负责串行化。保留两个入口是为了明确 input-started Run 与 overflow 后 input-free continuation，不使用 bool/mode 参数混合两种协议前置条件。
- Lifecycle mechanism：Session 内部只保留一个短锁、私有 `open/closing/closed` lifetime、一个可选 active control 和一个 shared close completion。active control 保存 `context.CancelCauseFunc` 与 `done`；Advance acceptance 使用 caller context 派生 execution context，caller cause 与 Session `Cancel/Close` 的 `context.Canceled` 按 first-cause-wins 传播。History/projection snapshot 与 commit 也只在短锁内完成；Provider、tool、observer、Workspace close 或 channel wait 期间不持锁。`Cancel()` 在锁内复制 cancel function、锁外调用；`Wait(ctx)` 在锁内复制 active `done` 后等待，已经 idle/closed 时 condition 优先并立即返回 nil。
- Close convergence：第一个 `Close` 先在锁内永久切换到 closing；busy 时锁外立即 cancel，然后所有调用者等待同一个 close completion，idle 时由该 Close 直接关闭 Workspace。若 Close 在 active Advance 中被接受，该 Advance 的既有 settlement path 在完成 History/projection/observer settlement 后接管唯一一次 Workspace close、记录 `closeErr` 并完成 shared close result；不建立 detached cleanup goroutine。caller context 只停止某个 Close caller 的等待，不能撤销 closing。重复或并发 Close 不重复 cancel/close，closed 后返回保存的同一 close result。已经完成 clean close 时 completion result 优先于 caller cancellation；仍在 closing 且 caller context 到期时返回 `context.Cause(ctx)`。
- 构造清理：`NewSession` 先验证 required scalar config 并创建当前无 `Close` contract 的 DeepSeek Provider，再打开 Workspace。Workspace 打开后安装一个仅在 constructor error 时生效的 cleanup；后续 Skill discovery、tools、prompt 或 Engine 构造失败时关闭 Workspace，并以 `errors.Join(primary, fmt.Errorf("coding: close workspace: %w", closeErr))` 保留两个原因。成功返回前撤销 constructor cleanup 并把唯一 ownership 转给 Session。当前只有 Workspace 是已证明的 owned closeable，不为“以后也许有更多资源”预建通用 stack/registry；离线测试通过 package-private Provider 与 open/close seam 使用 Faux 和注入 close failure。
- One-shot host：`cmd/pia` 不再注入 `coding.Run`，而是通过一个只含 `Info/Advance/Close` 的私有 `codingSession` interface 注入 `newSession` factory。正常路径是 `NewSession -> Info -> Advance once -> Close -> errors.Join(advanceErr, closeErr) -> optional BuildTrace -> diagnostics/final text`。`BuildTrace` 改为接收 `SessionInfo`、`AdvanceResult` 和 joined settlement error；已经接受的 Advance 仍在 Session/Workspace settlement 后尝试 trace。诊断 JSON 暂时保留既有 `transcript` 与 `run_error` wire fields，避免把 ownership refactor 扩大成 trace format migration。constructor 尚未产生 Session 或 accepted Advance 时直接返回 construction error，不制造 partial Session/result 或 trace。
- 当前边界：这些名字位于 `internal/`，是 Lesson 13 的清晰 application API 而不是 public SDK 稳定性承诺。没有外部 consumer 时不再加 builder、functional options、provider abstraction、session ID、command union、queue handle、state subscription 或 compatibility wrapper；后续真实输入类型、durable open/resume 和 Orchestrator service boundary 可以基于新证据修改该 internal surface。

### D101. 完整 Session action timeline 是 observer-owned 诊断投影，不是 Session 业务状态

- 日期：2026-07-29
- 用户需求：学习者希望以后能在一个位置查看单个 Session 的完整动作时间线，用于分析 compaction 是否发生、触发原因、成功/失败及其影响，并依据不同 Provider、模型和真实 workload 调整 threshold 或其他策略。这个需求服务 trace、诊断与优化，不改变 Conversation correctness。
- 责任边界：未来以可选的 host-owned diagnostic recorder 消费既有 ordered semantic events，并结合 immutable `SessionInfo` 形成 Session action timeline。Session 继续只拥有 authoritative History、projection、resources 与 lifecycle，不增加第二份 action history、event store 或 trace buffer；line renderer、diagnostic recorder 和未来 TUI 可以是同一 observer stream 的不同消费者。
- 与 journal 的区别：diagnostic timeline 可以记录分析所需的过程与指标，但不是恢复 authority；versioned Session journal 仍只保存 D80、D96 所需的 committed facts、compaction records、checkpoints 与 lifecycle facts。Timeline、journal、Conversation History 和 Working Context 不合并成一个万能日志。
- 当前缺口：现有 Compaction event 已有 `started/settled`、`threshold/overflow` 与 `success/error`，但没有 timestamp、stable sequence、取消的独立 outcome、压缩前后 context estimate、threshold/profile、retained/cut 规模或 Provider/model 维度；当前 trace 也不保存 event stream。因此现状只能实时观察，不能在 Advance 后完整分析。未来进入该能力时必须先根据真实优化问题重新检查 event coverage，定义最小、安全、有界且不含 credential 的记录；本决定不提前冻结 JSONL schema、文件位置、保留策略或 lesson 课号。
- 课程影响：这是一项独立的 observability/diagnostic consumer，不并入 Lesson 13 的 ownership/lifecycle capability，也不要求 Session API 预留 recorder。它加入第三阶段滚动方向，开课时再根据最小交互终端、Provider 对比与 journal 进度决定顺序和规模。

## 变更记录

- 2026-07-15：建立初始课程和架构决策，并补充 stream、tool validation、Session storage、平台范围与 Runtime/Manager 边界。
- 2026-07-15：增加证据驱动的学习原则、先讲后问顺序和 `run_start/run_end` 课程术语。
- 2026-07-15：移除课程专用 `internal/contract`；冻结基线仅保存在文档。
- 2026-07-15：取消当前阶段的外部调用承诺；`cmd/pia` 仅用于本地运行与验收。
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
- 2026-07-16：第 02 课已推送到 `main`，学习者明确要求开始第 03 课；课程先核对冻结 Pi 的 Tool Loop 与整批调度，再讨论 Pia 的 Tool contract 和屏障式分段实现。
- 2026-07-17：学习者接受第 03 课 Tool retry 结论；Agent 不自动重试 tool call，模型负责从 error tool result 恢复，后端内部重试必须有明确的瞬时错误证据。
- 2026-07-17：冻结 Pi 的 orphaned tool-call request 修复暴露出旧取消契约与完整 transcript 复用的冲突；学习者选择方案 2，Pia 在 Agent transcript 中为 canceled/not-executed 调用追加明确 settlement results，并要求后续真实 DeepSeek 验证特别覆盖该分歧。
- 2026-07-17：第 03 课实现 review 发现 Provider aborted assistant 可以保留已完成 tool calls，而第 01/02 课的旧表述只约束了 assistant exactly-once。学习者确认扩展方案 2：失败 Turn 不执行这些 calls，但在唯一 terminal assistant 后追加同 ID not-executed results，并同步修正旧课程记录。
- 2026-07-17：学习者明确开始继续课程并进入第 04 课；确认参考冻结 Pi 的 Provider/API 分层，以薄 DeepSeek 层复用最小 OpenAI-compatible Chat Completions 协议层，不扩建多 Provider registry。
- 2026-07-17：学习者提出统一 Provider 目录；确定 `internal/ai/provider/` 只归类 Faux、OpenAI-compatible 与 DeepSeek 实现，consumer-owned `ai.Provider` 仍留在 `internal/ai`，并记录这与 Pi 大 Provider runtime abstraction 的差别。
- 2026-07-17：学习者确认用标准库 `net/http` 实现已讲解的完整消息映射，并将共享线协议包定名为 Go 惯例下的完整名称 `provider/openaicompatible`；不要求沿用 Pi 的 `openai-completions` 命名。
- 2026-07-17：学习者确认 pull-based SSE parser、finish reason 加 `[DONE]` 双 settlement、tool-call 分片边界、零自动重试、64 KiB HTTP 错误体、1 MiB SSE event 和最小 usage 语义；第 04 课进入实现。
- 2026-07-17：学习者明确要求第 04 课不为没有真实 DeepSeek/Pi 证据的 3xx 增加特殊逻辑或专门测试；Provider 保持标准 `http.Client` 行为，redirect 影响记录为后续独立验证项，“零自动重试”只表示 Provider 不自行 retry。
- 2026-07-17：学习者开始第 05 课并确认 `read` 的 offset/limit、非法 UTF-8 error 与固定模型输出；共享 JSON/path 能力先进入 coding tools 的集中辅助 package，具体读取语义保留在 `read`。
- 2026-07-17：`read` 子阶段完成测试先行实现和审查修复；workspace root、non-regular/FIFO、symlink/`..`、分页/双上限、错误有界、取消和并发测试通过。学习者随后确认理解并要求本地提交，下一步进入 `write`。
- 2026-07-17：核对冻结 Pi 后确认其默认 `read` 没有 regular-file/FIFO 特殊处理；Pia 的 `O_NONBLOCK` 加 opened-handle 校验是有意增强。课程同时修正 `go:build` 理由：当前只将该保证开放给已有测试的 macOS/Linux，而不是声称其他 Go 平台一定缺少该常量。代码注释统一使用英文，课程和其他文档语言不作限制。
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
- 2026-07-20：为后续 Pi 横向评测复核 Lesson 06 system prompt 后，学习者明确要求以 Pia 能力为边界但尽量保留冻结 Pi 默认 Prompt，且不改 one-shot workflow。D42 因此从自由 Pia wording 修正为“冻结 Pi 基线加窄适配”，并以完整字符串测试固定 identity、四工具 snippets/guidelines、project-context 与 cwd 顺序。
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
- 2026-07-20：学习者明确开始 Lesson 08。开课源码校准确认 threshold compaction 仍是一个可进入的 Large 闭环：Pi 以有效 Provider usage 为主要预算事实、近似估算 usage 后的尾部，在 coding-owned `AgentSession` 边界生成 summary、保留 protocol-valid suffix 并替换 working messages；当前 Pia 已有 usage、独立 History owner 与 idle-only Working Context replacement，因此本课不需要先引入 tokenizer、Session persistence、branch 或事件系统。具体 budget owner、cut granularity、summary 表达和失败语义留待本课讨论后形成新决定。
- 2026-07-20：学习者确认 Lesson 08 采用 protocol-safe message-level cut：可以在一个长 Run 内从 user 或 assistant message 开始保留，绝不从 tool result 或 message 内部切分；若切进 Run，中间被移除的 prefix 进入 summary，完整 History 保持原样。该语义记录为 D53。
- 2026-07-20：学习者确认 Lesson 08 在下一次 Run 接受新 input 前执行 lazy threshold compaction；summary 或 replacement 失败时旧 History、Working Context 和 projection metadata 全部不变，新 input 未接受，commit 后的取消不回滚合法 projection。该 lifecycle 与失败语义记录为 D54。
- 2026-07-20：针对 1M window 是否会降低 coding 质量继续核对产品和研究证据。DeepSeek V4 官方 MRCR 曲线在 128K 后出现可见退化，coding 长上下文研究也显示未过滤长输入可能降低真实仓库修复表现；因此冻结 Pi 在 1M model 上约 983616 tokens 才触发的 capacity-oriented 默认值不能直接成为 Pia 的 quality policy。当前提出 projected input `128K` between-Runs quality-oriented threshold、`32K–64K` 正常高信号区间作为待验证假设，并明确它不是 active Run 内每次 Provider call 的 hard ceiling；尚未形成新 durable decision。
- 2026-07-20：学习者要求明确保留 ceiling 复评义务，并指出“在同一 Run 内选择 cut point”容易被误读为“active Run 内触发 compaction”。课程记录已澄清：D53 只允许事后切入一个 settled Run 的消息序列，D54 的执行时机严格位于两个 Runs 之间；`128K` 与 `32K–64K` 仅为首版可测参数，必须依据后续 DeepSeek coding 分桶评测以及模型、reasoning mode、tools 或 summary policy 的变化重新校准。
- 2026-07-20：重新对照当前代码后保持 D54 的 between-Runs 范围。Conversation Owner 只能在 `core.Run()` 返回后取得完整 delta，而 Core Agent 明确拒绝 active-time Working Context replacement；因此 run 内 compaction 不是同级参数选择，而会要求新的增量状态所有权和 safe point。当前 read/bash 结果已有单次 50 KiB 模型可见上限，且尚无单个 Run 经常超过候选阈值的 trace 证据；该风险保留为显式缺口，出现真实越界或 coding 质量分桶证据后再拆分能力。
- 2026-07-20：参考 Codex 通用客户端约 `244.8K` 默认 auto-compaction 点与高强度长任务评测每 `100K` compaction 的两端，并结合 DeepSeek 在 128K 后开始退化及 Pia 仅有普通文本 summary 的差异，学习者委托确定首版折中值：projected Provider input `192K` 触发、普通情况以 `64K` 为 post-compaction soft ceiling。该 policy 记录为 D55，并保留按真实 coding 长度分桶强制复评的义务。
- 2026-07-20：学习者说明尚未逐字 review summary prompt，并要求没有特殊理由时继续沿用 Pi。重新核对冻结源码后没有发现 DeepSeek 或当前内存边界要求改写 prompt；因此首次/update/split-turn 三套 prompt、对话序列化、tool-result summary truncation、file-operation tags 与 synthetic user-message projection 按 Pi 建立首版基线，排除 branch/extensions/persistence，并记录为 D56。
- 2026-07-20：学习者确认首版 budget allocation 参考 Pi，并要求把 `64K` 澄清为不要求填满的 soft ceiling。课程采用 `20K` retained raw、`13,107` initial/update summary、`8,192` split-turn prefix 和 `4,096` Provider safety；同时明确记录这组值可能不足以支撑真实大型项目，尤其未来 Skills 会改变 context 组成。当前不解决或自动调参，待 Skills 引入及真实长项目验证时强制复评。该决定记录为 D57。
- 2026-07-20：学习者要求开始实现，并在实现后展开说明无精确 tokenizer 时如何判断下一次请求达到 `192K`。实现采用最后有效 Provider usage 加尾部 `ceil(characters / 4)`、无 usage 时完整 request fallback，并在 compaction 后使旧 usage 显式失效；所有权和精度边界记录为 D58。
- 2026-07-20：Lesson 08 首版实现和最终审查完成。request-local output clamp、between-Runs lazy compaction、Pi prompts、message-level cut、重复 summary、完整 History/Working Context 分离以及失败、取消、并发和 protocol 校验均有确定性测试；审查把连续 cut point 前移的重复线性扫描收敛为二分查找，并补齐 projected input 恰好等于 threshold 的测试。`make check` 与 `go test -race ./...` 全部通过，课程进入待理解确认且尚未提交。
- 2026-07-20：学习者要求把 Lesson 08 直接提交并推送到 `main`；提交 `c967027` 已推送，未创建 feature branch 或 PR。学习者随后明确要求开始 Lesson 09。
- 2026-07-20：Lesson 09 开课源码校准确认渐进披露主线：冻结 Pi 启动时只把 Skill name、description 与 location 放入 system prompt，模型匹配后通过普通 `read` 获取 `SKILL.md` 正文。旧提纲同时被收紧：完整来源发现属于 ResourceLoader/package/settings/trust 组合，而 Pi 的 `read` 可读绝对路径、Pia 的 `read` 严格限制在 workspace；因此全局 Skill 不能在不改变读取安全边界的情况下机械移植。课程先讨论 project-only Skills 与外部 trusted roots 的边界，再形成实现决定。
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
- 2026-07-21：最新复审发现 Unix Skill directory name 可以包含非法 UTF-8 bytes，而 Provider JSON encoding 会用 replacement character 改写 catalog location，令模型随后无法用该 location 读取真实 `SKILL.md`。discovery 现在在候选进入 catalog 前跳过此类目录，并返回自身仍为合法 UTF-8 的有界诊断。
- 2026-07-21：再后续复审发现 `os.Root` 会安全跟随 workspace 内的 `.pia/skills` relative symlink，但 Pia Skill v1 已明确排除 symlink source。source opening 增加前置 non-following check 与打开后的 handle/entry identity 复核，既拒绝 symlink，也不引入 check-then-open 竞态。
- 2026-07-21：同一复审链继续暴露两个残余类问题：source handle 验证后，candidate metadata 仍从 workspace path 重开，source replacement 可混入另一目录；非 UTF-8 过滤也只覆盖 directory 而漏掉 symlink diagnostic path。discovery 改为在 verified source handle 上完成 direct-directory 与 `SKILL.md` 的逐级 no-follow `openat`，并在 filesystem type 分支前统一过滤所有 Skill-like invalid-UTF-8 entry names；回归测试覆盖 source replacement、directory/file symlink 与 Linux 非 UTF-8 directory/symlink。
- 2026-07-21：学习者明确批准此前延后的 repository/module 命名迁移。D59 更新为当前统一契约；module/import path、环境变量、临时文件前缀以及现行课程、计划和引用同步使用 Pia，带日期的历史实现记录保留原始 literal 并明确标注迁移。
- 2026-07-21：Lesson 10 开课先回顾 Provider Request Snapshot 与 compaction 的组成：stable system prompt（含 project instructions 与 Skill metadata）和 tool schemas 不被压缩，Working Context messages 被替换为 synthetic summary 与 retained suffix，完整 History 保持原始。学习者要求把重复 Skill activation dedupe 延后，同时保留有界 durable projection 与 token-budget 讨论义务，记录为 D66。
- 2026-07-21：进一步确认 protected Skill instructions 会成为普通 compaction 无法回收的固定 request 成本后，学习者撤回 dedupe 延后方案。Lesson 10 恢复 stable identity、重复 activation 不复制正文及每个 identity 最多一份 durable projection 的完整闭环；D67 supersedes D66，但第二次调用结果、refresh 与文件变化语义仍留待课程讨论。
- 2026-07-21：学习者继续指出“曾激活”不等于“当前相关”，多个完整 Skill block 永久投影会让 compaction 无法回收 token。课程撤回永久正文 pinning 候选，改为区分 activation snapshot、Working Context residency 与 current relevance；rehydratable snapshot 是当前推荐方向，具体 receipt 表达仍待确认。
- 2026-07-21：横向核对冻结 Pi、OpenCode、Codex open source 与 Grok Build 后，学习者要求选择一个成熟实现而不是自行创新。D68 采用 Grok Build 风格的无状态 `skill(name)` invocation：每次读取当前正文、result 按普通历史参与 compaction，不维护 snapshot、receipt、active set 或跨调用 dedupe；Lesson 10 因此从 Large 收敛为 Medium，D62 的 durable projection 条款和 D67 被 supersede。
- 2026-07-21：学习者确认 oversized Skill 首版采用 50 KiB final-result ceiling、full-or-error 和可恢复 call-local failure；D69 保留普通 `read` 分页降级，不增加 blacklist、failure cache、临时副本或 paged activation，并要求后续按真实效果复评阈值。
- 2026-07-22：横向实现与 Agent Skills client guide 复核后，撤回 current-frontmatter-name equality check。D70 冻结 Conversation catalog lookup，但每次 activation 重读当前位置正文；metadata 与 topology 由新 Conversation 刷新，当前调用不做第二次 discovery 或 stale-body fallback。
- 2026-07-22：学习者确认无状态方案足够简单且便于维护。D71 明确区分 discovery duplicate-name winner 与 activation dedupe：后者当前不支持，重复调用每次重读并产生普通 result；只有真实 trace 证明重复成本成为实际问题时才重新评估。
- 2026-07-22：实现前最后确认把单次 50 KiB 与 mid-Run aggregate context 分开。D72 不在 Skill 层增加计数器、半批跳过或特殊 compaction，把 aggregate overflow recovery 保留为通用 Runtime 韧性责任。
- 2026-07-22：Lesson 10 按 D68–D72 完成实现与主线程审查：coding-owned `skills` package、dedicated `skill(name)` tool、catalog-selected lookup、current-file reread、full-or-error ceiling、普通 compaction 和无 dedupe/protected state 均有确定性测试；`make check` 与 `go test -race ./...` 全部通过。课程进入待理解确认且尚未提交。
- 2026-07-22：学习者确认理解 Lesson 10，并明确要求使用 feature branch commit、push 和创建 PR；课程教学至此完成，真实效果评测义务继续保留。
- 2026-07-22：PR #5 合入 `main` 后，以调用前冻结的协议、固定 `deepseek-v4-pro` 产品配置和 fresh workspaces 完成 Skill v1 真实模型产品路径 checkpoint。连续 `2/2` 相关任务都只选择 `go-regression`、通过 tool result 获得未预载正文、遵守私有测试与 final-marker 要求，并产生在原错误实现上失败、修复后通过的回归测试；连续 `2/2` 无关任务均零 Skill 调用、零 workspace 改动且回答正确。该证据确认当前 one-shot 闭环符合 D68 的预期组合行为，没有暴露需要阻塞后续课程的实现缺陷；它不替代 dedicated-tool baseline 对照或真实 between-Runs compaction 实验，后者仍需未来 multi-Run surface。
- 2026-07-22：学习者明确开始 Lesson 11，并要求同时记录第三阶段系列大纲。开课校准确认 overflow recovery 必须把 failed terminal assistant 与 same-ID not-executed results 作为一个 projection exclusion group，而不能只删最后一条消息；课程按 D73 以 Lesson 11 收尾第二阶段，并把第三阶段组织成九个可重新校准的 Session Runtime capability slots，当前只固定 Lesson 12 全局编号。
- 2026-07-24：课堂追问区分了正常 pre-generation context overflow 与 finish reason 后的通用 stream failure。D74 纠正首轮校准：Lesson 11 只自动恢复不含 completed tool calls 的明确 overflow error；`E2 + N2` 继续是 D33 的通用失败结算形状，不再作为 overflow 主路径或本课验收要求。
- 2026-07-26：学习者确认当前 guard 只保护同一逻辑实例，并要求未来 Session 设计重新评估 Conversation/Core 职责。D75 要求 Session 成为候选唯一外层 lifecycle authority，吸收而不是叠加同义状态，同时把跨 Session workspace 冲突留给未来外层协调。
- 2026-07-26：学习者确认 recovery projection 使用 append-only History 加 absolute exclusions，所有 model-view consumers 共享同一过滤结果；D76 禁止临时 tail deletion 或按错误文本重建。
- 2026-07-26：学习者确认 Lesson 11 每个 accepted user advance 最多一次自动 recovery，同时要求未来 TUI 从用户角度允许失败 settled 后显式重试；D77 将自动 budget 与新的用户控制操作分开，并纠正此前对 Pi flag reset 边界的过度简化。
- 2026-07-26：学习者确认 forced compaction 普通失败采用两段提交：第一次 failed Run 的完整 History 不回滚，recovery exclusion/summary/cut/Working Context candidate 则全有或全无；D78 不复制 Pi 先删 active error、失败后不恢复的部分提交。
- 2026-07-26：学习者确认 cancellation 与 recovery model-view commit section 的边界；D79 在 commit 前丢弃 candidate，进入同步 replacement/publish section 后不回滚 projection，并让 accepted continuation 继续复用既有 Core cancellation settlement。
- 2026-07-26：横向核对 Pi、Codex 与 OpenCode 后，学习者确认 compaction 不进入普通 Conversation History，但未来以独立的 settled typed record 进入 versioned Session journal。D80 固定 journal/event/trace 的责任分离、最小 audit facts 与中途进程崩溃的首版边界。
- 2026-07-26：学习者确认 Lesson 11 先把 overflow classifier 放在 coding private policy，并要求明确标注这是当前阶段的位置。D81 固定窄 matcher、误判排除和代码 TODO 的复评触发条件；D82 同步收敛 Core Run/Continue 与 coding user advance 的术语。
- 2026-07-26：Lesson 11 按 D73–D82 完成实现与主线程多视角审查：Core input-free continuation、tool-call-free explicit-overflow classification、absolute History exclusions、forced compaction、两段提交、同步 model-view commit section 与每次 accepted user advance 一次自动 recovery 均有确定性测试。审查补齐了 continuation Working Context 中间 `nil` message 的拒绝、带 overflow 文案的 5xx 负例，以及 negative evidence 与 positive evidence 共享 normalization 的回归测试；`make check` 与 `go test -race ./...` 全部通过，课程进入待理解确认且尚未提交。
- 2026-07-26：学习者确认理解 Lesson 11，并明确要求直接提交到 `origin/main`。Lesson 11 与第二阶段至此完成；接下来先讨论和校准第三阶段课程系列，不因此自动开始 Lesson 12。
- 2026-07-26：第三阶段规划讨论把 `resume` 收窄为 clean settled Session 的重新加载，并暂定一个 Session 固定绑定创建时的 workspace。D83 记录 durable workspace metadata、原路径不可用时失败、禁止按调用者 cwd 静默 rebind 的理由，以及 repo relocation、worktree、跨主机和真实 TUI/Orchestrator consumer 出现后的强制复评条件。
- 2026-07-26：学习者确认第三阶段优先完成单 Session 日常使用主路径。D84 将课程重排为 events、lifecycle、follow-up、steering、最小交互终端、journal 与 clean resume，固定 `Enter`/`Tab`/`Esc`/`/exit` 产品语义，并把 Provider retry、interrupted recovery、multi-Session isolation 与完整 TUI 移到真实使用证据之后。
- 2026-07-26：Lesson 12 开课校准确认 Pi core event、coding `agent_settled` 与 Pia outer advance 是不同边界。学习者接受 D85 的单个同步只读 observer：交付纳入 settlement，但 observer 不持有业务锁、不能 re-entry，也不提前建设 event bus 或异步 queue。
- 2026-07-26：学习者确认 D86 的 observer failure 边界：首个 write error 停止后续渲染但不取消或回滚 coding work；execution 完整结算后由 host 报告 projection failure，内部 observer panic 不增加通用 recovery layer。
- 2026-07-27：学习者将 Lesson 12 的 cancellation observation 延后到 Session/`Esc` 控制面；本课 settled outcome 收窄为 `success/error`，不增加取消请求事件、专用 line 或专用 event tests，既有底层 cancellation 语义保持不变。
- 2026-07-26：对照 Codex `exec`、OpenCode `run` 与冻结 Pi print/json modes 后，学习者确认 D87：默认 one-shot events 写入 stderr、成功 final text 独占 stdout；显式 JSONL 与 TUI renderer 等待真实消费者。
- 2026-07-27：学习者要求把 Session、Conversation、Core 与 advance 从四个并列大概念收敛为长期对象、交互数据、执行引擎与短期操作，并确认 D88：事实由对应 execution/outer-operation owner 产生，经同一个同步 observer 串行交付；当前 `conversation` 不被固化成永久事件层。
- 2026-07-27：学习者确认 D89：Core terminal settlement 与 History commit 内部保持可区分，但 Lesson 12 不暴露独立 commit event；最终 user-advance settlement 对外保证本次所需 History commits 已完成，未来 journal durability 使用独立持久化协议。
- 2026-07-27：学习者确认 D90 的 Core 最小事件集合：Run、Turn、complete Message 与 Tool 四族；formation/progress、Provider-call、generic error 和 History-commit 不独立成事件，parallel completion 与 source-ordered Message 接受继续分开。
- 2026-07-27：学习者确认 D91 的 outer 最小事件集合：只增加 Advance 与 Compaction 两族；overflow recovery 由 `reason=overflow` 的 compaction 和 input-free continuation Run 组合表达，不建立 Recovery 事件族或 `compaction_skipped`。
- 2026-07-27：学习者确认 D92 的最小 payload，并在核对 Codex/OpenCode/Pi renderer 后补充真实 line consumer 所需的 tool-owned bounded safe summary；events 仍不复制 Message 正文、raw tool arguments/results、原始 call ID 或 error chain。
- 2026-07-27：学习者确认 D93：默认 append-only line observer 只在 tool/compaction 开始时输出 safe summary，成功 settlement 保持安静，error 才追加失败行；不输出通用 working/completed，也不在本课实现 Codex 式动态 read 聚合。
- 2026-07-27：Lesson 12 按 D85–D95 完成测试先行实现、简化与主线程多视角审查：internal value events、同一个同步 observer、Core/outer ownership、tool-owned safe summary、parallel completion handoff、compaction publish boundary、append-only stderr projection 和 observer failure 均有确定性覆盖；`make check` 与 `go test -race ./...` 全部通过。课程进入待理解确认且尚未提交。
- 2026-07-27：学习者明确要求把 Lesson 12 直接提交并推送到 `origin/main`。课程状态更新为已提交；下一课不会因此自动开始。
- 2026-07-28：学习者确认 D96，修正第三阶段 strict clean-only resume：hard-killed Session 可以丢弃 last committed Advance 之后的不完整 tail，从较早 checkpoint 恢复，并向模型提示 workspace 可能保留部分副作用；Runtime 不自动重放。只有保留和继续未完成 Advance 的 in-place interrupted execution recovery 继续延后。
- 2026-07-28：横向核对冻结 Pi、Codex 与 OpenCode 的长期交互对象后，学习者确认 D97：Lesson 13 删除 one-shot compatibility wrapper、coding-owned Conversation controller、长期 Core Working Context 与重复 guards；Session 成为唯一长期 owner，Working Context 按 execution 派生，`internal/agent` 收敛为 run-local execution engine。后续课程同样以新能力暴露的真实责任为准，不把学习过程中的旧结构当成兼容义务。
- 2026-07-28：学习者确认 D98 的两层退出：Session Close 立即停止 admission、取消 active work并在 caller context 内尝试 clean settlement；未来 terminal host 在短暂 grace period 后可直接结束进程，留下可按 D96 checkpoint fallback 恢复的 unclean Session。`3 秒`只作为首版候选，等待上限不固化进 Session Runtime。
- 2026-07-28：学习者确认 D99：Session controls 采用简洁的 `Cancel()` 与 `Wait(ctx)`，不增加冗余的 `Current`；Cancel 是 idempotent request，Wait 只观察 busy settlement 且不复制 Advance outcome，future queue 出现时再校准 quiescence。
- 2026-07-28：学习者授权在三份本地 coding-agent 源码证据上直接确定 Lesson 13 API。D100 固定 `NewSession/Info/Advance/Cancel/Wait/Close`、`SessionConfig/SessionInfo/AdvanceResult`、`agent.Engine` 的 run-local 输入、两个 Session sentinels、单锁/shared completion 的 Close 收敛、constructor ownership transfer 与 one-shot host/trace 迁移；本课不再保留实现前命名或 lifecycle mechanism 未决项。
- 2026-07-28：Lesson 13 按 D97–D100 完成 ownership refactor 与 lifecycle 实现：`Session` 成为 Workspace、History、projection 和 busy/cancel/wait/close 的唯一长期 owner，`agent.Engine` 只保留 immutable dependencies 与 run-local execution，旧 `coding.Run`、`conversation` controller、长期 Core Working Context、replacement API 和重复 guards 已删除。简化审查移除了 `SessionInfo` 与 Session 内部重复的 prompt/tool state；最终审查补齐 CLI settlement/输出契约测试，`make check`、`make race` 和 Session lifecycle 20 轮并发压测全部通过。课程进入待理解确认且尚未提交。
- 2026-07-29：学习者明确要求用 `ce-simplify-code` 再次复核 Lesson 13 是否受 Legacy 设计拖累。主线程三维审查应用四组 quality 整理：删除无消费者的 forwarding constructor wrapper；让 `FinalText`、trace Go 字段和 tests 使用当前 Advance/settlement/Session/Engine 词汇，同时保留 trace JSON `run_error` wire contract；删除 constructor cleanup 的重复条件；没有发现可复用 utility 缺口。减少重复 clone 的 efficiency 候选因会削弱 Session、Engine、Provider 之间的 ownership isolation 而跳过。`make check`、`make race`、Session lifecycle 20 轮并发压测和 `git diff --check` 再次全部通过，课程仍为待理解确认且尚未提交。
- 2026-07-29：学习者提出需要查看单个 Session 的完整动作时间线，以分析 compaction threshold、Provider/model 差异和策略效果。D101 将它记录为 host-owned observer diagnostic recorder，而不是 Session 内第二份 action history 或 durable recovery journal；当前不扩大 Lesson 13，也不提前冻结 trace schema或课号。
- 2026-07-29：学习者确认 Lesson 13 的关键 Session、Cancel/Wait 与 Close 语义，不再要求逐项展开 tests，并明确授权自动完成课程、提交及直接推送 `origin/main`。课程最终验证继续要求 `make check`、`make race`、Session lifecycle 并发压测与 `git diff --check` 全部通过；Lesson 13 至此完成。
