# 第 03 课：多轮 Tool Loop 与屏障式调度

## 当前状态

已提交。

开课记录：第 02 课于 2026-07-16 完成完整性审查、补充测试并提交到 `main` 后，学习者明确要求开始第 03 课。课程继续采用讲解优先、结合冻结 Pi 源码与实际 Go 代码的方式，不生成独立 HTML 讲义。

节奏调整：学习者于 2026-07-17 确认已理解本课总览，要求后续讲解只保留关键语义和设计分歧以加快进度；最终代码和测试仍按完整设计实现，不采用弱化的教学版本。

审查修正：实现后的第一轮 review 发现第 01/02 课“Provider aborted 后只追加一条 assistant”与第 03 课“权威 transcript 不保留 orphaned tool calls”之间存在遗漏。学习者确认按最新方向修正：error/aborted assistant 仍只追加一次且其中工具绝不执行；若它保留了已完成 tool calls，协调层随后追加同 ID `not executed` results，再返回 Turn error。

## 本课目标

完成本课后，学习者应能解释并从测试中验证：

1. 为什么一条包含 tool calls 的 terminal assistant message 结束的是当前 Turn，而不一定结束整个 Run。
2. Tool schema、原始 tool-call arguments、Go 解码校验、工具执行和 `ToolResultMessage` 各自属于哪一层。
3. 为什么未知工具、非法参数、执行失败和单工具 timeout 是模型可观察的 call-local tool error，而不是立即终止 Run 的基础设施错误。
4. 为什么 tool results 必须按模型源顺序进入 transcript，即使 parallel-safe 工具按不同顺序完成。
5. pi-go 的屏障式分段调度如何保留连续只读调用的并行能力，并避免 read/write/edit/bash 之间发生不明确的竞态。

## Pi 源码阅读路径

固定基线：Pi commit `dcfe36c79702ec240b146c45f167ab75ecddd205`，Pi Agent package version `0.80.7`。

- `packages/agent/src/agent-loop.ts`
  - `runLoop`
  - `failToolCallsFromTruncatedMessage`
  - `executeToolCalls`
  - `executeToolCallsSequential`
  - `executeToolCallsParallel`
  - `prepareToolCall`
  - `executePreparedToolCall`
  - `createToolResultMessage`
- `packages/agent/src/types.ts`
  - `ToolExecutionMode`
  - `AgentToolResult`
  - `AgentTool`
  - `AgentEvent`
- `packages/ai/src/types.ts`
  - `Tool`
  - `ToolCall`
  - `ToolResultMessage`

本课只移植通用 Tool Loop、参数保护、错误结果和调度语义。真正的 `read`、`write`、`edit`、`bash` 实现、`os.Root` workspace 边界和 bash process group 留到第 05 课；Agent event sink、流式展示、steering/follow-up、hook 系统和动态工具增加不在本课范围。

## 从第 02 课到第 03 课

第 02 课的一个 Run 只有一次 Provider call：

```text
user
  -> Provider call 1
  -> terminal assistant（无 tool calls）
  -> Run 结束
```

第 03 课加入工具后，一个 Run 可以包含多个 Turn：

```text
user
  -> Provider call 1
  -> terminal assistant 1（请求 tool A、tool B）
  -> 执行 tool A、tool B
  -> append tool result A、tool result B
  -> Provider call 2（收到完整 transcript）
  -> terminal assistant 2（无 tool calls）
  -> Run 结束
```

这里的 `terminal assistant 1` 已经是第一条 Provider stream 的权威终态，所以必须按第 02 课的 settlement 规则只追加一次；但它包含 tool calls，因此只结束当前 Turn。Agent 执行工具、追加结果后继续下一 Turn。只有 assistant 不再请求工具，或者出现 Provider/Run 级失败或取消时，整个 Run 才结束。

