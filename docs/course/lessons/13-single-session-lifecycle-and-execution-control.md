# 第 13 课：单 Session ownership、lifecycle 与执行控制

## 当前状态

学习者于 2026-07-27 明确开始本课。冻结 Pi `dcfe36c79702ec240b146c45f167ab75ecddd205`（Agent package `0.80.7`）的 core `Agent` lifecycle、coding `AgentSession`、`AgentSessionRuntime` 与并发/settlement 测试，以及当前 Pia 的 one-shot composition、Conversation、Core Agent、Workspace 和 Lesson 12 observation 路径已经完成开课校准。

本课状态为**已完成并提交**，规模保持 **Large**。多次顺序 Advance、busy/idle、cancel、wait、close、外层 guard 迁移和 workspace lifetime 共同定义一个不可再少的长期 Session lifecycle；follow-up、steering、终端、journal 和 resume 都可以独立验收，不并入本课。源码证据、边界、实现和验证结果均已记录。

## 解锁能力

开课前的 `internal/coding.Run` 每次调用都会：

1. 创建 Provider；
2. 打开 Workspace；
3. 发现 Skills，构造 tools、system prompt、Core Agent 与 Conversation；
4. 执行一次 user Advance；
5. 关闭 Workspace 并丢弃全部内存 owner。

这足以支持 one-shot 命令，却不能让同一个 coding task 在一个进程中继续对话，也没有可供未来 `Esc`、`/exit` 和输入 queue 调用的长期控制边界。

本课完成后，一个 in-memory Session 固定拥有同一个 Workspace/resources、Conversation History、compaction projection 和 lifecycle，并允许宿主顺序发起多次 Advance。它调用一个不保存长期 Conversation state 的 run-local Agent Execution Engine。宿主可以请求取消当前 Advance、等待它真正 settlement、从 admission error 识别 busy/closed，并在没有 active work 后确定性关闭资源；本课不增加公开状态轮询 API。

## 开课源码校准

### 已确认的大纲假设

- 冻结 Pi 的 core `Agent` 为每个 active Run 创建独立 abort controller 和 settlement promise。并发 `prompt()`/`continue()` 会被拒绝，`abort()` 只请求取消，`waitForIdle()` 等待该 Run 及其 awaited listeners 全部结算。
- Pi coding `AgentSession` 另有 session-level active/idle boundary。一个 prompt 可以在首个 core Run 后继续 retry、compaction 或 input-free continuation，最后才发一次 `agent_settled` 并唤醒 idle waiters。
- Pi 的 `AgentSession.abort()` 实际组合了“请求 core abort”和“等待 session idle”两个动作；`dispose()` 则同步请求若干活动停止并释放 Session resources，但本身不保证等待所有 active work settlement。
- 当前 Pia 的 semantic `Advance` 已经是正确的外层操作边界：它包含 pre-Run compaction、一个或多个 Core executions、History commits、final History snapshot 和 `advance_settled` observation。
- 当前 Pia 的 Workspace 拥有 `os.Root`，file tools 只借用它；因此 Workspace 只能在所有可能使用它的 tool work settlement 后关闭。Provider 当前没有额外的 `Close` contract。

### 细化后的认识

- Session 外层状态应命名为 `busy/idle/closed`，不应照搬 Pi coding 层的 `isStreaming`。Pi 在 `_runAgentPrompt()` 才把 `_isAgentRunActive` 设为 true，但 `prompt()` 在这之前可能先做 compaction；Pia 已经把这类 preflight 纳入 Advance，因而 busy 必须从 Advance acceptance 持续到最终 settlement。
- Cancel request 与 settlement 是两个不同事实。取消信号到达后，Provider、tool、terminal message、History commit、observer 和 cleanup 仍可能需要时间收敛；Session 不能因“已经请求取消”就提前发布 idle 或关闭 Workspace。
- Wait 是观察既有 active Advance 的 settlement，不是启动新工作，也不隐式取消。它必须等待 outer Advance settlement，而不是只等待其中一个 Core Run。
- Lesson 12 已经给出可复用的 `advance_started/advance_settled` 事实，Lesson 13 不需要增加第二套“session run”事件来表达同一操作。
- 当前 one-shot 命令的 final stdout、semantic-event stderr、trace、Skill diagnostics 与错误语义属于应保留的 observable behavior；product-level `coding.Run` 函数和相关输入/结果类型没有外部兼容契约，不需要作为 wrapper 保留。

### 被推翻的隐含假设

- “在现有 `conversation.active` 外再加一个 `session.busy` 最安全”不成立。两个 outer guards 保护同一个 user Advance，却可能在拒绝、取消、wait 或 close 时产生分歧；正式 Session 应吸收这项外层 active responsibility。
- “只要 Core Run 结束，Session 就 idle”不成立。Overflow recovery、History commit、final snapshot 和 synchronous observer settlement 都可能发生在某次 Core `run_settled` 之后。
- “Cancel 调用返回就可以关闭资源”不成立。取消只是 cooperative request；正在运行的 Provider/tool 必须先观察取消并完成既有 settlement。
- “Pi 的 `dispose()` 可以直接作为 Pia `Close` 契约”不成立。Pia 的 Go Workspace 持有真实 `os.Root`，在 active tools 仍借用它时关闭会破坏资源所有权；本课必须给出确定的 settlement-before-close 语义。
- “Lesson 13 只需把 `conversation.active` 移到 Session，stateful Core 可以原样保留”不成立。横向源码核对和后续 journal/resume 路径表明，长期 Working Context 与 idle-only replacement 会让 Session projection 和 Core state 形成双重提交；D97 因而确认删除长期 Core context、`ReplaceWorkingContext` 与 Core active guard，由 Session 派生每次 execution 的 snapshot。

## 冻结 Pi 源码与测试路径

