# 第 02 课：单次 Provider Turn 与 transcript

## 当前状态

已提交。

开课记录：第 01 课于 2026-07-16 提交到 `main` 后，学习者明确要求开始第 02 课。课程继续采用循序渐进、讲解优先、结合冻结 Pi 源码与实际 Go 代码的方式，不生成独立 HTML 讲义。

完成记录：Run、Turn、message、transcript、所有权、取消和 terminal settlement 已完成讨论；Go 实现、离线测试和完整性审查已经完成。学习者于 2026-07-16 确认无需继续讲解并明确要求补充完整后提交、push 到 `main`，随后开始第 03 课。

## 本课目标

完成本课后，学习者应能解释：

1. Run、Turn、Provider call 和 message 之间的包含关系。
2. 为什么原始 task 只在 Run 开始时转换成一条 `UserMessage`，而不是每次 Provider 调用再单独携带。
3. Agent 为什么保留同一对话的完整 transcript，Provider 为什么每次都只接收当时的完整快照。
4. 一次 Provider stream 的 terminal `AssistantMessage` 如何进入 transcript，并如何决定 Run 的正常、失败或取消终态。

## Pi 源码阅读路径

固定基线：Pi commit `dcfe36c79702ec240b146c45f167ab75ecddd205`，Pi Agent package version `0.80.7`。

- `packages/agent/src/agent-loop.ts`
- `packages/agent/src/types.ts`
- `packages/agent/src/agent.ts`
- `packages/ai/src/types.ts`

本课只提取一次无工具 Provider turn、基础 transcript 和终态行为。Pi 的工具执行、Agent event sink、运行过程展示、steering、follow-up、continue、context transform、Session/TUI 和多轮循环不在本课实现范围；工具结果驱动的下一轮从第 03 课开始，观察和展示边界等本地入口出现真实需求时再讨论。

## 第一阶段：先分清四个嵌套层级

```text
Run（一条 task 启动的完整执行）
└── Turn（一次 assistant response，后续课程还可包含 tool calls/results）
    └── Provider call（把当前完整上下文交给模型）
        └── stream events（这条 assistant message 的形成过程）
```

第 02 课刻意不执行工具，因此每个 Run 只有一个 Turn、一次 Provider call；测试可以顺序调用同一个 Agent 两次，以证明第二条 user input 会追加到已有 transcript。这里减少的是本课单个 Run 的 loop 次数，不是把 Agent transcript 弱化成一次性状态；第 03 课加入 tool results 后，同一个 Run 才会拥有多个 Turn 和多次 Provider call。

transcript 是 Agent 持有的同一对话的完整有序消息历史。第一次最小正常路径为：

```text
task
  -> UserMessage(task)
  -> transcript = [user]
  -> Provider.Request{SystemPrompt, Messages: [user], Tools}
  -> Provider stream
  -> terminal AssistantMessage
  -> transcript = [user, assistant]
  -> Run 正常结束
```

同一个 Agent 再接收一条 user input 时不会重新开始空 transcript：

```text
transcript = [user1, assistant1]
  -> append user2
  -> Provider.Request{Messages: [user1, assistant1, user2]}
  -> terminal assistant2
  -> transcript = [user1, assistant1, user2, assistant2]
```

Provider 不拥有 transcript，也不能替 Agent 追加历史。它只读取当前调用的 Request snapshot 并产生一条 assistant response；是否继续下一轮、是否执行工具以及 Run 如何结束都属于 Agent Runtime。

## 纠正：Phase 切片不能弱化 Agent 状态模型

先前讨论曾错误地从“第一期不做 Session”推导出“每次 Run 使用一次性 transcript，甚至不需要 Agent 对象”。该推导不成立，现已纠正：

- Pi 高层 `Agent._state.messages` 跨 `prompt()` 保存完整历史；`createContextSnapshot()` 把全部 messages 交给低层 loop。
- Pi 低层 `runAgentLoop()` 用 `messages: [...context.messages, ...prompts]` 追加新 user input，随后继续追加 assistant 和 tool results。
- pi-go 保留相同的核心所有权：Agent 保存完整内存 transcript，`Agent.Run()` 追加新的 user input；不做的是 Session 持久化、恢复、分支和管理层。
- 新对话需要显式新建 Agent 或未来明确的 reset boundary，不能把“收到下一条 user message”当成隐式清空信号。

## 已验证的 Pi 行为与当前课程切片

