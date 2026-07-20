# 第 08 课：Context budget 与 compaction 核心

## 当前状态

本课的语义讨论、首版实现、代码审查与质量门已经完成：pi-go 现在会在下一次 Run 接受输入前估算 projected Provider input，以 `192K` 触发 lazy compaction，按 protocol-safe message boundary 总结旧 prefix、保留近期 raw suffix，并原子替换 Core Agent Working Context；完整 Conversation History 不被改写。冻结 Pi 的 initial/update/split-turn prompts、`20K` retained target、summary budgets 和 request-local output clamp 已落入代码，重复 compaction、tool protocol、失败、取消、并发与 soft-ceiling 降级均有确定性测试。当前尚未提交，课程等待学习者理解确认与是否提交的决定。

## 开课源码校准

固定基线：Pi commit `dcfe36c79702ec240b146c45f167ab75ecddd205`，Pi Agent package version `0.80.7`。

### 已确认的大纲假设

- compaction 由 coding 层的 Conversation/Session owner 协调，而不是塞进低层 Provider 或 Core Agent tool loop；Core Agent 只需要提供 idle-time Working Context replacement。
- threshold compaction 位于一个 Run 已 settled、下一次 Provider call 尚未开始的边界。它先保留完整记录，再生成 summary、保留近期 suffix 并替换 Working Context。
- threshold compaction 不自动继续模型执行；只有后续新的输入才启动下一次 Run。context-overflow recovery 的 compact-and-retry 是另一项能力，不属于本课。
- 完整 Conversation History 仍保存原始消息；summary 是可替换 Working Context 的有损投影，不会反向改写历史事实。

### 细化后的认识

- Pi 的 threshold 判断不是对完整消息列表一律重新分词。它优先使用最近一次有效 assistant usage；当错误或零 usage 后还有新消息时，再以最近有效 usage 为锚点并估算尾部消息。当前 pi-go 已经在 `ai.AssistantMessage.Usage` 保存 input/output tokens，DeepSeek stream 也请求并解析 usage，因此本课不需要先补一套 usage 协议。
- `contextWindow`、`reserveTokens` 与 `keepRecentTokens` 是三个不同概念：前者是模型容量，第二个为下一次输入和输出保留余量，第三个决定压缩后保留多少近期原文。在冻结 Pi 的 capacity-oriented policy 中，trigger threshold 是 `contextWindow - reserveTokens`，不是 `keepRecentTokens`；这不自动等于 pi-go 后续采用的 coding quality ceiling。
- cut point 必须保持 Provider 消息协议可继续使用。Pi 允许在 user 或 assistant message 处切分，但不允许从 tool result 开始；若切进同一个 turn，还会单独总结被丢弃的 turn prefix，避免保留的 assistant/tool suffix 失去语义来源。
- Pi 的 summary 由一次独立模型调用生成；该调用不是正常 Agent Run，其 request、assistant response 和 usage 都不进入 Conversation History。只有 summary 成功后，compaction entry 与新的 Working Context 才成为可观察状态。
- 当前 pi-go 没有 `SessionManager` entry tree，但 Lesson 07 已经建立完整内存 History owner 与 idle-only `ReplaceWorkingContext`。本课可以在这个已证明的边界上完成内存 compaction，不需要为了 summary 先引入持久化 Session、branch 或事件系统。

### 被推翻的隐含假设

- “compaction 必须先有 Session persistence”不成立。冻结 Pi 使用持久 entry tree，是因为它还同时支持恢复、branch、extensions 和 UI；本课的内存闭环只需要完整 History、compaction projection state 与可替换 Working Context。
- “需要先引入精确 tokenizer 才能做 context budget”没有源码依据。Pi 以 Provider usage 为主，并只对 usage 之后的尾部内容使用近似估算；是否需要更精确估算应由后续误差证据决定。
- “模型声明的最大 context window 附近仍是 coding 的理想工作区间”不成立。容量上限只说明请求可以被接收，不能证明模型在该长度仍保持同等检索、推理或代码修改质量。

## 冻结 Pi 源码与测试路径