- [`packages/agent/src/agent.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/src/agent.ts#L231-L244)：awaited listeners 属于当前 core Run settlement。
- [`packages/agent/src/agent.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/src/agent.ts#L304-L350)：core `abort()`、`waitForIdle()` 与 active prompt/continue rejection。
- [`packages/agent/src/agent.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/src/agent.ts#L469-L527)：active Run 的 controller/promise、failure terminal 与最终 idle transition。
- [`packages/coding-agent/src/core/agent-session.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/agent-session.ts#L269-L304)：coding Session 的 active/idle、queue 与多个 cancellable activity fields。
- [`packages/coding-agent/src/core/agent-session.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/agent-session.ts#L515-L542)：session-level idle promise 与 `agent_settled` 后的唤醒顺序。
- [`packages/coding-agent/src/core/agent-session.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/agent-session.ts#L799-L845)：同步 `dispose()` 以及 `isStreaming/isIdle` 投影。
- [`packages/coding-agent/src/core/agent-session.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/agent-session.ts#L1023-L1034)：多个 post-run continuations 汇聚成一次 session settlement。
- [`packages/coding-agent/src/core/agent-session.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/agent-session.ts#L1076-L1224)：busy input handling、pre-Run compaction 与 core prompt 的实际先后。
- [`packages/coding-agent/src/core/agent-session.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/agent-session.ts#L1498-L1512)：coding `abort()` 请求取消并等待 session idle。
- [`packages/coding-agent/src/core/agent-session-runtime.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/agent-session-runtime.ts#L67-L106)：Runtime 拥有当前 AgentSession 与 cwd-bound services。
- [`packages/coding-agent/src/core/agent-session-runtime.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/agent-session-runtime.ts#L390-L397)：quit shutdown 与同步 Session dispose。
- [`packages/agent/test/agent.test.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/test/agent.test.ts#L195-L265)：`waitForIdle()` 等待 listener settlement，active signal 能被 abort。
- [`packages/agent/test/agent.test.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/test/agent.test.ts#L468-L548)：idle abort、并发 prompt/continue rejection 与取消 cleanup。
- [`packages/coding-agent/test/agent-session-concurrent.test.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/test/agent-session-concurrent.test.ts#L61-L180)：active prompt rejection、abort settlement 与后续 prompt reuse。
- [`packages/coding-agent/test/suite/regressions/6363-agent-settled-event.test.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/test/suite/regressions/6363-agent-settled-event.test.ts#L29-L125)：多个 core executions 之后只发一次 `agent_settled`，session wait 等到该边界。

## 开课时 Pia 路径

- `internal/coding/runtime.go`：当前 one-shot composition 同时负责依赖构造、单次 Advance 和 Workspace close；尚无可复用 lifecycle object。
- `internal/coding/conversation.go`：Conversation 保存 History/projection，也以 `active` guard 临时协调完整 Advance；这是本课要重新分配的重叠责任。
- `internal/agent/types.go`、`internal/agent/loop.go`：Core Agent 保存 Working Context，并用窄 guard 保护一个 `Run`/`Continue` execution 和 idle-only context replacement。
- `internal/coding/workspace.go`：Workspace 拥有 `os.Root`，所有 file tools 借用它；它是当前最明确的 Session-owned closeable resource。
- `internal/coding/runtime_test.go`：现有测试已证明 Workspace 必须等一个 Advance 的成功、失败或取消 settlement 后才能关闭。
- `internal/coding/conversation_test.go`：现有测试已证明一个 Conversation 可顺序复用，并对并发 Advance fail fast；本课要把后一个契约迁移到 Session。
- `internal/coding/observation_test.go`：已证明 `advance_settled` 位于所有嵌套 Run、compaction、History snapshot 与 observer settlement 之后。
- `cmd/pia/main.go`：当前 one-shot host 将直接构造 Session、Advance 一次并 Close；交互输入仍留给后续终端课程。

## 第一组教学：只保留一个长期控制器

先把四个容易混淆的词重新放回各自层级：

```text
Session（唯一长期 owner）
├── Workspace/resources
├── Conversation data（History + projection）
├── busy/cancel/wait/close
├── derives Working Context snapshot
└── Advance（短期操作）
      └── calls Agent Execution Engine（run-local model/tool loop）
```

本课新增的不是第四层 coordinator，而是把当前分散在 one-shot composition、`conversation` 和 stateful Core 中的长期职责收回 Session。Conversation、Working Context、execution engine 与 Advance 仍是不同概念，但不再是并列的长期 owner。

Session 的最小状态模型只有三个稳定状态：

```text
                 Advance accepted
      ┌─────────────────────────────────┐
      │                                 ▼
    idle  <──── complete settlement ── busy
      │                                 │
      └────────── clean close ──────────┘
                       │
                       ▼
                     closed
```

- **idle**：Session 仍可使用，目前没有已接受且未 settlement 的 Advance。
- **busy**：恰好一个 Advance 已被接受；它可能正在 compaction、Provider call、tool execution、overflow continuation、History commit 或 observer settlement。
- **closed**：长期资源已释放，永远不能再次接受 Advance。

`busy` 是一个跨时间状态，但保护它的 mutex 只用于很短的原子检查/切换；不能把 mutex 从 Provider call 一直持有到 Advance 结束。这样第二个 Advance 可以迅速看到 busy 并 fail fast，Cancel/Wait 也能读取当前 operation control，而不会等待模型或 tool 执行完才拿到锁。

### Cancel、Wait、Close 不是同一个动作

用一个正在执行 `bash` 的例子：

```text
host requests Cancel
  -> current Advance context receives cancellation
  -> bash/process observes it and settles
  -> execution engine returns an aborted terminal delta
  -> Session commits the Run delta
  -> advance_settled is delivered
  -> Session transitions to idle
```

- **Cancel** 只提出请求。它不伪造“已经停止”，也不释放 Workspace。
- **Wait** 等待上面整条链真正结束。它自己不发起取消。
- **Close** 结束 Session lifetime 并释放 Workspace。它必须发生在 active work 已 settlement 之后；busy 时究竟拒绝还是内部完成 Cancel/Wait 是这一阶段提出的问题，后续源码核对与 D98 已确定采用后者。

因此未来产品控制可以保持清楚：

```text
Esc     = Cancel current Advance，Session settlement 后仍可继续
/exit   = Cancel（如 busy） -> Wait -> Close
```

这也解释了为什么本课要先于终端实现：终端只负责把按键映射到这些已经稳定的 lifecycle controls，不拥有取消或资源状态。

## busy shutdown 的横向源码核对

本轮使用 Pia 根目录的 sibling checkouts，未通过网络搜索源码：

| 项目 | Checkout 与 commit | active execution 的取消 | 退出/长期资源关闭 |
|---|---|---|---|
| Pi | `../pi`，`dcfe36c79702ec240b146c45f167ab75ecddd205` | `AgentSession.abort()` 先请求 core abort，再 `waitForIdle()`；它把 cancel request 与 wait 合成一个 async operation | interactive `shutdown()` 调用 `runtimeHost.dispose()`；后者最终同步 `session.dispose()`，请求 abort、断开 listeners 并 cleanup resources，但不等待 AgentSession settlement，随后进程直接退出 |
| Codex CLI | `../codex`，`0fb559f0f6e231a88ac02ea002d3ecd248e2b515` | `Op::Interrupt` 调用 `interrupt_task()`，等待 active task abort handling，但保留 thread | `Op::Shutdown` 可以在 active 时接受：先 shutdown realtime/startup work、`abort_all_tasks()`、终止 processes 与 services、运行 end hooks、flush persistence、发 `ShutdownComplete`；`shutdown_and_wait()` 再等 session loop 终止。App-server/TUI 外层给 graceful shutdown 配置 bounded timeout，而不是返回 busy |
| OpenCode | `../opencode`，`cb562b2c6289c2eee707078f9ab644cbe1d3d8a9` | `SessionRunState.cancel()` 调用 Runner cancel；Runner 通过 `Fiber.interrupt` 等待 execution fiber 终止，再发布 idle | OpenCode 的 durable Session data 与 workspace Instance lifecycle 分离，没有与 Pia 完全同构的 `Session.Close()`。TUI 退出后，worker shutdown 在 5 秒外层 budget 内 dispose all Instances；`SessionRunState` scope finalizer 会取消并等待所有 runners，然后释放 Instance resources |

### 这三份实现能证明什么

- 三者的主要退出路径都没有采用“busy 时关闭直接返回 busy error”。Codex 和 OpenCode 把 shutdown 视为终止长期 owner 的请求：先禁止生命周期继续，再取消 active work、等待或 bounded-wait cleanup，最后释放资源。
- Pi 的 `abort()` 本身会等待 idle，但它的 interactive process shutdown 没有使用这条完整路径。由于它随后直接 `process.exit()`，操作系统会回收进程资源；这个做法不能证明 Pia 可以在仍有 goroutine/tool 借用 `os.Root` 时安全关闭 Workspace。
- Codex 明确保留 `Interrupt` 与 `Shutdown` 两个操作：前者只结束当前 Turn 并继续使用 thread，后者结束 thread lifetime。OpenCode 同样把 session cancel 与 Instance shutdown 分开。说明“Cancel 与 Close 概念分离”不要求 Close 在 busy 时拒绝；Close 可以在终止 lifetime 的同时内部执行 cancel-and-wait。
- Codex/OpenCode 都给 host-level shutdown 设置有界等待或强制退出 fallback，但它们的内部正常路径仍先等待取消和 cleanup。Timeout 属于进程/host policy，不应直接变成 Pia Session 悄悄跳过资源 settlement 的许可。

### 对先前推荐的修正

源码证据削弱了“`Close(busy)` 必须返回 `ErrSessionBusy`”这个先前推荐，并形成了后来由 D98 固定的方向：

```text
Close starts
  -> permanently reject new Advance
  -> if busy, request cancellation
  -> wait for the accepted Advance's full settlement
  -> close Workspace and other owned resources
  -> return the combined settlement/cleanup result
```

这里 `Cancel` 仍作为独立 control 保留给 `Esc`；`Wait` 仍允许 host 只观察 settlement；`Close` 则是终止整个 Session lifetime 的安全组合操作。它也消除了 host 在 `Wait` 返回与 `Close` 调用之间又有新 Advance 被接受的竞态。Context-bounded wait、shared close result 与内部 closing 状态随后分别由 D98–D100 固定。

### 学习者补充的退出体验约束

学习者于 2026-07-28 明确提出：用户已经选择退出时，Pia 必须尽快停止资源使用，不能用无限等待迫使用户从外部杀进程；即使最终只能依靠不保证 clean settlement 的进程终止，也应先立即传播取消，而不是把取消推迟到某个 timeout 之后。

这个要求当前解释为两层责任，而不是让 Session 在 active goroutine 仍借用资源时谎称已经安全关闭：

```text
Session Close request
  -> 立即拒绝新 Advance
  -> 立即传播 cancellation
  -> 在 caller 允许的短暂窗口内等待正常 settlement 与资源释放

Process host
  -> 窗口耗尽时报告未 clean close
  -> 允许直接结束进程，由操作系统回收剩余进程资源
```

- Cancellation 必须在 Close acceptance 后立即发生；timeout 只限制“取消后还愿意等多久”，不能成为“多久以后才取消”的延迟器。
- Session core 不应无限等待，也不应在 active tool 仍可能使用 `os.Root` 时强行关闭它并把结果称为 clean close。Go 也不能安全地强杀任意 goroutine；真正的 hard stop 是结束整个进程。
- Lesson 13 应让 Close 接受 caller-controlled cancellation/deadline，并在等待窗口耗尽时明确返回未 clean settlement；具体默认等待时长属于后续 terminal/process host，而不是 Session Runtime 常量。
- Close 一旦被接受，即使 caller 的等待 context 先结束，Session 也不得重新接受 Advance。若 active work随后正常响应 cancellation，owner 仍应完成资源释放；如何不增加泄漏 goroutine地完成这项 handoff，是实现设计要验证的内容。
- 未来 durable journal 必须把这种 hard process exit 视为 unclean/interrupted，而不是写成 clean close。它是否必须拒绝整个 Session，还是可以安全回退到更早的 committed checkpoint，继续在下面的恢复讨论中校准。

学习者于 2026-07-28 确认 D98 的两层退出契约：

```text
Session Close(ctx)
  -> 永久停止新 Advance admission
  -> 立即取消 active work
  -> 在 caller context 内尝试 clean settlement 和 resource close

terminal/process host
  -> 使用短暂 grace period 等待 Close
  -> 窗口耗尽则直接结束进程
  -> 不写 clean-close fact，未来按 D96 checkpoint fallback 处理
```

因此 Lesson 13 不承诺任意 Provider、tool、filesystem call、observer 或 resource close 都能在固定时间内安全收敛；它承诺取消不延迟、等待可由 caller 停止、timeout 后 Session 仍为 closing 且永不重新接受工作。真正类似 kill 的硬上限只能由结束整个进程实现，不能由 Session Runtime 假装强杀 Go goroutine。`3 秒`是学习者用于说明体验目标的候选数字，具体默认值留给最小交互终端课程根据真实取消延迟确定。

### Cancel 与 Wait 使用 Session-relative 的简短名称

学习者于 2026-07-28 确认不在 `Cancel`、`Wait` 后增加 `Current`。一个 Session 同时至多只有一个 active Advance，因此 receiver 本身已经给出作用域；为了尚未实现的 follow-up queue 提前把 `Current` 写进 API，只会把未来猜测固化到当前名称。

当前最小语义是：

```go
session.Cancel()
err := session.Wait(ctx)
err := session.Close(ctx)
```

- `Cancel()` 只向该 Session 唯一 active Advance 发送 cancellation 并立即返回；idle、重复 cancel、closing 或 closed 时都是 idempotent no-op，不因“当前没有工作”产生 error。
- `Wait(ctx)` 等待该 Session 不再 busy，不发送 cancellation，也不返回 active Advance 的 Provider/tool/business error；后者仍属于原始 `Advance` 调用者。idle 或 closed 时立即成功，caller context 到期只停止这次等待，不影响 Advance。
- `Close(ctx)` 继续使用 D98 的 lifetime 语义，不是 `Cancel` 或 `Wait` 的别名。
- Future follow-up queue 出现时，必须在该课程重新校准“真正 idle”是否包含 pending input；当前不通过 `Current`、`WaitIdle`、queue token 或额外 state API 预先下注。

### 取消后留下什么：三份本地源码的恢复语义

本轮进一步核对了同一批本地 checkout，问题不是 UI 如何显示取消，而是“下一次模型调用如何知道上一次可能做了一半，以及 runtime 会不会自动重做”：

| 项目 | clean cancellation 后保存的事实 | 下一次模型调用 | 是否自动重放旧调用 |
|---|---|---|---|
| Pi | LLM stream 被取消时，terminal assistant 以 `stopReason: "aborted"` 进入 Session message history；tool 已经开始时，tool 接收同一个 abort signal，失败会形成同 ID 的 error tool result。`AgentSession` 在每个 `message_end` 立即追加 user、assistant 和 tool result，resume 再把这些 message entries 原样投影回 context | aborted assistant 和已经形成的 tool result 都可以再次进入 context。Pi 没有额外的通用“工具可能部分执行” marker | 没有。下一次 prompt 是新的模型调用；模型仍可能根据现状自行选择同一种工具 |
| Codex CLI | interrupt 先取消 task，并把 `<turn_aborted>` 写入 conversation，明确说明 command/tool 可能部分执行；flush 该 marker 后再写 `TurnAborted` terminal event。构造后续 prompt 时，任何缺少 output 的 function/custom/local-shell call 会得到同 ID 的 synthetic `aborted` output | 测试明确验证下一次 `/responses` request 含 `<turn_aborted>`；prompt normalization 保证没有 orphan tool call | 没有。marker 是风险上下文，不是 replay instruction |
| OpenCode | assistant 被标为 `MessageAbortedError` 并完成；未完成 tool part 被持久化为 `status: error`、`Tool execution aborted`、`metadata.interrupted: true`，已经收集到的 bash output 可以保留。即使遇到仍为 pending/running 的持久化 part，model-message conversion 也会投影同 ID 的 interrupted tool result | 有实际内容或 tool part 的 aborted assistant 会被转换回模型消息；纯空 aborted assistant 可以被过滤。loop 明确忽略 cleanup 标记的 interrupted orphan，不把它当作待执行工具 | 没有。新的 user prompt 才会开始新的 loop，旧 tool part 不会再次 execute |

对应的关键源码与测试是：

- Pi `../pi/packages/agent/src/agent-loop.ts:192-224, 435-555, 602-705`、`../pi/packages/coding-agent/src/core/agent-session.ts:547-618`、`../pi/packages/coding-agent/src/core/session-manager.ts:375-465` 与 `../pi/packages/agent/test/e2e.test.ts:99-123`。
- Codex `../codex/codex-rs/core/src/context/turn_aborted.rs:3-34`、`../codex/codex-rs/core/src/tasks/mod.rs:854-938`、`../codex/codex-rs/core/src/context_manager/normalize.rs:20-130` 与 `../codex/codex-rs/core/tests/suite/abort_tasks.rs:176-255`。
- OpenCode `../opencode/packages/opencode/src/session/processor.ts:539-597, 627-681`、`../opencode/packages/opencode/src/session/message-v2.ts:244-360`、`../opencode/packages/opencode/src/session/prompt.ts:1081-1129, 1203-1219` 与对应 `processor-effect.test.ts:785-831`、`prompt.test.ts:463-489, 1083-1128`。

三份实现都只能阻止 runtime **自动**重放，不能证明一个已开始的外部副作用没有发生，也不能禁止模型在后续新请求中再次选择相同工具。尤其是 bash，收到 cancellation 前已经写文件、发送请求或启动子进程的部分不能被一般化回滚。Codex 的 marker、OpenCode 的 interrupted tool result，以及 Pi 的 error result，都在给后续模型提供事实，而不是提供 exactly-once 保证。

学习者于 2026-07-28 确认下面的 Pia 边界；其跨进程恢复部分由 D96 固定，Lesson 13 当前只实现同进程 cancellation settlement：

1. Session 收到 Cancel 或 Close 后立即取消 active Provider/tool context，不为持久化等待而延迟 cancellation。
2. 如果 cancellation 能正常 settlement，Conversation History 继续保存现有 aborted terminal；每个已经形成的 tool call 必须有同 ID 的终止结果。尚未开始执行的结果写明 `not executed: cancelled`；已经开始的结果必须写明 `cancelled; may have partially executed`，必要时保留已有 bounded output。
3. 后续同进程 Advance，以及未来 exact/fallback safe resume，使用协议完整的 committed History。runtime 不恢复半条 LLM stream，不自动重发 Provider request，也不自动 execute 旧 tool call；新的用户 Advance 才允许模型继续。
4. 模型若想再次执行有副作用的工具，应先检查 workspace/外部状态。这个提示只能降低误重做风险；通用 exactly-once、事务回滚、幂等 key 或危险操作审批不属于 Lesson 13。
5. 若 process 在 cancellation settlement 与 durable commit 前被强杀，未来 journal 只能证明上一次 execution 未 clean settlement。不能把不完整尾部当成已经完成，也不能自动重放其中的调用；是否允许退回更早 checkpoint 后继续，见下一节修正。

这个方向不要求 Lesson 13 提前实现 journal 或 resume。Lesson 13 只需保证同一进程内取消后的 History 可直接用于下一次 Advance，并为未来持久化保留正确的 settled state；journal 课程再负责让同一事实跨进程存活。

### 对“clean-only resume”的确认修正

学习者于 2026-07-28 进一步指出：进程被 hard kill 不应让整个 Session 永久不可恢复；可以接受丢失最新一轮 Run、若干 Turns，甚至整个未完成 Advance，只要能以简单、明确的规则恢复较早的健康上下文。

这个目标可行，而且比“修复并继续未完成 execution”简单得多。先前“只要没有 clean close 就拒绝整个 Session”的候选过于严格，应区分三个层级：

1. **Exact clean resume**：最后一个 owner 已正常 settlement/close，恢复到最新 committed state。
2. **Checkpoint fallback resume**：前一进程 unclean termination，丢弃最后一个完整 checkpoint 之后的全部 journal tail，恢复更早 committed state，并警告 workspace 可能包含未记录的部分副作用。
3. **In-place interrupted execution recovery**：保留未完成 Advance 中的部分 Turns，逐个判断 Provider/tool 是否开始、是否完成、是否应补结果或继续。这仍然复杂，继续延后。

建议把第二项纳入第三阶段的最小恢复能力，而只推迟第三项。恢复图景是：

```text
Advance A committed
Advance B committed          <- last healthy checkpoint
Advance C partial messages
tool started / side effects
process killed               <- uncommitted tail

resume
  -> rebuild History and Working Context through Advance B
  -> ignore Advance C's incomplete journal tail
  -> add a bounded model-visible recovery warning to the rebuilt projection
  -> wait for new user input; never replay Advance C automatically
```

恢复单元应优先使用整个 committed Advance，而不是逐条“过滤不健康的 LLM/tool message”：

- 一个 Advance 已经是包含 compaction、一个或多个 Core Runs、History commits 和 final snapshot 的现成原子业务边界。
- 单条 message 是否健康并不足以判断副作用。一个 bash tool result 尚未写入时，命令仍可能已经修改文件或调用外部服务。
- 删除一条 tool result、assistant 或 compaction record 可能破坏 tool-call 配对、History/Working Context 一致性和 projection boundary。
- 退回上一个 Advance checkpoint 虽会多丢一些最新对话，但恢复规则固定，不需要为 Provider in-flight、tool pre-start、tool running 和 result commit 分别建立状态机。

这里的“回退”只回退 Session 记录，**不回滚 workspace 或外部世界**。因此 fallback resume 必须让模型看到类似下面的 bounded guidance，但不把 control record 伪装成普通 Conversation Message：

```text
The previous process ended unexpectedly. Conversation state was restored
to the last committed advance. The workspace may contain partial changes
from later interrupted work. Inspect current state before repeating actions.
```

这条 guidance 应由 journal 的 unclean-recovery fact 派生到 Working Context projection；Projection 不是判断健康记录的 authority，Conversation History 也不保存被丢弃的 partial tail。首版不需要告诉模型每个被中断调用的精确清单，因为取得这种精度就需要 durable started/in-flight records，重新进入第三层复杂恢复。

最小 checkpoint fallback 的预期损失和边界是：

- 若 hard kill 发生在 Advance C 中，C 的 user input、已经完成但尚未随 Advance checkpoint 提交的 Turns 都可以丢失。
- Workspace 中 C 已经产生的文件或外部副作用仍然存在；后续模型必须先检查现状。
- 若进程在 idle 时被杀，last committed Advance 已经是最新状态，实际只多一条保守 warning。
- 若连第一个 Advance checkpoint 都不存在，可以恢复为空 Conversation 加 warning；Session identity/workspace binding 是否继续复用留到 journal 课程校准。
- journal 自身无法找到任何完整有效 checkpoint、版本不支持或 workspace 不可访问时，仍应明确失败。

学习者已经确认这个方向。第三阶段 resume 不再是“只接受 clean close”，而是“exact clean resume + last-committed-Advance fallback”；仍然不承诺保留或继续 interrupted execution。D96 同步修订第三阶段 roadmap 与 D84 的 strict clean-only 边界。

### 结构重新校准：不保留 one-shot compatibility layer

学习者指出“`coding.Run` 创建 Session”的表述仍然带着既有 one-shot API 的兼容包袱，而且 Session、Conversation 和 Core 仍像三个相邻 controller。这个质疑成立。上一版“保留 `coding.Run`、Session 包住 Conversation、Core 继续长期拥有 Working Context”的候选在形成 D97 之前撤回；它不是本课实现约束。

本轮重新读取本地三个参考 checkout，并以当前 Pia 的长期产品方向而不是最小 diff 为判断标准：

| 实现 | 长期交互对象与 Workspace | execution state 放在哪里 | 对 Pia 的启发 |
| --- | --- | --- | --- |
| 冻结 Pi `dcfe36c7` | `AgentSession` 同时持有 core `Agent`、`SessionManager`、cwd、tools/resources、compaction/retry/queue；外层 `AgentSessionRuntime` 再负责当前 Session 与 cwd-bound services 的替换 | core `Agent` 与 coding `AgentSession` 都有 active/idle state | 证明 stateful Core + Session 可以工作，但也展示了功能增长后双层 lifecycle 和 Runtime/Session 命名的复杂度；不应因 Pi 使用它就机械复制 |
| Codex CLI `0fb559f0` | `ThreadManager` 创建和持有多个 `CodexThread`；公开 Thread 是 IO conduit，内部 `Session` 持有 history、configuration/services、active turn 和 input queue，cwd/workspace selections 属于 Session configuration | 主要 user-operation authority 在内部 Session；没有另一个 Conversation controller 与它并列 | 支持“一个长期 live handle 内聚 history、workspace binding 和 active turn；Manager 留到多 Session consumer 出现后”的方向 |
| OpenCode `cb562b2c` | durable Session row 保存 directory/workspace metadata；directory-keyed Instance scope 持有 workspace runtime；instance-scoped `SessionRunState` 用 `Map<SessionID, Runner>` 管理并发 | Session data、workspace Instance 和 runner registry 分离 | 适合 server 同时服务一个 workspace 下的多个 durable Sessions，但会把 Pia 当前一个 Session 的 ownership 提前分散到 registry/services；现阶段不应复制 |

共同证据是：一次性 CLI 调用只是 host policy，不是长期 Session 本体。Pi SDK 返回 `AgentSession`，Codex 返回 `CodexThread`，OpenCode 以 Session ID 在 Instance scope 中推进；没有一个实现把 product-level `Run once` wrapper 当成需要保留的 Session architecture。

#### 三个 Pia 候选

**候选 A：保留 wrapper 和三个 stateful owners。**

```text
coding.Run -> Session -> Conversation -> stateful Core Agent
```

它的优点只是迁移小、旧 tests 改动少。缺点是 `coding.Run` 与长期 Session 重叠、Conversation 仍像 controller、History/Projection 与 Core Working Context 分属两个长期 owner、Session 与 Core 仍各有 guard。这个方案主要优化 diff，不优化目标架构，当前不推荐。

**候选 B：删除 `coding.Run` 和 Conversation controller，但保留 stateful Core。**

```text
CLI/TUI host -> Session
                  owns Workspace, lifecycle, History, projection
                  calls stateful Core Agent
                        owns Working Context and narrow guard
```

这比候选 A 清楚：当前 CLI 直接 `NewSession -> Advance once -> Close`，未来 TUI 在同一个 Session 上多次 Advance；Conversation 只保留为 Session 中的 domain data，不需要独立 controller/type。代价是 Working Context 仍由 Core 长期持有，compaction commit 和未来 resume 仍必须原子协调 Session projection 与 Core replacement。

**候选 C：一个 authoritative Session 加 run-local execution engine。**

```text
CLI / future TUI / future Orchestrator       host
                  |
                  v
             coding.Session                  only long-lived per-conversation owner
             ├── Workspace/resources
             ├── History + compaction projection
             ├── busy/cancel/wait/close
             └── immutable execution dependencies
                         |
                         v
             agent execution engine          no Conversation state
             ├── receives derived Working Context snapshot
             ├── runs Provider/tool loop
             └── returns ownership-independent message delta
```

Session 在每次 Core execution 前由 `History + projection` 派生 Working Context snapshot。execution engine 只在这次调用内部把新 assistant/tool messages 加到 run-local context，并在 settlement 时返回 delta；Session 把 delta 提交到 History。Compaction 只原子发布新的 projection，下一次 execution 自然从新 projection 派生 context，不再调用 `ReplaceWorkingContext`。

这样可以删除：

- product-level `coding.Run`、`runWithProvider` 和 `runWithWorkspaceOperations` 这套 one-shot application API；
- `conversation` type 及其 `active`、`beginRun/endRun` 和 data mutex；
- Core 长期 Working Context、`ReplaceWorkingContext` 和 Core active guard。

这里不是删除 Agent Loop。Provider/tool ordering、terminal settlement、not-executed tool results 和 run-local delta 仍属于 `internal/agent`；被删除的是它对某个 Conversation 的长期状态和 lifecycle authority。具体类型可在实现设计时从含糊的 `Agent` 改成表达执行职责的名称，避免再把它理解为第二个 Coding Agent。

学习者于 2026-07-28 确认采用候选 C，并要求把这种“新能力暴露旧结构包袱时允许较大重构”的做法作为后续课程常态。D97 固定这一方向，原因是：

- 一个 Session 同时成为 lifecycle、Conversation state 和 workspace resource 的唯一长期 owner；
- Working Context 从权威 History/projection 派生，不再是必须与 Session 同步提交的第二份长期状态；
- durable journal/resume 只需重建 Session 的 History/projection，不必恢复后再调用另一个 owner 的 replacement API；
- 正常路径只有 Session lifecycle mutex；History/projection 在唯一 accepted Advance 中串行变更。未来若出现 active-time snapshot consumer，再按真实访问面增加 data synchronization；
- CLI、TUI 和未来 Orchestrator 都创建同一种 Session。当前 CLI 只是用一次，未来交互 host 用多次，不需要 compatibility wrapper。

它的代价是 Lesson 13 从“移动 guard”扩大为一次明确的 ownership refactor：`internal/agent` 的输入从隐式持有 context 改为显式 snapshot，现有 agent/coding/CLI tests 需要迁移。这个成本发生在 journal、resume 和外部 consumer 之前，低于以后带着双重 state contract 再改。它仍是一个内聚且可独立验收的 Session ownership/lifecycle capability，因此规模保持 Large；若未来某课暴露出多个独立能力，仍按课程规则先拆分 XLarge，而不是把所有重构都塞进一课。

#### 采用候选 C 后，one-shot 流程如何命名

不存在“外层 Session 再创建内层 Session”。最外层是进程 host，不是 Session：

```text
cmd/pia.execute
  -> coding.NewSession
       -> open Workspace
       -> create Provider / tools / prompt / execution engine
       -> constructor success: transfer Workspace ownership to Session
  -> session.Advance(task) once
  -> session.Close
  -> host combines Advance and Close errors
```

`coding.Run` 直接删除，不改名保留。D100 把当前 `RunInput/RunResult` 按新语义拆成构造配置、Session 静态信息与一次 `AdvanceResult`。当前 CLI 的 final stdout、semantic-event stderr、trace、Skill diagnostics 和错误语义仍应保持，但这是 observable behavior，不是保留旧函数和类型的理由。

Workspace cleanup 的结论不因候选变化而改变：Session constructor 打开 Workspace 后若后续构造失败，立即关闭并用 `errors.Join` 保留 construction/close 两个错误；构造成功后 Workspace 只由 Session Close 关闭。当前只有 Workspace 是 owned closeable resource，不预建通用 cleanup registry。

## 最终命名与 Go surface

学习者于 2026-07-28 授权导师参考三份本地 coding-agent 源码直接确定命名与 API，不再把每个小接口拆成选择题。本轮进一步核对：

- Pi `../pi/packages/coding-agent/src/core/sdk.ts:34-90,167` 使用 `CreateAgentSessionOptions`、`CreateAgentSessionResult` 与 `createAgentSession()`，`agent-session.ts:799,1076,1501-1511` 在长期对象上提供 `dispose/prompt/abort/waitForIdle`；
- Codex `../codex/codex-rs/core/src/thread_manager.rs:125,664-705` 创建长期 `CodexThread`，`codex_thread.rs:205-216` 提供 `submit(Op)` 与 `shutdown_and_wait()`，`protocol/src/protocol.rs:528-674` 用 `UserInput/Interrupt/Shutdown` 区分输入、当前工作中断和 lifetime shutdown；
- OpenCode `../opencode/packages/opencode/src/session/prompt.ts:104-108,1052-1054` 以 `prompt(PromptInput)` 返回最新 message/parts，`session/run-state.ts:27-107` 以 Session ID 管理 `assertNotBusy/cancel/ensureRunning`。

三者共同证明应有长期 handle，以及输入推进、当前工作取消和 lifetime close 三个不同动作；它们没有证明 Pia 当前需要大 options object、generic command union 或 multi-Session registry。结合 Pia 已有的 Advance 语义，D100 固定下面的 application surface：

```go
type SessionConfig struct {
    WorkspacePath  string
    DeepSeekAPIKey string
    Observer       observation.Observer
}

func NewSession(config SessionConfig) (*Session, error)

type SessionInfo struct {
    WorkspacePath    string
    SystemPrompt     string
    Model            ModelInfo
    Tools            []ai.ToolSchema
    SkillDiagnostics []SkillDiagnostic
}

func (s *Session) Info() SessionInfo

type AdvanceResult struct {
    History []ai.Message
}

func (r AdvanceResult) FinalText() string
func (s *Session) Advance(ctx context.Context, input string) (AdvanceResult, error)
func (s *Session) Cancel()
func (s *Session) Wait(ctx context.Context) error
func (s *Session) Close(ctx context.Context) error
```

命名理由：

- 使用 `Session`，不使用 `AgentSession`：`coding` package 已经给出 coding-agent 语境，后者还会与 run-local Agent Execution Engine 再制造一个“Agent”歧义。
- 使用 `Advance`，不使用 `Prompt`：一次调用包含 preflight compaction、一个或多个 Runs、History/projection commit、observation 和 final snapshot，不只是把 prompt 发给 LLM。
- 不使用 Codex 式 `Submit(Op)`：它适合 client/server protocol 和多种异步 command；当前 Pia 只有一个 text user input，generic union 会提前引入 queue/approval/review 等不存在的操作。
- 直接接收 `input string`：只有一个 text 字段时，`AdvanceInput` 没有带来边界；真实 images、attachments 或其他输入出现后再引入结构体。
- `SessionInfo` 保存构造后不变的 canonical workspace、prompt、model、tools 与 Skill diagnostics；`AdvanceResult` 只保存本次结算后的权威 complete `History`。旧 `RunResult` 把两类生命周期不同的数据混在一起，且 `Transcript` 是诊断投影词，不是 application state 的准确名字。
- `DeepSeekAPIKey` 明示当前固定 Provider；不以通用 `APIKey` 暗示尚不存在的 Provider selection。Provider、Workspace opener 和 close seam 继续 package-private，只服务离线 Faux 与 ownership tests。
- `Info()` 和 `AdvanceResult.History` 都返回 deep-cloned ownership-independent values，且不包含 credentials。`FinalText()` 继续只从最后一个 assistant 的 text blocks 投影。

Session 只公开两个 lifecycle sentinels：

```go
var ErrSessionBusy = errors.New("coding: session is busy")
var ErrSessionClosed = errors.New("coding: session is closed")
```

blank input、pre-canceled context、busy、closing 或 closed 都在 acceptance 前拒绝，不产生 Advance events 或修改 History；结果仍携带拒绝当时的完整 History snapshot。Accepted Advance 即使 Provider/tool failure 或 cancellation 也提交既有 protocol-complete delta，并返回完整 History snapshot 和独立 error。Closing 与 closed 对新 Advance 都表现为 `ErrSessionClosed`；不公开 `closing` sentinel、state enum、`IsBusy` 或轮询 API。

### Run-local Engine

`internal/agent.Agent` 改名为 `Engine`，package 已经限定名称，因此 constructor 保持 `agent.New`：

```go
func New(config Config) (*Engine, error)

func (e *Engine) Run(
    ctx context.Context,
    workingContext []ai.Message,
    userInput string,
) (RunResult, error)

func (e *Engine) Continue(
    ctx context.Context,
    workingContext []ai.Message,
) (RunResult, error)
```

Engine 每次调用 deep-clone Working Context，维护 invocation-local messages，并继续返回 `RunResult.NewMessages`。它没有长期 Working Context、`ReplaceWorkingContext`、`ErrRunActive`、active mutex 或 Session lifecycle。Session 是当前唯一 caller 并保证串行调用。`Run` 与 `Continue` 两个入口继续区分“追加一个新 user input”和“从已有 user/tool-result tail 继续”；用 bool 或 mode 参数合并会隐藏两者不同的协议前置条件。

### 一个短锁如何完成 Cancel、Wait 与 Close

Session 内部不公开状态查询，只使用：

```text
lifetime: open | closing | closed
active:   nil | { cancel, done }
close:    { done, err }
```

- 一个短 mutex 保护 admission、这些 references、History/projection snapshot 与 commit；Provider、tool、observer、Workspace close 和 channel wait 都在锁外。
- Advance acceptance 通过 `context.WithCancelCause(callerCtx)` 建立 active execution context。Caller cause、`Cancel()` 或 `Close()` 的 `context.Canceled` 遵循 first-cause-wins，最终 error 仍可由 `context.Cause` 识别。
- `Cancel()` 只在锁内复制 cancel function，再在锁外调用；idle/closing/closed 是 no-op。
- `Wait(ctx)` 在锁内复制当前 active `done` 后等待。调用时已经 idle/closed 则 condition 优先、立即返回 nil；仍 busy 而 caller context结束时返回 `context.Cause(ctx)`，不取消 Advance，也不复制它的 business error。
- 第一个 `Close(ctx)` 先永久切换为 closing，再在锁外立即 cancel active work。Busy Close 与重复 Close 都等待同一个 close completion；caller context 只能停止自己的等待，不能撤销 closing。
- Idle Close 直接完成唯一一次 Workspace close。Busy Close 被接受后，active Advance 的原 settlement path 在完成 History/projection/observer settlement 后接管唯一一次 Workspace close，保存 `closeErr` 并完成 shared close result；因此不需要 detached cleanup goroutine。若某个 Close caller timeout，既有 Advance path 仍可在随后真正收敛时完成 cleanup。
- Repeated/concurrent Close 不重复 cancel 或 close；clean closed 后返回保存的同一 close result。若 close completion 已经可用，其结果优先于 caller cancellation；仍在 closing 时 caller context 到期才返回其 cause。

Close 不返回 active Advance 的 Provider/tool/business error，后者只属于原始 `Advance` caller；它只返回等待 cause或 Workspace close error。One-shot host 在 Advance 已经同步返回后用 `errors.Join(advanceErr, closeErr)` 形成 process-level settlement。

### Constructor 与 one-shot host

`NewSession` 先验证 required scalar config 并创建当前没有 `Close` contract 的 DeepSeek Provider，再打开 Workspace。Workspace 打开后安装 error-only cleanup；任何后续 Skill discovery、tool、prompt 或 Engine construction failure 都关闭 Workspace，并用 `errors.Join` 同时保留 construction error 与 close error。成功返回前移交 ownership 给 Session。当前没有第二个 owned closeable，因此不建立 cleanup stack/registry；测试使用 package-private Provider 与 workspace open/close seam。

`cmd/pia` 只定义满足当前测试需求的私有 host interface：

```go
type codingSession interface {
    Info() coding.SessionInfo
    Advance(context.Context, string) (coding.AdvanceResult, error)
    Close(context.Context) error
}
```

它的实际流程是：

```text
validate argv/env/cwd
  -> NewSession
  -> capture SessionInfo
  -> Advance once
  -> Close
  -> join Advance and Close errors
  -> optional typed trace
  -> Skill diagnostics / final text or process error
```

`BuildTrace` 改为接收 `SessionInfo`、`AdvanceResult` 和 joined settlement error。已经 accepted 的 Advance 仍在 Advance 与 Workspace settlement 后尝试 trace；trace 不继承已经 canceled 的 execution context。JSON 继续使用既有 `transcript` 与 `run_error` 字段，避免把本课扩大成诊断格式迁移。Constructor failure 尚未产生 Session 或 accepted Advance，直接返回 construction error，不发明 partial Session/result 或 trace。

## 当前课程边界

本课要完成：

- 一个长期 in-memory Session 的构造、状态和资源所有权；
- 在同一 Session 上多次顺序 Advance；
- 完整 Advance 范围的 busy/idle 与并发 Advance fail-fast；
- caller cancellation 与 Session 主动 Cancel 的统一执行上下文；
- 等待当前 Advance 最终 settlement；
- deterministic Close、closed rejection 与 Workspace cleanup；
- 落实 D97 的单一长期 Session owner：删除 Conversation lifecycle/controller，让 Working Context 成为 Session 的派生 snapshot，把 Core 收敛为 run-local Agent Execution Engine；
- 保留当前 one-shot command 的可观察行为；
- 用离线 tests 与 race tests 验证顺序、失败、取消、并发和资源关闭。

本课不做：

- follow-up、steering 或任何 pending input queue；
- `Esc`、`/exit`、键盘读取或交互 terminal；
- Session journal、identity、resume 或 crash recovery；
- public SDK、Manager、Gateway、RPC、IM 或多个 Session coordination；
- Provider retry、budget、permission/approval 或与 cancellation 无关的新增 model/tool semantics；
- 为未来 consumer 预建 callback bus、scheduler、lease 或通用 resource registry。

## 后续诊断时间线需求

学习者于 2026-07-29 提出：未来需要在一个位置查看单个 Session 的完整动作时间线，从而分析 compaction 何时发生、trigger、settlement 与实际影响，并比较不同 Provider/model/workload 后调整 threshold 或其他策略。当前 Compaction event 已能在进程内表达 `started/settled`、`threshold/overflow` 与 `success/error`，但默认 line observer 过滤成功结算，trace 不保存 event stream，事件本身也没有时间、稳定序号或压缩前后 context 指标，因此现状不足以支持事后优化分析。

D101 将后续方向固定为可选的 host-owned diagnostic recorder：它消费已有 semantic event stream，并结合 `SessionInfo` 形成 action timeline；不得让 Session 再拥有第二份 action history，也不得把 diagnostic timeline 与 D80 的 durable recovery journal、Conversation History 或 Working Context 合并。具体字段、格式、文件位置、保留策略和课程顺序等待真实 consumer 开课时校准，不扩大本课 ownership/lifecycle 实现。

## 实现结果

D97–D100 已经落实为下面的代码边界：

- `internal/coding/session.go` 新增唯一长期 `Session` owner，实现 `NewSession/Info/Advance/Cancel/Wait/Close` 对应的 application surface、单一 admission guard、shared close completion、History/projection ownership 和 Workspace 唯一 cleanup；
- `internal/agent` 将原有 stateful `Agent` 收敛为 `Engine`，每次 `Run/Continue` 显式接收并深复制 Working Context snapshot，只保留 run-local execution messages 和 `RunResult.NewMessages`；
- `internal/coding/conversation.go`、product-level `coding.Run`、长期 Core Working Context、`ReplaceWorkingContext` 和重复 active guards 已删除，没有增加 compatibility wrapper；
- compaction/recovery 直接读取 Session snapshot 并只发布 Session projection；下一次 execution 从权威 History 与 projection 重新派生 Working Context；
- `cmd/pia` 直接执行 `NewSession -> Info -> Advance once -> Close`，并在 Advance/Close settlement 后构建 trace；既有 stdout、stderr、trace 和 Skill diagnostic 可观察行为保持；
- tests 覆盖顺序复用、snapshot ownership、admission rejection、主动与 caller cancellation、Wait、并发/重复 Close、busy Close timeout、cleanup error、constructor 逆序清理、compaction/recovery、observer ordering、CLI settlement 和 trace migration。

实现后的首轮简化审查删除了 Session 中与 `SessionInfo` 重复的 system prompt/tool schema 状态；正式审查补回 CLI 组合错误、final text 换行/空文本和 diagnostic writer 等可观察契约测试。学习者随后于 2026-07-29 明确要求用 `ce-simplify-code` 复核 Legacy 负担；主线程按 reuse、quality、efficiency 三个维度再次检查后，应用了四组 quality 整理：删除只转发参数的 constructor wrapper；把 `FinalText` 与 `AdvanceResult` 放回同一 owner，并把 trace Go 字段改为 `SettlementError`、同时保持 JSON `run_error` 不变；把当前 tests 中遗留的 `conversation/core/RunWithProvider` 标识改为 `session/engine/SessionWithProvider`；删除 constructor cleanup 中由 ownership transfer 已经保证的重复 `err == nil` 判断。Reuse 没有发现遗漏的现有 utility；efficiency 唯一候选是减少 Working Context/Provider request clone，但这些 clone 分别保护 Session、Engine 和 Provider 所有权，按 D100 不能作为无行为变化优化删除。两轮审查均未留下需要修改本课架构的 finding。

最终 `make check`、`make race`、Session lifecycle 20 轮并发压测与 `git diff --check` 全部通过；结果记录在 D100 后的课程变更日志中。

学习者已经完成关键实现语义的讲解与确认，并于 2026-07-29 要求不再逐项展开 tests，由课程流程自动完成最终验证、提交和 `origin/main` 推送。Lesson 13 至此结束；后续课程仍须等待学习者明确开始。
