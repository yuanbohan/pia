# 第 14 课：Follow-up queue 与 quiescence

## 当前状态

学习者于 2026-07-29 明确开始本课。冻结 Pi checkout 已核对为
`dcfe36c79702ec240b146c45f167ab75ecddd205`，与课程固定基线一致；低层
Agent queue、Agent Loop drain points、coding `AgentSession` settlement 路径及
相关 tests，以及当前 Pia 的 Session、Agent Execution Engine、semantic events
与 lifecycle tests 已完成首轮开课校准。

课程状态为**已提交**。源码证据确认 Follow-up 必须等当前
tool/steering 工作结束后再消费，并让 Session 在 pending input 排空前保持非
quiescent；同时推翻了“先让整个 Session 变 idle，再把 Follow-up 当作一个无主
的新操作启动”的隐含理解。学习者确认的 Run、admission、异常 hand-back 与
event 边界已经完成实现、简化审查和验证：每条 Follow-up 启动新的
input-started Engine Run，但这些 Runs 保持在同一个 active `Advance` 内，直到
队列排空后才发布 Advance settlement 与 Session quiescence。学习者已确认理解
pre-Run failure 的 acceptance/hand-back 边界，并明确要求提交和推送到
`origin/main`。

本课暂保持 **Medium**：队列、admission、消费与 quiescence 都属于同一个
Session-owned capability；不修改 active tool loop、不加入 steering、持久化、
终端、后台 scheduler 或多 Session coordination。如果后续确认需要独立
per-follow-up result handles、后台 driver 或第二套 lifecycle state，本课将重新
评估规模并在成为 XLarge 前拆分。

## 解锁能力

Lesson 13 的 Session 已支持多个调用方**顺序**执行 `Advance`，但 active 时的
第二次 `Advance` 只能立即返回 `ErrSessionBusy`。这避免了隐式排队，却还不能
表达一个交互式 host 的常见输入：

```text
Agent 正在完成当前工作
  -> 用户提交“做完以后再补一个回归测试”
  -> 当前工作正常结算
  -> Session 自动按顺序处理这条后续输入
  -> 所有 active/pending 工作排空后才真正 idle
```

本课完成后，Session 能在 active 期间接受内存 Follow-up input，并在明确的
settled boundary 逐条消费。Pending input 在被消费前不是 Conversation Message；
只有它真正进入一个 input-started Engine Run 后才成为 authoritative History
中的 user message。

## 开课源码校准

### 已确认的大纲假设

- 冻结 Pi 明确区分 Steering 与 Follow-up。Steering 在当前 assistant turn 的
  tools 全部 settlement 后、下一次 Provider call 前拉取；Follow-up 只在已经
  没有 tool calls 和 Steering、Agent Loop 原本将停止时拉取。
- Pi 的 queue mode 可以是 `all` 或 `one-at-a-time`，默认
  `one-at-a-time`。后者每个 drain point 只取最老一条，保留其余输入等待下一次
  would-stop boundary。
- Queued input 在 enqueue 时不进入 transcript；低层 loop 真正消费它时才发
  user message lifecycle 并追加到 messages。
- Pi 的 coding `AgentSession` 在一个 prompt 内部协调 core run、retry、
  compaction、continuation 和 queued input。`agent_end` handler 晚到的
  Follow-up 也会触发 continuation；只有这些工作全部结束后才发一次
  session-level `agent_settled` 并唤醒 idle waiter。
- 当前 OpenCode checkout `cb562b2c6289c2eee707078f9ab644cbe1d3d8a9`
  的 `SessionRunner.run()` 使用 inner continuation/steering loop 和 outer
  queued-input loop；Steering 优先在当前 continuation 中提升，Follow-up
  则在 continuation 结束后 FIFO 一次提升一条。其 `session.prompt` 是 durable
  admission API，因此不能把其公开调用形状机械套到 Pia 的同步 `Advance`。
- 当前 Codex CLI checkout `0fb559f0f6e231a88ac02ea002d3ecd248e2b515`
  的 active input 通过 `turn/steer` 加入同一个 Turn；TUI queued input 只在
  当前 Turn 完成后逐条启动下一个 Turn。它证明 Follow-up 是新的工作单元，但
  由 TUI coordinator 驱动新 Turn 的 ownership 形状不适用于尚无 terminal
  coordinator 的本课。
