# 第 07 课：Conversation History、Working Context 与 Request Snapshot

## 当前状态

实现、离线验证和学习者理解确认均已完成，本课已提交。课程先重新阅读冻结 Pi 源码并推翻早期单 transcript owner 假设，再完成 Conversation History、Working Context 与 Request Snapshot 的独立所有权边界；讲解全程在对话中进行，没有生成 HTML 讲义。

## 开课源码校准

早期大纲把问题简化为“完整 conversation history”与“active model context”两层。方向正确，但冻结 Pi 源码表明 `active context` 这个名称混合了两个不同角色，因此本课将其修正为三层：

1. `SessionManager` 保存 append-only session entries 和当前 branch/leaf；这是完整 session record。
2. core `Agent.state.messages` 保存当前可继续执行的 working context；compaction 后它可以被完整替换。
3. 每次模型调用从 working context 建立 request-local snapshot，再经 `transformContext` 和 `convertToLlm` 生成 Provider 实际看到的 messages。

三种角色不等于 Go 中必须长期保存三个 slice。Provider request 必然只是临时值；working context 也可能由完整 history 和少量 projection state 按请求派生，而不是复制成第二份长期可变数组。源码校准先把语义责任分开，具体存储形态留给本课讨论。

Pi 的 `AgentSession` 负责同步这些角色：`message_end` 时把 Agent 已接受的消息追加到 `SessionManager`；恢复、导航或 compaction 后，再用 `buildSessionContext().messages` 替换 Agent working context。

进一步核对自动 compaction 路径后，课程还需要补充一个重要时序：Pi 在 core Agent 发出 `agent_end` 后，由外层 `AgentSession` 检查 threshold 或 context overflow。此时 `message_end` 已经把 terminal assistant（包括 overflow error）写入 `SessionManager` 的完整记录；随后外层生成并追加 compaction entry、重建 `agent.state.messages`，overflow 恢复时再移除 working context 中的 error assistant 并调用 `agent.continue()`。因此完整记录与可替换 working context 的分离不仅服务未来磁盘持久化，也是保留失败事实并安全恢复模型执行的现有语义基础。

Pi 的低层 `runAgentLoop()` 和 `runAgentLoopContinue()` 还会分别维护并返回本次运行产生的 `newMessages`；`agent_end.messages` 携带的也是这批 run-local 增量，而不是完整 Session 历史。core `Agent` 在 `message_end` 时把同一条已完成消息追加进自己的 working messages，外层 `AgentSession` 再消费该事件写入 `SessionManager`。这证明 run-local message delta 与逐消息事件是两个可独立使用的机制：前者足以在 Run settlement 后同步最小内存 Conversation Owner，后者在 Pi 中还服务实时持久化、extensions 和 UI。pi-go 当前没有后三类消费者，因此本课不应仅为复制 Pi 形状而提前引入完整事件流。

当前 pi-go 只有 `Agent.transcript` 一份长期消息状态。它同时承担完整 conversation history、当前 working context 和 Provider request 的来源；`requestSnapshot()` 已经深复制 Request，解决了调用方反向修改所有权的问题，但没有区分完整记录与未来可被压缩的工作上下文。

因此本课的大方向保留，但不能机械复制 Pi 的完整 `SessionManager`。本课要确定的是内存所有权边界；compaction、持久化、branch、事件和 UI 继续留在各自后续课程。

## 冻结 Pi 源码阅读路径

固定基线：Pi commit `dcfe36c79702ec240b146c45f167ab75ecddd205`，Pi Agent package version `0.80.7`。

