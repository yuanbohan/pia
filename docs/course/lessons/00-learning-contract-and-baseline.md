# 第 00 课：学习契约与冻结基线

## 当前状态

已提交。

开课记录：学习者于 2026-07-15 明确要求开始第 00 课。

## 为什么先上这一课

Agent 代码会跨越模型协议、并发、工具执行、文件系统和持久化。如果不先固定“阅读哪一版 Pi”“如何区分行为与机制”“什么时候能提交”，后续结论就无法追溯。第 00 课不实现 Agent 功能，它建立所有后续课程共用的学习与工程条件。

## 本课组成与当前进度

| 部分 | 内容 | 产物 | 当前进度 |
|---|---|---|---|
| 00-A | 学习合作契约 | 先讲后问、持续 challenge、记录纠正、用户控制 commit | 已建立，持续执行 |
| 00-B | 冻结参考基线 | Pi commit、package version、Go version | 已验证 |
| 00-C | 行为契约与实现机制 | 用 active Run、listener 顺序、退订和 RunEnd 区分“必须保留”与“可重新设计” | 已完成 |
| 00-D | 项目内验证边界 | Pi 源码用于提取行为；pi-go 用自身测试验证实现 | 已完成 |
| 00-E | 最小 Go module 边界 | `go.mod`；冻结基线只保存在课程文档 | 已完成 |
| 00-F | 本课验收与提交门禁 | 测试、理解确认、文档完整性、用户明确要求 commit | 已完成 |

本课不会实现 Agent Loop、listener 容器或 Run 生命周期。当前深入讨论这些概念，是为了学习如何从 Pi 中提取行为契约；对应 Go Runtime 在后续课程实现，其中 listener settlement 和 Run 生命周期集中在第 05 课。

## 学习目标

完成本课后，学习者应能解释：

1. 为什么本项目是行为语义移植，而不是 TypeScript 到 Go 的逐行翻译。
2. Pi 上游 commit 和本地 Go 工具链为什么必须冻结。
3. Pi 源码证据与 pi-go 自身测试分别承担什么责任。
4. 为什么当前代码放在 `internal/`，以及何时才应该设计公共 SDK。
5. 一课从阅读到提交的完整门禁。

## 共同阅读

Pi：

- `packages/agent/src/types.ts`
- `packages/agent/src/agent-loop.ts`
- `packages/agent/src/agent.ts`
- `packages/agent/test/agent-loop.test.ts`
- `packages/agent/test/agent.test.ts`

Go：