`AssistantMessage` 本身不是工具调用；它可以只包含 text/thinking，也可以额外包含零个或多个 tool-call content blocks。Loop 必须先追加这次 Provider call 的 terminal assistant，再明确分支，不能把“收到 assistant”误写成“一定执行工具”：

```go
func (a *Agent) Run(ctx context.Context, userInput string) (RunResult, error) {
	earlyResult, err := a.beginRun(ctx, userInput)
	if err != nil {
		return earlyResult, err
	}
	defer a.endRun()

	for {
		request := a.requestSnapshot()
		message, turnErr := receiveAssistant(ctx, a.provider.Stream(ctx, request))

		// 每次 Provider call 都只在这里追加一次 terminal assistant。
		a.appendAssistant(message)
		calls := toolCalls(message)

		// Provider failure、协议错误或 Run cancellation 不执行工具；
		// 但失败 assistant 中已完成的 calls 仍需在 transcript 中闭合。
		if turnErr != nil {
			if len(calls) > 0 {
				a.appendToolResults(failedTurnToolResults(calls, message.StopReason))
			}
			return a.snapshot(), turnErr
		}

		if len(calls) == 0 {
			// 没有工具请求就是正常最终回答；多数 Run 的最后一轮走这里。
			return a.snapshot(), nil
		}

		var results []ai.ToolResultMessage
		var runErr error
		if message.StopReason == ai.StopReasonLength {
			// 参数可能被截断：为每个 call 生成 error result，但绝不执行。
			results = truncatedToolResults(calls)
		} else {
			// 只有成功完成且确实包含 tool calls 的 assistant 才进入执行。
			results, runErr = a.executeToolBatch(ctx, calls)
		}

		// 普通工具错误已经编码在 results 中；结果按模型源顺序追加。
		a.appendToolResults(results)
		if runErr != nil {
			// 这里的非 nil error 只代表 Run-level cancellation/settlement failure。
			return a.snapshot(), runErr
		}

		// 只有完成 tool-result 回填后，才开始下一次 Provider call。
	}
}
```

上面是当前实现的责任结构摘要；具体代码还包含下一 Turn 前的 cancellation 检查。`toolUse` 却没有 tool call、重复/空 tool-call ID、长度截断、并行阶段取消、Provider error/aborted terminal 中的已完成 calls 等边界均已由确定性测试锁定。

## 已验证的 Pi 契约

- Pi 把 Turn 定义为一条 assistant response，以及它触发的 tool calls 和 tool results。
- `runLoop` 每次先接收并追加 terminal assistant；若 stop reason 是 `error` 或 `aborted`，立即结束 Run，不执行其中的工具。
- 对正常 assistant，Pi 提取全部 tool-call content blocks。执行完成后按顺序把 `ToolResultMessage` 追加到当前 context；存在可继续的 tool calls 时，下一次 Provider call 使用更新后的完整 context。
- 未知工具、参数校验异常和工具 `execute()` 抛出的异常都会被转换成 `isError: true` 的 tool result，让模型在下一 Turn 观察并恢复，而不是把普通工具错误直接抛成 Agent Loop failure。
- assistant 因输出长度限制而终止时，所有 tool calls 都被视为参数可能截断：Pi 不执行任何一个，而是为每个 call 生成同 ID 的 error tool result，再让模型重新发起完整调用。
- Pi 的 parallel 路径允许工具按实际完成顺序产生 execution-end 事件，但等待整批收敛后，按 assistant 中的 tool-call 源顺序创建并追加 tool-result messages。
- 冻结 Pi 的批次调度只有两种形状：整批 sequential，或者整批 parallel。配置要求 sequential，或批内任一工具声明 sequential 时，整批顺序执行；否则整批并行执行。
- Pi Agent Loop 对每个已准备的 tool call 只调用一次 `tool.execute()`；失败被转换成 error tool result，不在 Loop 内自动重试同一个 tool call。模型可以观察该结果并在下一 Turn 主动发起新的调用，这属于模型恢复而不是 Runtime retry。
- Pi 的 `read` 依次执行路径解析、单次 `access`、MIME 检测和单次 `readFile`，默认本地 `ReadOperations` 没有 retry/backoff。可注入 operations 理论上可以在自定义后端内部实现重试，但 core contract 不要求也不感知它。
- Pi coding-agent 的 auto-retry 和 Provider `maxRetries` 面向 retryable assistant/Provider errors，并带指数退避；它们不重放已经失败的 read。read 输出过长时提示模型用新的 offset 调用继续读取，也不是失败重试。
- Pi 的 sequential 工具批次在 signal aborted 后会结束循环，parallel 路径也会停止继续准备调用，因此取消可能让 Agent state 中的 assistant tool call 没有对应 tool result。Pi 不在 Agent Loop 补齐这些未执行调用。
- Pi 在 `packages/ai/src/api/transform-messages.ts` 的 Provider request 转换阶段扫描 orphaned tool calls，并临时插入 `isError: true`、内容为 `No result provided` 的 synthetic tool results；这些结果用于满足不同 Provider 的消息协议，不写回 Agent state。`packages/ai/test/tool-call-without-result.test.ts` 跨 Provider 验证了取消后直接追加新 user message仍可请求模型。

