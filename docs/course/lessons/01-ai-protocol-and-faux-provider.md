# 第 01 课：AI 协议与 Faux Provider

## 当前状态

已提交。

开课记录：学习者于 2026-07-15 明确要求开始第 01 课，并要求后续课程保持循序渐进、讲解优先、代码贯穿；练习只作为理解检查，遇到不确定或困难的地方先讨论并确认理解。课程直接在对话中讲重点，不生成独立 HTML 讲义。

当前阶段：AI 协议与 Faux Provider 已完成讲解、Go 实现、本地验证和代码串联。学习者于 2026-07-16 确认理解并明确要求提交到 `main`，尚未进入第 02 课的实现范围。

## 本课目标

完成本课后，学习者应能解释：

1. Provider、Agent Runtime、message、content block 和 stream event 分别处在哪一层。
2. 为什么流式增量事件和最终 `AssistantMessage` 是两种不同但相关的表示。
3. 为什么 `stop`、`length`、`toolUse`、`error` 和 `aborted` 不能压缩成一个普通 Go `error`。
4. Faux Provider 为什么是 Agent Loop 的确定性测试设施，而不只是一个返回固定字符串的 mock。
5. pi-go 第一阶段需要从 Pi 保留哪些协议语义，又应删掉哪些多 Provider、图像和缓存能力。

## Pi 源码阅读路径

固定基线：Pi commit `dcfe36c79702ec240b146c45f167ab75ecddd205`，`@earendil-works/pi-ai` version `0.80.7`。

- `packages/ai/src/types.ts`
- `packages/ai/src/utils/event-stream.ts`
- `packages/ai/src/providers/faux.ts`
- `packages/ai/test/faux-provider.test.ts`
- `packages/agent/src/agent-loop.ts`
- `packages/agent/src/proxy.ts`

本课只从这些文件提取协议和测试设施的行为，不移植 Pi 的 provider registry、认证、模型发现、多 Provider options、图像生成、prompt cache 估算或随机 token chunking。

## 第一阶段讲义：先建立三层边界

一次 coding Agent 调用至少涉及三层：

```text
Agent Runtime
  负责 transcript、何时再次调用模型、何时执行工具
        |
        v
模型无关 AI 协议
  定义 request、message、content block、stream event、stop reason
        |
        v
具体 Provider
  把 DeepSeek SSE 或 Faux 脚本翻译成统一 stream
```

`internal/ai` 属于中间层。它不应该知道 workspace 文件怎样读取，也不应该知道 Agent 在收到 tool call 后怎样调度工具。反过来，`internal/agent` 不应该解析 DeepSeek SSE 字段；它只消费 `internal/ai` 提供的统一协议。

这就是为什么第 01 课先于 Agent Loop：如果协议没有先稳定，后面的 loop 测试只能依赖真实 DeepSeek 或 provider 专用 JSON，无法区分“模型传输错误”和“Agent 调度错误”。

## Message 是历史事实，Event 是形成过程

Pi 的最终 assistant message 可以同时包含 reasoning、tool call 和文本：

```ts
{
  role: "assistant",
  content: [
    { type: "thinking", thinking: "I need to inspect the file." },
    { type: "toolCall", id: "call-1", name: "read", arguments: { path: "main.go" } }
  ],
  stopReason: "toolUse"
}
```

这是可以写入 transcript 的完整历史事实。传输过程中，同一条消息可能按下面的事件逐步形成：

```text
start
thinking_start
thinking_delta("I need ...")
thinking_end
toolcall_start(index=1)
toolcall_delta('{"path":')
toolcall_delta('"main.go"}')
toolcall_end(call-1, read, {path: "main.go"})
done(reason=toolUse, final message)
```

这里最容易混淆的是：`toolcall_delta` 中的参数只是 JSON 文本分片，可能暂时不是合法 JSON；只有 `toolcall_end` 才携带完整、可执行的 tool call。Agent 不能看到第一段参数就执行工具。

因此 stream event 适合表达“正在发生什么”，最终 message 适合表达“最终发生了什么”。两者不能只保留其一：只有最终 message 会丢掉流式观察与中途取消；只有 delta 会迫使每个消费者重复组装最终结果。

## Stop reason 不是普通错误码

冻结 Pi 基线区分五种 assistant stop reason：

| Stop reason | 含义 | 是否是 Provider 失败 |
|---|---|---|
| `stop` | Provider 报告模型正常结束 | 否 |
| `length` | 输出达到长度限制，消息可能不完整 | 否，但上层必须谨慎处理 |
| `toolUse` | 完整消息要求执行一个或多个工具 | 否 |
| `error` | Provider 或流处理失败 | 是 |
| `aborted` | 调用被取消 | 不是普通失败，是独立终态 |

`toolUse` 不能表示为 `nil error` 后丢弃，因为 Agent Loop 需要据此继续；`length` 也不能和 `stop` 合并，因为截断的 tool call 不允许执行。`error` 与 `aborted` 都通过 terminal stream event 携带最终 assistant message，使调用者仍能得到已经接收的 partial content、模型信息和错误说明。

