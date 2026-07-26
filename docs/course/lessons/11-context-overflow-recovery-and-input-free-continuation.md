# 第 11 课：Context overflow 恢复与无输入 continuation

## 当前状态

本课于 2026-07-22 由学习者明确开始。2026-07-26 已按校准后的边界完成 Go 实现、主线程多视角审查、`make check` 与 `go test -race ./...`，并由学习者确认理解后提交到 `origin/main`。Lesson 11 与第二阶段至此完成；尚未开始 Lesson 12。

2026-07-24 的第二次源码校准纠正了首轮设计：正常 pre-generation context overflow 只形成不含 completed tool calls 的 error assistant；`E2 + N2` 是 Lesson 03 的通用失败结算形状，不再作为本课自动 recovery 的主路径或验收要求。该修正记录为 D74。

2026-07-26 的 guard 边界讨论进一步确认：现有 Conversation/Core guard 只防止同一实例被并发推进，不是同一台机器的全局 Session 锁。未来正式引入 Session owner 时，必须重新审查三者职责，并优先让 Session 吸收外层 user-advance lifecycle，而不是继续叠加同义的 `active`/`busy` 状态。该约束记录为 D75。

同日后续讨论确认了两项实现前边界：recovery projection 采用 append-only History 加显式 absolute exclusion positions，所有 model-view consumers 共享同一过滤结果；Lesson 11 的“一次”只限制每个 accepted user advance 内隐藏的自动 compact-and-continue，未来交互式 Session 可在失败 settled 后接受用户显式发起的新 compact/retry control operation。两项决定分别记录为 D76、D77。

Forced compaction 普通失败的状态语义随后也已确认：第一次 Core Run 的 History delta 已经提交，不能因恢复失败回滚；但 exclusion、summary、cut 与新 Working Context 作为一个 recovery model-view candidate，只在全部生成、验证和 replacement 成功后共同发布。失败时不保留半成品 model view，也不调用 continuation。该决定记录为 D78。

Cancellation 边界也已确认：summary 与 candidate 阶段取消时丢弃未发布 candidate；最后一次 cancellation check 通过后，Working Context replacement 与 projection publish 是短小的同步 commit section，进入后不因晚到取消回滚。随后尚未接受的 `Continue` 可以按预取消拒绝，已经接受的 `Continue` 则复用 Core Agent 既有取消结算。该决定记录为 D79。

与 Pi、Codex 和 OpenCode 的持久化对照随后确认：compact 的内部 request/response 不进入普通 Conversation History，但未来成功或失败的已结算 attempt 应作为独立 typed record 进入第三阶段的 versioned Session journal。首版每个 settled attempt 只写一条包含时间、trigger、outcome 和必要 projection/error facts 的记录，不预写 durable Started record；该边界记录为 D80，Lesson 11 不实现它。

实现前最后两个边界也已确认并落实：首版 overflow classifier 暂时放在 `internal/coding` private policy，并以代码 `TODO` 明确稳定结构化 error code、第二个 Provider 或 generic retry consumer 出现时必须重新评估归属；Core Run 则统一表示一次 accepted Agent Loop execution，`Run` 追加输入而 `Continue` 不追加。两项决定分别记录为 D81、D82。

Lesson 11 是第二阶段的收尾课。它只完成一条闭环：一个已经接受 user input 的 coding 推进如果因明确的 context overflow 失败，Conversation Owner 先保存这次失败的完整事实，再压缩可继续使用的模型上下文，并让 Core Agent 在不追加第二条 user message 的前提下继续一次。完成实现、验证和学习者理解确认后，课程进入第三阶段的 Session Runtime 系列。

本课规模为 **Large**。Overflow 分类、recovery eligibility、强制 compaction 与无输入 continuation 不能拆开后仍独立验收；但 generic Provider retry、事件、steering/follow-up、Session 持久化和 Orchestration 都有独立 owner 与结束信号，不属于本课。

## 解锁能力

当前 Pia 只能在 Run N 已 settled、Run N+1 尚未把新 input 交给 Core Agent 时做 threshold compaction。如果同一个 Core Run 在执行多个 tool turns 后，下一次 Provider request 才发现真实 context overflow，当前路径会把 error assistant 保存下来，然后直接返回错误。

本课完成后，Pia 应能处理下面的恢复序列：

```text
accepted user input
  -> Core Run executes zero or more successful Provider/tool turns
  -> Provider returns an explicit context-overflow error
  -> Core Run settles and returns its complete message delta + error
  -> Conversation History commits that failed delta unchanged
  -> retry Working Context omits the eligible overflow error assistant
  -> coding-owned forced compaction summarizes older usable context
  -> Core Agent continues without appending another user message
  -> continuation delta is committed to the same complete History
  -> return only after success or the bounded recovery attempt settles
```

这里的“恢复”不表示删除失败。完整 Conversation History 仍能回答“Provider 曾经如何 overflow”；只是在重试使用的 Working Context 中不再把那次失败当作模型应继续响应的对话内容。

## 学习目标与前置知识

前置课程是 Lesson 03 的 tool-call settlement、Lesson 07 的 Conversation History/Working Context 所有权、Lesson 08 的 compaction projection，以及 Lesson 10 暴露出的 mid-Run aggregate context 缺口。本课不重新讲授这些完整实现，只在 recovery 路径上追踪它们如何组合。

完成本课后，学习者应能：

