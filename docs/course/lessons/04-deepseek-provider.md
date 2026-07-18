# 第 04 课：OpenAI-Compatible DeepSeek Provider

## 当前状态

已提交。

开课记录：第 03 课已于 2026-07-17 完成并提交到 `main`。学习者随后明确要求继续推动课程，并确认第 04 课采用 OpenAI-compatible 方式接入 DeepSeek，参考冻结 Pi 的 Provider 抽象。课程先讲清 Provider/API 分层、消息回放和 SSE settlement，再进入实现。

## 本课目标

完成本课后，学习者应能解释并从测试中验证：

1. `ai.Provider`、OpenAI-compatible Chat Completions 协议层和 DeepSeek 配置层各自负责什么。
2. Agent 的 `Request` 如何转换为 `system`、`user`、`assistant` 和 `tool` wire messages，又如何保留 tool-call 配对。
3. SSE 的 text、`reasoning_content`、并行 tool-call argument 分片和 usage chunk 如何形成一条权威 `AssistantMessage`。
4. 为什么 reasoning/tool-call turn 的 `reasoning_content` 必须在后续请求中回放，而不能只把 reasoning 当作终端显示文本。
5. HTTP failure、畸形 SSE、未知 `finish_reason`、流提前结束和 context cancellation 如何映射为统一 stream terminal。
6. 为什么默认离线 fixture 和显式真实 smoke test 互不替代。

## 证据边界

冻结 Pi 基线：commit `dcfe36c79702ec240b146c45f167ab75ecddd205`，Pi Agent package version `0.80.7`。

冻结源码阅读路径：

- `packages/ai/src/models.ts`
- `packages/ai/src/providers/deepseek.ts`
- `packages/ai/src/providers/deepseek.models.ts`
- `packages/ai/src/api/openai-completions.lazy.ts`
- `packages/ai/src/api/openai-completions.ts`
- `packages/ai/src/api/transform-messages.ts`
- `packages/ai/src/types.ts`
- `packages/ai/test/openai-completions-tool-choice.test.ts`

当前官方协议资料于 2026-07-17 核对：

