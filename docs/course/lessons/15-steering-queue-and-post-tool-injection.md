# 第 15 课：Steering queue 与 post-tool safe-boundary 注入

## 当前状态

学习者于 2026-07-29 明确开始本课。冻结 Pi checkout 已核对为
`dcfe36c79702ec240b146c45f167ab75ecddd205`，与课程固定基线一致；低层
Agent Loop 的 Steering drain points、stateful Agent queue、coding
`AgentSession` queue tests，以及当前 Pia 的 Session、Agent Execution Engine、
tool settlement、Follow-up 与 semantic events 路径已完成首轮开课校准。

课程状态为**已提交**。源码证据确认本课仍是一个独立的 **Large** capability：
它要同时建立 active-only admission、Session-owned pending input、同一 Engine
Run 内的 post-tool safe-boundary 注入、异常时的明确 ownership settlement 与
并发封口测试，但不需要合并最小交互终端、持久化或多 Session coordination。
这些边界已经完成测试先行实现、简化审查、结构化多视角审查与验证。Provider
failure 结束当前 Engine Run，但 pending Steering 会明确 hand back；已消费的
Steering 只进入 History，不会重复交还。未来 terminal 如何把 hand-back 文本
恢复到 composer 或桥接到新 execution，仍由后续课程实现。

## 解锁能力

Lesson 14 允许 active Advance 接受 Follow-up，但它必须等当前 Engine Run 完整
settlement 并提交 delta 后，才以一条新的 input-started Run 继续。Steering 的
目标不同：

```text
Run("修复 bug")
  -> assistant requests tool A and tool B
  -> tool A and tool B both settle
  -> inject Steering("先别改 API，只修内部实现")
  -> next Provider call sees tool results plus Steering
  -> same Run continues
```

本课完成后，Session 能在当前 execution active 时接受内存 Steering input，并
只在确定的 Agent Loop 安全点把它加入同一个 run-local Working Context。

## 开课源码校准

### 已确认的大纲假设

- 冻结 Pi 明确区分 Steering 与 Follow-up。Steering 在当前 assistant turn 的
  tools 全部 settlement 后、下一次 Provider call 前拉取；Follow-up 只在当前
  loop 已经没有 tool calls 或 Steering、原本将停止时拉取。
- Steering 不会抢占 Provider stream 或正在执行的 tool，也不会跳过同一个
  assistant message 中尚未执行的 tool calls。并行 tool batch 也必须完整
  settlement 后才允许注入。
- 冻结 Pi 默认使用 `one-at-a-time`，但也提供显式 `all` mode；当前 Codex Core
  则在下一 model step 前一次取走当时全部 pending input。Pia 不复制 Pi 默认值，
  而是把同一 safe boundary 已接受的全部 Steering 保持为独立 user messages，
  用一次 Provider request 处理。
- Queued Steering 在 admission 时不进入 authoritative messages；低层 loop
  真正消费它时才发 user message lifecycle 并追加到当前 run-local messages。
- 当前 Pia 的 Session 已是 queue、History/projection 与 lifecycle 的唯一长期
  owner；`internal/agent.Engine` 已负责 Provider/tool loop 与 run-local delta，
  因此安全注入点属于 Engine，而 pending queue ownership 仍属于 Session。

### 细化后的认识

- “post-tool boundary”不是“只在有 tool call 时检查”。冻结 Pi 在每个正常
  assistant turn settlement 后拉取 Steering；即使 terminal assistant 没有
  tools，Run 真正停止前仍有一次最后的 dequeue-or-stop 决定。
- 一条 Steering 不启动新的 Engine Run。它作为额外 user message 加入当前
  invocation 的 `NewMessages` delta，下一次 Provider call 仍属于同一个 Run；
  这与 Lesson 14 的 Follow-up 新 input-started Run 是主要可观察差异。
- Engine 不应反向持有 Session 或长期 queue。当前更符合 D97 的方向是让一次
  invocation 借用一个窄的 run-local input source/control，在安全点请求下一条
  input；具体 Go 形状仍需讨论，不在开课校准中提前固定。
- 当前 Codex checkout `0fb559f0f6e231a88ac02ea002d3ecd248e2b515`
  也只允许 active regular turn 接受 same-turn steering，并在下一次 model step
  前 drain pending input；其 turn ID、通用 input queue 与 service protocol
  不是 Pia 当前 internal Session 必须复制的 surface。

### 被推翻或需要修正的假设

- “Steering 等当前 Run 返回后再调用 `Engine.Continue`”不成立。那会先发布
  Run settlement、把 steering 变成新的 invocation，并失去“同一 ongoing Run”
  的语义与事件边界。
- “在每个 tool worker 完成时立刻检查 queue”不成立。这样会让输入插进同一
  assistant message 的 tool batch 中间，破坏既有 source-order settlement 与
  parallel-stage contract。
- “把 queue 直接放进 Engine 最接近冻结 Pi”不成立。Pi 的 stateful Agent 本来
  就长期拥有 messages 与 queues；Pia 已按 D97 把长期 Conversation ownership
  收敛到 Session，机械复制会重新引入第二个长期 owner。
- “只要 `Advance` active 就无条件接受 Steering”不成立。pre-Run compaction、
  相邻 Follow-up Runs、final stop seal、Provider/tool failure、Cancel 与 Close
  都会影响一条已接受输入的归属；Runtime 只在一条确定的 Engine invocation
  可以接管 same-Run input 时开放窄 admission window。

## 冻结 Pi 与对照源码路径

- `../pi/packages/agent/src/agent-loop.ts`：inner tool/Steering loop、正常 turn
  后的 Steering poll、outer Follow-up loop 与 error/aborted 直接 settlement。
- `../pi/packages/agent/src/agent.ts`：`PendingMessageQueue`、FIFO
  `one-at-a-time` drain 与 Steering/Follow-up 的独立 queues。
- `../pi/packages/agent/src/types.ts`：`getSteeringMessages` 的 post-tool contract。
- `../pi/packages/agent/test/agent-loop.test.ts`：同一 assistant 的全部 tools
  先完成，再注入 Steering；stop callback 与异常路径不会错误拉取后续 queue。
- `../pi/packages/coding-agent/test/suite/agent-session-queue.test.ts`：Steering
  在下一次 LLM call 前可见，多条输入按 one-at-a-time 顺序消费。