- `packages/agent/src/agent.ts`：core Agent 持有 `state.messages`，在 Run 前生成 context snapshot，并把 terminal messages 追加回 working state。
- `packages/agent/src/agent-loop.ts`：每次模型调用先应用 `transformContext`，再通过 `convertToLlm` 构造 Provider context。
- `packages/coding-agent/src/core/session-manager.ts`：保存完整 entry tree，并通过 `buildContextEntries()` / `buildSessionContext()` 投影当前 branch 的 compaction-aware context。
- `packages/coding-agent/src/core/agent-session.ts`：在 Agent 事件、SessionManager persistence 和 compaction/context replacement 之间进行协调。
- `packages/coding-agent/src/core/sdk.ts`：启动或恢复时从 `SessionManager` 构造 working messages，再交给 core Agent。
- `packages/coding-agent/test/session-manager/build-context.test.ts`：验证完整 entries 与模型 context 在普通对话、compaction 和 branch 下并不等价。

## 开课时的 pi-go 阅读路径

- `internal/agent/types.go`：`Agent` 当前声明自己拥有一段对话的完整内存 transcript。
- `internal/agent/loop.go`：`beginRun()`、`appendAssistant()` 和 `appendToolResults()` 修改同一 transcript；`requestSnapshot()` 把它完整复制进每次 `ai.Request`。
- `internal/agent/loop_test.go`：锁定顺序 Run 保留历史、返回 snapshot 独立，以及 Provider 不能反向修改 Agent transcript。
- `internal/coding/runtime.go`：当前 one-shot composition 每次创建一个新 Agent，尚无更高层的内存 conversation/session owner。

## 术语校准：transcript 与 messages

本课沉淀的跨课程术语统一记录在仓库根目录的 [`CONCEPTS.md`](../../../CONCEPTS.md)。本节保留 Pi 源码和当前 pi-go 命名的历史解释；后续讨论涉及所有权时，以 `CONCEPTS.md` 中的 Conversation History、Working Context、Core Agent 和 Coding Agent 为准。

`transcript` 作为领域概念来自 Pi，但不是对其字段名的机械翻译。冻结 Pi 在 `packages/agent/src/agent.ts` 中说明 core `Agent` owns the current transcript，在 `packages/agent/src/types.ts` 中也把 `state.messages` 解释为 conversation transcript；实际 TypeScript property、参数和 Provider context 仍统一使用 `messages`。

两者描述的层次不同：

- transcript 表示一段按顺序形成的对话记录，强调它是可继续使用的整体事实；
- messages 表示组成这段记录或某次模型输入的具体消息集合，是数据结构和协议字段名；
- context 表示从记录中选择、转换后供模型当前调用使用的内容；
- session 比 transcript 更宽，还可能包含模型设置、branch、compaction、标签和持久化信息。

pi-go 第一期把 Agent 的权威状态命名为 `transcript`，同时把 `ai.Request.Messages` 留给 Provider 协议，目的是避免两个同名 `messages` 被误认为同一个 owner。这一命名在第一期有帮助，但 Lesson 07 拆分完整记录与 working context 后必须重新审视：完整记录仍可叫 transcript/history；如果 core Agent 最终只持有 working context，其内部字段叫 `messages` 或 `contextMessages` 可能更准确。命名将在所有权确定后一起收敛，不能因为现有代码已经叫 `transcript` 就反推它必须继续拥有完整历史。

## 一个具体例子

假设完整对话已经产生六条消息：

```text
history = [user1, assistant1, user2, assistant2, user3, assistant3]
```

在 Lesson 07 结束时，三种角色的内容仍可以相同，但所有权必须明确：

```text
complete history       = 六条权威记录
working context        = 六条当前可继续运行的消息
Provider request       = working context 的独立 snapshot
```

Lesson 08 加入 compaction 后，它们才会在内容上真正分开：

```text
complete history       = 原始六条权威记录 + 后续记录
working context        = [旧内容摘要, user3, assistant3, ...]
Provider request       = working context 的独立 snapshot
```

Provider request 从来不是新的长期事实源；模型或 Provider 对 request slice 的修改也不能污染前两层。

## 本课闭环与非目标

本课将按以下顺序推进：

