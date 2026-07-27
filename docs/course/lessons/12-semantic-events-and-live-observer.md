# 第 12 课：Semantic events 与实时 line observer

## 当前状态

学习者于 2026-07-26 明确开始本课。冻结 Pi `dcfe36c79702ec240b146c45f167ab75ecddd205`（Agent package `0.80.7`）的事件定义、Agent 交付路径、coding `AgentSession` 转发/settlement 路径与相关测试，以及当前 Pia 的 Provider、Core Agent、Conversation、compaction、runtime 和 command 路径已经完成开课校准。

当前已完成源码校准、逐项设计、测试先行实现与主线程审查。最终实现使用 internal closed value event union 和单个同步 observer，Core/coding producers、tool-owned safe summary、parallel completion handoff 与 `cmd/pia` line projection 均已落地；学习者已确认提交并直接推送到 `origin/main`，课程状态为**已提交**。

本课保持 **Large**。一个真实 line observer 必须同时看到当前 coding advance 中已经存在的 Core Run/Turn/message/tool facts，以及 coding-owned compaction、overflow recovery 与最终 settlement；这些事实跨层但共同服务“执行发生时可观察”这一项独立能力。Session lifecycle、输入队列、持久化和 TUI 可以独立验收，不并入本课。

## 解锁能力

当前 `cmd/pia` 只在整个 one-shot coding advance 结算后打印最后一条 assistant 文本，可选 trace 也只在 Run 返回后从完整结果构造。用户在执行期间无法知道：

- Provider 是否仍在生成，还是已经进入 tool execution；
- 哪个 tool 已开始、哪个已经结束；
- parallel-safe tools 的真实完成先后；
- 是否启动了 threshold/overflow compaction；
- overflow recovery 是否将进入 input-free continuation；
- Core Run 已结束但整个 coding advance 是否仍在恢复；

本课完成后，一个真实非交互 line observer 应能在这些事实发生时按定义顺序观察它们，而不是在结束后反推 transcript。Observer 只投影事实，不获得 Agent Loop、Conversation History、Working Context、compaction 或未来 Session lifecycle 的所有权。

## 开课源码校准

### 已确认的大纲假设

- 冻结 Pi 确实提供 run、turn、message 和 tool 四层 lifecycle。Core `AgentEvent` 包含 `agent_start/end`、`turn_start/end`、`message_start/update/end` 与 `tool_execution_start/update/end`。
- Pi 的低层 loop 以 `await emit(...)` 顺序交付事件。普通 prompt 先发 `agent_start`、`turn_start` 和 user message start/end；每个 assistant terminal 与 tool result 都有 message lifecycle；一轮 assistant 加其 tool results 后才发 `turn_end`；最后发 `agent_end`。
- Pi 的 `Agent` 先依据事件更新自身 state，再按订阅顺序等待 listeners。`agent_end` 只是最后一条 core event；直到它的 awaited listeners 完成，`prompt()`、`continue()` 和 `waitForIdle()` 才真正结算，`isStreaming` 才变为 false。
- Pi 的 coding `AgentSession` 不把 core `agent_end` 当作一次用户操作的最终 settlement。它可以在其后执行 retry、compaction、continuation 或处理新 queue input，最后只发一次更高层的 `agent_settled`。
- Pi 在 coding 层增加 `compaction_start/end` 等事件，并在 `message_end` 路径保存 Session message。Core event delta、live event、Session persistence 和 UI projection 是相邻但不同的责任。
- 当前 Pia 已经有足够多的真实事实源：Core Agent 知道 Run/Turn 和 tool execution；Conversation Owner 知道 threshold compaction、overflow recovery、History commit 和整个 coding advance 的 guard。Lesson 12 不需要先创建 Session persistence 才能建立实时观察。

### 细化后的认识