## Pi stream 的已验证协议

从冻结源码和测试可以确认：

1. stream 在内容更新前发出 `start`。
2. text、thinking 和 tool call 分别拥有 start、delta、end 事件，并通过 `contentIndex` 对应最终 content block。
3. stream 最终必须以 `done` 或 `error` 之一结束；成功 terminal reason 只允许 `stop`、`length`、`toolUse`，错误 terminal reason 只允许 `error`、`aborted`。
4. Provider 调用一旦返回 stream，请求、模型和运行时失败应编码到该 stream，不应在异步生产过程中另走 throw/reject 通道。
5. terminal event 同时结束事件迭代并解析最终 `AssistantMessage`；terminal event 本身仍会被消费者读到。
6. 取消可发生在第一个 chunk 之前或任意 block 中途；结果是 terminal `error(reason=aborted)`，被中断 block 不会再发自己的 end event。

第 5 点对应 Pi `EventStream.push()` 的一个重要顺序：它先把 stream 标为完成并保存 final result，然后仍把 terminal event 放入队列或交给等待者。也就是说，“terminal”不是“消费者看不到的内部 EOF”。

## Faux Provider 解决的真实问题

`faux` 读作接近英文 `foh`，意思是“仿制的、人造的”。Faux Provider 就是一个测试专用的假模型提供方。它不是 transcript 中的新 message role，也不是 Agent 的特殊运行状态；Agent 仍然只处理 user、assistant 和 tool result。Faux 只是测试时注入到同一个 Provider 接口中的具体实现。

生产和测试的依赖关系分别是：

```text
生产：Agent Loop -> Provider 接口 -> DeepSeek Provider -> 网络模型
测试：Agent Loop -> Provider 接口 -> Faux Provider    -> 预写脚本
```

只要两者实现相同的 Provider 协议，Agent Loop 就不需要知道当前连接的是真实模型还是 Faux。Agent 不允许根据 provider 名称走不同的 tool-loop、transcript 或取消逻辑。

Faux Provider 不是为了假装联网。它把原本不稳定的模型输出变成可编排输入，让测试能精确构造：

- reasoning、text 和 tool call 的组合；
- tool-call JSON 参数分片；
- 正常 `done`、Provider `error` 和 context cancel；
- 多次 Provider 调用按脚本顺序返回不同结果；
- 脚本耗尽时的明确失败，而不是静默重复最后一个结果。

例如，第 02 课可以给 Faux Provider 两次脚本响应：第一次要求 `read(main.go)`，第二次返回最终文本。这样测试的是 Agent 是否把 tool result 放回 transcript 并发起第二次 Provider 调用，不需要 DeepSeek key，也不受模型随机性影响。

在这个例子中，Faux 不负责执行 `read`。它只扮演模型，说“请调用 read”；真正查找并执行工具的是 Agent，执行结果也由 Agent 写回 transcript。第二次调用 Faux 时，它再扮演模型给出下一步响应。因此 Faux 验证的是 Agent 面对确定模型输出时的行为，不验证模型本身是否聪明。

冻结 Pi 自带 Faux Provider。它的脚本单元主要是完整 `AssistantMessage` 或 response factory，再由 Faux 实现生成 delta。pi-go 当前计划强调“脚本事件”，目的是让事件顺序、参数分片和取消点完全确定；这是一项候选的 Go 测试机制，不应误称为必须复制的 Pi API 形状。

## 第一阶段保留与删除

### 已确定要保留的语义

- user、assistant、tool result 三种 transcript message 角色；
- text、reasoning/thinking 和 tool call 内容块；
- usage、stop reason、错误信息和时间信息中后续 loop 真正会观察的部分；
- start/delta/end/terminal stream 生命周期；
- 可取消的 pull/receive 边界；
- 可脚本且默认离线的 Faux Provider。

### 当前明确不照搬的部分

- Pi 的 TypeScript discriminated union 和 `AsyncIterable` API 形状；
- Provider registry、认证、模型发现和 provider-specific options；
- 图像输入与图像生成；
- prompt cache 估算、随机 token 大小和模拟吞吐速度；
- DeepSeek 专用 SSE 字段，这些留到第 04 课。

## 候选 Go 形状：现在只用于理解

下面的代码不是已确定实现，只把责任边界翻译成 Go 便于讨论：

```go
type StopReason string

const (
	StopReasonStop    StopReason = "stop"
	StopReasonLength  StopReason = "length"
	StopReasonToolUse StopReason = "tool_use"
	StopReasonError   StopReason = "error"
	StopReasonAborted StopReason = "aborted"
)

type Provider interface {
	Stream(ctx context.Context, req Request) Stream
}

type Stream interface {
	Receive(ctx context.Context) (Event, error)
}
```

这个草图表达两个已确定方向：Provider 依赖 `context.Context`，Agent 主动从 stream 拉取事件。它还没有回答三个困难问题：