- Pi `runAgentLoop()` 先把 prompts 追加到当前 context messages，再发送 `agent_start`、`turn_start` 和 prompt 的 message lifecycle events。
- 每次模型调用从当前 context 构造 `Context { systemPrompt, messages, tools }`，没有独立 task 或 workspace context 字段。
- Pi 在低层 assistant stream 开始时把 partial assistant message 放入私有 working context，随后用新的 partial snapshot 替换最后一条，terminal 时再替换成 final message。
- Pi 高层 `Agent.state.messages` 不保存 partial：`message_start/message_update` 只更新独立的 `streamingMessage`，`message_end` 才把 terminal message 追加到正式 transcript。coding-agent TUI 通过事件中的 partial snapshot 刷新显示，Session 也只在 `message_end` 持久化消息。
- pi-go 第一阶段没有需要查询完整 partial snapshot 的消费者，因此正式 transcript 只追加 terminal final/aborted assistant message，不维护可查询的 Run-owned partial view。第 02 课不设计 event sink；本地终端或未来 TUI 如何流式观察和展示，后续按真实消费者需求讨论。
- Pi 在 provider error 或 aborted assistant message 后结束当前 Turn 和整个 Run，不执行工具；完整接收且不含 tool calls 的 assistant response 正常结束，即使文本为空。
- pi-go 使用 `run_start/run_end` 表达一次 Run 的边界；引用 Pi 源码时仍保留原名 `agent_start/agent_end`。

## Partial assistant 的讨论结论

本课只确定执行数据流中的权威状态：

```text
Provider formation events
  -> terminal AssistantMessage
  -> Agent 正式 transcript
  -> 后续 Provider request
```

- partial text、thinking 或尚未闭合的 tool-call 参数只是当前 response 的形成过程，不能作为下一 Turn 的稳定上下文。
- 第一阶段不因为未来可能有终端或 TUI 展示就复制 Pi 的完整 `streamingMessage` 状态。
- formation events 以后如何进入 sink、终端或 TUI，属于观察和展示设计，不是本课 Agent 执行数据流的一部分。
- 该结论不单独决定 `ContentIndex` 是否继续出现在 Provider stream 协议中；它是否只是 Provider 内部映射细节，仍需结合第 04 课 DeepSeek 事件形状核对。

## 第一个理解重点

Agent Loop 的第一项责任不是“调用模型”，而是维护一条可信的状态转换：

```text
原始 task
  -> 首条 user transcript 事实
  -> 完整 Provider request snapshot
  -> 消费 response formation events
  -> terminal assistant transcript 事实
  -> 唯一 Run 终态
```

如果 task 同时保存在独立 Request 字段和 transcript 中，或者 Provider 可以直接修改 Agent 的历史，就会出现重复事实源。第 02 课先用 Faux 锁定这条单向所有权和事件顺序，再实现 `internal/agent`。

## RunResult 与 Go error 的讨论结论

第 02 课采用候选调用形状：

```go
type RunResult struct {
	Transcript []ai.Message
}

func (a *Agent) Run(ctx context.Context, userInput string) (RunResult, error)
```

- `AssistantMessage.StopReason` 保存这次模型响应为什么停止，是 transcript 事实。
- `RunResult` 返回 Agent 当前完整 transcript 的快照，不重复保存 `Outcome` 或 `Error`。
- 完整 `DoneEvent(stop|length)` 且当前没有工具继续条件时返回 result 与 nil error；这只表示 Loop completed，不表示 coding task 验收成功。
- `ErrorEvent(error)` 把 error assistant 追加到 transcript，并返回 result 与非 nil error。
- `ErrorEvent(aborted)` 把 received-so-far aborted assistant 追加到 transcript，并返回 result 与可通过 `errors.Is` 识别的 context cause。
- 非 EOF 的 `Receive` error 或 terminal 前 EOF 无法提供可信 terminal message；Agent 根据 `context.Cause(ctx)` 合成 aborted 或 error assistant，保留失败现场后返回包装错误。

该决定避免 `StopReason`、`RunOutcome` 和 Go error 三处重复分类。失败时 result 与 error 可以同时非零：调用方用 error 做控制流，仍可从 result transcript 读取完整对话和已经形成的模型内容。

## Transcript snapshot 的深复制结论

学习者确认“快照”必须真正与 Agent 内部状态隔离，不能只复制 `[]ai.Message` 外层。`AssistantMessage.Content` 是 slice，`ToolCall.Arguments` 和 `ToolSchema.Parameters` 是 `json.RawMessage`/`[]byte`；浅复制仍会共享 backing array，调用方或 Provider 可以借此反向修改 Agent transcript。

第 02 课在三个所有权边界深复制：

```text
Provider terminal message --deep clone--> Agent transcript
Agent transcript          --deep clone--> Provider Request
Agent transcript          --deep clone--> RunResult.Transcript
```

复制逻辑由拥有 message/content 结构的 `internal/ai` 统一提供，并让 Faux 与 Agent 复用；不在两个 package 中分别维护 type switch。Agent 内部的普通顺序读取和追加不重复复制。

## 预取消与 Run acceptance point

Pi 高层 `Agent.prompt()` 不接收外部 signal；它只在 prompt 通过 active-Run 检查后创建内部 `AbortController`，无 active Run 时调用 `abort()` 是 no-op。因此 Pi 公共 Agent API 没有与 Go“传入一个已经取消的 context”完全相同的调用形态。Pi 低层 `runAgentLoop()` 则先把 prompts 加入 current context 并发送 user message lifecycle，之后才把 signal 交给 Provider；Pi AI 的预取消测试也从已经包含 user message 的 context 得到空内容 aborted assistant。

pi-go 明确增加 Go API 边界：