- Pi 的 `AgentEvent` 不是纯 semantic/terminal stream。`message_update` 承载 text/thinking/tool-call formation delta，`tool_execution_update` 承载工具 partial result；它把高频 progress 与低频 lifecycle 放在同一个 union。Pia 本课只需要后者，不机械复制整个 Pi event set，也不把已有 `ai.Event` 直接暴露成 Runtime event。
- Core `agent_end` 与 coding `agent_settled` 是两个不同边界。当前 Pia 一次 accepted coding user advance 可能依次协调 pre-Run compaction、一个 input-started Core Run、overflow compaction 和一个 input-free Core continuation；任何单个 Core `run_end` 都不足以表示最外层操作已经完成。
- Pi 的 awaited core listeners 不只是 UI callback。`AgentSession` 借它顺序执行 extension handling 和 message persistence，所以慢 listener 会延迟 Run settlement，错误或错误的 re-entry 也可能干扰执行。Pia 的首个 observer 是只读 line projection，不能未经讨论就复制这种权力。
- Pi public `AgentSession.subscribe()` listener 与内部 awaited extension/persistence path 也不是同一种契约：前者是同步通知，后者参与内部正确性和 settlement。Pia 必须先区分 authoritative internal mutation 与 external observation，再决定是否需要一个或多个交付边界。
- Parallel tools 的完成事件与 Conversation 中的 tool-result Message 有意使用两种顺序：完成事件可以反映真实完成顺序，History/Working Context 中的结果必须恢复模型 source order。一个单独的“全都按 source order”规则会丢掉实时性；一个单独的“全都按完成顺序”规则会破坏模型协议。
- 当前 Pia 的完整 History 在每个 Core execution 返回后批量提交 `NewMessages`。因此 Core 产生的 terminal-message observation 不能自动宣称“已经提交到 Conversation History”；若要观察 History commit，必须由实际 owner 在 commit 后报告，不得靠事件名称模糊两者。

### 被推翻的隐含假设

- “在 Core Agent 增加一个 callback 就能覆盖 Lesson 12 全部事实”不成立。Core 不拥有 threshold compaction、overflow recovery、History commit 或外层 coding advance settlement。
- “把 Pi 的 `AgentEvent` union 翻译成 Go 就完成了 semantic events”不成立。Pi union 包含本课明确延后的 token/thinking/tool-call delta 和 tool partial updates，还承载 extension/persistence 所需的 awaited semantics。
- “看到 `run_end` 就表示用户本次操作结束”不成立。Overflow recovery 可以在第一次 Core Run 结束后继续 compaction 和第二次 Core execution；类似 Pi 的 higher-level settled boundary 才能关闭整个 coding advance。
- “事后读取 Conversation History 或 trace 可以等价生成实时事件”不成立。History 不记录 tool 开始时刻、并行完成顺序、compaction attempt 或等待区间；当前 trace 又只在操作返回后构建。
- “事件一旦出现就必然是 durable 或 authoritative state”不成立。Semantic Event 是有序但短暂的 observation；History、Working Context、未来 Session journal 和 observer projection 各自有不同 owner。

## 冻结 Pi 源码与测试路径