- `packages/coding-agent/src/core/compaction/compaction.ts`：定义 usage 计算、threshold、字符近似、protocol-safe cut point、summary prompt、前次 summary 合并与 retained suffix。
- `packages/coding-agent/src/core/agent-session.ts`：在 `agent_end` 后及新 prompt 前检查 compaction，生成 summary 后追加 compaction entry、重建 session context 并替换 `agent.state.messages`。
- `packages/coding-agent/src/core/session-manager.ts`：完整保存原始 entries，并用最新 compaction summary、`firstKeptEntryId` 和后续 entries 构造模型上下文。
- `packages/coding-agent/src/core/compaction/utils.ts`：把原对话序列化为 summary request 文本，并限制单个 tool result 在摘要输入中的体积。
- `packages/coding-agent/test/compaction.test.ts`：验证 usage/尾部估算、threshold、cut point、重复 compaction 与 summary 加 retained suffix 的重建。
- `packages/coding-agent/test/session-manager/build-context.test.ts`：验证完整 entries 与 compaction-aware model context 保持不同事实边界。
- `packages/coding-agent/test/suite/regressions/pre-prompt-compaction-no-continue.test.ts`：验证 pre-prompt compaction 不错误调用 `continue()`。

## 当前 pi-go 路径

- `internal/ai/message.go`：`AssistantMessage.Usage` 已保存 Provider 返回的 input/output tokens。
- `internal/ai/estimate.go`：使用最近有效 usage 加尾部字符近似估算完整 request；无有效 usage 时估算 system prompt、tool schemas 与全部 messages。
- `internal/ai/provider/openaicompatible/stream.go`：解析 streamed usage 并写入 terminal assistant；失败或取消 terminal 也保留已观察到的 usage。
- `internal/ai/provider/openaicompatible/request.go`：把 request-local `MaxOutputTokens` 映射为 `max_tokens`。
- `internal/ai/provider/deepseek/provider.go`：DeepSeek profile 显式启用 streamed usage。
- `internal/agent/context.go`：`ReplaceWorkingContext` 只在 Core Agent idle 时深复制并原子替换未来 Provider calls 使用的消息；每个正常 request 也在这里计算自己的 output cap。
- `internal/coding/conversation.go`：Conversation Owner 的 active guard 现在覆盖 pre-Run compaction、Core Run、History commit 与返回 snapshot。
- `internal/coding/compaction.go` 与 `compaction_prompt.go`：拥有 private policy、projection metadata、cut/forecast、Pi prompts、summary call、candidate validation 与 commit lifecycle。
- `internal/coding/runtime.go`：固定 DeepSeek product profile 组装 request limits 与 compaction policy；产品仍是 one-shot composition，本课不借 compaction 扩展公共 SDK、Gateway 或 Session persistence。

## 当前 DeepSeek 产品证据