- 区分 input-started Run、input-free continuation 与空字符串 user message；
- 区分正常的 pre-generation overflow error 与 Lesson 03 已处理的 completed-call failure settlement；
- 解释为什么 History 保存失败而 retry Working Context 必须排除失败，以及 projection 如何保证以后不重新引入；
- 把 Provider failure evidence、overflow classification、retry policy 和 recovery budget 分成不同责任；
- 推导 forced compaction、continuation、cancellation、History commit 与 concurrent guard 的先后顺序；
- 用确定性测试证明“只继续一次、user 不重复、tool 不重放、失败事实不丢失”。

## 开课源码校准

固定基线：Pi commit `dcfe36c79702ec240b146c45f167ab75ecddd205`，Pi Agent package version `0.80.7`。

### 已确认的大纲假设

- 冻结 Pi 把 overflow 与普通 retry 分开：context overflow 走 compaction，rate limit、server error 和 transport failure 才走 generic retry policy。
- Overflow recovery 需要 input-free continuation。Pi 的 `runAgentLoopContinue()` 不追加 prompt，并明确拒绝空 context 或以 assistant 结尾的 context。
- Coding 层拥有恢复策略。Pi 的 core `Agent` 只提供 `continue()`；`AgentSession` 负责识别 overflow、限制恢复次数、执行 compaction、替换 agent context 并决定是否再次 continue。
- 错误 assistant 已经作为历史事实保存，但重试前会从 active agent state 中移除。Threshold compaction 则不会自动 continue。
- 冻结 Pi 用 `_overflowRecoveryAttempted` 阻止未取得中间进展的连续 overflow recovery：新 user message 或任何 non-error assistant 会重置该 flag，因此它不是严格的“每个 user prompt 一次”。Lesson 11 选择更严格的每个 accepted user advance 最多一次自动 compact-and-continue，这是 Pia 当前 one-shot 阶段的简化，不宣称为 Pi 的完全相同行为。

### 细化后的认识