## 已确定的 pi-go 决策

下面是实施计划和决策记录中已经确认的 pi-go 契约，不等同于 Pi 的原始调度机制：

- `internal/agent` 拥有通用 Tool contract；后续 `internal/coding/tools` 只实现该 contract，Agent 不依赖 workspace 细节。
- Tool 对模型提供 JSON Schema，同时由具体 Tool 自己完成 Go 参数解码和语义校验；第一阶段不实现通用 JSON Schema evaluator。
- 普通工具错误形成 call-local error result，同批后续工作继续，下一轮模型观察全部有序结果；只有 Run context 取消才停止未开始的阶段。
- 调度器按模型源顺序扫描 tool calls：连续且显式 parallel-safe 的调用组成并行阶段；每个非 parallel-safe 调用各自构成串行屏障。能力默认 false，第一阶段只有 `read` 为 true。
- 并行 worker 只产生 outcome；协调层等待阶段收敛并按模型源顺序把 tool results 追加到 transcript。
- Run 取消停止新工具副作用并等待所有已启动 worker 收敛；terminal assistant 中每个 tool call 都得到一个同 ID settlement result：completed 保留实际结果，执行中记录 canceled，未启动调用不执行但明确记录 not executed。结果按模型源顺序追加后返回 context cause，不再调用 Provider。
- Provider turn 以 `error` 或 `aborted` 失败时，assistant 仍是该 Turn 唯一的 terminal assistant，其中已完成 tool calls 不执行但分别得到同 ID `not executed` results；随后直接返回 Turn error。取消后的下一次 Run 因而能直接发送完整配对历史。
- Agent Runtime 对每个 tool call 只执行一次；失败作为 tool result 交给模型恢复。第一阶段本地文件工具不重试，未来具体远程后端只有在能明确分类瞬时错误时，才可在一次 `Execute` 内部做有界、可取消的重试。

Pi 的“混合批次整体降级为串行”容易理解，但会让 `read, read, write, read, read` 五个调用全部串行。pi-go 的有意差异是把它切成三个阶段：

```text
stage 1: read 1 || read 2
barrier: write
stage 3: read 3 || read 4
```

这样既不会让 write 与 read 竞争，也保留了屏障两侧各自的只读并行。

## 取消 settlement 与 Pi 的有意分歧

学习者于 2026-07-17 选择在 Agent transcript 中显式闭合全部 tool calls，而不是复制 Pi 的 Provider request 临时修复：

```text
Run cancel
  -> 停止启动新工具副作用
  -> 取消并等待已启动 workers
  -> completed call      = 实际 result
  -> executing call      = canceled error result
  -> not-started call    = not-executed error result
  -> 按模型源顺序追加全部 results
  -> 返回 context cause，不调用 Provider
```