- `../codex/codex-rs/core/src/session/mod.rs`、`session/input_queue.rs` 与
  `session/turn.rs`：active regular turn admission、pending input ownership 与
  下一次 model step 前 drain 的对照证据。
- `../opencode/packages/opencode/src/session/processor.ts`、`session/retry.ts` 与
  `cli/cmd/run/runtime.queue.ts`：Provider retry/error settlement 与 interactive
  prompt FIFO 在 terminal error 后继续推进的对照证据。

## 开课时 Pia 路径

- `internal/agent/loop.go`：当前 `executeRun` 在 tools settlement 后直接开始
  下一 Turn，在无 tool calls 时直接返回；本课需要把 dequeue 与继续/停止决定
  放进这两个分支共享的正常 turn boundary。
- `internal/agent/tool.go`：并行 stage 等全部 workers settlement，并按模型
  source order 构造 tool results。Steering 只能发生在整批结果追加之后。
- `internal/coding/session.go`：Session 与 `activeAdvance` 已持有 Follow-up
  admission/queue；本课需要增加 Steering ownership，同时保持一个 lifecycle
  guard、一个 Session mutex 与 Engine 的 invocation-local state。
- `internal/observation/event.go`：同一 Run 中已有 Turn 与 user Message facts；
  consumed Steering 可由这些既有事件表达。是否需要 queue-specific event 要
  等真实 terminal preview consumer，而不是在本课预造第二份 queue state。

## 第一组教学：安全点改变的是下一次决策，不是当前副作用

这里的“当前 tool work 全部 settlement”有一个严格范围：只指**当前这一条
assistant message 已经发出的所有 tool calls**，不指整个任务未来还可能出现的
tools。每个 call 都必须得到且只得到一个 terminal tool result；result 可以是
成功、tool-local error、canceled 或 not-executed，因此 settlement 不等于全部
成功。并行 calls 可以按任意实际顺序完成，但它们的 results 仍按模型 source
order 追加。只有这些配对全部闭合后，Steering 才能成为下一条 user message。

如果 cancellation 或其他 execution-level error 使当前 Run 异常结束，即使
剩余 tool calls 已用 canceled/not-executed results 闭合，也不会继续注入
Steering；pending input 的 ownership 如何交还属于本课后续的异常 hand-back
讨论。对于正常返回但刻意留下后台进程的 Bash 调用，tool settlement 指 Bash
tool 已返回自己的 result，不承诺其启动的所有外部后台进程都已经退出。

考虑模型一次返回两个 tool calls，用户在第一个 tool 仍运行时提交 Steering：

```text
assistant(tool A, tool B)
  -> tool A starts
  -> Steering admitted, still pending
  -> tool B starts or waits behind the scheduling barrier
  -> tool A settles
  -> tool B settles
  -> append tool result A, tool result B in model source order
  -> dequeue Steering
  -> append Steering as user message
  -> next Provider request
```

这条顺序同时保护三个契约：

1. 已经由模型请求并开始调度的 tool work 不被 Steering 隐式取消或跳过；
2. 下一次 Provider request 一定看到完整 assistant/tool-result 配对；
3. 用户的新约束尽早影响下一次尚未发生的模型决策。

因此 Steering 更准确的心智模型是“在下一个决策点补充输入”，不是“中断当前
副作用”。真正要停止正在执行的工作仍使用 `Cancel`；要等当前目标完整结束后再
做另一件事仍使用 Follow-up。

## 已确认的第一项边界

学习者确认上述 Run boundary：Steering 消费后仍属于同一个
Engine Run，只增加一条 user message 和后续 Turn；Follow-up 则在当前 Run
settlement 并提交 delta 后开启新的 input-started Run。

## 第二组教学：终端接收与 Runtime admission 是两层

`Steer(input)` 首先是 ownership-transfer acknowledgement，不是同步执行 API：

```text
caller owns input
  -> Steer(input)
       -> rejected: caller 保留 input
       -> accepted: Session 对 input 负责
            -> 在当前 Engine Run 的 safe boundary 消费
            -> 或在 Run 异常结束时明确交还
```

如果只检查 `Advance active`，窗口会过宽。一次 Advance 还包含 pre-Run
compaction、overflow compaction/recovery、相邻 Follow-up Runs 之间的协调和最终
snapshot/observation settlement；这些时刻未必存在一条可以让 Steering 加入的
current Engine Run。无条件 acceptance 会迫使实现把输入悄悄改投到另一条 Run，
或者在 final boundary 留下 orphan。

学习者指出，普通 Codex CLI 使用中并不会因为 agent 正在工作而禁止编辑或回车。
重新核对三个 terminal surface 后，这项观察得到确认：

- 冻结 Pi 在 streaming 时把普通 Enter 作为 Steering 交给 Session；compaction
  期间则由 interactive mode 先保存输入，compaction 后再根据 retry/idle 状态
  路由。
- 当前 Codex CLI 的 composer 正常接收提交；Core 只允许 active regular turn
  接受 same-turn steering。没有 active turn 时同一 `UserInput` 启动新 Turn；
  review/compact 等 active-but-not-steerable rejection 会由 TUI 保存在
  “end of turn” queue。
- 当前 OpenCode direct interactive mode 也保持 footer 可输入，但把 active
  ordinary turn 期间的新 prompt 留在可编辑、可删除的本地 FIFO，之后作为新
  turn 执行；该路径并不提供 same-run Steering。

因此前一个“整个 Advance admission 或 Engine-active-only admission”的二选一
混合了 UI 与 Runtime 两层，现予纠正：

1. **terminal input acceptance**：终端始终接收文本与回车；尚未转移给 Session
   的输入由 terminal/caller 保留并自动投递，不能丢失、拒绝或要求用户重输；
2. **Runtime Steering admission**：`Session.Steer` 只在能够把输入绑定到唯一
   current Engine Run 时接受；
3. Session 即将调用 `Engine.Run` 或 `Engine.Continue` 时打开该 invocation 的
   Steering admission window；Provider stream 与 tool work 期间可以入队；
4. normal would-stop boundary 必须在同一个 Session mutex 下执行
   dequeue-or-seal，避免 accepted input 留在 final boundary 成为 orphan；