- [`packages/agent/src/types.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/src/types.ts#L408-L430)：`AgentEvent` 的四层 lifecycle 和 `agent_end` listener settlement 注释。
- [`packages/agent/src/agent-loop.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/src/agent-loop.ts#L100-L275)：prompt/continue 的起始差异、Turn 循环、tool result 与 `agent_end` 顺序。
- [`packages/agent/src/agent-loop.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/src/agent-loop.ts#L319-L373)：assistant formation updates 与唯一 terminal `message_end`。
- [`packages/agent/src/agent.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/src/agent.ts#L231-L244)：订阅顺序与 awaited listener contract。
- [`packages/agent/src/agent.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/src/agent.ts#L469-L573)：active Run、failure lifecycle、state reduction 与最终 idle。
- [`packages/coding-agent/src/core/agent-session.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/agent-session.ts#L126-L156)：core events 之外的 `agent_settled`、compaction、queue 与 retry event 类型。
- [`packages/coding-agent/src/core/agent-session.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/agent-session.ts#L500-L619)：extension、public notification、message persistence 与 settled coordination。
- [`packages/coding-agent/src/core/agent-session.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/agent-session.ts#L1023-L1065)：多个 core executions 与 post-run work 汇聚成一次 `agent_settled`。
- [`packages/agent/test/agent.test.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/test/agent.test.ts#L126-L228)：失败时完整 lifecycle，以及 async subscriber 对 `prompt()`/`waitForIdle()` settlement 的阻塞。
- [`packages/agent/test/agent-loop.test.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/test/agent-loop.test.ts#L525-L618)：parallel tool completion events 与 persisted results 的不同顺序。
- [`packages/coding-agent/test/suite/agent-session-retry-events.test.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/test/suite/agent-session-retry-events.test.ts#L239-L303)：普通 prompt 和 tool-call Run 的完整 Session event order。
- [`packages/coding-agent/test/suite/regressions/6363-agent-settled-event.test.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/test/suite/regressions/6363-agent-settled-event.test.ts#L29-L90)：retry 和 `agent_end` handler 新增 follow-up 后仍只产生一次最终 `agent_settled`。

## 当前 Pia 路径

- `internal/ai/stream.go`：`ai.Event` 描述一个 assistant response 的形成过程；`TextDeltaEvent` 等属于 Provider protocol/formation，不是本课要直接公开的 semantic lifecycle。
- `internal/agent/loop.go`：`receiveAssistant()` 消费 formation events，但只返回一个 authoritative terminal assistant；`executeRun()` 追加 terminal、执行 tools、追加 ordered results，最终同步返回 `NewMessages`，尚无实时 observer。
- `internal/agent/tool.go`：staged scheduler 已区分 parallel-safe stage 与 serial barrier，但 parallel workers 只把 outcomes 写回 source-order slots；真实 start/end/completion timing 目前不向外暴露。
- `internal/coding/conversation.go`：Conversation Owner 持有整个 accepted coding advance 的 guard，协调 pre-Run compaction、Core Run、History commit、overflow recovery、Core Continue 和最终 History snapshot；它是 outer settlement 的事实源。
- `internal/coding/compaction.go`：threshold/overflow trigger、summary request、candidate validation 和 Working Context/projection commit 都发生在 coding 层；内部 summary response 不是 Conversation Message。
- `internal/coding/recovery.go`：只负责当前 explicit-overflow eligibility，不拥有事件交付。
- `internal/coding/runtime.go`：one-shot composition 目前没有 observer dependency，也没有长期 Session。
- `cmd/pia/main.go`：stdout 只在 operation settled 后输出 `FinalText()`；可选 trace 也在返回后构造，不能证明实时事件。

## 第一组概念边界

Semantic Event 是“运行过程中，一个有意义的事实已经发生”的短暂、有序通知。它不是形成中的 token，也不是权威状态本身。

### 收敛后的运行心智模型

`Session`、`Conversation`、`Core` 和 `advance` 不是四个并列的控制器，而是四种不同性质的概念：

```text
Session                              长期 lifecycle 对象
├── Conversation state               Session 拥有的数据
├── Core execution engine            Session 调用的执行机制
└── Advance(user input)               Session 执行的一次短期操作
```

- **Session 是唯一外层 lifecycle 对象。** 它将来负责同一个 workspace/Conversation 的 busy、queue、cancel、close、journal 和 resume；正式 Session 出现后，不能再与 Conversation/Core 叠加同义的外层状态。
- **Conversation 是 Session 内的交互数据。** 它以 complete History 和当前 model-view projection 为核心，不是独立接收命令、排队或关闭的控制器。
- **Core Agent 是执行引擎。** 一次 Core Run 完成一个 Agent Loop execution：一个或多个 Provider calls、模型要求的 tool calls，以及这些调用的结算。`Run(userInput)` 和 `Continue()` 是同一引擎的两种启动方式。
- **User advance 是一次操作。** 它处理一个用户提交，按需 compact，调用 Core 一次或多次，提交 History 并返回结果；它不是必须拥有独立类型、package、锁或持久状态机的第四个长期对象。
- **Event 和 Trace 只在旁边观察。** Event 是实时观察，Trace 是事后诊断；两者都不能取得 lifecycle 所有权、改变状态或决定下一步执行。

当前 Lesson 12 尚无正式 Session。现有 coding-owned `conversation` 同时持有 History/projection 并临时承担 user-advance coordination，因此是 Session 出现前的过渡性 owner，而不是长期要与 Session 并列的控制层。本课可以从它产生外层事实，但事件语义不得绑定这个临时类型名。后续 Session lifecycle 课程必须让 Session 吸收重叠的 `active` 职责，再依据真实 restore/safe-point 需求复评 Core 的窄执行 guard；不能机械增加第三层 `busy`。

一次操作的流程可以继续写成：

```text
Session.Advance(user input)
├── 按需整理上下文（compaction）
├── 调用一次 Core Run
├── 必要时 recovery，再调用一次 Core execution
└── 提交 Conversation History 并返回本次结果
```

Compaction 虽然也可能调用 LLM，但不属于 Core Run。判断 owner 不能只看“有没有调用 LLM”，而要看调用目的：Core Run 用模型完成用户任务并可执行 coding tools；compaction 用模型整理下一次执行所需的 Working Context，不提供 coding tools，也不产生普通 Conversation assistant message。因此 compaction 由协调 History 与 Working Context 的 coding 层负责。

Turn、Message、History、Working Context 等词仍有精确含义，但它们不是额外的顶层控制器。平时理解流程时只需先记住“一个 Session 拥有对话数据并调用 Core”；进入具体排序时，再区分一次 user advance 可能包含一个或多个 Core executions。

| 事物 | 回答的问题 | 当前或未来 owner | 是否权威/持久 |
|---|---|---|---|
| Provider formation event | assistant 内容正在怎样形成 | `internal/ai` stream | 否；本课不向 Runtime observer 暴露 delta |
| Semantic Event | Run、Turn、tool、compaction 或 settlement 刚发生了什么 | 知道事实的 execution/outer-operation owner | 有序 observation；默认不持久 |
| Conversation History | 这个 Conversation 接受过哪些完整 Messages | 当前 Conversation role；未来 Session | 当前内存权威；未来由 Session journal 支撑恢复 |
| Working Context | 下一次 Provider call 实际使用哪些 Messages | Core Agent | 可替换模型视图，不是完整历史 |
| Trace | 一次已经返回的执行有哪些诊断投影 | coding/command diagnostics | 可选事后产物，不是恢复源 |
| Session Journal | 跨进程恢复时哪些 committed facts 可被信任 | 未来 Session persistence | 持久权威；不在本课实现 |

一个包含 `read` 的普通 user advance 可以概念性地观察为：

```text
user advance accepted
  core Run accepted
    Turn started
      terminal assistant accepted: read(call-1)
      tool call-1 started
      tool call-1 settled
      terminal tool result accepted
    Turn settled
    Turn started
      terminal assistant accepted: final answer
    Turn settled
  core Run settled
user advance settled
```

History 最终只保留 user、两个 terminal assistants 和 tool result。它不会也不应该伪造 `tool started`、耗时或 outer settlement。反过来，observer 看到了 `tool settled` 也不表示它可以修改 History 或成为未来 resume 的数据源。

Overflow recovery 又说明为什么需要两个 lifecycle 层：

```text
core Run settled with overflow error
  -> coding-owned compaction attempt
  -> committed projection
  -> input-free Core continuation
  -> user advance finally settled
```

这里是两个嵌套的**操作边界**，不是两个长期对象：第一次 Core `run_end` 是事实，但不是整个 user advance 的结束。

## 当前课程边界

本课要完成：

- 为当前已经存在的 Core Run、Turn、terminal Message 与 tool execution 建立有界 semantic observations；
- 为 coding-owned compaction 和 outer coding-advance settlement 建立对应 observations，并用这些事件与 continuation Run 的组合表达 overflow recovery；
- 明确定义 serial/parallel tool event ordering 和 observer failure/re-entry 边界；
- 提供至少一个真实 line observer，在执行期间消费这些 events；
- 用 Faux Provider 和受控 tools 确定性验证顺序、错误、并行完成与 compaction/recovery。

本课不做：

- token、thinking 或 tool-call argument delta；
- tool partial-progress callback；
- steering、follow-up 或输入 queue；
- Session busy/idle/wait/close API；
- event persistence、durable journal、resume 或 event sourcing；
- full-screen TUI、主题、按键和 slash commands；
- public SDK、Gateway、gRPC、IM 或多 Session orchestration；
- cancellation request event、专用 settlement outcome 或专用 line；这些与 Session-owned `Cancel()` 和 TUI `Esc` 一起设计。
- generic Provider retry。

## 第二组教学：observer delivery

事件事实由执行 owner 产生，但还需要决定怎样交给只读 observer。当前只比较两个足以覆盖主路径的候选：

| 候选 | 好处 | 代价 |
|---|---|---|
| 单个同步 observer | 没有后台 goroutine、queue close、丢弃和 drain 协议；事件交付结束后调用才返回，顺序容易验证 | 慢或永不返回的 observer 会延迟 execution settlement |
| 后台异步 queue | 正常情况下 line rendering 不阻塞执行 | 必须决定容量、满队列 backpressure/丢弃、writer failure、关闭、drain、取消和 goroutine 生命周期；返回时还可能有未显示事件 |

### 已确认的最小方向

学习者确认 Lesson 12 使用一个由 composition 安装的**单个同步只读 observer**，而不是通用多订阅者 event bus：

- producer 在语义事实成立后调用 observer；observer 只接收 ownership-independent 的有界 event；
- observer 调用发生在一个 coordinator 路径，不从 parallel tool workers 并发调用同一个 observer；
- 调用 observer 时不持有 Agent/Conversation state mutex，observer 不能通过 event 引用修改权威状态；
- final event 交付完成后，当前 Run/advance 才向调用方返回，因此返回后不会再出现属于它的 late events；
- observer 只做有界 line projection，不提供 reentrant Run、wait-for-idle 或状态修改能力；
- parallel tool workers 仍可并行执行；coordinator 按观察到的 completion 顺序交付 tool-settled events，同时继续按模型 source order 提交 tool-result Messages。

这里“observer delivery 属于 settlement”只表示返回前已把最后一条事实交给 observer，不表示 observer 成为 History/persistence owner。同步方案确实允许一个错误实现通过永久阻塞来拖住调用；首版用已知 line observer、无业务锁调用和禁止 re-entry 把风险限定在输出 adapter。只有真实慢 consumer、网络订阅或多个独立 subscribers 出现后，才值得引入异步 queue 及其完整 lifecycle。该决定记录为 D85。

Line writer failure 采用同样的 projection/authority 分离：observer 记录第一项 write error 并停止后续渲染，Core Run 与 outer coding advance 仍完整结算，之后由 host 报告 projection failure；若 coding execution 也失败，则两项原因都必须保留。Write failure 不回滚已完成的 Provider/tool/History 事实，也不自动启动 cancellation。同步 writer 永不返回仍是首版已知限制；内部 observer panic 视为程序错误，本课不增加通用 recovery layer。该决定记录为 D86。

### 已确认的默认 one-shot 输出契约

横向核对的产品都先区分交互 UI、人类可读 one-shot 和机器事件模式，而不是让一种 stdout 格式同时承担三种责任：

- [Codex 非交互模式](https://learn.chatgpt.com/docs/non-interactive-mode)明确规定 `codex exec` 将 progress 写入 `stderr`、只将 final message 写入 `stdout`；`--json` 会把 `stdout` 切换成完整 JSONL event stream。
- [OpenCode CLI](https://opencode.ai/docs/cli/)区分默认 TUI、`run` 和 `run --format json`。固定源码中的 [`run.ts`](https://github.com/anomalyco/opencode/blob/7534d23551f665e65080809975b4ca5c7d63807b/packages/opencode/src/cli/cmd/run.ts#L678-L773)将 JSON events 写入 stdout、在 non-TTY 下将 final text 写入 stdout；[`ui.ts`](https://github.com/anomalyco/opencode/blob/7534d23551f665e65080809975b4ca5c7d63807b/packages/opencode/src/cli/ui.ts#L31-L39)则将普通 UI/tool 状态写入 stderr。其显式 `--thinking` 在 non-TTY 下可以混入 stdout，不作为 Pia 要复制的严格契约。
- 冻结 Pi 的 [`print-mode.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/modes/print-mode.ts#L17-L19)直接把 text 定义为 final-response-only、json 定义为 all-events；[`output-guard.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/output-guard.ts#L45-L69)还把非交互模式中的意外 stdout 重定向到 stderr。

学习者据此确认 D87：

```text
semantic events ── line observer ──> stderr
successful coding final text ──────> stdout
```

Line observer 不复制完整 final text。成功 advance 即使遇到 observer write failure，settlement 后仍尝试一次 final stdout，再由 host 报告 projection error；coding advance 自身失败则保持错误写 stderr、非零退出，不产生伪成功 stdout。两个 OS streams 被外部合并后的相对顺序不作保证。

这只是 `cmd/pia` 的默认 human-readable renderer contract，不进入 Core/coding control flow。未来 JSONL renderer 会显式把 stdout 解释成结构化 event stream；未来 TUI 则直接消费 semantic events 并管理整块终端。Lesson 12 不实现两者，也不在没有污染证据时复制 Pi 的 process-wide stdout takeover。

### 已确认的事实产生与串行交付边界

学习者在收敛 Session/Conversation/Core/advance 概念后确认 D88：事实由最先权威知道它的责任产生，再交给 composition 安装的同一个同步 observer；不让外层 coding package 在 Core 返回后重建实时过程。

- Core execution engine 产生 Core Run、Turn、terminal Message 和 tool execution 事实。
- Outer user-operation coordinator 产生 compaction、overflow recovery 和 user-advance settlement 事实，也是任何 History-commit observation 的唯一合法事实源。当前该责任由 coding-owned `conversation` 临时承担；未来由正式 Session 吸收，不改变事件语义。是否实际暴露独立 History-commit event 仍由下一项讨论决定。
- 两个 producer 共用一个 observer。当前调用链是 outer coordinator 同步调用 Core，因此除 parallel tools 外天然形成一条嵌套的串行交付路径，不需要中央 event queue 或长期 serializer goroutine。
- Parallel tool workers 不直接调用 observer。Worker 只向本次 stage 的 Core coordinator 交回一次 outcome；coordinator 按实际观察到的完成顺序交付 `tool-settled` observation，同时继续按模型 source order 保存并提交 tool-result Messages。
- 这个有界、stage-local completion handoff 在 stage 返回前全部消费完，不是 observer queue：它没有独立 drop/backpressure policy、返回后的 late events、长期 goroutine 或跨 advance lifecycle。
- Observer 调用继续遵守 D85：不持有 Core/outer-owner state mutex，不允许 re-entry；writer 阻塞只按已经接受的同步 projection 限制处理。

因此这里的“统一”只表示共享一个 observer 和一条可见顺序，不表示建立一个拥有所有运行事实的中央控制层。事件类型和文案使用稳定的 semantic responsibility，不把当前过渡性 `conversation` 类型提升成永久公开架构。

### 已确认的 terminal 与 History commit 观察边界

Core terminal settlement 与 Conversation History commit 在内部仍是两个事实：Core 先接受 terminal assistant 或 ordered tool-result Messages 并更新 Working Context；Core execution 返回后，outer coordinator 才把完整 Run Message Delta 追加到 complete History。前一个 live observation 不宣称 durability 或 complete-History commit。

学习者确认 D89：Lesson 12 不增加独立 History-commit event。Observer 看到 terminal Message、Core Run settlement、compaction/recovery 和最终 user-advance settlement；最后一项保证本次操作需要的 History commits 与 final snapshot 都已完成。Overflow recovery 中，失败 Run delta 先提交，随后才开始 recovery compaction，但 line observer 不为这次内部 ownership handoff 增加一行。

未来 Session Journal 的 crash-safe append 是另一套持久化协议。已交付 live event 不能作为恢复证据，journal commit 也不伪装成普通 Conversation Message。只有出现真实逐 Run commit consumer 后，才重新讨论由 outer owner 增加该 observation。

### 已确认的 Core 最小事件集合

学习者确认 D90：Core execution engine 只保留四个 semantic event families：

| 事件族 | 事件 | 成立边界 |
|---|---|---|
| Run | `run_started` / `run_settled` | 一个 input-started Run 或 input-free continuation 被接受 / 其全部 Provider、tool 与 terminal work 已结束 |
| Turn | `turn_started` / `turn_settled` | 一次 Provider response cycle 即将开始 / terminal assistant 与其全部 ordered tool-result Messages 已接受 |
| Message | `message_accepted` | 一条完整 user、terminal assistant 或 tool-result Message 已进入 Core Working Context |
| Tool | `tool_started` / `tool_settled` | 一次模型请求的工具执行开始 / 结束 |

普通工具流程按语义观察为：

```text
run_started (input)
message_accepted (user)
turn_started
message_accepted (assistant with tool call)
tool_started
tool_settled
message_accepted (tool result)
turn_settled
turn_started
message_accepted (final assistant)
turn_settled
run_settled
```

Parallel tools 的真实完成顺序与 model-message order 保持分离：

```text
tool_started A
tool_started B
tool_settled B
tool_settled A
message_accepted result-A
message_accepted result-B
```

本课不增加 `message_started/update`、token/thinking/tool-call formation、`tool_update`、独立 Provider-call、generic error 或 History-commit event。所有非成功 settlement 暂时统一为 `error`；cancellation-specific observation 延后到 Session/`Esc` 课程。事件存在不等于 line renderer 必须为它输出一行。

### 已确认的 outer 最小事件集合

学习者确认 D91：outer user-operation coordinator 只保留两个 semantic event families：

| 事件族 | 事件 | 成立边界 |
|---|---|---|
| Advance | `advance_started` / `advance_settled` | 一次 user advance 已被接受 / 其中全部 Core executions、必要 commits、compaction/recovery work 与 final snapshot 已结算 |
| Compaction | `compaction_started` / `compaction_settled` | 一次 threshold 或 overflow compaction attempt 真正开始 / 在 commit 前失败，或新的 model-view projection 已成功发布 |

普通 threshold compaction 路径为：

```text
advance_started
compaction_started (reason=threshold)
compaction_settled (success)
run_started (input)
...
run_settled
advance_settled (success)
```

Overflow recovery 路径为：

```text
advance_started
run_started (input)
...
run_settled (error)
compaction_started (reason=overflow)
compaction_settled (success)
run_started (continuation)
...
run_settled
advance_settled
```

这里有一个重要 ownership 边界：第一次 Core Run 只知道自己以 error 结算，不拥有 coding 层的 context-overflow classifier。后续 `compaction_started(reason=overflow)` 才说明 outer coordinator 已将该错误判定为 eligible overflow，紧接的 input-free continuation Run 表明 recovery 正在继续。因此本课不增加独立 Recovery 事件族、Recovery 状态机或 `compaction_skipped`。

Concurrent-advance rejection 发生在 acceptance point 之前，不产生 `advance_started`。一旦 advance 被接受，无论 success 还是 error 都必须恰好产生一次 `advance_settled`。同理，只有真正进入 compaction attempt 才产生 start/settled pair；`compaction_settled(success)` 必须位于 projection commit 之后。既有 cancellation 若发生，不改变原有执行与 commit 语义，但在本课 event outcome 中只按 generic `error` 观察。

结合 D90，当前完整语义集合只有 Advance、Compaction、Run、Turn、Message 与 Tool 六族。Recovery 是这些事实的一种有序组合，不是第七个顶层概念。Observer write failure 继续只属于 projection failure，不改变这六族事件的业务 outcome。

### 已确认的 bounded payload

学习者确认 D92：event 只带识别实时事实所需的最小有界状态，不复制业务正文或权威对象。

| Event | 最小 payload |
|---|---|
| `advance_started` | 无 |
| `advance_settled` | `outcome` |
| `compaction_started` | `reason: threshold/overflow` |
| `compaction_settled` | `reason + outcome` |
| `run_started` | `mode: input/continuation` |
| `run_settled` | `outcome` |
| `turn_started` | 无 |
| `turn_settled` | `outcome` |
| `message_accepted` | `role`；assistant 可带固定 `stop_reason`，tool result 可带 `is_error` |
| `tool_started` | 本 Turn 内的 source-order index、有界 tool display name 与 tool-owned safe summary |
| `tool_settled` | 同一 index、display name/summary、`outcome` |

Settled outcome 只使用 `success/error` 两个类别。既有 assistant `stop_reason=aborted` 仍可出现在 `message_accepted` 中，因为它是已接受 Message 的协议事实，但不为其他 event 增加 aborted outcome。Tool source index 用来配对同名 parallel calls；event 不传播 model-generated ToolCall ID。Tool display name 与 safe summary 在 event construction 时独立复制并限制长度，renderer 再转义换行、ANSI 与其他控制字符。

当前 tools 只产生窄的 operator-facing summary：`Read <path>`、`Write/Edit <path>`、`Bash <bounded command>` 或 `Skill <name>`。理解 schema 的 tool responsibility 生成该 summary；generic renderer 不接收后再按 tool name 解码 raw JSON。Event 不携带 user/assistant text、compaction summary、raw tool arguments/result、raw error text/chain、workspace identity、Provider/model 或 token usage，也不暴露 `ai.Message`、`json.RawMessage`、slice、map 或其他可变状态引用。Run/advance error 继续从调用结果交给 host，Provider/tool 详细错误继续留在其 authoritative Message，trace 仍是事后诊断面。

因此本课 line observer 只能报告有界 action summary 与结算状态，不是第二份 History。One-shot final assistant text 仍从 settled advance result 输出；未来 TUI 若需要实时正文、tool-result preview、duration 或 cross-Session identity，必须由真实 consumer 重新证明并增加独立的有界 projection。

### 已确认的默认 line projection

学习者要求先对照真实产品，再决定 human renderer。对本地源码的复核得到：

- Codex `0fb559f0` TUI 把相邻 read-shaped calls 合并并去重到一个可更新的 `Exploring/Explored` cell，例如 `Read auth.rs, shimmer.rs`；没有 per-file completed line。`codex exec` 是另一套 append-only renderer，会分别显示 command start 与 succeeded/exited。
- OpenCode `cb562b2c` TUI 默认逐 tool 显示，running read 使用 spinner，成功后留下 `Read <path>` 而不写 completed；`opencode run` 对普通 read 主要在 completed/error 时输出单行。
- 冻结 Pi `dcfe36c7` TUI 同样为每个 tool call 建立可更新 component，成功 read 只改变 pending/success visual state且默认不展开 result；text print mode 不输出 progress。

因此学习者确认 D93：完整 event stream 与默认文本密度分开。

```text
tool_started  -> pia: Read internal/agent/loop.go
tool_settled(success) -> no line
tool_settled(error)   -> pia: Read internal/agent/loop.go failed
```

Compaction 采用同样原则：start 报告 `threshold` 或 `overflow` action，success 安静，error 追加 failed。Run、Turn、Message 和正常 Advance events 仍完整交付，但默认不输出；也不增加通用 `working/completed`、timestamp、duration、spinner、ANSI overwrite 或颜色。

Codex-style read aggregation 暂不进入本课。它依赖可变 TUI cell 与 grouping boundary；为 append-only renderer 增加 buffer/stage metadata 会让简单 line output 获得不必要的调度状态。未来最小交互终端可直接从 per-tool start/settled events 动态合并相邻 reads，不需要修改 Core settlement。

## 已确认的 cancellation 延后边界

学习者确认 D94：本课不增加 `cancel_requested`、`aborted` settlement outcome、cancellation-specific line、监听 `ctx.Done()` 的 observer goroutine 或专用 event tests。现有 caller-context cancellation 行为保持原样；若它在本课观察路径发生，settled event 暂按 generic `error` 表达。等 Session 拥有 `Cancel()` 且最小交互终端实现 `Esc` 时，再依据真实控制面区分 cancellation request、`Canceling` 与真正 `Canceled`。

设计阶段至此收敛了 Lesson 12 的观察语义；随后的结构审查只选择 Go event representation、package placement 与 tool safe-summary hook，没有扩张课程能力。

## 最终实现落点

实现没有建立 Session、event bus 或第二套 lifecycle owner，也没有重排现有 package：

- `internal/observation/event.go` 定义仅供仓库内部使用的六族 closed value event union、nil-safe 同步 `Observer` 和二元 settlement outcome。Tool display name 与 summary 分别在事件构造时复制并限制为 64/512 bytes，截断保持合法 UTF-8 并留下可见 `...`。
- `internal/agent` 在既有 acceptance/settlement 点产生 Run、Turn、Message 与 Tool facts。`Tool.DescribeInvocation` 是 tool-owned、无副作用的窄 projection hook；没有 observer 时不调用。Parallel workers 只向 stage-local buffered completion channel 交回 outcome，Core coordinator 串行发布 completion-order Tool events，最后仍按 source order 接受 ToolResult Messages。
- `internal/coding` 把同一个 observer 交给 Core 与当前 outer coordinator。Advance settlement 发生在 History commit 和 final snapshot 之后、active guard 释放之前；compaction success settlement 发生在 Working Context replacement 与 projection metadata publish 之后。
- `cmd/pia/line_observer.go` 是本课真实 consumer：只把 tool/compaction start 与 error settlement 写入 `stderr`，成功 settlement 安静，控制字符转义为一行。首个 writer error 被保存并停止后续 projection；coding work 继续结算，成功 final assistant text 仍尝试写入 `stdout`，host 最后保留 projection error。

确定性测试直接固定普通多-Turn 顺序、serial tool call-local error、parallel completion/source-order 分离、Provider failure、threshold compaction 成功与失败、projection publish 边界、overflow recovery 与 continuation、Core/outer active guard，以及 line rendering 和 output failure。Cancellation-specific event 与 test 按 D94 没有加入；既有底层 cancellation tests 保持不变。

最终 `make check` 与 `go test -race ./...` 均通过。学习者随后明确要求把本课直接提交并推送到远端 `main`；Lesson 12 至此完成，下一课仍需在学习者明确开始后才能进入。

## 完成信号

本课只有在以下条件同时成立时才完成：

- 学习者能区分 Provider formation、Semantic Event、Conversation Message、History、trace 和 journal；
- 事件 owner、交付/阻塞、失败、re-entry 与并行排序契约已经讨论确认；
- 一个真实 line observer 在当前 coding execution 发生时按约定显示事实；
- 确定性测试覆盖普通多-Turn Run、serial/parallel tools、Provider/tool failure、threshold compaction、overflow recovery 和最终 outer settlement；
- `make check` 与 `go test -race ./...` 通过；
- 课程记录、共享词汇和 durable decisions 与最终实现同步，并由学习者确认理解。