2026-07-20 核对 DeepSeek 官方 [Models & Pricing](https://api-docs.deepseek.com/quick_start/pricing/) 与 [Chat Completions API](https://api-docs.deepseek.com/api/create-chat-completion/)：`deepseek-v4-pro` 的 context length 为 1M，maximum output 为 384K；Chat Completions 支持 request-local `max_tokens`，并约束 input 加 generated tokens 不超过 context length。冻结 Pi model catalog 中的 `contextWindow: 1000000` 与 `maxTokens: 384000` 和官方资料一致。

当前实现已由 coding 产品 profile 固定保存 hard capacity/max output，由 `ai.Request.MaxOutputTokens` 承载窄的 request-local generation limit，并由 OpenAI-compatible payload 映射 `max_tokens`；正常 coding call 与独立 summary call 都使用该字段，但采用不同的上限计算。Pi 的 `reserveTokens` 不再被同时当作 1M capacity threshold，因为 pi-go 已单独确定 `192K` quality threshold。

### 最大容量不等于 coding 质量区间

2026-07-20 继续核对 DeepSeek 官方 [V4 技术报告](https://huggingface.co/deepseek-ai/DeepSeek-V4-Pro/resolve/main/DeepSeek_V4.pdf)。其 MRCR 8-needle 曲线显示 DeepSeek-V4-Pro-Max 在 8K 到 128K 输入内基本稳定，报告明确写明 128K 之后开始出现可见退化；图中平均 MMR 从 128K 的 `0.92` 降到 256K 的 `0.82`、512K 的 `0.66` 和 1M 的 `0.59`。这是检索评测，不是 coding 成功率，因此它能证明“1M 不是无损质量区间”，但不能单独证明“128K 是 DeepSeek coding 的精确最优点”。

coding 专项证据给出同一方向但不支持一个跨模型通用常数：[LongCodeBench](https://openreview.net/pdf?id=GFPoM8Ylp8) 在真实仓库理解和修复任务中观察到多种模型随输入增长而退化，例如 Claude 3.5 Sonnet 的 LongSWE-Bench resolved rate 从 32K 的 `29%` 降至 64K 的 `19%`、128K 的 `15%` 和 256K 的 `3%`；不同模型的拐点并不完全相同。[SWE-ContextBench](https://arxiv.org/abs/2602.08316) 则显示，正确选择并压缩的历史经验可提高 coding resolution 并降低 token 成本，而未过滤或错误选择的 context 收益有限甚至为负。

工程实践也不把“填满窗口”当作目标。Anthropic 的 [agent context engineering 指南](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents) 建议只保留最小的高信号 token 集合，通过工具按需读取外部资料，并以 compaction 保留架构决定、未解决问题和近期工作而丢弃冗余 tool output。该方向与 pi-go 已有限制 read/bash tool result、完整 History 与有损 Working Context 分离的架构一致。

Codex 给出了两种不同目标下的实际参照。OpenAI 的 [Codex agent loop 说明](https://openai.com/index/unrolling-the-codex-agent-loop/)确认其超过 `auto_compact_limit` 后会自动压缩；该说明链接的 Codex [ModelInfo 源码](https://github.com/openai/codex/blob/99f47d6e9a3546c14c43af99c7a58fa6bd130548/codex-rs/protocol/src/openai_models.rs#L191-L207)在没有显式 override 时把 threshold 推导为 model context window 的 `90%`，其 [默认/fallback coding context](https://github.com/openai/codex/blob/99f47d6e9a3546c14c43af99c7a58fa6bd130548/codex-rs/core/src/models_manager/model_info.rs#L23-L55) 为 `272K`，即约 `244.8K`。另一方面，[GPT-5.3-Codex system card](https://deploymentsafety.openai.com/gpt-5-3-codex/gpt-5-3-codex.pdf) 记录的一项最多 1,000 turns、以最大化表现为目标的长任务评测每 `100K` tokens 就触发 compaction。前者偏向通用产品容量与成本，后者偏向极长任务质量；两者都证明“模型最大窗口”不是唯一 policy。Codex 当前还使用 OpenAI 模型[原生的 opaque compaction item](https://openai.com/index/equip-responses-api-computer-environment/)保存潜在状态，而 pi-go 首版只会得到普通文本 summary，因此不能机械照搬其更高阈值。

因此首版产品策略明确区分三层，并记录为 D55：

- `1_000_000` 是 Provider hard capacity，只用于协议校验和最终安全边界，不是日常目标；
- settled Working Context 在下一次 Run 边界以 projected Provider input `192K` 作为首版 quality-oriented compaction threshold；这个值比 DeepSeek 已观察到的 `128K` 稳定区间多 `50%` 容量，又比 Codex 的约 `244.8K` 默认触发点保守，给普通文本 summary 和缺少 run 内 compaction 留出余量；
- compaction 后的 projected Provider input 以 `64K` 作为普通情况 soft ceiling，而不是要求填满的目标；首版按 Pi 尽量保留 `20K` raw suffix，并给 summary 设置独立输出上限。达到 ceiling 时可获得约 `128K` 的重新增长空间，事实需要时继续靠 read/bash 从 workspace 重取。

这里的 `192K` threshold 与 `64K` post-compaction soft ceiling 是 Lesson 08 首版 policy，不是长期模型真理，也不是 active-Run hard ceiling。`64K` 包含 system prompt、tool schemas、synthetic summary、retained raw suffix 与尚未接受的新 user input；它不是 summary size，也不是必须达到的精确值。如果不可压缩内容或 protocol-safe message granularity 使 `64K` 无法达到，允许以高于 `64K` 但低于 `192K` 的 candidate 成功；连 `192K` 都无法降到时才按 D54 原子失败。后续在真实 DeepSeek coding traces、按长度分桶的任务成功率、compaction 前后质量和成本数据可用后，必须重新评估；更换模型、reasoning mode、tool schema、Skills 暴露或 summary policy 时也必须重新校准，不能沿用旧值而不验证。

冻结 Pi 的默认 `reserveTokens: 16384` 会让 1M 模型直到约 `983616` context tokens 才自动 compaction。这个默认值适合解释 Pi 的 capacity/overflow policy，却已经被 DeepSeek 自身的 128K 后退化证据否定为 pi-go 的 coding quality policy；pi-go 不应仅为了源码形状一致而照搬。

当前 Lesson 08 的 lazy compaction 只发生在两个 Run 之间，不能限制一个仍在执行的 Core Agent Run 内连续 Provider/tool turns 增长到 `192K` 以上。首版显式保留这一缺口；若后续真实 traces 显示单个 Run 经常越过阈值，或按长度分桶的 coding 成功率确认 run 内退化，再另行建立 run 内安全 compaction point，不能假装现有 lifecycle 已经提供保证。

## Compaction 整体模型

Lesson 08 同时涉及几种不同的“长度”和阶段，不能把它们混成一个参数：

- Conversation History 是完整、原始、持续增长的应用状态；本课不删除或用 summary 覆盖它。
- Working Context 是 Core Agent 下一次调用 Provider 时使用的可替换投影；compaction 真正缩短的是它。
- projected Provider input 是 Working Context 再加稳定 system prompt、tool schemas 和尚未接受的新 user input；threshold 判断针对这个下一次请求规模。
- retained suffix 是 compaction 后继续逐条保留的近期原始消息，按 token budget 和协议边界选择，不是固定“最近 N 条”。
- summary 是对较旧 prefix 的有损表示；summary 自己也需要独立输出上限，并占用压缩后的 Working Context budget。
- model context window 是 input 加 generated output 的 Provider 硬容量，不能直接当作 coding quality threshold。

完整顺序如下：

1. Run N 完全 settled，Conversation Owner 提交其原始消息到完整 History；此时不 eager compaction。
2. Run N+1 被调用并取得 Conversation guard，但新 user input 尚未交给 Core Agent。
3. Conversation Owner 估算下一次完整 Provider input；未超过 threshold 就直接继续。
4. 超过 threshold 时，从 settled Working Context 中选择 protocol-safe cut point：较旧 prefix 进入独立 summary request，近期 suffix 保留原文。cut 可以落在 Run N 的消息序列内部，但这是事后切分，不是在 Run N active 时执行。
5. summarizer 成功后构造 candidate Working Context：D56 确定的 synthetic user summary message 加 retained raw suffix。summarizer 返回的 assistant response 不作为普通对话消息写进完整 History。
6. candidate 验证成功并通过最后一次 cancellation check 后，原子替换 Working Context，再接受 Run N+1 的 user input并进入正常 Core Agent loop。
7. summary、验证或 replacement 失败时，History 与旧 Working Context 都不变，Run N+1 的 input 未接受。

压缩频率由 threshold 与压缩后的实际规模之间的间隔决定，而不是只由 threshold 决定。首版在约 `192K` 触发、普通情况以 `64K` 为 soft ceiling，会留下至少约 `128K` 的增长空间，不会因为刚压缩完便在下一条普通消息再次压缩；实际 candidate 小于 `64K` 时空间更大。若不可压缩内容使 candidate 高于 `64K`，增长空间会相应缩小。真实 coding Run 每次增长多少、重复 summary 损失多少以及多大项目会持续错过 soft ceiling，必须进入后续 trace/质量评测。

代码库总规模也不等于模型输入规模。coding agent 把 workspace 当外部事实源，通过 read/bash 按需取得相关片段；当前 read 与 bash 的单次模型可见结果都限制在 50 KiB。大仓库可以保持小 Working Context，而一个反复读取大页、携带冗长测试输出的 Run 也可能快速增长。首版优先依靠有界 tool results、按需重读源文件、保留近期原文和高保真 summary，避免把整个 repository 当作 prompt 常驻内容。

## 本课闭环

本课只完成一个能力：当同一 Conversation 的已 settled Working Context 超过 threshold 时，在后续 Provider call 之前，用独立 summary call 把较旧内容投影成 summary、保留 protocol-valid 的近期 suffix，并原子替换 Core Agent Working Context；完整内存 Conversation History 保持原始、有序且不丢失。

课程按下面顺序推进：

1. 讲清 context window、usage、reserve、retained suffix 与 summary 各自解决的问题。
2. 讨论并确定 budget/policy 的 owner、触发时机与无有效 usage 时的行为。
3. 讨论并确定 protocol-safe cut point、重复 compaction 与 summary request/result 契约。
4. 讨论 summary 失败、取消和并发时的原子状态语义。
5. 在不引入 Session 基础设施的前提下实现内存闭环和确定性 Faux Provider 测试。
6. 运行 `make check` 与 `go test -race ./...`，同步课程记录，再由学习者确认理解和是否提交。

## 已确认的 protocol-safe message-level cut

pi-go 首版不把完整 Run 作为最小 retained unit。这里描述的是对一个已经 settled 的 Run 做事后 cut：cut finder 可以在该 Run 的消息序列中选择较后的 user 或 assistant message 作为 retained suffix 的第一条原始消息，但不能从 tool result 开始，也不能拆分一条 message 的 content blocks。若第一条 retained assistant 含有 tool calls，其后所有 matching tool results 必须继续保留。这不表示 active Run 执行到该消息时会就地触发 compaction。

当 cut 位于一个 Run 内部时，被移除的 Run prefix 与更早 History 一起进入 summary input；summary 必须保留当前 user request、已完成的早期工作和理解 retained suffix 所需的状态。这样一次极长的 coding Run 也能被压缩，同时 Working Context 不会产生 orphaned tool result。完整 Conversation History 仍保留 cut 两侧的所有原始消息。

实现把 cut finder 和 candidate validator 保持为 `internal/coding` 私有 helper；summary 在 model-visible Working Context 中仍是普通 `ai.UserMessage`，private projection metadata 保存 summary、History 的 absolute first-kept index，以及哪些 retained assistant usage 已因 compaction 失效。

## 已确认的 lazy compaction 与原子失败语义

Threshold compaction 严格发生在两个 Run lifecycle 之间：下一次 `conversation.run()` 已取得 Conversation active guard、但 Core Agent 尚未接受新的 user input 时检查和执行。没有后续 Run 就不产生 summary call；threshold compaction 成功后也不自动调用 Core Agent `continue`。Conversation guard 覆盖 budget check、summary call、Working Context replacement、后续 Core Run 和 History commit，因此同一 Conversation 的另一个 Run 不能观察中间状态。本课不在一个 active Run 的相邻 Provider Turns 之间执行 compaction。

Summary request 是独立 Provider call，不经过正常 Core Agent loop，不提供 coding tools，其 request、terminal assistant 和 usage 都不进入 Conversation History。只有成功得到并验证 candidate summary、构造 protocol-valid retained context、确认 cancellation 尚未发生后，才调用 idle-only `ReplaceWorkingContext` 并同步发布新的 projection metadata；二者之间不插入可失败或可取消的异步步骤。

Summary Provider error、取消、空文本、意外 tool call 或 replacement failure 都保留旧 History、旧 Working Context 和旧 projection metadata，新 user input 未被接受，并返回当前完整 History snapshot 与错误。compaction commit 完成后才到达的取消不回滚已经合法发布的 Working Context；Core Agent 仍可因预取消而拒绝新 input，History 保持不变。

## 已确认的 Pi-aligned summary prompt 与重复更新语义

首版在没有具体反证时沿用冻结 Pi 的 summary prompt 文本和输入组织，不为 DeepSeek 主观改写一套“看起来更合适”的 prompt。冻结 Pi 实际包含三条相互配套的 prompt：

- 首次 compaction 使用 structured checkpoint prompt，固定输出 `Goal`、`Constraints & Preferences`、`Progress`、`Key Decisions`、`Next Steps` 与 `Critical Context`，并要求保留精确 file path、function name 和 error message；
- 已有前次 summary 时使用 update prompt，把本次新丢弃的消息放进 `<conversation>`、把旧 summary 单独放进 `<previous-summary>`，要求保留旧事实并按新进展更新状态，而不是把旧 summary 当普通对话再次摘要；
- message-level cut 落在一个 settled turn 内部时，对被丢弃的 turn prefix 使用专用 prompt，保存 `Original Request`、`Early Progress` 与理解 retained suffix 所需的 `Context for Suffix`。

三种调用共享 Pi 的 summarization system prompt：只生成指定结构的 summary，不继续原对话，也不回答被序列化对话中的问题。待摘要消息先转成标记了 `[User]`、`[Assistant thinking]`、`[Assistant]`、`[Assistant tool calls]` 与 `[Tool result]` 的纯文本，再整体放进一个独立 user message；每个 tool result 在 summary input 中最多保留 `2000` characters。首版还沿用 Pi 的确定性 file-operation 补充：从 tool calls 提取 read/modified paths，并在模型 summary 后追加 `<read-files>` / `<modified-files>`，避免完全依赖模型记住文件清单。

Summary 成功后，pi-go 的 Working Context 以一个 synthetic user message 开头，使用冻结 Pi 的提示语和 `<summary>` tags，后接 protocol-valid retained raw suffix。这个 synthetic message 不是用户实际输入，也不进入完整 Conversation History；Conversation Owner 的私有 projection metadata 需要分别保存 summary 与 cut boundary，供下一次 compaction 使用 update prompt。无需为此给 `internal/ai` 增加 `compactionSummary` role，因为 Provider 最终看到的本来就是普通 user message。

沿用 prompt 不表示复制 Pi 的全部扩展面：本课不增加 custom summary instructions、branch-summary prompt、extension hook 或持久化 compaction entry。D54 已确定的空文本、意外 tool call、错误与取消校验也继续比冻结 Pi 更严格；这些是 pi-go 已确认的状态安全契约，不是 prompt 改写。Summary output limits 与 retained target 已由 D57 按 Pi 数值映射确定。

## 已确认的 Pi-aligned budget mapping

首版区分模型硬容量、质量触发点、压缩后 soft ceiling 和各组成部分的局部 budget：

| 项目 | 首版值 | 语义 |
|---|---:|---|
| DeepSeek hard context capacity | `1_000_000` | input 加 generated output 的 Provider 安全边界 |
| DeepSeek model max output | `384_000` | 单次正常生成的模型上限，不表示应生成到该长度 |
| between-Runs quality threshold | `192_000` | projected input 达到后尝试 compaction |
| post-compaction soft ceiling | `64_000` | 完整 projected input 的普通情况预算上界，不要求填满 |
| retained raw suffix target | `20_000` | 按 Pi 尽量保留的近期原文；协议边界与总预算优先 |
| initial/update summary max output | `13_107` | `floor(0.8 * 16_384)`，沿用 Pi |
| split-turn prefix summary max output | `8_192` | `0.5 * 16_384`，沿用 Pi |
| Provider context safety | `4_096` | 正常生成的 hard-cap headroom，沿用 Pi |

普通 compaction 的 summary 加 retained raw 上限约为 `33.1K`；split-turn 两份 summary 加 retained raw 上限约为 `41.3K`，剩余预算容纳 system prompt、tool schemas、新 input、summary envelope 与确定性 file lists。所有 output 数字都是 request 上限，不要求模型填满。若预计完整 candidate 超过 `64K`，cut finder 应在 D53 允许的边界内减少 retained suffix；若实际 summary、单条大 message 或不可压缩内容仍使 candidate 超出 `64K`，可以保留高于 soft ceiling 但低于 `192K` 的安全结果。不得为了凑 `64K` 丢弃未被 summary 覆盖的消息。

正常 coding call 也参考 Pi 使用 request-local output cap：`min(384_000, max(1, 1_000_000 - projectedInput - 4_096))`。Summary calls 分别使用 `13_107` 或 `8_192`，并继续受同一 hard-cap clamp 约束。Model capacity/max output 属于固定 coding product profile；threshold、soft ceiling、retained target 与 summary budgets 属于 coding-owned Conversation compaction policy；`ai.Request` 只承载已经计算好的单次 output limit，Provider 不拥有 compaction policy。

## 已实现的 projected-input 估算

pi-go 不用字符近似替代 Provider usage，而是把 usage 当作已知 prefix 的锚点，只估算 usage 之后新增的尾部：

1. Conversation 在 Run N+1 取得 guard 后，构造“当前 Working Context + 尚未接受的新 user input + 稳定 system prompt/tool schemas”的只读 request view。
2. `ai.EstimateRequestTokens` 从后向前采用最后一条有效 terminal assistant usage；`error`、`aborted` 和 input/output 都为零的 usage 不作为锚点。
3. 有效 usage 的 `input + output` 近似表示下一次请求已经拥有的完整 prefix：前次 input 已含 system prompt、tools 和旧 messages，前次 output 会作为 assistant message 进入下一次 input。因此这些内容不重复估算，只对该 assistant 之后的 tool results、messages 和新 user input使用近似。
4. 尾部按冻结 Pi 的 `ceil(characters / 4)` 规则逐条估算：user/tool-result 使用正文字符数；assistant 累加 text、thinking、tool name 与原始 JSON arguments。当前 Go 实现以 Unicode code point 计字符，仍是粗略预算而不是模型 tokenizer。
5. 完全没有有效 usage 时，才对完整 request 做 fallback 估算：system prompt、JSON tool schemas、全部 Working Context messages 和新 user input都进入字符近似。
6. 结果 `< 192_000` 时直接接受新 input；结果 `>= 192_000` 时才进入 compaction。正常 Agent request 随后用同一 projected input 计算自己的 `max_tokens`。

例如最近一次有效 assistant usage 为 input `181_500`、output `6_000`，锚点总量是 `187_500`。之后新增约 `12_000` 字符的 tool result，估算 `3_000` tokens；新 user input 约 `2_000` 字符，估算 `500` tokens。下一次 projected input 约为 `191_000`，暂不触发。若又增加约 `8_000` 字符，即再增加约 `2_000` tokens，估算达到 `193_000`，下一次 Run 接受输入前触发 compaction。

Compaction 后 retained assistant 中的旧 usage 描述的是压缩前的大 context，不能再当锚点。candidate 发布时，Working Context 会清空所有 retained assistant usage；private `UsageValidFrom` boundary 让 Conversation 从完整 History 重建投影时只恢复 compaction 后新 Run 产生的 usage。在第一次 post-compaction Provider response 返回新 usage 之前，估算器会对 summary、retained suffix、system prompt、tools 和新 input 做完整 fallback 估算，避免旧 `200K` usage 让刚压缩到约 `64K` 的 context 立即误触发。

这套估算的目标是确定、低成本且以 Provider 事实为主，不是宣称 `characters / 4` 精确。真实 traces 必须比较 estimated 与 Provider-reported input 的误差分布；若误差会改变 coding 质量或频繁越过 policy boundary，再评估 tokenizer 或 Provider-specific estimator。

## 已实现的 Go 所有权映射

- `ai.RequestLimits` 只表达调用方提供的 context capacity、model max output 与 safety；`ai.Request` 只携带已算好的单次 `MaxOutputTokens`。
- Core Agent 不拥有 compaction threshold、summary prompt 或 retained policy；它只为自己即将发送的每个 request 应用 output clamp。
- coding-owned Conversation 保存 private `compactionPolicy` 与 `compactionProjection`，并直接借用同一个 `ai.Provider` 发起无 tools 的 summary request。
- `FirstKept` 是完整内存 History 的 absolute message index。重复 compaction 从上次 boundary 继续，只序列化新丢弃的 raw messages，并把旧 summary 放入 `<previous-summary>`。
- cut forecast 先以 summary output caps、synthetic envelope、file lists、stable prompt/tools、新 input 和 retained suffix 尝试满足 `64K`；若实际 summary 或不可压缩 message 仍高于 soft ceiling但低于 `192K`，candidate 可以提交。
- summary、空文本、意外 tool call、取消、无效 protocol candidate、candidate 仍不低于 threshold 或 idle-only replacement 失败，都在 Working Context replacement 前返回；新 input 不被 Core Agent 接受。

### 大项目与未来 Skills 的强制复评备注

这组数值尚未在真实大型项目上证明充分，尤其没有覆盖未来 Skills 加入后的 context 组成。大项目可能带来更长的 project instructions、更广的相关文件集合、更多 tool turns 和更高的跨 Run 状态保真需求；Skills 即使只在初始 prompt 暴露 metadata，按需读取的 Skill 正文与其引导出的工具结果也会增加动态 Working Context。`20K` retained suffix、Pi prompt 的 summary 容量和 `64K` soft ceiling 都可能不足，当前不能宣称它们适用于所有项目规模。

Lesson 08 不预先提高这些值，不设计按仓库大小自动调参，也不把 Skills 提前吸收进本课。至少在 Lesson 09 引入 Skills 时、以及产品声称支持长时间真实项目工作前，必须用真实仓库和任务重新验证：

- system prompt、project instructions、tool schemas、Skills metadata/正文和 tool results 各自占用多少 projected input；
- compaction 前后实际 token 分布、错过 `64K` soft ceiling 的比例、两次 compaction 之间的 Run/token 间隔；
- `<128K`、`128K–192K`、`192K–256K` 与更长区间的 coding 完成率，以及 summary 后继续修改和验证的成功率；
- 重复 compaction 是否遗失目标、约束、精确文件位置、未完成修改或验证状态，模型是否被迫重复读取大量事实；
- `20K` retained suffix 是否经常切掉仍在使用的原始上下文，以及增大或减小 budget 后质量、成本和频率如何变化。

真实证据可以要求修改 threshold、soft ceiling、retained target、summary budget 或 prompt；在这些数据出现前，首版值只是一组明确、可测试的基线。

## 当前非目标

- context-overflow 检测、compact-and-retry、其他 Provider retry、自动 Run/turn wall-clock budget 或循环保险丝；
- Session 持久化、恢复、entry tree、branch summary、导航或跨 Session context；
- manual compaction 命令、extensions/hooks、事件订阅、steering/follow-up、TUI 或公共 SDK；
- 精确 tokenizer、多模型 registry、运行时 model switching 或可配置 Provider 产品矩阵；
- 将 summary 写回 Conversation History，或删除被 summary 覆盖的原始消息。

## 完成信号与验证结果

确定性测试已使用人为缩小的 budget 和 Faux Provider 完成顺序 Runs：threshold 前不调用 summary；threshold 后先发独立 summary request，下一次 coding request 只看到 synthetic summary、protocol-valid retained suffix 和新 user input，而返回的完整 History 仍保留所有原始消息。

当前测试还覆盖：projected input 恰好等于 threshold 时触发；重复 compaction 使用 update prompt 前进；soft-ceiling forecast 会减少 retained suffix，但实际 candidate 可以在 ceiling 与 threshold 之间成功；cut 不从 tool result 开始；summary Provider error、空文本、意外 tool call、取消、candidate 仍过大与 replacement failure 都不发布 projection或接受新 input；Conversation guard 在 summary 阻塞期间拒绝并发 Run；正常和 summary requests 使用各自 output caps。

实现后 `make check` 已通过 `gofmt`、`go vet ./...`、`go test ./...` 与 `golangci-lint`，`go test -race ./...` 也已通过。最终审查发现 cut point 连续前移时的重复线性查找会在长列表上退化为二次复杂度；实现已改为有序切点的二分查找，并由定向测试与上述完整质量门重新验证。

## 实现后仍明确保留的缺口

- 单个 active Run 超过质量目标的风险作为明确缺口保留；只有真实 traces 或 coding 分桶评测证明该问题后，才另行规划 run 内 compaction；
- `characters / 4`、`192K`、`64K`、`20K` 和 summary budgets 仍需按前述真实大型项目、Skills-enabled context 与 Provider usage 误差数据复评；
- context-overflow compact-and-retry、active-Run compaction、Session persistence/恢复、manual compaction 和事件/UI 生命周期仍属于后续能力，不因本课已有 private helpers 而被视为完成。