5. Provider/execution error 或 cancellation 不消费新 Steering，而是 seal 并把
   尚未消费的输入留给异常 hand-back；
6. compaction、Run 间隙或已封口状态中的终端输入仍被 UI 接收，但不能由
   Runtime 假装成已加入原 Run；未来 terminal coordinator 负责在同 Run 再次
   steerable 时重试，或在当前 settlement 后自动把它送入下一工作单元。

这既保留“用户始终可以输入”的产品体验，也让每次 Runtime `nil` acknowledgement
仍有严格含义：Session 已经接管输入，而且它确实属于一条确定的 Engine Run。

## 已确认的第二项边界

学习者确认 TUI/terminal 交互与 Session Runtime admission 必须分层：前者不阻止
输入，后者只对能加入同一 Engine Run 的 Steering 转移 ownership。此前忽略
terminal buffering、把问题压成 Runtime admission 二选一的表述被本节取代。

## 第三组教学：Steering 在哪些位置改变下一次 Provider request

冻结 Pi 在 Run 开始时和每个正常 assistant turn settlement 后检查 Steering。
“正常 turn settlement”包含两种形状：

1. assistant 没有请求 tools：terminal assistant 已完整形成，Run 原本将停止；
2. assistant 请求了 tools：同一 assistant message 的全部 tool calls 已各自形成
   terminal result，并已按模型 source order 追加。

如果 safe boundary 有 pending Steering，loop 不会使用旧 Working Context 直接
发起下一次 Provider call，也不会把 Steering 作为独立 Provider 参数。它先把
该原子切点前已经接受的全部 Steering 按 admission order 作为独立 user messages
追加到 run-local Working Context，再从这份完整消息序列构造下一次 Provider
request：

```text
existing context
  -> assistant(tool call A, tool call B)
  -> tool result A
  -> tool result B
  -> user Steering
  -> build Provider request snapshot
  -> Provider call
```

所以“带上 Steering 一起调用 Provider”是正确的，但准确含义是：**Steering 已经
成为下一次 request 的一条 user message**；它不是 system prompt 补丁、tool
result 附件或额外的 `steer` request field。若 Steering 在下一次 Provider stream
已经开始后才到达，它不能修改这次 in-flight request，只能等待该 assistant turn
及其 tools 再次完整 settlement。

学习者在确认单条 Steering 的 request ordering 后提出，多条已经 pending 的
Steering 应像 `tool results -> user1 -> user2 -> Provider` 一样一起进入下一次
request，并确认 Pia 采用该行为。它让模型在产生新决策和工具副作用前看到当前
safe boundary 已知的全部用户修正，也减少额外 Provider turn；各 Steering 仍是
独立 Conversation user Messages，只是不要求它们之间各产生一次模型响应。

这项决定推翻此前沿用冻结 Pi 默认 `one-at-a-time` 的 Pia 课程假设。Pia 采用
Pi 已支持的 `all` 能力方向，并以当前 Codex pending-input drain 作为额外产品
证据；Follow-up 仍保持 Lesson 14 已确定的 FIFO one-at-a-time 新 Run 语义。

无论采用哪种 queue mode，Provider error、aborted response、execution-level
tool error 或 cancellation 都直接异常结束 Run，不在错误路径继续消费 Steering。

## 已确认的第三项边界

学习者已经理解 Steering 不修改正在进行的 Provider 请求，而是在当前正常
assistant turn（含其完整 tool batch）settlement 后作为 user message 进入下一次
Provider request，并确认每个 safe boundary 原子取出当时全部 pending Steering，
保持各自独立的 user messages，以一次 Provider request 处理。

## 第四组教学：drain-or-seal 消除最后一条输入的竞态

Run 原本要停止时，Engine 与并发 `Steer` caller 可能同时观察 queue。若 Engine
先在无锁状态看到空 queue，随后 caller 成功入队，而 Engine 又直接返回，就会留下
一条 Session 已确认接管、却再也没有 Engine 消费的 orphan。

Pia 必须把最后边界收敛为同一个 Session mutex 下的原子决定：

```text
normal assistant turn settled
  -> lock Session
     -> pending Steering 非空：detach entire batch, keep window open
     -> pending Steering 为空：seal this Engine invocation
  -> unlock Session
     -> detached batch: append user messages and call Provider
     -> sealed: return Run result
```

Runtime 的 same-Run routing 只会得到两种合法顺序：

1. `Steer` 先拿锁：input 进入 queue，Engine 随后取出它并继续同一 Run；
2. Engine 先拿锁：它看到 empty、封口并准备返回；terminal 提交仍已被产品层
   接受，但 ownership 暂时留在 terminal coordinator，等待当前 settlement 后
   自动进入后续工作。

有 tool results、无论如何都要继续 Provider protocol 时，empty drain 不封口；
Steering 在切点后到达会等待这个 Run 的下一个正常 turn boundary。只有
assistant 无 tools、Run 原本将停止的 boundary 才执行 empty-and-seal。

## 已确认的第四项产品边界

学习者要求 terminal submission 一定被消费，不能拒绝。`drain-or-seal` 因此只决定
它能否仍作为 Steering 加入原 Run，不决定产品是否接收输入：封口前由 Session
接管并进入同一 Run；封口后由 terminal coordinator 保持 ownership，并在当前
settlement 后自动路由到后续工作。两条路径都不丢失或要求用户重输。

这项保证以 terminal 仍开放且用户未执行显式退出/丢弃为边界；Provider failure、
process termination 与 `/exit` 不能被描述为模型已经成功处理，未完成输入的恢复或
丢弃仍按对应控制与未来 persistence 课程决定。

## 第五组教学：Provider failure 停 Run，但不停止输入所有权

冻结 Pi、当前 Codex CLI 与当前 OpenCode 对 Provider failure 的低层 retry
mechanism 并不相同：

- 冻结 Pi 的 Agent Loop 在 assistant error/aborted 后立即结束，但 coding
  `AgentSession` 可以先执行有界 retry，并在仍有 queued messages 时发起
  continuation；
- 当前 Codex 对 retryable sampling error 保持原 request input 重试，pending
  input 等 retry 结束；最终 error 结算 Turn 后，Core task 与 TUI 都会继续发送
  queued input；
- 当前 OpenCode 对 retryable error 继续当前 prompt 的 retry policy；non-retryable
  error 写入 assistant、发布 error 并转为 idle，direct interactive FIFO 随后
  执行下一 prompt。