1. 已通过源码校准讲清 Pi 三种角色与当前 pi-go 单 transcript 模型的差别。
2. 已确认外层 Conversation Owner 与 Core Agent Working Context 的语义所有权。
3. 已确认 Core Agent 在 Run settlement 后同步返回 run-local message delta，不引入 channel、callback 或完整事件流。
4. 已确认首份 Conversation Owner 是 `internal/coding` 的私有应用层实现，不提前建立通用 package。
5. 已确认 Conversation Owner 使用 fail-fast active guard，覆盖 Core Agent Run 到 History commit 和返回 snapshot 的完整边界。
6. 已确认 Core Agent 返回独立的 `NewMessages`，Conversation Owner 接管该 delta，外部完整 History snapshot 再深复制。
7. 已确认 idle-only `ReplaceWorkingContext`：深复制并原子替换，active 时立即失败。
8. 已实现该边界，并用 Faux Provider 锁定顺序、深复制、失败和取消后的权威状态。
9. 已完成测试并记录与 Pi 的有意差异，为 Lesson 08 提供安全的 compaction 接入点。

本课不实现 token budget、摘要算法、context-overflow retry、Session 文件、branch、steering/follow-up、事件订阅、Skills 或 TUI。Conversation Owner 的首份 package 归属、Core Agent result shape、active 边界、消息复制和 Working Context replacement API 已在本课讨论中确定。

## 已确认的所有权方向

Pi 的证据确认“一个 slice 永远同时代表完整 history 和可替换 working context”并不是 Pi 的模型，也暴露了这种合并状态在 compaction 前的长期张力。学习者已经确认 pi-go 采用以下语义所有权：

- 外层最小内存 Conversation Owner 保存完整 Conversation History；
- Core Agent 只拥有可替换的 Working Context，并负责 Provider/tool loop；
- 每次 Provider request 继续使用与前两者隔离的临时 deep-cloned snapshot。

这只是责任边界，不要求立即建立与 Pi 同规模的 `SessionManager`。持久化、branch、compaction 算法、事件展示和 TUI 仍不属于本课。

## 已确认的同步方向

Core Agent 的一次 accepted Run 可以包含多个 Provider Turns，并产生多条新消息。它在 Run settlement 后同步返回完整、有序、ownership-independent 的 run-local message delta；Conversation Owner 无论 Run 最终返回 nil 还是 non-nil error，都先把这批消息一次性提交到 Conversation History，再把控制流结果交给调用者。Run 在 acceptance point 前取消或因已有 active Run 被拒绝时，delta 为空且 History 不变。

这条同步路径使用普通函数返回，不使用 Go channel，也不调用外部 message callback。当前没有需要在 Run 完成前读取 History 的持久化或 UI 消费者；批量提交避免提前定义 channel close/backpressure/drain、callback failure/panic/reentrancy，以及 listener settlement 等生命周期。未来事件课程可以根据实时消费者另建观察投影，但不改变 run-local delta 是 Core Agent settlement 结果这一事实。

## 已确认的 package 归属

首份 Conversation Owner 放在现有 `internal/coding` package 的独立 `conversation.go` 中，并保持为未导出的应用层实现。它协调具体 Core Agent 与完整内存 History；`coding.Run` 仍是当前 one-shot composition entrypoint，本课不因此建立公共 `CodingAgent` SDK。

当前不创建 `internal/conversation` 或 `internal/session`，也不把完整 History 的 owner 放回 `internal/agent`。Conversation 在概念上可能被其他 Agent 应用复用，并不等于它们已经证明需要完全相同的 history policy。未来出现第二个非 coding 消费者时，必须同时 review 双方的 ownership、lifecycle 与 persistence 需求，只提取已经成立的共同责任。

## 已确认的 active Run 边界

同一 Conversation 同时最多接受一个 active Run。Conversation Owner 在接受 Run 时设置自己的 active guard，调用 Core Agent，并在 Core Agent 返回后无论 error 是否为 nil，都先提交 run-local message delta、生成完整 History snapshot，最后才释放 active。并发调用立即返回 active-run error，不等待，也不形成尚未定义取消和排序语义的隐式输入队列。