- 当前 Pia 的 Session 已经是 queue、busy/cancel/wait/close、History/projection
  和 Workspace 的唯一 owner；`internal/agent.Engine` 是 run-local component，
  不应重新取得长期 queue 或 Conversation state。

### 细化后的认识

- “Follow-up 在 Agent 完成后执行”中的“完成”是 **would-stop boundary**，
  不是 Session 已发布 idle。当前 Run/loop 已得到 terminal assistant、没有更多
  tool/steering 工作后可以选取下一条 input，但 session-level settlement 仍未
  发生。
- Pi 把 queue 放在 stateful core `Agent`，这是它的类型与所有权选择，不是 Pia
  必须复制的产品契约。Pia 的 D97/D100 已有更强证据支持 Session-only long-lived
  ownership，因此 queue 应由 Session 持有，Engine 仍只接收 snapshot、返回
  delta。
- Follow-up 的安全点比 Steering 更外层。它不需要修改
  `internal/agent/execution.executeRun()` 的 post-tool loop；Session 可以在一个
  Engine Run 已 settlement、delta 已提交 History 后再决定是否派生下一份
  Working Context 并启动另一个 input-started Run。
- Quiescence 比 `active Run == nil` 更强。它至少要求没有正在执行的 Run、没有
  pending Follow-up、对应 History commits 和 synchronous observations 都已
  settlement。Lesson 13 的 `Wait`/`Close` 必须最终观察这个外层边界。

### 被推翻或需要修正的假设

- “Follow-up 必须等当前公开 `Advance` 完全返回后，再成为一个独立公开
  `Advance`”没有得到冻结 Pi 支持。Pi 的普通 Follow-up 保持同一个
  session-level operation active，队列排空后才 settled。
- “复用并发 `Advance` 的 fail-fast 路径，把第二个调用改成阻塞等待即可”不
  成立。普通 `Advance` 和 Follow-up 有不同 admission intent；把所有 busy 调用
  自动排队会隐藏调用方错误，也无法区分后续 Steering。
- “Engine 直接轮询 Session queue 最接近 Pi”不成立。这样会让 run-local Engine
  依赖并持有 Session lifecycle input，并重新形成 Lesson 13 已删除的双重 owner。
- “当前 Run 返回就可以短暂发布 idle，再开始 Follow-up”不成立。这个 gap 会让
  `Wait`/`Close` 看到虚假 quiescence，也会与并发 enqueue 形成丢消息竞态。

## 冻结 Pi 源码与测试路径