三者共同支持的产品事实不是“把失败伪装成成功”，而是：失败 attempt 先形成
terminal boundary，已经接受或保留的用户输入不会因此丢失，也不要求用户重新输入。

Pia 当前已有更严格的 Engine contract：Provider/stream failure 提交 terminal
assistant 后返回 non-nil error；automatic Provider retry 又已由 D84 明确延后。
因此本课不让失败 Run 继续调用 Provider。正确的 ownership 顺序是：

```text
Engine Run A
  -> Provider failure
  -> append terminal assistant error
  -> seal Steering admission
  -> return Run A error plus pending-Steering hand-back

future terminal coordinator
  -> display Run A failure
  -> automatically route retained Steering into execution B
  -> restore retained Follow-up to the composer
  -> preserve user1, user2... as distinct ordered user messages
  -> build execution B's first Provider decision from the complete settled History
```

execution B 是新的工作单元，不是 Run A 的成功 continuation。它仍从完整
Conversation History 派生 Working Context，因此保留 Run A 的失败事实；但
lifecycle、Run events 与 Go error 不会把两次 execution 混成一个既失败又成功的
outcome。

这里的“消费”沿用 Lesson 14 的 message boundary：输入成为 Conversation user
Message 就算 consumed。若 execution B 的 Provider 仍失败，这些 user messages
留在 History，不再重复投递；用户之后的新操作自然从包含它们的 History 继续，
避免同一 batch 因持续 Provider failure 无限重放。

同一个 Advance 还可能持有通过 `Tab` 接受的 Follow-up。Provider failure 表示
当前工作没有正常完成；Follow-up 的 intent 又是“当前工作结束后再做”，因此不能
随着 Steering 自动启动。future terminal 只把 pending Steering 自动路由到新的
execution，把未消费 Follow-up 恢复到 composer，等待用户编辑或重新提交。两类
input 都不丢失，但 failure 后的默认 delivery policy 不同。

## 已确认的第五项边界

学习者确认采用“停失败 Run，不停输入队列”：Provider failure 不在同一 Engine
Run 内继续消费 Steering，也不等待用户重新输入。Runtime 明确交还尚未成为
Conversation Messages 的 pending batch；未来 terminal coordinator 在失败
settlement 后自动将 Steering 路由为新的 execution，并把 Follow-up 恢复到
composer，不自动执行。Lesson 15 不实现 automatic Provider retry，也不把通用
terminal submission scheduler 下沉进 Session。

## 第六组教学：Esc 撤销当前工作意图

Provider failure 与用户主动 `Esc` 不能共享 terminal routing policy。前者表示
系统没有完成用户已经提交的意图，因此正常产品路径仍应保留并自动推进 pending
input；后者表示用户主动改变主意并要求停下。

若 `Esc` 只取消当前 Run、随后又自动把 pending Steering 启动为新 Run，用户需要
连续按两次 `Esc` 才能真正停住：

```text
first Esc
  -> cancel Run A
  -> automatically start retained Steering as Run B

second Esc
  -> cancel Run B
```

这与 Pia 的 `Esc` 产品含义不符。学习者明确要求一次 `Esc` 就停止当前工作，
queued Steering 也不应再执行。因此 terminal 在当前 cancellation settlement
后把所有仍未成为 Conversation Messages、且归属于被取消工作意图的 Steering
与 Follow-up 从 execution queues 移回 composer；不自动重投、不启动下一 Run，
也不要求第二次 `Esc`。

这不违反“普通 submission 必须消费、不能拒绝”的要求。Runtime 已经接受过输入，
而用户随后通过显式 Cancel 暂停了尚未执行的意图；terminal 继续持有原文本并让
用户选择编辑、重新提交或清空，而不是 admission rejection、silent loss 或
Provider failure 下的错误丢弃。已经在此前 safe boundary 成为 Conversation user
Message 的 Steering 或已经开始新 Run 的 Follow-up 仍留在 History，不会因
Cancel 回滚。

## 已确认的第六项边界

学习者确认 `Esc` 表示停止并重新取得输入控制：它取消当前 Engine Run，并在
settlement 后把全部尚未消费的 queued Steering 与 Follow-up 交还给 TUI composer，
不自动启动后续 Run。一次 `Esc` 必须足以让 Session 回到不再自动执行 pending
input 的 idle 状态；原文本仍在输入框中，由用户编辑、重新提交或清空。

## 第七组教学：composer restore 按 intent 简单分组

`Esc` 后不需要重建 Steering 与 Follow-up 交错提交的全局时间顺序。TUI 使用简单、
稳定的分组规则：

```text
all unconsumed Steering in FIFO order

all unconsumed Follow-up in FIFO order

existing unsent composer draft
```

各文本块之间用空行分隔。Steering 放在上面，Follow-up 放在下面，原输入框里尚未
提交的 draft 放在最后。恢复后它们共同成为一份普通可编辑 draft，不再自动保留
原 delivery intent；用户之后按 Enter 或 Tab 重新决定下一次提交的 intent。

这个选择刻意放弃两类 queue 之间的 global admission chronology。Session 只需
分别保持两类 input 自己的 FIFO，不必为 composer restore 增加跨类型 sequence、
统一 pending record 或第二份 terminal-side authoritative queue。

## 已确认的第七项边界

学习者选择简单分组恢复：`Esc` settlement 后，TUI 把未消费 Steering 按 FIFO
放在 composer 上部，把未消费 Follow-up 按 FIFO 放在下部，既有 draft 最后；
不要求保持两类 input 的交错提交顺序。

## 第八组教学：hand-back 保持两个独立 FIFO 字段

Lesson 14 已经让 `AdvanceResult.UnconsumedFollowUps []string` 明确交还未消费
Follow-up。Steering 与 Follow-up 的 terminal policy 已经不同，且 `Esc` restore
不要求跨类型 chronology，因此本课不替换为带 kind/sequence 的统一结构，只增加
平行字段：

```go
type AdvanceResult struct {
    History             []ai.Message
    UnconsumedSteering  []string
    UnconsumedFollowUps []string
}
```