1. Go 中 content block 和 event 应使用“带 kind 的单一 struct”，还是用多个具体类型组成受约束 interface？
2. terminal event 之后重复 `Receive` 应统一返回 `io.EOF`，还是保存并重复暴露 terminal 状态？
3. Faux 脚本的最小单元应是完整 event 序列、完整 assistant message，还是同时提供一个核心表示和便捷构造器？

这些问题会在写测试前讨论清楚。当前不能仅因为 TypeScript 使用 union 和 async iterator，就机械选择最像它的 Go 写法。

## 当前讨论点

第一轮先确认这个核心心智模型：**最终 message 是 transcript 事实，stream event 是该事实的形成过程，Faux Provider 则是可确定性制造这一过程的测试设施。**

理解确认后，下一步会对照实际 Go 调用代码讨论上述三个接口问题，再先写 Faux Provider 的失败测试，不直接从实现开始。

### 01-A 理解确认：取消发生在未完成 tool call 中途

- 场景：stream 已发出 `toolcall_start` 和不完整的 `toolcall_delta('{"path":')`，随后 Run 被取消。
- 学习者判断：不能执行 `read`；Run 应停止，只保留一条 aborted assistant message。
- 课程结论：核心判断正确。没有 `toolcall_end` 就没有完整、可执行的 tool call，因此不能开始 `read`，这个未完成调用也不会进入 aborted assistant 或生成 tool result。“只保留一条 aborted assistant”约束的是 assistant 不能重复追加，不代表该 assistant 中其他已完成 tool calls 永远不能追加非执行型 settlement results。
- 关键修正：取消是 cooperative cancellation request，不是立即清空状态或回滚。Provider、已启动工具和事件处理仍需观察取消并完成清理，Run 只有在这些工作 settlement 后才真正结束。
- Pi 源码事实：Faux stream 在中途取消时发出 terminal `error(reason=aborted)`；其中的 final assistant message 可以保留取消前已经形成的 partial content。Agent Loop 用该 final message 替换正在形成的 partial message，将它保留在当前内存 context，然后以 aborted 结束而不进入工具执行。
- pi-go 已定边界：取消后不启动新的 Provider 调用或工具阶段，等待所有已启动工作收敛，再产生唯一 canceled Run 终态。取消不撤销此前已经完成的文件副作用。第 03 课补充确认：若 aborted assistant 保留了取消前已经组装完成、但尚未执行的 tool calls，Agent 在这唯一一条 assistant 后为它们追加同 ID 的 `not executed` error results，再返回取消原因；这些 result 只闭合 transcript，不表示执行过工具。

### 01-A 补充讲解：Faux 在 Agent 中的位置

- 学习者问题：`Faux` 是什么意思，还不清楚这个语义在 Agent 中的作用。
- 课程纠正：此前直接讲 Faux 的事件能力，缺少了它与 Provider 接口、真实 DeepSeek 和 Agent Loop 的责任关系。
- 核心结论：Faux 是测试专用 Provider 实现，不是 message role。它按预写脚本产生模型响应；Agent 仍负责 transcript、工具执行和继续循环。
- 验证边界：Faux 能证明给定确定输入时 Agent Loop 的事件、工具、错误、取消和 transcript 行为；它不能证明 DeepSeek 网络协议可用，也不能证明真实模型会选择正确工具。
- 理解进度：学习者确认该定位并要求继续讲解。

### 01-B 讲解：Provider request、Message 与 Content Block

Provider 边界传递的不是一个孤立 prompt，而是当前模型调用所需的完整快照：包含 workspace/cwd 说明的稳定 system prompt、有序 transcript 和当前 tool schemas。原始 task 已经是 transcript 的首条 user message。Agent 拥有并更新 transcript；Provider 只读取 request 并产生 assistant stream，不能替 Agent 修改历史或执行工具。

transcript 由三类 message 组成：

- user message：用户输入；
- assistant message：模型生成的有序 content blocks、usage 和 stop reason；
- tool result message：通过 `toolCallID` 对应此前 tool call，并带有内容与 `isError`。

assistant content 使用有序 blocks，而不是独立的 `Text`、`Thinking`、`ToolCalls` 字段，因为 reasoning、text 和多个 tool call 的源顺序是协议的一部分；stream 的 `contentIndex` 也必须稳定指向最终 block。

当前待讨论的 Go 机制：

1. 用带 `Kind` 和多个可选字段的单一 struct，还是用受约束 interface 加 `TextContent`、`ThinkingContent`、`ToolCall` 具体类型表达 content union；
2. tool-call arguments 用 `map[string]any` 保存解析后对象，还是用 `json.RawMessage` 保存完整 JSON，等具体 Tool 做强类型解码与校验。

这两项是 Go 表达机制，不直接由 Pi 的 TypeScript union 形状决定；选择必须同时考虑非法状态、JSON 编解码、调用代码清晰度和后续 Tool validation 责任。