- [`packages/agent/src/types.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/src/types.ts#L43-L49)：`all` 与 `one-at-a-time` drain 语义。
- [`packages/agent/src/types.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/src/types.ts#L224-L248)：Steering 与 Follow-up 的不同 poll boundary。
- [`packages/agent/src/agent-loop.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/src/agent-loop.ts#L155-L275)：inner tool/steering loop、outer Follow-up loop 与最终 `agent_end`。
- [`packages/agent/src/agent.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/src/agent.ts#L123-L150)：FIFO `PendingMessageQueue` 与 drain mode。
- [`packages/agent/src/agent.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/agent/src/agent.ts#L264-L301)：两个 queue 的 API、clear 与 pending 判断。
- [`packages/coding-agent/src/core/agent-session.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/agent-session.ts#L1023-L1065)：queued continuation 与一次 session-level settlement。
- [`packages/coding-agent/src/core/agent-session.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/core/agent-session.ts#L1286-L1359)：Follow-up admission、UI queue projection 与 message construction。
- [`packages/coding-agent/test/suite/agent-session-queue.test.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/test/suite/agent-session-queue.test.ts#L125-L259)：当前工作结束后消费、FIFO one-at-a-time 与 batch mode。
- [`packages/coding-agent/test/suite/regressions/6363-agent-settled-event.test.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/test/suite/regressions/6363-agent-settled-event.test.ts#L64-L90)：`agent_end` handler 新增 Follow-up 后仍只发布一次 `agent_settled`。
- [`packages/coding-agent/src/modes/interactive/interactive-mode.ts`](https://github.com/badlogic/pi-mono/blob/dcfe36c79702ec240b146c45f167ab75ecddd205/packages/coding-agent/src/modes/interactive/interactive-mode.ts#L2541-L2549)：Esc 由 host 先恢复/清理 queues，再请求 Agent abort；这说明 queue cleanup 与取消仍需在后续教学中单独决定，不能从 core abort 自动推导。

## 开课时 Pia 路径

- `internal/coding/session.go`：`Session.active` 当前只表示一个公开
  `Advance`；`beginAdvance` 对 busy 调用 fail fast，`finishAdvance` 随后直接
  清空 active 并可能触发 close。本课需要在不增加第二个 lifecycle owner 的前提
  下加入 pending/quiescence。
- `internal/coding/session.go` 的 `executeAdvance`：一个 Advance 可以因 overflow
  recovery 顺序调用 input-started `Engine.Run` 和 input-free
  `Engine.Continue`，并在每次 execution settlement 后提交 delta。这已经证明
  Session 可以协调多个 Engine Runs，但 Follow-up 将首次引入第二条 user input。
- `internal/agent/loop.go`：Engine Run 在 terminal assistant 没有 tool calls 时
  立即返回；它不读取 Session state。这是应保留的 run-local boundary。
- `internal/coding/observation_test.go`：当前 `advance_settled` 发生在所有嵌套
  Run、compaction、History snapshot 与同步 observer delivery 之后。Follow-up
  不能让它在 queue 排空前出现。
- `internal/coding/session_test.go`：当前 `Wait` 只等待 `active.done`，`Cancel`
  只取消该 active execution context，`Close` 永久停止 admission 后等待
  settlement 和 Workspace cleanup。队列会迫使这些 tests 从“一个 Run/输入”
  提升到真正 quiescence。
- `cmd/pia/main.go`：仍只调用一次 `Advance`；本课不增加 terminal input，后续
  最小交互终端才成为真实 Follow-up producer。

## 第一组教学：Run、Follow-up 与 quiescence

先用一个具体序列区分三个边界：

```text
Advance("修复 bug")
  -> Run(input="修复 bug")
       -> assistant(tool call)
       -> tool result
       -> assistant("已经修复")       # Run 1 settled
  -> commit Run 1 delta
  -> dequeue Follow-up("再加回归测试")
  -> Run(input="再加回归测试")
       -> assistant(tool call)
       -> tool result
       -> assistant("测试已补")         # Run 2 settled
  -> commit Run 2 delta
  -> queue empty
  -> Advance settled
  -> Session quiescent
```

Follow-up 与 Steering 的核心差异不只是“早一点/晚一点”：

| 输入 | 何时允许进入模型上下文 | 是否属于当前 Run | 是否等待当前目标先完成 |
|---|---|---|---|
| 普通 idle input | 新 Advance acceptance | 新 input-started Run | 不适用 |
| Steering | 当前 tool work settlement 后 | 同一 ongoing execution 的后续 Turn/Run 语义留到 Lesson 15 校准 | 否 |
| Follow-up | 当前 Run 原本将停止且 delta 已安全提交后 | 新 input-started Run 候选 | 是 |

“Quiescent”不能只实现成一个新布尔字段。它是从 authoritative state 推导的
事实：没有 active execution，也没有已接受但未消费的 Follow-up，并且最后一次
commit/observation 已完成。若用独立 `idle` 字段缓存它，就必须维护额外状态同步；
当前更小的方向是让 Session 的 active lifecycle 一直覆盖 queue drain。

## 已确认的第一项边界

学习者确认源码校准后的方向：

1. 一个公开 `Advance(initialInput)` 建立唯一 active guard；
2. active 期间，专门的 Follow-up admission 把非空输入放入 Session-owned FIFO；
3. 当前 Engine Run settlement 并提交 delta 后，同一个 Advance 逐条启动新的
   input-started Engine Runs；
4. `Advance` 返回的 `History` 和 `FinalText` 覆盖队列排空后的最终状态；
5. `Wait` 等待同一 quiescence，期间不出现 idle gap；
6. Engine 不保存 queue，不出现后台 scheduler，也不把普通并发 `Advance`
   静默改成 Follow-up。

这个方向已修正“一个 User Advance 恰好只接受一条 user input”的旧表述：
User Advance 是“由显式初始输入启动、可在安全边界消费已接受 Follow-up、
最终驱动 Session 到 quiescence 的短期操作”。`CONCEPTS.md` 与 D102 记录该
确认结果。

## 第二组教学：admission、ownership transfer 与封口

`FollowUp` 不是另一种 execution API。它只回答一个问题：**当前 Session
是否接过这条输入，并承诺让 active Advance 对它负责？**

```text
caller owns input
  -> FollowUp(input)
       -> rejected: caller 仍拥有 input，可以保留或稍后作为 Advance 重试
       -> accepted: Session 拥有 pending input，caller 不再重复提交
            -> dequeue into Engine.Run
            -> become Conversation user Message
```

学习者确认首版使用：

```go
func (s *Session) FollowUp(input string) error
```

它是非阻塞 acknowledgement，不接收 context、不等待执行完成、不返回模型答案，
也不为每条输入创建 result handle。返回 `nil` 表示 ownership 已转移，不表示
输入已进入 History；空白输入在 mutation 前拒绝，idle 或当前 Advance 已关闭
admission window 时返回 `ErrFollowUpUnavailable`，closing/closed 返回
`ErrSessionClosed`。普通并发 `Advance` 仍返回 `ErrSessionBusy`。

这要求 queue-empty 边界不能写成无锁的“看一眼空队列，然后稍后结束”：

```text
错误：
  Advance sees empty
  FollowUp appends and returns nil
  Advance settles
  pending input orphaned

正确：
  Session mutex
    -> queue non-empty: dequeue oldest, keep admission open
    -> queue empty: seal this Advance's admission window
  unlock
  -> final snapshot / synchronous observations / settlement
  -> clear active
```

这个 `seal` 只是 active Advance 的私有 admission phase，不是公开 lifecycle
state，也不是第二个 owner。它让 `FollowUp` 的结果具有线性化含义：抢在封口前
取得锁的输入一定被当前 Advance 接收；封口后的输入一定明确失败。Session 的
active guard 仍一直保持到最终 snapshot 和 observer delivery 完成，避免
`Wait`/`Close` 提前观察到 idle。

该确认结果记录在 D103。

## 第三组教学：异常 settlement 时不能自动继续，也不能静默丢弃

三份本地源码在具体机制上不同，但共同否定了“当前工作失败或被取消后，仍照常
自动运行 Follow-up”：

- 冻结 Pi 的 Agent Loop 在 terminal assistant 为 `error` 或 `aborted` 时直接
  发布 `agent_end` 并返回，早于 outer Follow-up poll。Core `abort()` 只请求并
  等待 active work 停止，并不自动消费或清空 queue；interactive host 的 Esc
  路径先调用 `clearQueue()`，把 steering/follow-up 文本恢复到 editor，再 abort。
- 当前 OpenCode 把 admitted input 持久化。`session-runner.test.ts` 中
  “preserves durable queued input for a later wake after interruption” 明确断言：
  interrupt 后 queued row 仍 pending，后来 `resume` 才提升并执行。这个结果依赖
  durable input store 和显式 resume，不是 Pia 当前内存 Session 已有的能力。
- 当前 Codex CLI 的 Follow-up queue 由 TUI 持有。`input_restore.rs` 在 user
  interrupt、budget exhaustion 或 review completion 后 drain pending/queued
  input 并合并回 composer，不自动开始下一 Turn；对应 tests 也要求 budget
  stop 后只恢复文本、不提交新 operation。

因此“不要自动继续”与“怎样保存”是两项不同决定：

| 当前 Run 结果 | 是否继续取下一条 Follow-up | 未消费输入需要什么归宿 |
|---|---|---|
| 正常成功，包括 overflow recovery 成功 | 是 | 继续由当前 Advance FIFO 消费 |
| Provider/tool fatal error 或 overflow recovery 失败 | 否 | 必须显式交还或持久保存 |
| caller cancellation、`Cancel`、`Close` | 否 | 必须显式交还或持久保存 |

Pia 本课没有 terminal editor、durable input store、resume scheduler 或 per-input
handle，因此已确认的最小方向不是照抄三者的 storage mechanism，而是让
`AdvanceResult` 增加 FIFO `UnconsumedFollowUps []string`。异常 settlement 时，
Session seal admission、原子 detach 尚未成为 Conversation Message 的输入，并把
它们交还给 Advance caller；未来 terminal 可在 Esc 后恢复到 editor，`/exit`
则可明确忽略这份 hand-back。已经进入 Engine Run、因 Provider/tool error 而写入
History 的那条 Follow-up 已经被消费，不会重复出现在这个列表中。

学习者选择了“异常即停止并交还”：正常 Run（包括成功 overflow recovery）继续
drain；Provider/tool fatal error、overflow recovery 失败、caller cancellation、
`Cancel` 或 `Close` 都停止取新输入。若已 dequeue 的输入在 pre-Run compaction、
Working Context derivation 或 pre-canceled Engine acceptance 前失败，它尚未成为
Message，必须作为交还列表第一项。该确认结果记录在 D104。

## 第四组教学：当前事件足以表达执行，queue preview 等真实 consumer

现有 event ordering 已能表达 Follow-up 的 execution 事实：

```text
Advance started
  Run(input) started       # initial input
  Message(user)
  ...
  Run settled
  Run(input) started       # consumed Follow-up
  Message(user)
  ...
  Run settled
Advance settled            # once, after drain
```

异常路径同样由已有 facts 和 result 共同表达：

```text
Run settled(error)
Advance settled(error)
AdvanceResult.UnconsumedFollowUps = [...]
```

`FollowUp()` 的 caller 已通过同步返回值知道 admission 成功或失败；消费时已有
`Run(mode=input)` 与 user `Message` event；未消费 hand-back 属于
`AdvanceResult`。当前真实 line observer 不显示 pending queue，one-shot CLI
也没有 Follow-up producer。此时新增 `FollowUpAdmitted/Consumed/Returned`
event 会先迫使本课设计 input ID、queue count 或正文投影，却没有 consumer
证明哪些字段必要。

因此当前推荐是本课不增加 queue event family，只测试既有 nested Run 与单次
Advance settlement 的顺序。未来最小交互终端需要 pending preview 时，再根据
真实 UI 决定使用 bounded queue events 还是另一种只读 projection；Session
仍保持唯一 queue owner，terminal 不应凭猜测维护第二份 authoritative queue。

学习者确认本课不新增 queue Semantic Event；该结果记录在 D105。

## 实现结果

D102–D105 已落实为下面的代码边界：

- `internal/coding/session.go` 让每个 `activeAdvance` 持有 admission window 与
  Session mutex 保护的 FIFO pending inputs；`FollowUp(input) error` 只做同步、
  非阻塞 admission，不调用 Engine，也不创建第二个 lifecycle owner；
- `Advance` 在一个外层循环中先完成并提交当前 input 的 Run，再通过同一 mutex
  执行 dequeue-or-seal。队列非空时启动下一条 input-started `Engine.Run`；队列
  为空时先封住 admission，再做最终 History snapshot、Advance observation 与
  active guard settlement；
- `executeInput` 保留原有 threshold compaction、overflow compact-and-continue
  与 run-local delta commit 语义，并用 Engine 返回的非空 delta 判断该输入是否
  已越过 Message acceptance boundary；
- error 或 cancellation 路径原子封口并 detach pending queue。已被 Engine 接受
  的 Follow-up 只保留在 History；尚未被接受的当前输入排在
  `UnconsumedFollowUps` 首位，其后保持原 FIFO 顺序；
- 现有 `Run(mode=input)`、user `Message` 和单次 outer `Advance` settlement
  直接表达实际执行，本课没有新增 queue event、terminal producer、持久化、
  scheduler、per-input handle、Steering 或任意 queue capacity 配置。

确定性 tests 覆盖：

- 多条 Follow-up 的 FIFO consumption、累积 Provider context 与最终
  `FinalText`；
- 相邻 Runs 之间 `Wait` 仍阻塞、普通 `Advance` 仍 busy，且最终 observation
  之前 admission 已封口；
- idle、blank、sealed、closing 与 closed admission；
- Provider failure、pre-Run compaction failure、caller cancellation、
  `Cancel`、`Close` 与“当前 Run 忽略取消但成功返回”的 hand-back；
- Follow-up overflow recovery 后继续 drain；
- final seal 与 concurrent `FollowUp` 的竞态：输入只能被完整消费或明确拒绝，
  不能 orphan。

测试先行时，新测试首先因缺少 `FollowUp`、`ErrFollowUpUnavailable` 与
`UnconsumedFollowUps` 而编译失败；实现后 focused tests、`make check` 与
`go test -race ./...` 全部通过。最终主线程按 correctness、standards、testing、
maintainability、agent-native、reliability 与 adversarial lenses 复审，没有发现
需要修改的 finding。首版内存队列没有任意容量限制或 durable backpressure；
当前没有 terminal/Gateway producer 或来源证据支持一个阈值，因此这项能力继续
等待真实 consumer，而不是在本课猜测配置。

学习者已确认关键语义并明确要求提交和推送到 `origin/main`。Lesson 14 至此
结束；不会自动开始 Steering 课程。