`Steering` 在这里是 category name，因此字段使用 `UnconsumedSteering`，不使用
不自然的复数 `Steerings`。两个 slices 都保持各自 FIFO，并继续遵守相同 consumption
boundary：只有尚未成为 Conversation user Message 的输入才会 hand back；已经消费
的输入只留在 History，不重复出现在 result 中。

正常完整 settlement 时两个字段都为空。Provider failure 时 future terminal 自动
路由 `UnconsumedSteering`，把 `UnconsumedFollowUps` 恢复到 composer；`Esc` 时把
两者按已确认的分组顺序恢复；`/exit` 可以在 settlement 后明确忽略两者。

## 已确认的第八项边界

学习者确认 `AdvanceResult` 保留既有 `UnconsumedFollowUps []string`，并增加
`UnconsumedSteering []string`。不引入统一 pending-input result、kind enum 或
跨 intent sequence。

## 第九组教学：Steer 是窄的非阻塞 admission API

`Session.Steer` 沿用 Follow-up 已建立的 ownership-transfer 形状：

```go
var ErrSteerUnavailable = errors.New("coding: steering is unavailable")

func (s *Session) Steer(input string) error
```

方法先使用与 `Advance`/`FollowUp` 相同的 blank-input validation，再在 Session
mutex 下检查 lifetime 与当前 Engine invocation 的 Steering admission window：

- closing/closed 返回 `ErrSessionClosed`；
- idle、compaction、Run 间隙、Provider failure/cancellation 后或 final seal
  返回 `ErrSteerUnavailable`；
- 当前 Provider stream 或 tool work 所属的 Engine invocation 仍开放时，复制
  input 并追加到 Session-owned Steering FIFO，返回 `nil`。

`nil` 只确认 ownership 已从 caller 转给 Session；它不等待 safe boundary、不调用
Provider/tool，也不表示 input 已经成为 Conversation Message。该操作是短锁内的
内存 admission，因此不增加 `context.Context`、result handle 或 blocking variant。

final drain-or-seal race 继续由同一个 Session mutex 线性化：`Steer` 先取得锁就
accepted，Session 必须在正常 safe boundary 消费或异常 settlement hand back；
Engine 先 seal 则返回 `ErrSteerUnavailable`，mutation 尚未发生，caller 仍拥有
input。future terminal 吸收这个 Runtime unavailable result 并保留文本，不把它
呈现为用户可见 rejection。

## 已确认的第九项边界

学习者确认增加非阻塞 `Session.Steer(input string) error` 与
`ErrSteerUnavailable`。blank input 先 validation，closing/closed 使用
`ErrSessionClosed`；`nil` 只表示当前 Engine invocation 已接管 input。

## 第十组教学：Close 不成为第二个 hand-back owner

Session 已由负责执行 `Advance` 的 caller 持有唯一最终 result。`Close(ctx)` 只
改变 lifetime、拒绝新 admission、请求 active cancellation 并等待 settlement；
它不复制返回 pending queue：

```text
Session.Close(ctx)
  -> mark closing
  -> reject new Steer/FollowUp
  -> cancel active execution
  -> wait for settlement

active Advance caller
  -> receives UnconsumedSteering
  -> receives UnconsumedFollowUps
```

因此不同 host intent 继续由一个 result owner 处理：

- TUI `Esc` 使用 `Session.Cancel()`，在 `Advance` 返回后恢复两类文本到 composer；
- `/exit` 等待既有 Advance settlement，明确忽略两个 hand-back 字段，再完成 close；
- caller context cancellation 只要仍能正常返回，就由 Advance caller 接收并决定
  是否恢复；
- `Close(ctx)` wait timeout 不重开 Session，也不把 queue 转交给已经超时的 Close
  caller；active Advance 最终仍向原 caller返回 hand-back；
- hard kill 无法执行内存 hand-back，只能等待 future journal/checkpoint 提供有限
  recovery，本课不承诺。

这延续 D98/D104 的 ownership：Close 不是 Advance outcome channel，queue 也不会
同时交给两个 callers。`Session.Steer` 在 closing/closed 仍返回
`ErrSessionClosed`，mutation 前 caller 保留那条尚未 admission 的文本。

## 已确认的第十项边界

学习者确认 pending Steering/Follow-up 始终随 active `AdvanceResult` hand back；
`Close(ctx)` 不增加 queue result。`Esc` 恢复、`/exit` 忽略、caller cancellation
恢复与 hard-kill limitation 都由 host 根据同一个 Advance result 处理。

## 第十一组教学：不增加 queue Semantic Events

Lesson 15 的 observable facts 已由现有 surface 完整表达：

- `Steer()` return 表达 admission accepted/unavailable；
- consumed Steering 在同一 Run 内自然产生既有 user Message event；
- 多条 Steering 按 admission order 产生多条 Message events，然后才发生下一
  Provider Turn；
- Run/Advance failure 或 cancellation 继续使用既有 settlement events；
- 未消费输入由 `AdvanceResult.UnconsumedSteering` 与
  `UnconsumedFollowUps` hand back；
- `Esc` 把文本恢复到 composer 是 future terminal 的本地 UI 操作，不冒充 Runtime
  semantic fact。

因此本课不增加 `steering_admitted`、`steering_dequeued`、
`steering_restored` 或通用 `queue_changed` events。当前没有 pending-preview
consumer；预造事件会同时引入 payload、ordering、observer failure 与 authoritative
queue projection 问题。future terminal 若确实需要 pending preview，应从真实
consumer 重新评估只读 projection，而不是让 terminal 根据零散 events 维护第二份
authoritative queue。

## 已确认的第十一项边界

学习者确认 Lesson 15 不新增 queue Semantic Events。admission、consumption、
settlement 与 hand-back 分别复用 API return、Message/Run/Advance events 和
`AdvanceResult`。

## 第十二组教学：successful overflow recovery 跨 invocation 保留 Steering

明确可恢复的 context overflow 与 ordinary terminal Provider failure 不同。当前
Session 会先提交 failed Engine Run delta，再执行 bounded compaction，并在成功时
调用 `Engine.Continue`；整个 input execution 和 outer Advance 尚未 settlement。
此时也没有可供 terminal 立即接收 queue 的中途 `AdvanceResult`。