- 理解进度：学习者确认继续，进入 stream 的 Go 边界。

### 01-C 讲解：Stream terminal、EOF 与取消

stream 是 pull-based 状态机，而不是让 Provider 回调 Agent：

```text
created -> start/updates -> done|error -> EOF -> EOF ...
```

`done` 和 `error` 是协议内 terminal event，携带最终 assistant message；`io.EOF` 只表示 terminal event 已经被读走、此后没有更多事件。重复读取结束后的 stream 必须稳定返回 `io.EOF`，不能重复 terminal event、阻塞或 panic。

当前更合适的候选 Go 边界是创建 stream 时绑定 Run context，读取时不再传另一个会抢先返回的 context：

```go
type Provider interface {
	Stream(ctx context.Context, request Request) Stream
}

type Stream interface {
	Receive() (Event, error)
}
```

原因：如果 `Receive(ctx)` 直接在 `ctx.Done()` 时返回 `context.Canceled`，Agent 可能来不及读取 Provider 根据 partial content 形成的 terminal aborted event。让 stream 自创建时持有 Run context，Provider 观察取消、完成清理并产出一次 aborted terminal，Agent 再读到 EOF，可以保持单一完整终态。

这仍要求防御 Provider 违反契约的情况。正常 Provider 网络、解析和取消失败都应转成 terminal error event；`Receive` 返回的非 `io.EOF` error 只保留给 stream 自身无法继续履约的意外故障，Agent 需要把它归一化为 Provider failure，不能把 Run 留在无 terminal 状态。

Faux 可以先用无后台 goroutine 的 slice-backed stream：每次 `Receive` 返回脚本中的下一个 event，并在每个读取边界检查创建时绑定的 context。测试在读到一个 delta 后调用 cancel，下一次 `Receive` 应返回唯一 aborted terminal，再后续始终返回 `io.EOF`。这比依赖计时器或网络竞争更确定，也避免测试设施自身产生 goroutine 泄漏。

- 理解进度：学习者确认继续，进入第一阶段完整类型草图和删减审查。

### 01-D 讲解：完整类型草图与第一阶段删减

第一阶段协议只需要覆盖 Agent Loop 会观察的内容：

- Request：包含 workspace/cwd 说明的 system prompt、有序 transcript、tool schemas；原始 task 是 transcript 的首条 user message；
- Message：user、assistant、tool result；
- Assistant content：text、thinking、tool call 的有序 blocks；
- Result metadata：usage、stop reason、error message；
- Stream：start、各类 block 的 start/delta/end、done/error terminal 和 terminal 后稳定 EOF；
- Provider：创建绑定 Run context 的 pull stream。

候选 Go 机制是 message/content/event 使用包内受约束的具体类型，tool-call arguments 使用 `json.RawMessage`，Provider call 不额外返回同步 error；构造或配置错误也返回一个 terminal error stream，从而避免 Agent 同时处理两套 Provider 失败通道。

当前删掉 Pi `types.ts` 中第一阶段没有消费者的能力：image content 与 image generation、Provider registry 和多 API options、认证/header/cache/session affinity、费用估算、provider-specific signature/diagnostics、持久化 timestamp 以及公共扩展 API。`Usage` 只保留 token 计数，不引入价格模型。

仍需在实现前共同确认的边界：

1. assistant message 是否保留 model ID 作为诊断元数据，还是只由具体 Provider 配置持有；
2. usage 的 cache/reasoning 细分是否在第 01 课先定义，还是等第 04 课核对 DeepSeek 官方字段后添加；
3. event 是否保留 Pi 每次携带完整 `partial` snapshot 的形状，还是只携带 delta、index 和 terminal final message，由 Agent 自己维护 partial。

课程当前倾向最小方案：不在每个 event 重复 partial snapshot，不预定义未核验的 cache/reasoning usage 字段，model ID 是否进入 message 留到查看第 04 课 trace 需要后再定。这些是范围建议，尚未形成 pi-go 决策。

### 01-D 补充讨论：`WorkspaceContext` 的责任

- 学习者问题：候选 `Request` 中的 `WorkspaceContext string` 是做什么的。
- 语义作用：它是发送给模型的当前工作环境说明，例如文件路径应相对选定 workspace、bash 从 workspace 启动，以及哪些 workspace 内容可能进入模型上下文。它帮助模型正确选择路径和工具参数。
- 非作用：它不执行路径校验，也不是 sandbox 或权限边界。文件工具的真实边界由后续 `os.Root` 实现，bash 的真实行为由 executor 的 cwd、环境和进程管理实现；提示文字不能替代这些约束。
- Pi 源码事实：冻结 Pi 的 AI `Context` 只有 `systemPrompt`、`messages` 和 `tools`，不存在 `WorkspaceContext`；coding-agent 的 `buildSystemPrompt()` 把 `Current working directory: ...` 直接追加到 system prompt。
- 课程纠正：此前展示的 `WorkspaceContext string` 是未经充分标注的 pi-go 候选，不是 Pi 字段，也没有必要成为独立 AI 协议字段。删除该候选；coding composition root 负责在 Run 开始时把 workspace/cwd 说明组装进稳定 system prompt，每轮 Provider 调用复用同一内容。
- 测试边界：Faux 记录完整 request，后续测试断言每轮 system prompt 不变且包含预期 workspace/cwd 说明；不再断言独立 `WorkspaceContext` 字段。