- [DeepSeek API documentation](https://api-docs.deepseek.com/)
- [Chat Completion API](https://api-docs.deepseek.com/api/create-chat-completion)
- [Thinking Mode](https://api-docs.deepseek.com/guides/thinking_mode)
- [Tool Calls](https://api-docs.deepseek.com/guides/tool_calls)

冻结 Pi 说明“基线当时怎样适配”；当前官方文档说明“今天的 DeepSeek endpoint 接受什么”。模型别名和兼容细节可能变化，因此模型 ID 与 endpoint 保持运行配置，不写成长期课程契约。

## 已验证的 Pi 分层

冻结 Pi 并没有在 `deepseek.ts` 中重新实现 HTTP 和 SSE：

```text
deepseekProvider()
  注册 id / base URL / auth / model metadata
            |
            v
openAICompletionsApi()
  复用 Chat Completions 消息转换与流解析
            |
            v
统一 AssistantMessageEventStream
```

Pi 的完整 `Provider` 还拥有模型目录、认证解析、动态刷新和按 API dispatch。这些能力服务于多 Provider 产品，不是 DeepSeek 线协议本身。pi-go 增加 `internal/ai/provider/` 作为具体实现的归类目录，但不把它做成一个新的 Go `provider` 抽象包。consumer-owned `ai.Provider` 继续和 `Request`、`Stream` 放在 `internal/ai`，第一阶段只增加两个对当前目标有直接责任的实现层：

```text
internal/ai/provider/deepseek
  DeepSeek config + auth + endpoint policy + compatibility profile
            |
            v
internal/ai/provider/openaicompatible
  Chat Completions messages + HTTP/SSE + response assembly
            |
            v
internal/ai
  model-neutral Request / Message / Event / Provider
```

`internal/agent` 仍然只依赖 `internal/ai`，不会看到 DeepSeek JSON、SSE 或 API key。`provider/` 在 Go import 关系中只是实现目录：Faux、OpenAI-compatible 和 DeepSeek 都向内实现 `ai.Provider`，不会反过来让 `ai` 依赖具体实现。

这与 Pi 有两层明确差别：

1. Pi 的 `Provider` 是拥有 id/name、base URL、auth、模型列表、刷新和 stream dispatch 的大运行单元；pi-go 的 `ai.Provider` 目前只有 `Stream(ctx, Request) Stream`，是 Agent 所需的最小端口。
2. Pi 通过 `createProvider()` 和 `Models` 运行时管理多个 Provider；pi-go 第一阶段由 composition root 直接构造 DeepSeek 并注入 Agent，`internal/ai/provider/` 不承担注册、发现或生命周期管理。

## 当前 DeepSeek 线协议快照

当前官方 Chat Completion 接口使用 `POST /chat/completions`。流式请求设置 `stream: true`；设置 `stream_options.include_usage: true` 后，`[DONE]` 前会多一条 `choices: []` 的 usage chunk。普通 chunk 的首个 choice 通过 `delta` 逐步提供：

- `content`：最终回答文本分片；
- `reasoning_content`：thinking 分片；
- `tool_calls[]`：以 `index` 识别多个并行调用，`function.arguments` 是 JSON 字符串分片；
- `finish_reason`：当前文档列出 `stop`、`length`、`tool_calls`、`content_filter` 和 `insufficient_system_resource`。

工具参数在 streaming 中只是逐段字符串，不能在中途要求 `json.Valid`。Provider 先按 tool-call `index` 累积原始 bytes，流 settlement 时再形成 `ai.ToolCall`；参数是否满足具体工具语义仍由第 03 课确定的 Tool 层负责。

## Request 消息转换

第一阶段的确定性映射是：

| pi-go | Chat Completions wire message |
|---|---|
| `Request.SystemPrompt` | 首条 `{"role":"system","content":"..."}` |
| `ai.UserMessage` | `{"role":"user","content":"..."}` |
| assistant text blocks | assistant `content`，按 block 源顺序连接 |
| assistant thinking blocks | assistant `reasoning_content` |
| `ai.ToolCall` blocks | assistant `tool_calls[]`，arguments 保持 JSON 文本 |
| `ai.ToolResultMessage` | `{"role":"tool","tool_call_id":"...","content":"..."}` |
| `Request.Tools` | `tools[].function{name,description,parameters}` |

`StopReason`、`ErrorMessage` 和 `ToolResultMessage.IsError` 是 pi-go 内部协议事实，DeepSeek wire schema 没有同名字段。它们不能被随意塞入非标准 JSON；本课必须逐项确定是由消息结构隐含、编码进模型可见 content，还是只留在本地控制面。

## 本课第一个困难点：reasoning 必须可回放

普通流式 UI 可以在显示后丢弃 delta，但 DeepSeek thinking tool-call turn 不允许把完整 reasoning 只当展示数据。当前官方文档要求：assistant 发生 tool call 时，该 turn 的 `reasoning_content` 必须在后续请求中完整传回，否则请求可能得到 HTTP 400。

因此数据链不是：

```text
reasoning delta -> 打印 -> 丢弃
```

而是：

```text
reasoning_content deltas
  -> ai.ThinkingContent
  -> terminal AssistantMessage
  -> Agent transcript
  -> 下一次 Request
  -> assistant.reasoning_content
```

现有 `ai.ThinkingContent{Thinking string}` 已能保存文本，但第 04 课还要确认 OpenAI-compatible 转换层是否需要一个极小兼容 profile 来声明回放字段。第一期只有 DeepSeek，不引入任意 Provider 插件系统。

## SSE settlement 的责任

Provider stream 应按下面的顺序组装：

```text
HTTP 2xx + text/event-stream
  -> StartEvent
  -> content/reasoning/tool-call formation events
  -> 收到 finish_reason 并闭合全部已形成 block
  -> 合并 usage
  -> DoneEvent(final AssistantMessage)
  -> 后续 Receive 永久 io.EOF
```

下列情况进入 `ErrorEvent`，而不是让 Agent 解析 Provider 专用错误：

- 非 2xx HTTP response；
- response body 不是合法 SSE/JSON；
- `[DONE]` 前没有有效 `finish_reason`；
- tool-call 分片无法形成可配对的完整调用；
- DeepSeek 返回不支持的 finish reason；
- 网络读取失败。

若 `context.Cause(ctx)` 存在，terminal 必须是 `aborted` 并保留该 cause；否则是 `error`。已完成的 content blocks 可保留到 terminal assistant，未完成 tool call 不得伪装成可执行调用。Agent 随后沿用第 02/03 课的 exactly-once assistant 与 tool-result settlement 规则。

## 已确定的 pi-go 决策

- 继续使用现有模型无关 `ai.Provider`；Agent 不新增 DeepSeek 分支。
- `internal/ai/provider/` 只归类具体实现；consumer-owned `ai.Provider` 仍留在 `internal/ai`。
- 增加最小 `internal/ai/provider/openaicompatible` 线协议层，DeepSeek 是其第一个且第一期唯一的厂商配置层。
- 不移植 Pi 的 Provider registry、模型目录、动态刷新、认证存储、lazy module、多 API dispatch 或兼容性自动探测。
- endpoint、模型 ID 和 API key 来自配置；远程明文 HTTP 拒绝，本地测试 server 需要显式开发允许。
- 默认测试使用注入 transport/本地 fixture，不能读取真实 key；真实请求只在显式 smoke test 中运行。
- API key 只用于 Authorization header，不进入 request body、transcript、tool 环境、事件、日志或错误。

### 04-A：Provider 目录与抽象边界确认

- 学习者提议把 Provider 相关实现和抽象归类到 `internal/ai/provider/`，并要求记录与 Pi 的差别。
- 课程结论：采用该目录归类具体实现，但不把 consumer-owned `ai.Provider` 移出 `internal/ai`。Agent 继续只依赖模型无关 `ai`；Faux 已迁移到 `provider/faux`，后续 OpenAI-compatible 与 DeepSeek 分别位于 `provider/openaicompatible` 和 `provider/deepseek`。
- 学习者于 2026-07-17 确认理解该边界。目录重排后 `go test ./...`、`go vet ./...` 与 `go test -race ./...` 均通过，并按学习者要求在 `main` 本地提交为 `e97c2ac`；本课收尾时学习者另行授权与实现统一 push。

### 04-B：消息映射、实现机制与完整包名确认

- 学习者确认采用已讲解的完整 request 映射：system prompt、user、assistant text/reasoning/tool calls、tool results 和 tool schemas 分别转换为 Chat Completions 标准角色与字段；tool arguments 在线协议中保持 JSON 字符串。
- `reasoning_content` 经 `ThinkingContent` 保存在 terminal assistant 和 Agent transcript 中，并在工具结果后的下一次 Provider request 中回放，不能只用于显示后丢弃。
- `StopReason`、`ErrorMessage` 和 `ToolResultMessage.IsError` 不作为非标准 wire 字段发送。error/aborted assistant 已形成的 text、reasoning 和完整 tool calls 正常转换，closed calls 后已有明确 settlement results；空失败 assistant 显式编码为空 assistant，不像冻结 Pi 那样在 request transform 中静默删除。
- 线协议实现使用 Go 标准库 `net/http` 和可注入 client/transport，不引入 OpenAI SDK。这样 SSE、context cancellation、错误体上限与零自动重试都是本项目可见、可测试的语义。
- 学习者要求共享包使用完整名称且不必照搬 Pi；最终采用符合 Go 命名习惯和正确英文拼写的 `internal/ai/provider/openaicompatible`，package name 为 `openaicompatible`。

### 04-C：SSE 组装与错误边界确认

- `Provider.Stream()` 只创建 pull stream 状态；第一次 `Receive()` 发起一次绑定 context 的 HTTP 请求，后续调用按需解析 SSE，不使用后台 goroutine 或自动重试。
- 第一阶段不为 3xx 增加特殊逻辑或专门测试，完全服从注入 `http.Client` 的标准 redirect policy。这里的“零自动重试”只指 Provider 不自行 retry；redirect 对请求体、凭据传播和最终 SSE 的影响等出现真实证据后单独验证。
- 成功 terminal 同时要求有效 `finish_reason` 和 `[DONE]`。usage chunk 可以在 finish reason 后到达，因此 `DoneEvent` 只能在 `[DONE]` 后携带最终 usage 发出。
- tool calls 按 wire `index` 聚合。最终 arguments 即使不是合法 JSON，也保留为 `json.RawMessage` 交给 Agent 形成 call-local error；缺少可靠 index、ID 或 name 才形成 Provider protocol error。
- finish reason 前取消不保留没有完成证据的 tool calls；finish reason 后、`[DONE]` 前取消可保留已完整组装的 calls，再由 Agent 追加 not-executed results。
- 第一阶段 `Usage` 不扩展 cache/reasoning 分项；HTTP 错误体上限为 64 KiB，单个 SSE event 上限为 1 MiB，错误不包含 API key、headers 或完整 request。
- 学习者于 2026-07-17 确认以上边界并要求继续，第 04 课进入测试先行实现。

### 04-D：测试先行实现记录

- request conversion 测试先因缺少 `buildRequestPayload`/`Profile` 红灯，再实现 system、完整 transcript、tool schemas、reasoning replay 和内部字段隔离。
- Provider startup 测试先因缺少 `Config/New/responseStream` 红灯，再实现 stable config 校验、`Stream()` 时深复制 request、首次 `Receive()` 发请求和终止后永久 `io.EOF`。
- SSE 测试先在占位 parser 上整体红灯，再实现无 goroutine 的按需 SSE reader、1 MiB event 上限、content block 源顺序、按 wire index 聚合交错 tool calls、usage-after-finish 和 `[DONE]` settlement。
- cancellation fixture 分别覆盖 finish reason 前后：前者 terminal 只保留 text/reasoning，后者保留完成 calls，供 Agent 追加 not-executed results。malformed final arguments 保持 raw，缺失 index/ID/name 才升级为 Provider error。
- `provider/deepseek` 只验证 key/model、远程 HTTPS 与显式 loopback HTTP override，并冻结当前 DeepSeek profile；它直接返回共享实现背后的 `ai.Provider`，没有第二套 Provider 类型或 registry。

默认测试全部离线。真实兼容测试同时要求 build tag 和运行时开关，因此普通 `go test ./...` 不会读取 `DEEPSEEK_API_KEY`：

```bash
PI_GO_RUN_DEEPSEEK_SMOKE=1 \
DEEPSEEK_API_KEY=... \
DEEPSEEK_MODEL=... \
go test -tags=integration ./internal/ai/provider/deepseek -run TestDeepSeekSmoke -v
```

真实 smoke 依次验证文本响应、tool call，以及一份 aborted assistant 后已有同 ID not-executed tool result 的完整历史可以继续请求。没有显式 opt-in 和凭据时只编译并 skip，不把未执行的真实请求记成通过证据。

3xx redirect 不是本课观察到的 DeepSeek 或冻结 Pi 行为，因此当前没有实现或 fixture。若真实 endpoint 后续出现 redirect，再单独记录 status、目标 host、method/body/header 变化与最终 SSE 行为后决定是否需要策略。

## 当前实现结果

本课代码与离线验证已完成。学习者随后确认继续课程并明确开始第 05 课，因此第 04 课完成理解确认；目录重构、证据化备注规则和 Provider 实现已经按学习者要求提交并 push 到 `main`。