因此第一次 overflow Run 结束时，Session 关闭旧 Steering window，但继续持有该
window 已经 accepted、尚未 consumed 的 batch。compaction 期间不开放新的 Runtime
Steering admission；新的 terminal submission 仍由 caller 保留。compaction 成功
后，既有 batch 转移给 recovery `Engine.Continue`，并在第一次 recovery Provider
request 前按 admission order 追加为独立 user Messages：

```text
overflow assistant error
  -> compact
  -> user Steering 1
  -> user Steering 2
  -> recovery Provider request
```

context overflow 没有形成可用的模型决策；让 Steering 参与 recovery 后的第一个
Provider decision，可以避免 recovery model 在看到修正前产生新的 tool side
effects。Follow-up 仍保持 D104：只有整个 input recovery 成功后，才作为新的
input-started Run drain。

若 overflow compaction、Working Context derivation、recovery Continue、第二次
overflow 或 cancellation 最终失败，outer Advance 才 settlement，并通过
`UnconsumedSteering` hand back 仍未成为 Conversation Messages 的 batch。successful
overflow recovery 是跨 Engine invocation 保留 Steering 的唯一当前例外；ordinary
Provider failure 仍直接 hand back。

这里的 hand-back 是明确的 ownership transfer，不是 UI 动作本身：

```text
Steer returns nil
  -> Session owns pending text

Advance settles before consumption
  -> result.UnconsumedSteering contains the text
  -> Session no longer owns or executes it
  -> Advance caller owns it again
```

future terminal 再根据 settlement 原因选择自动重投、恢复到 composer 或在 `/exit`
时明确忽略。已经成为 Conversation user Message 的 Steering 只留在 History，
不会同时出现在 hand-back result 中。

## 已确认的第十二项边界

学习者确认 successful context-overflow recovery 跨 failed Run、compaction 与
`Engine.Continue` 保留已接受 Steering，并在第一次 recovery Provider request 前
批量消费；recovery 最终失败或取消才 hand back。学习者同时确认 hand-back 表示
Session 通过 `AdvanceResult` 把未消费文本的唯一 ownership 交还给 Advance caller。

## 第十三组教学：Steering queue 是 Session 状态，不是 channel pipeline

虽然 Steering 有一端 admission、一端 consumption，但当前问题的核心不是持续工作
流，而是 Session 必须原子决定“接受当前 input”“取走当前 batch”和“空队列时封闭
当前 invocation”。buffered channel 需要预设容量，满时只能阻塞或拒绝；channel
close 又不能在 concurrent sender 仍可能发送时单独承担 final seal。专门建立
coordinator goroutine 虽可串行化这些动作，却会额外引入 command acknowledgement、
goroutine lifetime、cancellation、close 与 backpressure，而 atomic/CAS queue
也会增加复制或 lock-free ownership 推理，并不能删除 Session 已有的 lifecycle
mutex。

因此 `activeAdvance` 继续是 invocation control 的唯一 mutable owner：

```go
type activeAdvance struct {
    // existing lifecycle and Follow-up fields
    acceptingSteering bool
    pendingSteering   []string
}
```

`Session.Steer`、normal batch drain、final drain-or-seal 和 abnormal detach 都只在
现有 Session mutex 下访问这些字段，不增加第二把锁、channel 或 background
goroutine。Engine 不直接读取或修改这个共享 slice；它只通过借用的窄 control 请求
一次原子操作，并取得 ownership-independent batch。锁释放后，Engine 消费的是已
detached 的独立 `[]string`。

这也解释了为什么当前方案不是“双方通过锁共同消费 slice”：Session 单独拥有和
mutate queue，Engine 只消费 Session 已经转交的值。即使当前 terminal 通常只有
一个 input producer，`Session.Steer` 仍可能被多个 goroutine 调用；更重要的是，
final empty 与 seal 的线性化本身就需要同步，单纯的 single-producer/
single-consumer 假设不能消除这个协议。

## 已确认的第十三项边界

学习者确认 Steering queue 直接放在 `activeAdvance`，并复用现有 Session mutex；
不使用 buffered channel、专用 coordinator goroutine、atomic/CAS queue 或第二把
锁。Session 是共享 queue 的唯一 owner，Engine 只借用 control 并接收 detached
batch。

## 第十四组教学：Engine 定义自己消费的窄 control

`internal/agent` 不能反向依赖长期状态 owner `internal/coding.Session`，也不应知道
`activeAdvance`、mutex 或 hand-back。按照 consumer-owned interface，Engine 只
定义自己在 run-local loop 中需要的两个原子操作：

```go
type SteeringSource interface {
    Drain() []string
    DrainOrSeal() []string
}
```

`Drain` detach 当前 batch 并保持 admission window 开放，用于下一次 Provider call
无论如何都会发生的边界；`DrainOrSeal` 返回当前 final available batch并保持
开放，或在 invocation 应停止时原子 seal并返回 empty，用于 assistant 无 tools、
Run 原本将停止的边界。返回值已经是 ownership-independent batch，因此接口不暴露
queue、lock、peek、length、close 或 hand-back。

`internal/coding` 提供引用 `Session` 与当前 `activeAdvance` 的薄 adapter 来实现该
接口。adapter 的方法回到 Session mutex 下完成原子操作；Engine 只迭代返回值并把
每条文本追加为独立 user Message。一个带 `sealIfEmpty bool` 的 `Drain` 在功能上
等价，但会让调用点出现语义不透明的 `Drain(true)`；两个方法直接对应两种协议
边界，且没有增加新的 runtime state。

## 已确认的第十四项边界

学习者确认 `SteeringSource` 由 `internal/agent` 定义，包含 `Drain` 与
`DrainOrSeal`；`internal/coding` 的 invocation-local adapter 实现它。Engine 不
依赖 Session 类型，也不接触 queue 或 hand-back。

## 第十五组教学：每个 Engine invocation 显式要求 control

Steering control 随一次 invocation 变化，不能进入保存稳定 Provider、prompt 与
tools 的 `agent.Config`。`Run` 与 `Continue` 因此都显式接收 non-nil source：

```go
func (e *Engine) Run(
    ctx context.Context,
    workingContext []ai.Message,
    userInput string,
    steering SteeringSource,
) (RunResult, error)

func (e *Engine) Continue(
    ctx context.Context,
    workingContext []ai.Message,
    steering SteeringSource,
) (RunResult, error)
```