### 01-E 讲解与待确认修正：原始 `Task` 是否属于 Provider Request

- Pi 源码事实：冻结 Pi 的 `runAgentLoop()` 把传入 prompts 追加到 `context.messages`；每次模型调用再从当前完整 messages 构造 AI `Context { systemPrompt, messages, tools }`。没有独立 task 字段。
- pi-go 当前计划：此前要求每次 Provider 调用同时携带原始 task 和完整 transcript。这不是 Pi 字段，而是早期计划中的 pi-go 候选契约。
- 风险：第一阶段不做 compaction，原始 task 已经是 transcript 中的第一条 user message。再保存独立 `Task` 会建立两个事实源；二者不一致时 Provider 应相信哪一个没有可靠答案。
- 分层建议：Run/coding 入口仍接收 `task string`，但 Agent 只在 Run 开始时把它转换成一次 `UserMessage`；AI Provider request 保持 `SystemPrompt + Messages + Tools`。以后若引入 compaction，再由那一课明确如何长期保留原始任务，不为未来需求提前复制字段。
- Faux 测试：断言首次 request 的 messages 包含原始 user message，后续 request 的完整 transcript 仍以同一 user message 开始；不需要独立 `Request.Task`。
- 学习者确认：2026-07-16 接受该修正。已同步 `AGENTS.md`、实施计划和 D13；Provider Request 不再包含独立 `Task`。

### 01-F 讲解：`AssistantMessage` 字段删减

冻结 Pi 的 `AssistantMessage` 除了 `content`、`usage`、`stopReason` 和 `errorMessage`，还包含 `api`、`provider`、`model`、可选 response model/ID、diagnostics 与 timestamp。这些是 Pi 多 Provider、路由、诊断、Session/UI 生态的一部分，不等于第一阶段 Agent Loop 的必需字段。

当前建议保留：

- `Content []AssistantContent`：Agent 查找 text、thinking 和 tool calls；
- `Usage`：Faux 和 DeepSeek Provider 的统一 token 结果；
- `StopReason`：决定正常结束、tool loop、截断失败、Provider error 或 aborted；
- `ErrorMessage string`：为 error/aborted terminal 提供稳定、可比较的说明。

当前建议删除或推迟：

- `api`、`provider`、`model`、response ID/model：第一阶段只有一个由 composition root 配置的真实 Provider/模型，Agent Loop 不按这些字段分支；真实 smoke test 可以从 Provider 配置记录模型 ID；
- diagnostics：第一阶段没有通用 provider diagnostics/redaction 协议；
- timestamp：transcript 不持久化，当前事件顺序由有序流定义，不依赖消息时间排序；
- usage cost：当前不实现价格表或费用核算。

`Usage` 第一版建议只存 `InputTokens` 和 `OutputTokens`，用方法派生 `TotalTokens()`，避免在当前语义下保存可漂移的重复总数；cache 和 reasoning 细分等第 04 课重新核对 DeepSeek 官方协议后再决定。Faux 默认可使用零值或测试显式脚本值，不复制 Pi 的随机 token 估算。

关于 stream partial snapshot 的复查：Pi 每个内容事件都携带完整 partial message；pi-go 当前 headless 范围没有需要完整 partial snapshot 的 UI 消费者。Provider 负责累计并在 terminal event 交付权威 final/aborted message，因此当前仍建议不在每个 delta 中复制 partial message。第 02 课后来确认不设计 Agent event sink；等本地入口出现真实展示消费者时，再以该消费者为证据决定观察协议。

- 学习者确认：2026-07-16 接受第一阶段 `AssistantMessage` 只保留 `Content`、`Usage`、`StopReason` 和 `ErrorMessage`，并接受本节列出的字段删减与推迟范围。

### 01-G 讲解：User、Tool Result 与 Tool Schema

冻结 Pi 的 user message 支持 text/image blocks 和 timestamp；第一阶段只有一个文本任务且不实现多模态，因此候选 `UserMessage` 只保留 `Content string`。

冻结 Pi 的 tool result 还包含 text/image blocks、provider/UI details、动态新增工具名和 timestamp。第一阶段候选只保留 `ToolCallID`、`ToolName`、文本 `Content` 和 `IsError`：ID 关联原 tool call，name 支持诊断和部分 Provider 转换，content 发送给模型，isError 保留 call-local failure 语义。工具特有的结构化 outcome 可以存在于 Agent 执行事件中，不进入 AI transcript message。