Core Agent 仍保留自己的 guard：它保护可替换 Working Context；Conversation Owner 的 guard 则保护一次 Run 与完整 History commit 的顺序。如果只有前者，Core Agent 在返回前释放 active 后，下一次 Run 可能在上一次 delta 尚未写入外层 History 时进入，造成模型实际处理顺序与完整历史提交顺序分叉。

这里的串行边界只属于同一 Conversation。不同 Conversation 可以并行；一个 Run 内连续的 parallel-safe tools 也继续按既有调度契约并行执行。steering、follow-up 和显式输入队列留待出现真实交互消费者后再设计。

## 已确认的消息复制边界

Core Agent 的 `RunResult.NewMessages` 是本次 accepted Run 的完整有序 delta，并与 Agent 自己的 Working Context 深度隔离。Conversation Owner 是这份返回值的下一个且唯一内部 owner，可以直接把它追加到完整 History；没有第二个共享 owner 时不做重复 clone。

Coding Agent 返回给调用者的 `RunResult.Transcript` 仍表示完整 Conversation History，并且必须是 deep-cloned snapshot。这样调用者可以自由检查或修改自己的结果，而不能通过 slice、content blocks 或 tool-call JSON bytes 反向修改权威 History。Provider Request Snapshot 和 Provider terminal message 继续遵守各自已有的深复制边界。

## 已确认的 Working Context replacement

Core Agent 提供 `ReplaceWorkingContext([]ai.Message) error`，只在 idle 时深复制输入并原子替换 Working Context。active Run 期间调用立即失败，不等待、不排队；同一个 Run 的不同 Turns 因此不会意外使用两套 context。该操作不读取或修改 Conversation History，也不返回旧 context 或暴露可变 Agent state。

冻结 Pi 的 `agent.state.messages` setter 只复制传入数组的顶层，coding-agent 在 `agent_end` 后、下一次 prompt 前用 compaction-aware Session context 赋值。pi-go 保留该 replacement 时序，但使用窄方法、active guard 和深复制表达 Go 的所有权边界。本课只建立可替换能力，不实现 compaction algorithm、token budget、overflow recovery 或额外协议验证器。

## Go 实现

- `internal/agent/types.go`：Core Agent 的权威字段改为 `workingContext`；`RunResult.NewMessages` 明确表示一次 Run 的增量，而不是完整 History。
- `internal/agent/context.go`：集中 Working Context 的 request snapshot、append、run-local snapshot 与 idle-only replacement；`ReplaceWorkingContext` 深复制输入并在 active Run 时立即失败。
- `internal/agent/loop.go`：Run acceptance 时记录 Working Context 起始位置，所有正常、错误和取消 settlement 都只返回该位置之后的独立 delta。
- `internal/coding/conversation.go`：未导出的 `conversation` 保存完整 settled History；自己的 active guard 覆盖 Core Agent Run、delta commit 和完整 History snapshot，不建立等待队列。
- `internal/coding/runtime.go`：one-shot composition 仍创建同一个 Core Agent，但通过私有 Conversation Owner 驱动 Run；对外 `RunResult.Transcript` 的内容和 final projection 行为保持不变。

没有加入通用 Conversation interface、Session package、channel、callback、event sink 或 compaction 实现。`internal/agent/context.go` 是既有 package 内按状态责任拆出的实现文件，不是新的抽象层。

## 测试与验证

确定性测试覆盖：顺序 Core Runs 返回各自 delta 但下一次 Provider request 保留完整 Working Context；Core result、replacement 输入、Provider request 和完整 History snapshot 的嵌套内容互不反向修改；accepted error/cancellation delta 仍提交 History；预取消不追加消息；同一 Conversation 的并发 Run fail-fast；active Run 拒绝 Working Context replacement；idle replacement 原子生效且不修改 Conversation History。

验证结果：

```text
go test ./...       PASS
go vet ./...        PASS
go test -race ./... PASS
git diff --check    PASS
```

本课没有修改 coding prompt、Provider wire protocol、工具行为或 `pia` 的 stdout/stderr 契约，因此不重新调用付费模型或重复 Lesson 06 的真实 bug-fix 验收。