`nil` 不表示“本次关闭 Steering”。允许 optional source 会使 production wiring
遗漏静默退化为旧 loop，也会在多个 safe boundary 引入重复 nil branches。当前
production Engine invocation 只由 Session 协调，Session 总能提供 control；Agent
package 的独立单元测试则使用 test-local empty source 明确表示没有并发输入。这里
不增加 variadic compatibility parameter、context value、Engine-level default 或
公开 no-op implementation。

## 已确认的第十五项边界

学习者确认 `Run` 与 `Continue` 都显式要求 non-nil invocation-local
`SteeringSource`；source 不进入 `agent.Config`，测试使用 test-local empty
implementation，旧调用点直接迁移而不保留兼容 shim。

## 第十六组教学：三个消费点与 cancellation 共同决定 ownership

Engine 在三个位置消费 Steering：

1. `Run` 已追加 initial user Message，或 `Continue` 已验证 continuation context
   后，在第一次 Provider call 前调用 `Drain`；
2. assistant 的 tool calls 与全部 terminal tool results 已追加、Turn 已
   settlement，且下一次 Provider call 由 tool protocol 强制发生时调用 `Drain`；
3. assistant 无 tools、Turn 已 settlement、Run 原本将停止时调用
   `DrainOrSeal`，非空则追加 batch 并继续，empty 则正常结束 Run。

Run/Continue start 与 post-tool 两个必然继续 Provider protocol 的位置先检查
invocation context。source detach 一个 batch 后，Engine 必须把整个 batch 按
admission order 追加为独立 user Messages，再次检查 cancellation，然后才开始
下一 Provider Turn。不能在 batch 中间检查 cancellation，因为 ownership 已离开
Session；部分追加会让剩余文本既不在 History、也不能 hand back。

这给 cancellation/drain race 一个明确切点：

- cancellation 先被 source 观察到时，不 detach，pending Steering 在 abnormal
  settlement 时 hand back；
- drain 先 detach 时，整个 batch 立即成为 Run delta 中的 user Messages。随后
  cancellation 即使阻止下一 Provider call，该 batch 也已经 consumed，只保留在
  History，不再 hand back。

would-stop 边界需要保留既有的 terminal-wins contract：Provider 已经返回完整
`Done` terminal 时，恰好同时到达的迟延 cancellation 不应把这个已完成 Turn
改写为 Engine failure。Engine 因此直接调用 `DrainOrSeal`；Session source 在
mutex 内同时检查 invocation context。若 cancellation 已胜出，source seal
admission、保留 pending queue 并返回 empty，Engine 正常结束已经完成的 terminal
Run；外层 Session 随后因 pending input 与 cancellation 明确 hand back。若没有
pending input，迟到 cancellation 不覆盖已完成 terminal。若 drain 先取得非空
batch，Engine 先完整追加，再检查 cancellation；此时 cancellation 可以阻止下一
Provider call，但 batch 已 consumed。

Semantic Event 顺序保持现有事实关系：Run started 与 initial user Message 先于
start drain；当前 Turn 的 assistant/tool-result Message events 与 Turn settled
先于 post-turn Steering Message events；下一 Turn 只在全部 Steering Messages
追加后 started。

## 已确认的第十六项边界

学习者确认 start、post-tool 与 would-stop 三个 consumption boundary。start 与
post-tool 使用“检查 cancellation、detach、完整追加 batch、再次检查
cancellation”；would-stop 保留 terminal-wins，由 Session source 原子决定正常
drain 或因停止而 seal 并保留 pending。cancellation 先胜出则 hand back；drain
先胜出则整个 batch consumed，不能部分恢复。

## 第十七组教学：一个 bool 足够表达 invocation window

Session 不在 pre-Run compaction、Working Context derivation 或相邻 Run 间隙接受
same-Run Steering。`activeAdvance` 因此只需增加 queue 与 admission bool：

```go
type activeAdvance struct {
    // existing lifecycle and Follow-up fields
    acceptingSteering bool
    pendingSteering   []string
}
```

Session 在即将同步调用 `Engine.Run` 时把 bool 设为 true。normal `Drain` 保持
true，empty `DrainOrSeal` 改为 false；ordinary error/cancellation 返回后 Session
也立即改为 false。进入下一个 Follow-up Run 时，pre-Run 阶段仍保持 false，直到
新的 `Engine.Run` 即将开始。

recoverable overflow 只需要暂时关闭 admission 并保留同一个 slice：failed Run
返回后设为 false，compaction 期间不接收新 Steering；compaction 成功、即将调用
`Engine.Continue` 时重新设为 true，Continue 的 start `Drain` 取走此前保留的
batch。recovery 最终失败时才 detach hand back。

逻辑上虽然可以称为 closed/open/paused 三个阶段，并不需要保存 enum。并发
`Steer` 只询问当前能否 admission；“之后 reopen 还是 final detach”由 Session
正在同步执行的 overflow/settlement path 决定。也不需要 invocation generation
token：一个 Session 至多运行一个 Engine invocation，`Run`/`Continue` 在所有
tool work settlement 后同步返回，adapter 不逃逸，旧 invocation 不可能与新
invocation 并发调用 source。若未来引入 concurrent invocation 或长期后台持有
source，再重新增加 identity，而不为当前不存在的并发预造状态。

每次 Engine 调用可以创建一个引用同一 `Session + activeAdvance` 的无状态薄
adapter；adapter 实例是 invocation-local 的，但 queue 仍由 active Advance
连续拥有。这避免为 overflow transfer 移动或复制 batch。

## 已确认的第十七项边界

学习者确认使用 `acceptingSteering bool + pendingSteering []string`，不增加 window
enum 或 generation token。pre-Run/Run gap 关闭，Engine invocation 前开放，
ordinary settlement 后关闭；overflow 暂停 admission、保留同一 queue，并在
Continue 前重新开放。

## 第十八组教学：异常路径只做一次 ownership transfer

Engine 每次返回后，Session 先关闭当前 Steering admission，但不立即假设 queue
应该 hand back。normal Run 已通过 final seal 排空 queue；recoverable overflow
需要在 compaction 期间保留它；ordinary failure 或最终 recovery failure 才结束
整个 input execution。

实现因此分成两个操作：

- `pauseSteering` 只把 `acceptingSteering` 设为 false，保留
  `pendingSteering`，用于每次 Engine return 到下一步 Session 决策之间；