Go concrete message type 本身已经确定 role，因此当前建议 `Message` 使用包内受约束 interface，不再额外存一个可与实际类型冲突的 `Role` 字段或 enum。Provider converter 通过 type switch 映射为 wire role。

Tool schema 保留 `Name`、`Description` 和 JSON Schema `Parameters`；候选使用 `json.RawMessage`，由 Agent Tool 提供有效 schema，具体 Provider 只负责映射到自己的 wire format。动态工具加载、TypeBox 和通用 JSON Schema evaluator 均不进入第 01 课。

- 理解进度：学习者确认继续，进入 stream event 状态机与 Faux 测试矩阵。

### 01-H 讲解：Stream event 合法顺序

第一阶段 event 集合保留 Pi 的生命周期语义，但不在每个事件重复 partial message：

- stream：`Start`；
- text：`TextStart`、`TextDelta`、`TextEnd`；
- thinking：`ThinkingStart`、`ThinkingDelta`、`ThinkingEnd`；
- tool call：`ToolCallStart`、`ToolCallDelta`、`ToolCallEnd`；
- terminal：`Done` 或 `Error`。

Start/Delta/End 使用 `ContentIndex` 对应最终 assistant content；text/thinking end 携带完整 block 内容，tool-call start 携带 ID/name，delta 携带 argument JSON 片段，end 携带完整 `ToolCall`。Done/Error 只携带最终 `AssistantMessage`，不再重复存一个可能与 message `StopReason` 冲突的 reason 字段。

合法状态机：有内容的正常流先 Start，再产生零个或多个完整 block 生命周期，最后恰好一个 Done；运行期故障可在 Start 前或任意中间点产生唯一 Error。terminal 必须是消费者能读到的最后一个 event，之后每次 Receive 都稳定返回 `io.EOF`。EOF 前没有 terminal、terminal 后仍有 event、同一 block delta 早于 start、重复 block end 或 Done 携带 error/aborted reason 都是协议违例。

Faux 需要区分两类耗尽：Provider 已没有下一次 scripted response 时，返回一次正常协议内 Error terminal；单个测试 Step 自身缺 terminal 或 terminal 后还有事件，则是测试脚本配置错误，应在构造/设置脚本时提前拒绝，而不是运行时伪装成模型错误。

取消规则：创建 stream 前 context 已取消时，首次 Receive 直接返回一次 aborted Error，再后续 EOF；中途取消时丢弃尚未读取的脚本事件，下一次 Receive 返回携带已形成 partial content 的 aborted Error；terminal 已读后再取消不改变 EOF 状态。

首批测试矩阵将覆盖纯文本、thinking、tool-call JSON 分片、多 block 顺序、正常 Done、显式 Provider Error、开始前取消、中途取消、response queue 耗尽、非法 Step、terminal 后重复 Receive 和 terminal reason/message 不一致。

### 01-H 补充讲解：partial text/thinking 的含义

- 学习者反馈：接受 aborted message 丢弃未完成 tool call 的建议，但不确定 partial text/thinking 指什么。
- 术语澄清：partial 不是新的 content 类型，只是取消前已通过若干 delta 收到的字符串前缀。例如预期完整文本是 `I found the bug in main.go`，取消前只收到 `I found the bug`，后者就是 received-so-far partial text。
- final 表达：aborted assistant message 仍使用普通 `TextContent`/`ThinkingContent` 保存这些字符串，并由 message 级 `StopReasonAborted` 表明整个响应未完整结束；不增加每个 block 的 partial flag。
- 与 tool call 的区别：任意 text/thinking 前缀都是合法字符串，可以安全保存但不能当作完整回答；tool-call argument 前缀可能不是合法 JSON，不能构造成完整、可执行 `ToolCall`，所以未到 `ToolCallEnd` 的 block 从 final aborted content 中丢弃。已经到达 `ToolCallEnd` 的调用可以保留在 aborted assistant 中，但因 Provider turn 已失败而绝不执行；第 03 课的 Agent 会为它追加 `not executed` settlement result，避免后续 transcript 出现 orphaned call。
- Pi 对照：冻结 Pi 的 Faux 在 text/thinking delta 时持续把 chunk 追加到 partial message，取消时用当前 partial 构造 aborted message。pi-go 保留该 received-so-far 语义，同时对未完成结构化 tool call 采用更严格的丢弃规则。

- 学习者确认：2026-07-16 理解 partial 是取消前 received-so-far 的字符串前缀，并接受 aborted final message 保留 partial text/thinking 与已完成 tool calls、丢弃未完成 tool call 的处理。2026-07-17 在第 03 课实现审查中补充确认：保留已完成 tool call 不等于执行它；Agent 通过后续同 ID `not executed` result 显式闭合该调用，同时仍只追加一条 aborted assistant。

### 01-I 讲解：测试优先与 Faux API 边界