- [Organizing a Go module](https://go.dev/doc/modules/layout)
- [The Go Programming Language Specification](https://go.dev/ref/spec)
- [`context` package](https://pkg.go.dev/context)

本课不会逐段讲完这些 Pi 文件。目标是先画出模块责任边界；具体行为在对应课程中回到源码和测试逐项验证。

## 第一阶段讲义：行为契约与实现机制

行为契约是 Runtime 使用者或测试能够观察到、替换实现后仍必须成立的事实。判断时问三个问题：

1. 调用方能否在不读取内部字段的情况下观察它？
2. 改变它是否会改变 transcript、事件、工具副作用、错误或停止结果？
3. 能否为它写一个不依赖 TypeScript 私有实现的黑盒测试？

三个答案大多为“是”，它通常就是需要移植的行为契约。

Pi 当前基线中的典型契约：

- Agent 忙碌时再次 `prompt()` 必须失败，调用方应该使用 steering、follow-up 或等待完成。
- `agent_end` 是最后一个运行事件，但只有订阅者处理完该事件后 Agent 才真正回到 idle。
- steering 在内部 tool loop 的 turn 边界消费，follow-up 在 Agent 原本准备停止时消费。
- 模型因长度限制而截断的 tool call 不能执行，即使残缺参数碰巧能够解析。
- 并行工具的完成事件可以按实际完成顺序出现，但写回模型 transcript 的 tool results 保持原 tool call 顺序。

实现机制是当前代码为满足契约选择的语言和数据结构。替换它不会自动改变产品行为。Pi 当前实现中的例子：

- 用 TypeScript `class Agent` 封装状态。
- 用 `AbortController` 表达取消。
- 用一个手工创建的 `Promise<void>` 表示 active run settlement。
- 用 `Set` 保存 listeners 和 pending tool call IDs。
- 用 async callback 和 `Promise.all()` 组织事件等待与并行工具。

Go 版本不复制这些机制。我们会根据契约选择 `context.Context`、显式状态、接口、mutex、goroutine 或其他 Go 结构。选择是否正确，由 pi-go 的事件、transcript、错误、取消和工具副作用测试验证。

### Headless 讨论给出的第一个例子

“没有 TUI，能够在本地驱动 Agent Runtime”是当前产品边界；必须保留。“现在就提供外部调用协议”不是当前需求，因此没有进入设计。`cmd/pi-go` 只承担本地运行与验收，未来 Agent Manager 如何调用 Runtime 留待 gRPC 或公共 SDK 设计。

## 概念层级：Agent、Run、Turn、Event、Handler 与 Idle

课程后续使用 `run_start/run_end` 作为语义术语。冻结 Pi 源码中的字面事件仍是 `agent_start/agent_end`；引用源码时保留原名。

- `Agent` 实例：长期存在的有状态对象，持有 transcript、model、tools 和消息队列；它可以先后执行多个 Run。
- `Run`：一次 `prompt()` 或 `continue()` 启动的低层完整循环，从 `agent_start` 开始。`agent_end` 是该 Run 的最后一个 loop event，但 Run settlement 还要等待该事件的 handlers 和运行期清理。
- `Turn`：一次模型响应，加上该响应触发的工具调用和 tool results。一个 Run 可以因工具结果、steering 或 follow-up 包含多个 Turn。
- `Event`：Run 中的可观察事实，例如 `message_end`、`tool_execution_end`、`turn_end` 和 `agent_end`。
- `Handler`：订阅并消费 Event 的回调。handler 的“结束”是函数返回或异步结果 settle；Pi 不会为单个 handler 发出 `handler_end`。
- `Idle`：不是 AgentEvent，而是运行状态。只有最终事件的 awaited handlers 返回，`finishRun()` 清理 runtime state、resolve active-run promise 并删除 `activeRun` 后，Agent 才 idle。

`agent_end` 的准确含义是：“本次 agent-core Run 不会再产生新的 loop events。”它不表示 Agent 实例被销毁，不表示 Session 永久关闭，也不表示收到该事件的 handlers 已经执行完成。

Pi 的实际时序是：

```text
loop 决定停止
  -> emit(agent_end)
  -> Agent 归约当前事件状态
  -> 按顺序等待 agent_end handlers
  -> loop executor 返回
  -> finishRun()
  -> idle
```

因此，一个 `agent_end` handler 阻塞两秒时，`state.isStreaming` 仍为 `true`，新的 `prompt()` 因 `activeRun` 仍存在而被拒绝，`waitForIdle()` 也仍在等待。

coding-agent 在 agent-core 上又增加 `agent_settled`：一次低层 Run 的 `agent_end` 之后，上层可能继续自动重试或压缩；这些后处理链全部完成，`AgentSession` 才发出 `agent_settled`。课程讨论“结束”时必须明确是 agent-core Run、coding-agent Session 操作，还是未来 Manager 管理的任务。

### 理解检查 3：Agent 实例与 Run

- 问题：同一个 Agent 实例先后执行两次 `prompt()`，会有几个 Agent、几个 Run 和几个 `agent_end`？
- 学习者回答：一个 Agent 实例、两个 Run、两个 `agent_end`。
- 课程结论：正确。`agent_end` 不销毁 Agent 实例，每个 Run 都有自己的 `agent_start` 和 `agent_end`。

### 讨论：`agent_end` 是否应该改名为 `run_end`

- 语义判断：`run_start/run_end` 更准确，因为事件界定的是一次 `prompt()` 或 `continue()` 启动的 Run，不是 Agent 实例的生灭。
- 课程决定：Go 的运行时概念、字段和讲解统一使用 Run，例如 `activeRun`、`finishRun`、`run_start` 和 `run_end`。
- 源码引用：讲解冻结 Pi 实现时保留其字面事件名 `agent_start/agent_end`，避免把课程术语误写成上游事实。
- 后续门禁：第 05 课在生命周期模型确定后定义 Runtime 内部的 Go 事件命名；开始和结束事件必须成对命名，不能只改一端。未来外部协议不在该课提前确定。

### 讲解：错误和取消为什么仍然产生 `agent_end`

- 教学修正：学习者表示尚不清楚错误或取消时是否应该产生 `agent_end`，需要先讲解再检查；不能要求学习者在没有行为模型时猜答案。
- 两个维度：`agent_end` 表达 Run 的事件流已经终止；成功、错误和取消表达 Run 的结果。生命周期闭合与结果分类不能混为一谈。
- 正常路径：最后一个 Turn 完成，loop 没有更多工具、steering 或 follow-up，发出 `agent_end`。
- 模型错误或取消路径：最终 assistant message 的 `stopReason` 为 `error` 或 `aborted`；Pi 先发出 `turn_end`，再发出 `agent_end`，然后退出 loop。
- 意外异常路径：`Agent.runWithLifecycle()` 捕获异常，构造失败 assistant message，并补齐 `message_start`、`message_end`、`turn_end` 和 `agent_end`；`finally` 中始终执行 `finishRun()`。
- 必要性：所有终止路径拥有统一的最终事件，订阅者才能刷新输出、持久化失败记录和执行清理；`waitForIdle()` 才能收敛，Agent 也不会永久停留在 busy 状态。
- 语义结论：`agent_end` 不等于 success。结果应从最终 message 的 `stopReason`、`errorMessage` 等字段判断，不能通过是否出现 `agent_end` 判断。
- 理解检查：判断 Run 是否成功，应看是否出现 `agent_end`，还是最终 assistant message 的 `stopReason` 和 `errorMessage`？
- 学习者回答：选择最终 assistant message 的 `stopReason` 和 `errorMessage`。
- 课程结论：正确。终止事件和结果字段承担不同责任。

### 教学顺序纠正：RunEnd settlement

- 原问题：RunEnd handler 阻塞两秒时，`isStreaming`、新 `prompt()` 和 `waitForIdle()` 分别是什么状态？
- 学习者反馈：尚未获得足够讲解，无法回答；后续课程必须先讲再问。
- 讲解：RunEnd 事件通过 `processEvents()` 顺序等待 handlers。handlers 未返回时，loop executor 尚未返回，`runWithLifecycle()` 的 `finally` 尚未调用 `finishRun()`。
- 直接结果：`activeRun` 仍存在，因此 `isStreaming=true`，新 `prompt()` 被拒绝，active-run promise 尚未 resolve，`waitForIdle()` 继续等待。
- 命名观察：Pi 的 `isStreaming` 不只表示 Provider 正在输出 token，它覆盖整个尚未 settlement 的 Run，语义更接近 `isRunActive` 或 `busy`；Go 版本不能机械照搬字段名，第 05 课再决定状态模型。
- 课程修正：本题不再作为未讲解的预测题；完成上述源码时序讲解后，才允许进行新的理解检查。

### 讲解：Streaming、Running 与 Busy 不是同一层状态

- 学习者观察：`isStreaming` 容易被理解为 LLM 正在流式输出；如果它表达整个 Run 正在处理，`isRunActive` 或 `isRunBusy` 更准确。
- 源码结论：方向正确。agent-core 在创建 `activeRun` 后把 `isStreaming=true`，直到 RunEnd handlers settle 并执行 `finishRun()` 才恢复为 false；它覆盖模型流、工具执行、Turn 间切换和最终 settlement。
- Provider streaming：只描述 assistant message 是否正在接收增量，Pi 主要通过 `message_start/update/end` 和 `streamingMessage` 表达。
- Agent running：描述低层 active Run 是否存在；对 agent-core，`IsRunning()` 或 `activeRun != nil` 最准确。
- Session busy：可能还包含自动重试、压缩、post-run continuation 或其他上层工作；`busy` 必须先说明所属层，不能直接作为 agent-core 字段名。
- 单一事实源：Go 版本不应分别维护 `activeRun` 和可写 `isRunning` 布尔值。应以 active Run 为事实源，公开状态从它派生，避免两个字段更新顺序不同而失配。
- Pi 风险证据：`Agent.reset()` 会直接设置 `isStreaming=false`，但不清除 `activeRun`。若调用方在 Run 中直接 reset，状态显示与 `prompt()` 的 busy 检查可能不一致；现有 coding-agent 清理流程通常先 abort、等待 idle，再 reset，但 agent-core 没有在 API 上强制该顺序。
- 第 05 课候选：内部保存单个 active-run record；公开 `IsRunning()` 从它派生。是否需要显式 `Running/Settling/Idle` phase，要由真实调用方和状态测试证明，当前不提前加入状态枚举。
- 理解检查：应同时保存 `activeRun` 和 `isRunning` 并手动同步，还是只保存 `activeRun`、让 `IsRunning()` 从它派生？
- 学习者回答：选择只保存 `activeRun`，从它派生 `IsRunning()`。
- 课程结论：正确。运行对象是事实源，布尔查询不应成为第二份可独立修改的状态。

### 讲解：Active Run 的互斥、取消与等待

Active Run 同时承载三个可观察契约：

1. 互斥：同一个 Agent 同时最多一个 Run；Run active 时新 `prompt()` 或 `continue()` 必须失败。
2. 取消：调用取消只发送 cooperative cancellation request，不会把 Run 立即变成 idle；Provider、工具和 handlers 仍要观察信号并退出。
3. 等待：`waitForIdle()` 只有在 RunEnd handlers settle 且 `finishRun()` 完成后才返回。

Pi 使用 `ActiveRun { promise, resolve, abortController }` 实现这三项。它们是 TypeScript 机制，不是必须逐字移植的 API。Go 候选是一个私有 active-run record，包含 `context.Context`、`context.CancelFunc` 和只在 settlement 后关闭的 `done` channel；Agent 用 mutex 原子地检查并安装 active Run。

```go
type activeRun struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

type Agent struct {
	mu        sync.Mutex
	activeRun *activeRun
}
```

Go 中“先检查 `activeRun == nil`，再赋值”必须位于同一临界区；否则两个并发 goroutine 都可能看到 nil 并同时启动 Run。取消函数只请求停止，`activeRun` 仍保留到所有执行路径退出和 handlers settle。等待方读取当前 Run 的 `done`，在锁外等待，避免阻塞取消和清理。

对应关系：

| 行为契约 | Pi 机制 | Go 候选机制 |
|---|---|---|
| 单 active Run | `activeRun` 检查 | mutex 下检查并安装私有 run record |
| cooperative cancellation | `AbortController.signal` | `context.Context` + `CancelFunc` |
| settlement wait | 手工 `Promise<void>` | settlement 后关闭 `done` channel |

当前只建立候选机制，不在第 00 课实现；并发线性化、取消传播和 channel 关闭顺序在第 05 课用 race test 和确定性场景决定。

- 理解检查：调用取消后应立即让 `IsRunning()` 变为 false，还是等待 Provider、工具和 handlers 退出并完成 settlement？
- 学习者回答：选择完成 settlement 后才变为 false。
- 课程结论：正确。取消请求不是完成信号；提前清除 active Run 会允许新旧副作用并发。

### 讲解：并行工具的完成顺序与 Transcript 顺序

一个 assistant message 可以按源码顺序请求多个工具，例如 A、B。并行执行时 B 可以先完成，因此实时 `tool_execution_end` 事件允许按 B、A 出现；但写入 transcript 的 tool-result messages 和 `turn_end.toolResults` 必须恢复为 assistant tool-call 源顺序 A、B。

这两个顺序服务不同责任：

- completion order 表达真实时间，供进度 UI、日志和监控及时观察；不应为了 transcript 排序而延迟已经完成的工具事件。
- source order 表达模型原始请求结构，供 transcript、重放和后续模型上下文保持确定性。

Pi 的并行机制为每个 source-order entry 创建 Promise；工具完成时立即发出自己的完成事件，`Promise.all()` 的返回数组仍按输入 Promise 顺序排列，之后再按该数组写入 tool-result messages。`Promise.all()` 是机制，“事件按完成顺序、transcript 按源顺序”才是契约。

Go 候选会为 tool calls 预分配按 source index 排列的 result slots，goroutine 把结果写回自己的 slot，同时通过单独的 completion channel 发送实时完成事件；等待全部完成后再按 index 生成 transcript。不能让 goroutine 按完成时间并发 append 到共享 transcript slice，否则既有 data race，也会把偶然调度顺序固化进上下文。

完整并行实现、race test 和顺序测试属于第 04 课；第 00 课只学习识别这两个可观察顺序。

- 理解检查：A 先被模型请求、B 先完成时，应如何排列完成事件和 transcript？
- 学习者回答：完成事件 B、A；transcript A、B。
- 课程结论：正确。真实时间顺序和模型源顺序是两个独立契约。

## 第二阶段讲义：项目内验证边界

Pi 源码和测试用于理解上游行为，回答“Pi 在冻结版本中做了什么、为什么这样做”。它们是学习材料，不进入 pi-go Runtime。

pi-go 只验证自身，按责任使用三类测试：

1. 单元测试：验证 active Run 互斥、参数校验、队列规则等局部不变量。
2. 组件与集成测试：用 Faux Provider、临时 workspace 和可控工具结果验证完整事件流、取消、handler settlement 与 transcript。
3. 端到端验收：驱动 pi-go 的 headless 入口完成受控 coding 任务，检查进程退出、文件副作用和任务断言。

前两类默认离线且确定性；真实 DeepSeek 调用只能是显式启用的 smoke test。端到端失败说明 pi-go 没有满足该场景，但仍需用更小的确定性测试定位根因，不能只根据最终 patch 猜测 Agent Loop 的具体缺陷。

## 讨论与练习

### 已确认的前置讨论

- 日期：2026-07-15
- 结论：本课程中的 headless agent 指 `headless Agent Runtime`，不是简单的一次性 CLI，也不是最终的多用户 Agent Manager。
- 边界：Runtime 负责 Agent Loop、Goal、工具和 Session；Manager 后续负责用户、仓库、并发、worktree 和 IM 路由。
- 影响：当前课程保留本地可运行入口和可恢复 Session，但不提前实现外部调用协议或常驻多租户服务。

### 练习 1：识别产品契约

从 `agent-loop.ts` 中分别找出：

- 对外可观察、需要在 Go 中保持的行为；
- 只属于 TypeScript 实现方式、可以改变的机制。

先写出自己的判断，再与课程实现约束对照。

请先判断下面五项属于“行为契约”还是“实现机制”，并为每项写一句理由：

1. 同一个 Agent 同时只能有一个 active run。
2. active run 使用 `AbortController` 保存取消信号。
3. `agent_end` listener 完成前，新的 `prompt()` 仍然被拒绝。
4. listeners 保存在 JavaScript `Set` 中。
5. 并行工具写入 transcript 时保持模型给出的 tool call 顺序。

### 练习 2：选择 pi-go 的测试层级

讨论下面三个现象分别最适合由哪类测试发现：

1. 并行工具执行完成顺序不同，导致 transcript 顺序变化。
2. 取消 bash 后子进程仍存活，Run 无法 settlement。
3. headless Runtime 完成任务后修改了错误文件。

## 本课 Go 实现范围

本课只创建：

```text
go.mod
```

冻结 Pi commit 和 package version 只保存在课程文档。第 00 课不创建课程专用 Go package。本课不引入第三方依赖。

## 验证场景

1. `go list -m` 返回 `github.com/yuanbohan/pi-go`。
2. 课程总纲、本课文档与决策记录使用同一冻结 Pi baseline。
3. 根 module 只建立模块边界，不形成公共 SDK 承诺。
4. 仓库状态能清楚区分本课文件与尚未开始的课程。

## Go 实现记录

实现日期：2026-07-15。

新增文件：

- `go.mod`：module 为 `github.com/yuanbohan/pi-go`，Go language version 为 `1.26.0`。

曾创建 `internal/contract` 保存 Pi commit 和 package version，并用测试检查文档一致性。学习者 review 后确认这是错误边界：课程元数据不属于 Runtime 产品能力。该 package 已完整删除，冻结基线恢复为纯文档责任。

验证结果：

```text
go list -m
github.com/yuanbohan/pi-go
```

本课没有 Go package 或测试，没有引入第三方依赖，没有生成 `go.sum`，也没有创建 public SDK、Agent API、Provider 或工具实现。

## 完成门禁

- 学习者能用自己的语言回答五个学习目标。
- 本课 module 边界验证通过；本课没有 Go package 测试。
- 本课讨论造成的决定已经写入本文件或 `docs/course/decisions.md`。
- 学习者明确确认理解。
- 学习者明确要求 commit；在此之前保持未提交。

## 讨论记录

### 基线现场验证

- 验证日期：2026-07-15
- Pi checkout HEAD：`dcfe36c79702ec240b146c45f167ab75ecddd205`
- `packages/agent/package.json` version：`0.80.7`
- 结论：本地 Pi checkout 与课程冻结基线一致，可以作为后续源码阅读参考。

### 理解检查 1：上游变化时对齐哪个版本

- 问题：Pi 最新 `main` 改变行为时，第一轮 pi-go 移植应该先对齐冻结 commit、最新 `main`，还是效果更好的版本？
- 学习者回答：选择冻结的 Pi commit。
- 课程结论：回答正确。冻结 commit 是当前源码阅读坐标；升级到最新 `main` 必须作为单独、可追溯的基线迁移进行。

### 理解检查 2：Listener 容器与顺序

- 问题：把 TypeScript `Set` 换成无序 Go `map`，导致 listeners 随机执行，是否可以接受？
- 学习者回答：不可以；需要先判断 `Set` 是否用于去重，以及顺序是否影响 Agent 行为。如果顺序是核心行为，Go 必须保证顺序。
- 课程结论：回答正确，而且提出了需要分开验证的两个语义。
- 源码事实：Pi 使用 `Set` 注册和删除 listener，因此同一函数引用重复添加会去重，遍历遵循插入顺序。
- 契约证据：`packages/agent/README.md` 明确声明 listeners 按 registration order 等待；`Agent.subscribe()` 的源码注释也作出相同承诺。
- 当前判断：注册顺序是必须保留的契约。相同函数引用去重是可观察行为，但当前没有文档或测试证明它是有意承诺；在 Agent 生命周期课程决定 Go 订阅 API 时再显式选择，不通过模拟 JavaScript 函数身份来盲目复制。
- 影响：listener 顺序会决定外部持久化、日志、排队消息和错误传播的先后。即使 Agent 在通知前已经更新内部状态，顺序仍可能影响下一轮输入和调用方观察结果。

### 讨论：`listeners` 命名与实际职责

- 学习者问题：`listeners` 是否与这组回调的实际作用匹配？
- 表层职责：接收 `AgentEvent`，用于流式输出、UI 更新、日志等事件通知。
- 核心职责：`processEvents()` 先把事件归约进 Agent state，再按订阅顺序逐个等待回调；因此这组回调还构成事件生产者的 backpressure 和当前 Run 的 settlement barrier。
- 上层证据：coding-agent 的 `AgentSession` 使用一个 agent-core listener 串联 extension 事件、Session 事件通知和消息持久化，说明它不只是 UI observer。
- 边界：listener 不返回新的 Agent 决策，不负责选择模型、调用工具或控制 loop；它消费已经产生的事件。其异步完成和失败仍会影响事件流水线与 Run 结算。
- 命名判断：Pi 中的 `listeners` 符合常见事件 API 习惯，但没有表达“有序、被等待、形成背压”的强语义。`observers` 更不准确，因为 observer 通常暗示纯观察；`lifecycleHooks` 也过窄，因为这里还有 message 和 tool 事件。
- Go 最小候选命名：公开方法保留 `Subscribe`，回调类型使用 `AgentEventHandler`，内部先表达为 `handlers []AgentEventHandler`，事件处理方法使用 `dispatchEvent` 或 `reduceAndDispatchEvent`。
- 设计修正：仅为保证顺序不需要 `subscription` 抽象，但 Pi 的 `subscribe()` 已经返回退订函数，所以要保留完整订阅行为就需要稳定的注册身份。Go 函数不能相互比较，不能按函数值从 slice 中可靠删除指定 handler；纯 `handlers []AgentEventHandler` 只能表达有序分发，尚不能独立解决完整退订语义。
- 当前边界：稳定注册身份可以由退订闭包捕获私有注册项，不等于需要公开 `ID()`。是否增加私有注册项、内部 ID 或公开 handle，必须等 Agent 生命周期课程明确退订时序后再选择最小机制。

### 讨论：退订是否需要公开 ID

- 学习者假设：如果退订是必要 feature，可能需要 `ID()` 或类似方法定位需要取消的订阅。
- Review 结论：退订是 Pi 已存在的公开能力，当前移植范围需要保留；但“需要稳定注册身份”不能直接推出“需要公开 ID”。返回的幂等退订函数可以捕获私有注册对象或内部 token，不向调用方暴露身份管理。
- API 压力：公开 ID 只有在调用方需要跨所有者、跨进程、持久化或稍后按编号管理订阅时才有明显价值；当前 Agent 内事件订阅没有这些要求。公开 ID 还会引入 ID 作用域、失效、复用和错误退订等额外契约。
- Pi 现场实验：`Set` 的当前遍历是活的。已执行的 A 删除尚未执行的 B 并新增 C 时，本次遍历结果为 A、C；B 被跳过，C 会收到当前事件。
- 未解决契约：退订是否幂等；同一 handler 重复注册是否形成一个或多个注册；当前事件分发期间退订何时生效；分发期间新增 handler 是否接收当前事件；订阅与退订是否要求并发安全。
- 课程要求：第 05 课先从 Pi 源码和测试确认已承诺的行为，再为 pi-go 写确定性测试，最后决定使用 live traversal、snapshot、私有注册项、内部 ID 或公开 handle。不能从数据结构反推产品契约。
- 学习者回答：A 在处理事件 E 时退订尚未执行的 B，选择 B 不再接收当前事件；对于分发期间新增的 C 是否接收 E，当前不知道如何判断。
- 补充证据：Pi 文档和 agent-core 测试没有声明分发期间新增或退订的语义；当前仓库用法也没有发现依赖“新 listener 立即接收正在分发的事件”。
- 可靠性候选：事件开始时复制有序注册项，调用每项前再检查其 active 状态。这样 B 在轮到自己前被退订会跳过，事件开始后新增的 C 从下一个事件才生效。
- 候选理由：退订表示调用方不再希望接收通知，应尽快生效；C 在事件开始时尚不是接收者，而且 live addition 允许 handler 不断新增新 handler，使一次事件分发可能无界增长。这个候选不同于 Pi 未文档化的 `Set` 遍历结果，必须在第 05 课作为明确的 pi-go 决策记录后才能实现。
- 术语澄清：“从下一个事件生效”指下一次 `dispatchEvent()` 调用，而不是下一 Agent turn 或下一次 Run。C 会立即进入真实注册表，但不会进入当前事件已经创建的 handler 快照；下一事件创建新快照时才会包含 C。

### 学习原则修正

- 学习者要求：导师需要持续 grill 和 challenge 学习者，不因学习者的局部判断而降低最终实现可靠性。
- 课程接受：合理。导师也必须挑战自己先前给出的结论；双方赞同不构成证据。
- 本次反例：上一轮过快把退订当成尚未确定的未来需求，忽略了 Pi `subscribe()` 已返回退订函数。修正为：退订能力属于当前移植范围，具体 Go 身份机制仍未决定。

### Review：为什么当前没有 `.go` 源文件

- 学习者观察：仓库当前只有 `go.mod` 和 Markdown，没有 Go package 或测试。
- 课程结论：这是第 00 课的有意边界。`go.mod` 只建立 module path 和 Go version；真正的 package 必须由承担已学习行为的 `.go` 文件建立。
- 设计约束：不为显示项目结构而创建空 package、占位接口或只验证课程元数据的测试。第一份 Runtime Go 代码应在第 01 课讲清 AI 协议后实现。
- 文档纠正：课程地图曾写“最小测试骨架”，与实际范围不符，现已删除。第 00 课的代码产物只有 `go.mod`。

### 00-D 理解检查：源码证据与项目内验证

- 问题 1：是否应在尚未讲清行为责任时创建空的 `internal/ai` package？
- 学习者回答：不应该；应在第 01 课讲清行为责任后创建对应代码。
- 问题 2：pi-go 的 listener 顺序测试能否直接证明 Pi 的行为？
- 学习者回答：不能；它只证明 pi-go 满足测试契约，Pi 的行为需要从冻结源码、文档和测试中确认。
- 问题 3：遇到 Pi 未文档化且可能不安全的行为时，应机械复制、静默修改，还是形成显式决定？
- 学习者回答：记录 Pi 的源码事实，检查调用方和测试依赖，再明确决定 pi-go 契约并为它写测试。
- 课程结论：三项回答正确。学习者已经区分上游证据、Go 设计决定和 pi-go 自身验证责任。

### 00-E 范围修正：当前不提供外部调用

- 学习者决定：当前 pi-go 不给外部项目调用；以后根据真实需求考虑 gRPC 或公共 Go SDK。
- 课程结论：`cmd/pi-go` 只作为本地运行和验收入口，其输入输出不构成稳定兼容协议。`internal/` 保留核心实现可调整性。
- 后续边界：跨进程、跨语言调用更可能使用 gRPC；同进程 Go 嵌入更可能使用公共 SDK。两者可以共存，但当前课程不做选择或预留虚构接口。

### 00-F 理解确认

- 确认日期：2026-07-15
- 学习者确认：已理解第 00 课，同意进入“待提交”。
- 提交授权：学习者已明确要求 commit，并直接推送到 `main`。

后续在这里继续记录练习答案、分歧和对课程的调整。

## 提交记录

- 提交日期：2026-07-15
- 提交说明：第 00 课学习契约、冻结基线、课程计划与最小 Go module。
- 目标分支：`main`