- `activeAdvance.sealAndDetachInputs` 在同一个 Session mutex 临界区关闭
  Steering 与 Follow-up admission，并分别 detach 两个 pending slices。若当前
  Follow-up 已 dequeue、但在 Engine acceptance 前失败，它仍作为 Follow-up
  hand-back 的第一项。

ordinary Provider/tool failure、pre-Run failure、cancellation、compaction failure、
recovery Continue failure 与第二次 overflow 最终都通过同一个
`stopPendingInputs` 进入该 helper。`AdvanceResult.UnconsumedSteering` 与
`UnconsumedFollowUps` 仍是两个独立 FIFO；统一的是一次原子 ownership transfer，
不是两类 intent 的数据结构。

normal successful settlement 预期两个 slices 都为空；final drain-or-seal 与
Follow-up queue-empty seal 已经在各自 consumption boundary 完成。异常 helper
保证任何已 admission、尚未成为 Conversation Message 的文本不会因分支差异被
静默遗留。

## 已确定的第十八项边界

学习者委托后续实现决定。Pia 使用 `pauseSteering` 保留 recoverable state，并以
一个 lock-held `sealAndDetachInputs` 原子关闭并交还两类 pending input；所有最终
异常路径复用它，不在各 error branch 分别清 queue。

## 第十九组教学：Cancel 返回前封闭 admission 并发布 cancellation

`activeAdvance` 保存本次 execution context。`Steer` 与 `FollowUp` 在 Session
mutex 内除检查 admission bool 外，也检查该 context 的 cause；caller context
已经取消时，即使 active pointer 尚未清理，也不再接受 input。

`Session.Cancel` 与 active `Close` 在同一个 Session mutex 临界区：

1. 关闭 Steering 与 Follow-up admission；
2. 调用该 active Advance 的 `CancelCauseFunc` 发布 cancellation；
3. 释放 mutex 并返回或继续等待。

Steering drain 使用同一 mutex，因此只有两种线性顺序：

- drain 先取得锁并 detach batch，随后 Cancel 只能取消下一步工作，batch 已作为
  user Messages consumed；
- Cancel/Close 先取得锁，cause 与封口在它返回前均可见，后续 source 不 detach，
  pending input 在 outer settlement hand back。

caller context cancellation 不经过 Session mutex，但 source 在锁内读取
`context.Cause`：若 cause 先可见则保留 queue；若 drain 的检查先发生则 batch
consumed。这个边界不承诺撤销已经发生的 drain，也不会出现部分 batch。

would-stop 仍服从第十六组的 terminal-wins 修正：成功 terminal 已形成且 source
因 cancellation 返回 empty 时，Engine Run 可以正常 settlement；只要仍有
pending Steering/Follow-up，Session 的 Run 后 queue boundary 就以 cancellation
结束 Advance 并 hand back。没有 pending input 时不把已完成 terminal 改写成
canceled。

## 已确定的第十九项边界

学习者委托后续实现决定。`activeAdvance` 保存 execution context；`Cancel` 与
active `Close` 在 Session mutex 内先封闭两类 admission并发布 cancellation，
`Steer`/`FollowUp` 与 source drain 检查同一 context cause。该机制不增加
stopping enum、第二把锁或 condition channel。

## 第二十组教学：实现与验证边界

实现保持现有 package ownership：

- `internal/agent.SteeringSource` 是 consumer-owned 两方法接口；
- `internal/agent` 的 run-local execution 保存 source，在 start、post-tool 与
  would-stop 三个位置消费，并用既有 user Message event 表达 consumption；
- `internal/coding/steering.go` 保存 Session adapter 与 admission/window 操作；
- `activeAdvance` 继续放在 Session lifecycle 文件中并持有两个独立 queues；
- `AdvanceResult` 增加 `UnconsumedSteering`，不增加 queue events、generic pending
  record、background coordinator 或 terminal policy。

验证必须同时覆盖：

- Run start batch、Continue start transfer、完整 tool batch 后 ordering 与
  would-stop continuation；
- Provider failure 不 drain、drain 中 cancellation 仍完整追加 batch，以及既有
  terminal `Done` 胜过迟到 cancellation；
- active/idle/blank/sealed/closed admission、final seal race、同一 Run 的
  Message/Run event boundary；
- Provider failure、caller cancellation、`Cancel`、`Close` 的双 queue hand-back；
- overflow compaction 期间拒绝新 Runtime Steering、成功 Continue 消费旧 batch；
- `make check` 与 `go test -race ./...`。

## 已确定的第二十项边界

学习者委托后续实现与验证决定。上述文件边界与测试矩阵作为 Lesson 15 的最终
implementation direction；不扩大到 terminal、persistence、Provider retry 或
multi-Session coordination。

## 实现与验证结果

本课已按上述 contract 落地：

- `internal/agent` 增加 invocation-local `SteeringSource`，并在 Run/Continue
  start、完整 tool batch settlement 后与 would-stop 三个安全点消费；
- `internal/coding` 增加 `Session.Steer`、Session-owned FIFO、原子
  drain-or-seal、异常双 queue hand-back 与 overflow recovery transfer；
- consumption 保持 batch 内 admission order，并把每条 Steering 作为独立 user
  Message 追加到同一个 Engine Run；Provider/tool-stage cancellation 后不再
  drain，已经进入下一 Provider request 的 Steering 不重复 hand back；
- final-seal、Cancel/Close、caller cancellation 与 drain 使用同一个 Session
  mutex/context cause contract 线性化，没有增加第二把锁、background goroutine
  或 queue event。

测试采用 proof-first 路径，覆盖 start/post-tool/would-stop ordering、Provider
failure、tool-stage cancellation、whole-batch cancellation ownership、所有
admission 状态、双 queue hand-back、overflow recovery 与 final-seal race。
最终 `make check` 和 `go test -race ./...` 均通过；简化审查合并了 input
validation 与 abnormal detach 路径，结构化审查没有发现剩余生产逻辑缺陷，并
补齐了 consumed Steering 不重复 hand back 的回归测试。

## 当前待确认点

产品、ownership、concurrency、implementation 与 verification contract 已全部
确定，没有剩余设计或代码项。学习者明确要求把本课提交并推送到 `main`；本课
至此完成，但不会因此自动开始下一课。