第 01 课实现从 public behavior tests 开始，不先写 stream queue。候选 Faux API：`New(steps ...Step) (*Provider, error)` 在构造时拒绝非法 Step；`Provider.Stream(ctx, request)` 每次消费一个 Step 并记录 request snapshot；`Requests()` 供后续 Agent 测试断言完整上下文；没有更多 Step 时返回一个协议内 Error stream。

一个 Step 是一次 Provider 调用的完整 event 序列。第一版不加入 response factory、动态 set/append、随机 chunk、模拟 token speed 或 builder DSL；多轮测试通过多个确定 Step 表达。

测试分三层：

1. 消息与 schema 类型：有序 content、Raw JSON arguments、派生 usage total；
2. stream 状态机：各 block 生命周期、terminal、EOF、非法顺序；
3. Faux Provider：按调用消费 Step、记录 request、queue exhaustion、开始前/中途取消和 aborted partial content。

建议实现顺序：先定义最小 AI value types，让测试编译；再写 Faux public tests 使其因缺行为失败；实现 Step validation；实现 slice-backed Receive 与 terminal/EOF；最后实现取消时的 partial collector。每完成一小段都运行对应 package test，全部完成后再运行 `go test ./...`、`go vet ./...` 和取消相关的 `go test -race ./...`。

### 01-J 实现：从协议类型到可取消 Faux stream

实现遵循了测试优先顺序。先创建 `internal/ai/faux/provider_test.go`，第一次聚焦运行因 `internal/ai` 尚不存在而失败；补入协议 value types 后，第二次运行因 Faux package 没有实现而失败。随后才实现 Step validation、stream、取消时 partial collector 和 request snapshot。

实际 Provider 边界为：

```go
type Request struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolSchema
}

type Provider interface {
	Stream(ctx context.Context, request Request) Stream
}

type Stream interface {
	Receive() (Event, error)
}
```

这里没有独立 `Task` 或 `WorkspaceContext`；Run 在第 02 课把 task 转成首条 `UserMessage`，coding composition root 将 workspace/cwd 说明组装进 `SystemPrompt`。`Stream` 在创建时绑定 context，所以取消后的下一次 `Receive()` 可以返回权威 `ErrorEvent{StopReason: aborted}`，再从后续读取返回稳定 `io.EOF`。

`Message`、`AssistantContent` 和 `Event` 都使用 `internal/ai` 包内受约束 interface 加具体 value type。具体类型已经确定 user/assistant/tool-result 或 text/thinking/tool-call 语义，因此没有增加可能与具体类型冲突的 `Role`、`Kind` 字段。tool-call arguments 和 tool schema parameters 使用 `json.RawMessage`；AI 层保留 JSON，具体 Tool 在后续课程负责强类型解码和运行时校验。

Faux 的实际调用边界为：

```go
provider, err := faux.New(
	faux.Step{Events: firstResponseEvents},
	faux.Step{Events: secondResponseEvents},
)

stream := provider.Stream(ctx, request)
event, err := stream.Receive()
recorded := provider.Requests()
```

`New` 会深拷贝并提前拒绝非法 Step；`Stream` 每次按调用顺序消费一个 Step，并保存独立 request snapshot；`Requests` 再返回独立副本，测试或调用方后续修改 slice/Raw JSON 不会污染历史记录。队列耗尽是运行期 Provider 情况，因此形成一次 `ErrorEvent(error)`；Step 缺 terminal、terminal 后仍有 event、delta 早于 start、end 与 delta 不一致或 terminal message 与流式内容不一致是脚本配置错误，因此由 `New` 返回 error。

实现时补严了一个从既有语义直接推导出的不变量：stream 中已开始的 `ContentIndex` 必须从 0 连续出现；正常 Done 时，它们逐一对应最终 `AssistantMessage.Content` 的位置。只产生 index 1、终态却只有一个 content block 的脚本会在构造时被拒绝。Error/aborted 是特殊情况：未完成 tool call 会从终态 message 中删除，剩余有效 blocks 保持原相对顺序并紧凑排列，因此原 `ContentIndex` 只是形成过程中的坐标，不会保存为最终 block 字段。

取消时的 partial collector 只保存当前已接收的字符串前缀和已经到达 `ToolCallEnd` 的完整 tool call：

```text
未完成 tool call(index 0, '{"path":') -> 丢弃
text delta(index 1, "working") -> 保留并成为终态 content[0]
完整 tool call(index 2) -> 保留并成为终态 content[1]
terminal -> ErrorEvent(aborted)
下一次 Receive -> io.EOF
```

Faux 不执行任何 tool；第 02/03 课的 Agent 读取 terminal assistant message，只有正常 `toolUse` 且 tool call 完整时才可能进入工具执行。`aborted` message 即使保留了已完成 tool call，也会因为 message 级 stop reason 阻止执行。

实现后的简化检查删除了 partial collector 对未完成 tool JSON 的无用累计、text/thinking end 时的重复字符串写入，以及 Step 在每次创建 stream 时的第二次拷贝；保留了构造时和对外返回时的必要深拷贝。text、thinking、tool-call 的校验分支没有强行合并，因为各自的错误信息和完成条件不同，显式状态更容易审查。