未启动 result 只记录 settlement 事实，不暗示工具执行过。该分歧必须由离线 Faux 测试锁定，并在第 04/06 课的真实 DeepSeek 验证中额外检查：取消后的同一 Agent 追加新 user message 时，Provider 能直接接受完整配对历史，且 Provider conversion 不静默插入或删除消息。

取消还可能发生在 Provider stream 尚未完成时。Faux 会把取消前已经完成组装的 tool calls 保留在 aborted assistant 中，因此 pi-go 同样显式闭合它们：

```text
Provider turn aborted/error
  -> append 唯一 terminal assistant
  -> 不执行其中任何 tool call
  -> 每个已完成 call 追加同 ID not-executed result
  -> 返回 Turn error，不继续调用 Provider
```

这条跨课修正保留了第 01/02 课的 assistant exactly-once settlement，同时补齐了当时尚未引入 Tool Loop、因而没有讨论到的 transcript pairing。

## 实现中已确认的边界

- `internal/agent.Tool` 通过 `Definition() ToolDefinition` 暴露冻结 schema 与 `CanRunParallel`，通过 `Execute(ctx, json.RawMessage) (string, error)` 让具体 Tool 自己解码、语义校验和执行。Agent 构造时拒绝 nil Tool、空/重复工具名和非法 schema JSON，并深复制 schema bytes。
- 单工具 timeout 由具体 Tool 在一次 `Execute` 内从 Run context 派生 child context；Agent 只观察 Run context 是否取消。因此 child timeout 是 call-local error，Run cancellation 才停止后续阶段。
- assistant 的 text/thinking 原样保留；是否继续 Tool Loop 取决于是否存在完整 tool-call content，而不只依赖 stop reason。`stop + calls` 执行，`length + calls` 全部不执行并生成 truncation results。
- `toolUse` 却没有 call、空或重复 tool-call ID 属于 Provider protocol error，malformed terminal 不进入 transcript，改为 synthetic error assistant；空 tool name、未知工具、非法 JSON 和语义错误在 ID 有效时属于 call-local error result。
- Tool definitions 在 `Agent.New` 时读取一次并冻结；Provider request、terminal assistant、tool arguments 和 RunResult 的可变 JSON/slice 都不能反向修改 Agent state。

这些边界已经进入实现和离线测试。真实 DeepSeek 对 error/aborted tool-call pairing 的接受程度仍按计划留到第 04/06 课 compatibility smoke 验证。

## 实际实现与验证范围

- `internal/agent/tool.go`
- `internal/agent/validation.go`
- `internal/agent/loop.go`
- `internal/agent/loop_test.go`
- `internal/agent/tool_loop_test.go`
- `docs/course/lessons/03-tool-loop-and-staged-scheduling.md`

默认测试使用 Faux/脚本 Provider 和受控测试 Tool，不访问网络、真实 workspace 或 Provider key。当前已经覆盖成功工具循环、未知工具、非法 JSON、语义校验失败、执行错误、单工具 timeout、长度截断不执行、错误后继续、完整下一轮 request、schema 冻结、只读并行、串行屏障、完成顺序与 transcript 顺序分离、Run 取消等待 worker 收敛、补齐 completed/canceled/not-executed results、Provider error/aborted calls 的非执行 closure，以及两类取消后同一 Agent 继续 Run。并发实现完成后运行 `go test -race ./...`；第 04/06 课再执行显式 opt-in 的真实 DeepSeek compatibility 验证。

## 提交记录

代码、离线测试、race 检查、并发/取消压力回归和课程文档审查均已完成。学习者于 2026-07-17 确认理解并明确要求直接提交到 `main`，不创建 PR；本课文件及第 01/02 课的跨课语义修正随本次 lesson 03 commit 提交。