- Pi 的 `isContextOverflow()` 是多 Provider 产品长期积累出的启发式集合：它包含大量 provider-specific error text、negative patterns、usage 超窗和 `length + zero output` 等分支。Pia 当前只有 DeepSeek 产品路径，不能把整张 regex 表复制成没有证据的“兼容性”。
- DeepSeek 官方 [Error Codes](https://api-docs.deepseek.com/quick_start/error_codes/) 只把 400 描述为 request format error、422 描述为 invalid parameters，并要求参考 error message；[Chat Completions](https://api-docs.deepseek.com/api/create-chat-completion/) 只说明 input 加 generated tokens 受 context length 限制。官方没有公开一个稳定的 context-overflow 专用 HTTP status、error code 或 message。因此仅凭 400/422 不能启动 recovery。
- DeepSeek 的 `finish_reason=length` 同时可能表示 request 的 `max_tokens` 用尽或 conversation 超过 context length。自动丢弃一次成功或部分成功的 length assistant 并重试，可能造成重复工作；没有新的真实证据前，本课只处理显式 `stopReason=error` 的 overflow。
- 当前 OpenAI-compatible Provider 在请求建立、HTTP status error 或 finish reason 之前失败时，形成不带 completed tool calls 的 error assistant；这正是正常 pre-generation context overflow 的形状。只有已经收到 finish reason 后的失败才可能保留完整 calls，并由 Core Agent 追加同 ID 的 not-executed results。后者是 Lesson 03 的通用失败结算，不是已观察到的 overflow 常态。
- Lesson 11 因此只自动恢复不含 completed tool calls 的明确 overflow error。如果 terminal 同时带 completed calls，即使错误文本命中 overflow classifier，本课也保留完整失败并返回错误，不猜测性 compact-and-continue。
- 当前 `compactionProjection` 用 `summary + History[FirstKept:]` 重建 Working Context。只临时修改 Core Agent state 不够：下一次从完整 History 重建时，会把已保存的 overflow failure 重新投回模型。恢复排除必须成为 coding-owned projection 的显式契约。
- Pi 的持久 Session entry tree 与 Pia 的内存 absolute History index 不是同一种结构。Pi 先从 agent state 移除 error，append compaction entry 后从 Session 重建 context，再防御性移除尾部 error；Pia 不能机械复制两次 `slice(0, -1)`。

### 被推翻的隐含假设

- “Lesson 11 只需在 `conversation.run()` 看到 overflow 后再调用一次 `core.Run(ctx, "")`”不成立。空字符串仍是一条 user input，会改变 transcript、prompt 语义和 token 预算。
- “完整 History 既然要保留错误，Working Context 也必须保留同一错误”不成立。Lesson 07 已经把两种 owner 分开；compaction 和 recovery 正是允许模型投影有损、历史事实无损的原因。
- “只要 error text 看起来像 overflow 就应自动恢复”不成立。带 completed tool calls 的 terminal 已进入另一项既有结算契约；当前没有证据证明它仍是安全的 pre-generation overflow，因此本课不恢复这种相交形状。
- “HTTP 400 就是 overflow”不成立。DeepSeek 官方把 400 用于一般 request format error；误判会对无效请求反复 compaction，并掩盖真正故障。
- “有了 overflow recovery 就完成了 active-Run compaction”不成立。本课只在一次 Core Run 已因 overflow settled 后反应式恢复，不会在正常相邻 Provider turns 之间主动执行 quality-threshold compaction。

## 冻结 Pi 源码与测试路径

- `packages/agent/src/agent-loop.ts`：`runAgentLoopContinue()` 的无 prompt 启动、空 context 检查和 assistant-tail 拒绝。
- `packages/agent/src/agent.ts`：`Agent.continue()` 的 active-run guard、queued-message 分支与 input-free continuation 调用。
- `packages/coding-agent/src/core/agent-session.ts`：`_runAgentPrompt()`、`_handlePostAgentRun()`、`_checkCompaction()` 和 `_runAutoCompaction()` 共同组成 retry/overflow/compaction/continue 协调链。
- `packages/ai/src/utils/overflow.ts`：显式 error、silent overflow 和 length-stop heuristic，以及避免把 rate limit 误判为 overflow 的 negative patterns。
- `packages/ai/src/utils/retry.ts`：只做 retryability classification，并明确要求调用方先单独处理 overflow。
- `packages/coding-agent/src/core/session-manager.ts`：完整 Session entries 与最新 compaction-aware model context 的重建规则。
- `packages/agent/test/e2e.test.ts`：`continue()` 的空 context、assistant tail、user/tool-result tail 与 queued-message 行为。
- `packages/coding-agent/test/suite/agent-session-compaction.test.ts`：一次 overflow recovery 上限，以及成功 response 超窗时只 compact、不 retry。
- `packages/coding-agent/test/agent-session-auto-compaction-queue.test.ts`：overflow、compaction 和 queued messages 的交互边界。
- `packages/ai/test/overflow.test.ts`：positive、negative、silent 和 length-stop 分类样例。
- `packages/coding-agent/test/suite/regressions/pre-prompt-compaction-no-continue.test.ts`：新 prompt 前的 compaction 不能误用 continuation。

## 当前 Pia 路径

- `internal/agent/loop.go`：`Run(ctx, userInput)` 总会在 acceptance point 追加一条 user message；Provider/tool loop 本身已经可从现有 Working Context 继续执行。
- `internal/agent/context.go`：Core Agent 独占 Working Context，`ReplaceWorkingContext` 只允许在 Core Agent idle 时深复制替换。
- `internal/agent/tool_loop_test.go`：锁定 error/aborted assistant 中 completed tool calls 不执行，并追加同 ID not-executed results 的现有通用契约；Lesson 11 不改写这项行为。
- `internal/ai/message.go`：`AssistantMessage` 当前只有 content、usage、stop reason 与 bounded error message，没有 machine-readable failure kind。
- `internal/ai/provider/openaicompatible/provider.go`：HTTP error body 最多读取 64 KiB、加入 status context、做 credential redaction 后形成 terminal error message。
- `internal/coding/conversation.go`：Conversation guard 当前覆盖 pre-Run compaction、一次 Core Run、一次 History commit 和返回 snapshot；`commitRun()` 同时释放 guard，尚不能在同一 accepted user advance 中协调两个 Core executions。
- `internal/coding/compaction.go`：threshold compaction 假设存在一条尚未接受的新 user input，并按 `summary + History[FirstKept:]` 重建投影；尚无 overflow error assistant 排除语义。
- `internal/coding/runtime.go`：仍是固定 DeepSeek profile 的 one-shot composition；Lesson 11 不改变 CLI 参数、final-only stdout 或公共接入契约。

## 本课责任边界

本课的一个闭环是“**显式 context-overflow 后，保存失败事实并安全继续一次**”。四个组成部分共同服务这一个能力：

1. **识别：** 只把有充分证据的 error terminal 分类为 context overflow。
2. **结算：** 先让失败 Core Run 完整 settled，把它的 message delta 提交到 Conversation History。
3. **投影：** 从 retry Working Context 排除 eligible overflow error assistant，对其余可用 context 做 forced compaction，并保持后续可重复重建。
4. **继续：** 让 Core Agent 从 user/tool-result tail 启动新 execution，不添加第二条 user message，并最多尝试一次 recovery。

Core Agent 不决定“这个错误值得 compact-and-retry 吗”；Provider 不决定 Conversation retry policy；Conversation History 也不因恢复而删除事实。Coding-owned Conversation Owner 是同时看得到 complete History、compaction projection、Core settlement 和产品 policy 的最窄 owner。

## 建议的 lifecycle 模型

下面是源码校准后的推荐方向，仍需在实现前逐项讲解并由学习者确认：

```text
Conversation accepts one user advance and holds its active guard
  |
  +-- optional existing pre-Run threshold compaction
  |
  +-- Core Run(user input)
        |
        +-- success ------------------------------+
        |                                         |
        +-- non-overflow / cancellation ----------+--> commit delta -> snapshot -> release guard
        |
        +-- explicit overflow
              -> commit failed delta, keep guard
              -> verify the terminal has no completed tool calls
              -> build usable model projection without that error assistant
              -> force compaction without a new input
              -> atomically publish Working Context + projection metadata
              -> Core Continue(no input)
                    -> commit continuation delta
                    -> success or any failure settles the outer advance
                    -> no second overflow recovery
```

Conversation guard 应覆盖第一次 Core settlement、第一次 History commit、overflow classification、summary call、projection commit、continuation settlement、第二次 History commit 和最终 History snapshot。Core Agent guard 仍只覆盖各自的 `Run` 或 `Continue`；两次 Core execution 之间 Core Agent 必须 idle，才能合法替换 Working Context。

这不是提前引入 Session 的 busy/idle/wait/close API。它只是让现有私有 Conversation owner 在一次 accepted user advance 内协调一个有界恢复事务；第三阶段再把长期 lifecycle 提升为正式 Session Runtime。

### 已确认的 guard 边界与未来 Session 交接

当前 guard 解决的是两个调用者同时推进同一个逻辑 Conversation/Core 实例，而不是多个独立 Session 在同一台机器上运行。严格串行使用当前 one-shot CLI 时，这项保护通常不会被触发；未来 Gateway、IM、UI 或其他 Go consumer 向同一实例重复提交时，它才形成可观察的 fail-fast 边界。

不同 Session 若各自拥有独立的 Conversation、Core Agent 和可变状态，应当允许并发运行。它们若共享同一个 workspace，可能产生文件和外部工具副作用冲突；那属于未来 workspace/worktree 或 Orchestrator 协调，不由 Conversation/Core guard 解决。

第三阶段进入单 Session lifecycle 课程时，必须重新追踪当时的调用路径和封装边界：

- 如果 Session 独占 Conversation 与 Core Agent，且所有 user advance 都只能经过 Session，那么 Session 应成为唯一外层 lifecycle authority，并评估吸收现有 Conversation `active` 职责。
- Conversation 默认继续负责 complete History 及其 projection，不应再复制 Session 的 busy、wait、cancel 或 close 状态。
- Core Agent 默认继续负责 Working Context 与一次模型/工具 execution；其本地 guard 是否仍需保留，取决于它届时是否仍是可独立使用的 package contract，而不能仅因已有 guard 就永久保留。
- 不允许 Session、Conversation 与 Core 同时维护语义重叠且可能分歧的 lifecycle 状态。确有必要保留的局部 invariant，必须能指出独立调用路径、失败语义或并发测试证据。

因此，Lesson 11 只延长现有 Conversation guard 以覆盖一次 recovery，不提前增加 Session 层。D75 记录的是第三阶段开课时必须执行的责任重审，不是已经确定的 Session API 或 package layout。

## Recovery eligibility，而不是所有 error terminal

Lesson 11 只把下面的 terminal 作为自动恢复来源：

```text
assistant.stopReason = error
error evidence        = explicit context overflow
completed tool calls  = none
```

典型序列是一个 Run 已完成 `read`，下一次 Provider request 因增长后的 context 被拒绝：

```text
History
  user task
  assistant -> read(call-1)
  tool result call-1 = source text       # 已执行，保留
  error assistant = context overflow     # 无 tool call，保留为历史事实
  recovered assistant = ...              # continuation 后追加

Retry Working Context
  summary / retained context
  user task
  assistant -> read(call-1)
  tool result call-1 = source text
  recovered assistant = ...              # continuation 后出现
```

这里不能再次执行 `read(call-1)`。Recovery 只移除 model view 中的 overflow error assistant；它不会回退或重放已经发生的工具副作用。

如果 Provider error terminal 含 completed tool calls，Core Agent 仍按 D33 追加 same-ID not-executed results，使完整 History 保持 protocol-valid。但这种形状不满足本课 recovery eligibility：Conversation 不删除其中任何消息、不调用 summarizer 或 `Continue`，而是返回原失败。这样把“完整结算未知失败”与“有证据的 input-overflow 恢复”保持为两个独立能力。

## Complete History 与 recovery projection

当前 projection 只有一个 absolute `FirstKept` boundary，无法表达“History 中这条 overflow error assistant 存在，但 recovery context 必须跳过”。本课应把 projection 从单一 suffix 规则扩展成可确定重建的 model-view contract。

开课时比较了三个候选；学习者确认采用第三种：

| 候选 | 优点 | 问题 | 当前判断 |
|---|---|---|---|
| 只在 Core Agent state 临时删尾部 | 改动最小 | `projectedMessages()` 会从 History 重新引入失败；无法跨后续 compaction 重建 | 淘汰 |
| 保存一份完整 Working Context snapshot 加 History cursor | retry 当下直观 | 容易复制另一份长期权威 message state；重复 compaction 仍需恢复 raw History mapping | 不优先 |
| append-only History 加显式排除位置/边界 | History 仍是唯一完整事实源；能确定重建 summary、retained raw suffix 和后来消息 | compaction planning 必须在过滤后的 model source 上工作，并维护 absolute History mapping | 采用 |

推荐方向的关键不变量不是某个 Go field 名，而是：

1. Projection 能精确省略一个已经验证的 overflow error assistant，而不是按文本或“最后一条”临时猜测。
2. `projectedMessages()`、token estimation、compaction summary input、retained suffix 和 request snapshot 都消费同一个过滤后的 model source。
3. Complete History 的 absolute index 仍可追踪 cut 与新消息；后来 continuation delta 自动出现在被排除位置之后。
4. 重复 compaction 永远不会把已排除 overflow assistant 重新写进 summary；当新 cut 已越过某个旧排除位置后，可以丢弃那段 projection metadata，而不是无限累积。
5. Projection replacement、Core Working Context replacement 与 cancellation commit point 保持 Lesson 08 的原子语义。

这项变更必须先用两次 overflow 分属不同 accepted user advances、且中间发生普通 threshold compaction 的测试证明。只验证一次 tail deletion 不足以证明 projection 可持续使用。

学习者确认的 representation 只固定这些可观察不变量，不预先锁定 Go field 名或 collection 类型。首版预计 exclusions 很少，但实现仍必须以 absolute History position 精确定位，不能按 error 文本、message equality 或“当前最后一条”在重建时重新猜测。D76 记录该边界。

## 无输入 continuation 契约

Core Agent 需要一个显式 continuation entrypoint，不能用空字符串、特殊 sentinel user message 或 coding prompt 拼接模拟。推荐契约为：

- Core Agent idle、context 未预取消且 Working Context 非空时才可接受 continuation；
- Working Context 最后一条必须是 user 或 tool result，不能是 assistant；
- 整个 Working Context 必须保持 assistant tool-call 与 tool-result pairing 合法；
- continuation 不追加 user message，第一次 Provider request 直接使用当前 Working Context snapshot；
- 返回的 ownership-independent `NewMessages` 只包含本次 continuation 新产生的 assistant/tool-result messages；
- Provider error、tool error、cancellation、source ordering、parallel-safe stages 和 terminal settlement 继续复用现有 Agent Loop 契约；
- `Run` 与 `Continue` 共享一个 active guard，不允许两者并发，也不允许 active 时替换 Working Context。

这会修正当前 `CONCEPTS.md` 中“Run 一定由新 user input 启动”的边界。推荐把 Core Run 解释为一次 accepted Agent Loop execution：它可以由新 input 或显式 continuation 启动；Run Message Delta 只有在 input-started Run 中包含 initiating input。一次 private coding user advance 可能在 recovery 时协调两个先后 settled 的 Core Runs。术语只有在课堂确认并实现后才更新，当前不提前改 glossary。

## Overflow 分类的首版边界

### 已有证据允许的部分

- 只检查 `stopReason=error` 的 terminal assistant；aborted、stop、toolUse 和 length 不进入本课 recovery。
- 以 Provider 已经生成的 bounded、credential-redacted error evidence 为输入，不解析日志、stderr 或 trace。
- 使用窄的、可测试的 context-overflow error code/message 证据；HTTP 400/413/422 本身不是充分条件。
- 明确排除 rate limit、too many requests、server overload 和一般 invalid-parameter 文本。
- Classification 只回答“是否进入 overflow path”，不包含重试次数、backoff、summary 或 continuation policy。

### 已确认的实现形状

实现前比较了两个合理方案：

1. 在 coding 层使用 private predicate，从 terminal assistant 的 bounded error text 中识别首版 DeepSeek/OpenAI-compatible patterns。
2. 在 Provider terminal 中增加最小 machine-readable failure category，由 wire/profile 层保留原始分类证据，coding 层只消费 category。

D81 确认首版采用第一种。理由是当前只有一个 consumer、一条 Provider 产品路径，官方也没有承诺 overflow code；为一个不稳定文本启发式扩张所有 `ai.AssistantMessage`、clone、trace 和 Provider fixture 的公共内部协议，可能制造虚假的强类型保证。实现中的英文 `TODO` 必须明确这只是当前阶段的归属；未来出现稳定 structured code、generic retry 或第二个 Provider 后，再凭真实 consumers 重新评估共享 failure classification。

首版 matcher 不能直接复制冻结 Pi 的全部多 Provider patterns。若真实 DeepSeek trace 与候选 pattern 不一致，应先记录 sanitized evidence，再修正 matcher 和课程决定；不能把所有 400 或所有包含 “token” 的错误放宽成 overflow。

## 自动 Recovery 次数与未来显式重试

Lesson 11 按一次 accepted user advance 维护局部**自动** recovery budget：

- 第一次明确 overflow：允许一次 forced compaction 加一次 continuation；
- continuation 再次 overflow：提交第二次失败 delta，返回“overflow recovery exhausted”上下文与第二次 Provider error，不再进行第二次 compaction；
- continuation 遇到其他 Provider/tool failure：提交 delta并返回该失败，不转入 generic retry；
- summary call 自己失败或 overflow：不递归进入 overflow recovery；summary 不是普通 Core Run；
- 新的 user advance 可以重新拥有一次 recovery 机会，不能把前一次的 attempt flag 永久留在 Conversation。

这里的“一次”只约束当前调用内部没有用户确认的自动行为，不表示 Conversation 以后不能再次 compact。当前 one-shot CLI 没有可恢复 Session 或交互式 control surface，自动 recovery 失败后只返回错误并结束本次调用。

未来 TUI/Session Runtime 可以在失败已经 settled、失败阶段与原因已经展示后，让用户显式发起新的 compact/retry control operation。每次用户动作启动一个新的、有来源的控制尝试；它不伪装成 Conversation user message，也不靠偷偷重置当前自动 attempt flag 实现。是否提供换模型、减少 context、重试 summary 或开始新 Session 等选项，要在真实交互课程中参考当时产品证据重新设计，本课不提前定义 UI 或 Session API。

该边界比冻结 Pi 更严格：Pi 只阻止没有 non-error assistant progress 的连续 recovery chain；OpenCode 与当前 Codex 也允许成功 compaction 后重新进入循环，并把手动 compact 作为独立操作。Pia 当前选择每个 accepted user advance 最多一次自动 recovery，是为了在没有 TUI、Session lifecycle、generic retry 与 cost/turn budget 时先保证隐藏行为有界；D77 明确保留未来用户显式重试，而不是把首版限制误写成长期产品规则。

### Forced compaction 普通失败

Overflow recovery 有两个不同的 commit boundary，不能把“整个 user advance 全部回滚”误称为原子：

1. 第一次 Core Run settled 后，Conversation 先把原 user、已经完成的 Provider/tool turns 和 overflow error assistant 原样提交到 complete History。这些事实以及已经发生的工具副作用不回滚。
2. Conversation 从该 History 和旧 projection 构造过滤后的 summary source，再生成、验证 candidate summary、retained suffix、absolute exclusions 与 Working Context。只有 candidate 全部有效且 idle-only replacement 成功后，才同步发布新的 recovery projection。

因此，compaction planning、summary Provider、summary shape、candidate capacity/protocol validation 或 `ReplaceWorkingContext` 任一普通失败时：

- complete History 保留第一次 failed Run 的完整 delta，包括 overflow assistant；
- Core Working Context 保持 recovery 前状态，仍包含该 overflow assistant；
- 旧 projection 保持不变，不发布 exclusion、summary、cut 或 usage boundary；
- 本次局部生成的 summary 即使已有一部分成功，也直接丢弃；
- 不调用 Core `Continue`；
- 外层返回当前 complete History snapshot，以及带 `recover context overflow`/`compact context` operation context 并保留最终 compaction cause 的 error。

Summary request 不属于 Coding Conversation，不提供 coding tools，其 request、terminal 与 usage 不进入 complete History。已经发生的 Provider 请求、数据外发和计费不能回滚，但不会因为付出了成本就把未完成 candidate 发布成 Agent state。

冻结 Pi 在调用 auto-compaction 前已经从 mutable `agent.state.messages` 删除 overflow error，并在 compaction failure 后不恢复它；其完整 Session entries 仍保存错误。Pia 不复制这一没有显式 durable projection 的部分提交。D78 选择与 Lesson 08 candidate-then-commit 一致的 model-view 原子语义，使“返回失败”不会同时隐藏一份已经改变但尚不可安全继续的 Working Context。未来显式 retry 由 D77 的新控制操作从已提交 History 与失败事实重新构造 candidate。

### Cancellation 与 recovery model-view commit section

Lesson 11 的取消仍来自调用方传入的 Go `context.Context`；当前 CLI 的 `SIGINT`/`SIGTERM` 只是这个来源之一，本课不实现 TUI cancel command。

Candidate 构造阶段仍然可取消。Summary Provider、planning、validation 或最后一次 cancellation check 返回 `context.Cause(ctx)` 时，第一次 failed Core Run 已提交的 History 保留，未发布 candidate 丢弃，旧 Working Context/projection 不变，也不调用 `Continue`。

最后一次 cancellation check 通过后，idle-only Working Context replacement 与 projection metadata publish 构成短小、同步、没有异步等待的 commit section。进入该 section 后，即使取消同时到达，也完成两个 owner 的一致发布而不回滚。随后：

- `Continue` 尚未接受时看到预取消，只返回 context cause，不增加 continuation delta；已发布 projection 保留。
- `Continue` 已接受后发生取消，Core Agent 按既有 aborted terminal 和 tool-call settlement 契约收敛，Conversation 提交其完整 delta。

这里的 commit 是内存状态的线性化边界，不是 Git commit。实现和 race/cancellation tests 必须证明不会出现“Core 已换 Working Context，但 projection 仍旧”或反向的可观察中间状态。D79 记录该边界。

返回语义继续使用现有 `RunResult + error`：

- recovery 成功时，外层 coding 调用返回 nil error；初始 overflow 作为已处理历史事实保留在 transcript；
- forced compaction 普通失败时，返回带 recovery operation context 且保留 compaction cause 的 error；已提交 History 保留，但 recovery Working Context/projection candidate 不发布；
- cancellation 始终保留 `context.Cause(ctx)`，不能被“recovery failed”字符串替代；
- recovery model-view commit 前失败时，旧 Working Context/projection 不变；History 已提交的 overflow delta 不回滚；
- projection 已成功发布后、continuation 接受前才到达的 cancellation 不回滚合法 projection，History 仍保存 overflow delta；
- 不新增重复的 outcome enum、retry count field 或 terminal message，完整 History 加 idiomatic Go error 仍是事实边界。

### 与第三阶段 events / Session journal 的交接

Lesson 12 是第三阶段的第一课，不是对第二阶段 compaction 的返工。第二阶段先把 compaction 与 overflow recovery 的状态语义做正确；Lesson 12 再让真实 headless observer 在执行时看到 compaction started/settled 等 semantic lifecycle。Event 可以驱动未来 TUI 或 host 展示，但不承担 durable restore。

Session persistence 已确定属于第三阶段。D80 选择把 CompactionRecord 放在 versioned append-only Session journal，作为与 message entries 并列的 control/checkpoint record：

- Conversation History 只保存真实 user/assistant/tool facts；
- latest committed compaction record 保存恢复 Working Context 所需的 projection；
- failed/canceled records 提供有界、脱敏的时间、trigger 与 outcome 追溯，但不改变 model view；
- raw summary request/response、局部 candidate 和 HTTP payload 不默认持久化；
- trace 可以显示 journal record 或 live event，但不成为权威 owner。

“Compaction 中途崩溃”特指 summary Provider call、planning 或 validation 尚未正常返回并结算时，Pia 进程因 `SIGKILL`、未恢复的 panic、OOM、断电等突然终止。普通 Provider error，或者能被 Go context 接住并正常返回的 `SIGINT`/`SIGTERM` cancellation，不属于这个特例。

首版只有 settled record，所以突然终止的 attempt 不产生新 CompactionRecord；重启后仍使用上一个 committed projection，且不自动重放 continuation。由于 summary path 没有 coding tools，这个缺口不会造成未知 workspace tool 副作用，只可能失去一次内部 attempt 的审计信息，并保留已发生的 Provider 数据外发或计费。只有真实需求要求追踪这类 interrupted attempt 时，第三阶段才把记录扩展为 durable Started + Settled pair。

这些结论只固定 owner、持久化位置和可追溯边界，不提前固定第三阶段的 Go package、JSON schema、journal filename 或写入顺序。Durable journal 引入后，当前 D79 的纯内存 commit section 必须重新校准为 crash-safe commit protocol。

## 教学与讨论顺序

本课按下面顺序推进，不先写代码：

1. 用冻结 Pi 的 `runAgentLoopContinue()` 解释“新 prompt”与“从现有 user/tool-result tail 继续”的区别。
2. 区分正常的 tool-call-free overflow error 与 Lesson 03 的 completed-call failure settlement，确认后者不进入本课 recovery。
3. 画出 complete History、current Working Context 和 retry Working Context 三种视图，讨论并确认 projection representation。
4. 比较 Pi 的多 Provider matcher 与 DeepSeek 官方证据，确认首版只处理 explicit error-based overflow，并选择 classifier placement。
5. 确认一次 recovery budget、compaction/cancellation commit point 与成功/失败返回语义。
6. 再细化 Go 文件责任和测试 seam，进入实现。
7. 实现后运行 `make check` 与 `go test -race ./...`，同步课程、决策和 glossary，等待学习者理解确认。

三个实现前讨论点均已确认：

- D76：projection 使用显式 absolute History exclusion positions，并让所有 model-view consumers 共享过滤结果；
- D81：overflow classifier 当前留在 coding private policy，并留下有明确复评条件的代码 `TODO`；
- D82：Core Run 表示一次 accepted Agent Loop execution，一次 coding user advance 可以顺序协调 input-started Run 与 input-free Continue，而不提前发明 Session API。

## Go 实现结果

实现前的结构复核没有要求新 package 或目录重组：Core continuation 仍属于 `internal/agent`，coding recovery policy 与 compaction projection 仍属于 `internal/coding`。为避免把阶段性 matcher 混入 Conversation coordination，首版只增加一个聚焦的 `recovery.go` 文件；`compaction.go` 继续聚合 budget、planning、summary candidate 与 projection commit 这一项 cohesive responsibility。

- `internal/agent/loop.go` 让 `Run` 与新增的 `Continue` 进入同一个 Provider/tool execution；`validation.go` 在 acceptance 前验证 continuation 的非空 user/paired-tool-result tail 与整个 tool-call/result protocol。两条入口共享 active guard、取消和 completed-call settlement，但 continuation delta 不包含既有输入。
- `internal/coding/recovery.go` 实现 coding-private explicit-overflow predicate 与 tool-call-free eligibility。Matcher 只接受窄的 context-length/window phrases，并优先排除 rate limit、429、5xx、server overload 与普通请求错误；文件中的英文 `TODO` 明确该位置只属于当前阶段，并列出 D81 的三个复评触发条件。
- `internal/coding/conversation.go` 在一次 accepted user advance 内保持 Conversation guard：先提交初始 Core Run delta，再判断 recovery、构造 forced compaction candidate、执行一次 Core `Continue` 并提交 continuation delta，最后才返回完整 History snapshot。第二次 overflow 只提交失败事实并以 exhausted error 结算。
- `internal/coding/compaction.go` 使用同一份 filtered model source 驱动 threshold 与 forced compaction。Source 同时保存 filtered messages 和 absolute History positions；projection 以 absolute exclusions 表达被保留在 History、但不能再投给模型的 overflow assistant，并在 candidate validation、idle-only Working Context replacement 成功后同步发布。
- `internal/ai/provider/openaicompatible` 无需扩张协议：已有 64 KiB HTTP error-body bound、operation/status context 与 configured-credential redaction 足以给首版 private classifier 提供输入。`internal/coding/runtime.go` 与 `cmd/pia` 的 one-shot/final-only 产品契约没有改变。
- `internal/agent/continuation_test.go` 和 `internal/coding/recovery_test.go` 覆盖 continuation acceptance、delta/tool loop、classifier 正反例、两段提交、取消 commit point、guard、重复 compaction、两次独立 overflow、product trace/final text 与 ownership independence。实现审查还补上了 Working Context 中间 `nil` message 的拒绝、带 overflow 文案的纯 500 status 负例，以及 hyphen/underscore normalization 不能绕过 rate-limit negative evidence 的回归测试。

## 确定性验证矩阵

默认测试继续离线并使用 Faux Provider；不要求为了制造 1M-token overflow 支付一次真实 DeepSeek 调用。

### Core Agent continuation

- 空 Working Context、assistant tail、预取消 context 和 active Run 都在 acceptance 前拒绝且不修改 context；
- user tail 与 paired tool-result tail 可以 continuation，首个 Provider request 不多出 user message；
- continuation delta 只含本次新 assistant/tool results，并保持深复制与 source order；
- continuation 内的 tool stages、错误、取消和 completed-call settlement 与普通 Run 相同；
- Run、Continue 与 `ReplaceWorkingContext` 的并发 guard 由 race tests 锁定。

### Overflow classification

- 明确的 context-length error evidence 命中；
- generic 400 invalid format、422 invalid parameters、429/rate limit、500/503、aborted 和普通 `length` 不命中；
- 即使错误文本命中 overflow，含 completed tool calls 的 terminal 也不满足 recovery eligibility；
- credential redaction 与 64 KiB error-body bound 不因 classification 退化；
- local HTTP fixture 可以证明 DeepSeek/OpenAI-compatible error body 经现有 terminal path 到达 classifier，但 fixture 不能被描述成官方 live error contract。

### Conversation recovery

- 非 overflow error 只提交一次失败 delta，不调用 summary 或 continuation；
- 第一次 Provider turn overflow 后，user input 在 complete History 中恰好一次；
- 先有成功 tool turn、后 overflow 时，成功 tool result 保留且不重跑，overflow error assistant 只从 retry projection 排除；
- forced compaction request 不把 overflow assistant 写进 summary input，也不追加 synthetic “new user input”；
- recovery success 返回 nil error，complete History 按顺序包含原 user、overflow assistant 与 recovered delta；
- summary error、空 summary、unexpected tool call、candidate 仍过大和 replacement failure 都不启动 continuation；它们保留已提交 overflow History，但不发布 exclusion、summary、cut 或新 Working Context；
- summary/continuation cancellation 保留 context cause，且各 commit point 前后的 History/Working Context/projection 状态符合契约；
- continuation 再 overflow 只提交第二个失败 delta，不做第二次 recovery；
- Conversation guard 在 summary 与 continuation 阻塞期间都拒绝 concurrent user advance，不 queue、不越过 commit；
- 后续普通 Run、第二次独立 overflow recovery 和重复 threshold compaction 都不会把旧 excluded assistant 重新投进 request 或 summary；
- 返回 History 和 Provider request snapshots 继续保持 ownership independence。

### Product composition

- `coding.Run` 在 recovery 成功后，`FinalText()` 选择 recovered terminal assistant；
- 可选 trace 保存完整 History，包括初始 overflow error assistant，而 stdout 仍只有最终 recovered text；
- recovery 失败保持现有 non-zero CLI 行为，不新增交互提示或自动 retry progress 输出。

## 当前非目标

- rate limit、5xx、transport failure 的 generic retry、exponential backoff 或 retry-after；
- silent overflow、所有 `finish_reason=length` heuristic、Provider usage 超窗后的成功-response retry；
- 正常 active Core Run 中按 `192K` quality threshold 主动 mid-Run compaction；
- wall-clock/model-turn/tool-call budget、loop circuit breaker 或 cost budget；
- semantic events、token/text deltas、subscription、live progress、TUI；
- steering、follow-up、并发 user input queue 或 prompt preemption；
- Session identity、持久化 journal、恢复、branch、multi-Session manager 或 scheduler；
- Gateway、IM、gRPC、公共 SDK、Goal Runtime、worktree/GitHub 管理；
- 删除或重写 complete Conversation History 中的 error/aborted messages；
- 为未来 Provider 预建 registry 或完整 error taxonomy。

## 本课产物边界

设计回合产生本 lesson 文档、课程总纲中的 Lesson 11/第三阶段大纲，以及 D73–D82 课程边界决定；实现回合完成 Core continuation、coding recovery/projection/compaction coordination、对应离线测试与术语同步。学习者确认理解后，这些改动作为同一个 Lesson 11 提交进入 `origin/main`。

最终结构没有修改 Provider wire protocol，也没有加入第三阶段 semantic events。实现落在 `internal/agent` 的 continuation、`internal/coding` 的 recovery/projection/compaction coordination、对应离线测试和文档同步；Lesson 11 commit 仍不能夹带第三阶段代码。

## 完成信号

本课只有同时满足以下条件才进入“待理解确认”：

1. 一个有足够旧 context 的 accepted user advance 在明确 overflow 后，只自动做一次 compact-and-continue，并能成功得到 recovered final assistant。
2. Complete History 原样保留初始 overflow error assistant；retry Provider request 与以后所有 projection/summary 都不再包含该 assistant。
3. 原 user input 只出现一次，已成功执行的 tools 不重跑；含 completed tool calls 的 error terminal 不进入本课 recovery。
4. 第二次 overflow、非 overflow error、summary failure 和 cancellation 都有有界、保因且无死循环的 settlement。
5. 同一 Conversation 的 guard 覆盖整个 recovery；不同 Conversation/Agent instances 仍没有 package-global state。
6. `make check` 与 `go test -race ./...` 通过，课程文档、`decisions.md` 与 `CONCEPTS.md` 按最终实现同步。
7. 学习者能解释 complete History 为什么保留 overflow、Working Context 为什么排除 error assistant，以及为何 `Continue` 不是空 user message。

完成 Lesson 11 不等于 Session Runtime 已经完成。它只把第二阶段的内存 Conversation/Working Context/compaction 闭环补到“遇到真实 overflow 仍可有界继续”；第三阶段才系统建设可观察、可控制、可持久化、可恢复的 Session lifecycle。