验证结果（2026-07-16）：

- `gofmt -w internal/ai/message.go internal/ai/model.go internal/ai/stream.go internal/ai/faux/provider.go internal/ai/faux/provider_test.go`：通过；
- `go test ./...`：通过；
- `go vet ./...`：通过；
- `go test -race ./...`：通过；
- 实现后手工审查补充了 thinking partial cancel、terminal content/delta 一致性，以及“前置未完成 tool call 被删除后剩余 blocks 紧凑排列”的回归覆盖；
- 默认测试没有网络访问、Provider key 或付费 API。

### 01-K 补充讨论：`ContentIndex` 的实际消费者与封装边界

- 学习者问题：未来 Agent Loop 会用 `ContentIndex` 做什么；如果流式组装已经封装在 Provider 内部，是否还应把该字段暴露给 Agent。
- Pi 源码事实：冻结 Pi 的普通 Agent Loop 不使用 `contentIndex` 定位或组装 content block。每个内容事件已经携带 Provider 组装好的完整 `partial` message；Agent Loop 只用 `event.partial` 替换当前 partial message，并把原事件作为 `message_update` 的附加信息继续转发。
- 工具执行事实：Pi 等 terminal final message 形成后，从 `message.content` 中筛选 `toolCall` blocks，再按 tool call 的 ID、name 和 arguments 执行；`contentIndex` 不是 Agent 的工具选择、调度或关联字段。
- 已验证的真实消费者：Pi Provider adapter 在构造流式 message 时维护有序 content blocks；Pi Proxy 为减少网络流量会移除每个事件携带的完整 `partial`，客户端随后使用 `contentIndex` 把 text、thinking 或 tool-call delta 写回正确的 `partial.content` 位置。这里的 `contentIndex` 是流式内容重建坐标，而不是 Agent 决策字段。
- pi-go 当前实现：事件没有重复携带完整 partial snapshot，`ContentIndex` 目前仍暴露在通用 `ai.Event` 上；Faux 内部用它校验 block 生命周期，并在取消时组装 received-so-far 内容。尚未实现的 Agent Loop 没有已经证明的业务用途。
- 学习者方向：第一阶段如果始终只有 Provider/stream 实现需要 block 坐标，应优先把该坐标封装在实现内部，不向 Agent 或其他外部消费者暴露无用协议细节。
- 暂不定案：不在第 01 课仅凭“当前 Agent 不使用”立即删除字段。第 02 课已确认基础 transcript loop 不实现 Agent event sink；第 04 课实现 DeepSeek SSE 映射时检查是否只有 adapter 内部需要路由，后续本地入口设计流式展示时再检查观察者是否需要 block 坐标。若两处都不需要外部坐标，再把 `ContentIndex` 收回 Provider 私有状态；若观察者确实需要独立重建多个 blocks，则保留或改成由真实消费者驱动的更明确标识。

### 01-L 收尾确认：错误边界、所有权与完整调用链

- 配置错误与运行期错误：非法 Faux Step 是测试作者在 Provider 构造前制造的配置错误，由 `New()` 直接返回 Go error；Provider 已接受一次调用后发生的队列耗尽、失败或取消，则形成调用方可读取的 terminal `ErrorEvent`。terminal 交付后的 `io.EOF` 只表示没有更多事件，非 EOF 的 `Receive` error 保留给无法形成可信协议事件的读取机制故障。
- 数据所有权：`New()` 深拷贝脚本，`Stream()` 深拷贝调用时的 Request snapshot，`Requests()` 再返回独立副本，事件交付时也复制嵌套的 `json.RawMessage`。调用方传入的数据、Faux 保存的数据和调用方取回的数据不共享可变 slice；mutex 保护并发访问，深拷贝解决跨边界别名，两者职责不同。
- 完整路径：学习者结合纯文本测试逐步确认了 `Step -> New/validate -> Provider.Stream/consume -> Stream.Receive -> Start/Delta/End -> Done|Error -> io.EOF`。正常 final message 来自 terminal event；partial collector 只在取消时用 received-so-far 内容构造 aborted final message。
- 理解确认：学习者于 2026-07-16 确认第 01 课讲解完成，并要求提交到 `main` 后开始第 02 课。

## 本课文件范围

计划中的实现文件：

- `internal/ai/message.go`
- `internal/ai/model.go`
- `internal/ai/stream.go`
- `internal/ai/faux/provider.go`
- `internal/ai/faux/provider_test.go`
- `docs/course/lessons/01-ai-protocol-and-faux-provider.md`

以上 Go 文件和测试均已创建并随第 01 课提交；根 README、课程总纲、决策记录和计划中开课前发现的 `Task`/`WorkspaceContext` 冲突也已同步修正。

## 提交记录

学习者已确认理解并明确要求提交到 `main`；本课文件随本次 lesson 01 commit 提交。