```text
已有 active Run -> ErrRunActive，不观察半完成 transcript
无 active Run、ctx 已取消 -> Run 未接受，transcript 不变，不产生 aborted assistant
ctx 有效 -> 在同一个锁内设置 active 并追加 user，构成 acceptance point
acceptance point 后、terminal settlement 前取消 -> 保留 user，只追加一条 aborted assistant
```

context 检查与 active/user 状态转换在同一个短临界区完成。若 context 在检查通过后立刻取消，该 Run 已越过 acceptance point，按已开始取消处理。

## 每个 Provider turn 的唯一 terminal settlement

这里的 terminal 指一次 Provider stream 的终态，不等于整个 Run 永远只有一条 assistant。第 02 课一个 Run 只有一个 Turn，所以两者暂时重合；第 03 课加入工具循环后，一个 Run 可以有多个 Turn，每个 Turn 都各有一条 terminal assistant。

每次 Provider call 开始后必须最终形成恰好一条 terminal assistant。stream consumer 只负责返回 `(terminal AssistantMessage, error)`，不写 Agent transcript；Turn/Run coordinator 在该次调用的唯一位置深复制并追加：

```text
DoneEvent                         -> Provider final assistant + nil
ErrorEvent(error)                 -> Provider error assistant + provider error
ErrorEvent(aborted)               -> Provider aborted assistant + context cause/fallback error
terminal 前 EOF / receive error    -> synthetic aborted 或 error assistant + error
                                      |
                                      v
                  append exactly once for this Provider turn
```

第一条有效 `DoneEvent` 或 `ErrorEvent` 是这次 Provider turn 的 terminal settlement point；之后发生的 context cancel 不追溯修改已经完成的 assistant。若 Provider 尚未给出 terminal 而 raw stream failure 发生，Agent 以 `context.Cause(ctx)` 是否存在决定合成 aborted 还是 error assistant。

`Stream.Receive()` 的返回值按“有效 terminal event 优先”解释：如果同一次调用同时返回有效 terminal event 和 error，terminal 已经构成 settlement，error 或此时刚发生的 context cancel 不再把结果改写成 aborted。若没有有效 terminal，才解释 raw receive error。Provider 返回 nil stream 则是 terminal 前的协议错误，Agent 合成一条 error assistant。

Provider stream 在创建时已绑定 context。Agent 不额外启动 goroutine 与 `ctx.Done()` 竞争后提前返回，而是等待阻塞的 `Receive()` 因取消真正收敛；否则可能让网络读取 goroutine 在 Run 返回、active 清除后继续存活。

## 实际 Run 结构

第 02 课最终实现保持了讨论中的三层责任：

```go
func (a *Agent) Run(ctx context.Context, userInput string) (RunResult, error) {
	request, earlyResult, err := a.beginRun(ctx, userInput)
	if err != nil {
		return earlyResult, err
	}
	defer a.endRun()

	stream := a.provider.Stream(ctx, request)
	message, runErr := receiveAssistant(ctx, stream)
	return a.appendTerminalAndSnapshot(message), runErr
}
```

- `beginRun` 独占 acceptance point：拒绝并发 Run、检查预取消、设置 active、追加 user、创建 Provider request deep clone。
- `receiveAssistant` 只把正常、Provider error/aborted、nil stream、terminal 前 EOF、receive error 和取消收敛成 `(AssistantMessage, error)`，不接触 Agent。
- `appendTerminalAndSnapshot` 是越过 acceptance point 后的唯一 terminal 写入点；它在一个锁内深复制并追加 assistant，再返回完整 transcript deep-clone snapshot。
- `defer a.endRun()` 只释放 active 状态，不追加消息，因此成功、失败或取消分支不会通过 cleanup 再写第二条 assistant。

## 本课实现范围

- `internal/ai/clone.go`
- `internal/ai/clone_test.go`
- `internal/agent/types.go`
- `internal/agent/loop.go`
- `internal/agent/loop_test.go`
- `internal/ai/faux/provider.go`（改为复用统一 clone 规则）
- `docs/course/lessons/02-agent-loop-and-transcript.md`

测试已锁定单轮文本、空文本、`length` 正常结束、thinking/text 混合、同一 Agent 顺序两次 Run 保留完整历史、并发 Run 拒绝、完整 Provider request、Provider error/aborted、nil stream、terminal 前 EOF、非 EOF receive failure、stream cancel、非法 terminal reason、取消等待已启动 stream 收敛、terminal 与取消同返时 terminal 优先、明确的 Run 返回结果，以及外部修改 Provider request、Provider terminal message 或 RunResult 的嵌套 snapshot 不能反向修改 Agent transcript。默认测试只使用 Faux Provider，不访问网络或 Provider key。

验证结果：`go test ./...`、`go vet ./...`、`go test -race ./...` 和 `go test ./internal/agent -count=100` 全部通过。

## 提交记录

学习者已确认第 02 课不需要继续讲解，并明确要求完成完整性审查和必要补充后提交、push 到 `main`。本课文件随本次 lesson 02 commit 提交。
