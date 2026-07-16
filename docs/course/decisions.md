# 课程与架构决策

这个文件记录跨课程有效的当前决定。新证据推翻旧决定时，直接更新对应决定，并在“变更记录”说明原因；Git 历史保存旧版本。

## 当前决定

### D1. 做语义移植，不做逐文件翻译

- 日期：2026-07-15
- 决定：Go 实现对齐 Pi 的可观察行为与设计约束，允许采用 Go 原生接口、goroutine、channel 和 `context.Context`。
- 原因：TypeScript 的类、Promise 和异步迭代器不是产品契约；机械复制会掩盖 Go 的并发与取消语义。

### D2. 第一期只实现模型与工具循环

- 日期：2026-07-15
- 决定：`internal/agent` 负责通用 Provider turn、transcript 和 tool loop；第一期不实现结构化 plan/progress/replan/done/blocked Goal Runtime。模型停止调用工具时只表示 loop 正常结束，coding task 是否成功由固定外部验收独立判断。
- 原因：当前目标是证明最小 coding loop 能完成真实任务。提前增加 Goal 状态机会把 Agent Loop 能力和上层目标策略混在一起。

### D3. DeepSeek-first，但 Provider 边界可替换

- 日期：2026-07-15
- 决定：第一个真实 Provider 只实现 DeepSeek；模型 ID、base URL 和密钥来自运行配置。非本地 base URL 默认必须使用 HTTPS，测试服务器需要显式开发覆盖。实现 Provider 课程时重新核对官方模型和协议，不把当前模型别名写成课程契约。
- 原因：先减少兼容矩阵，同时保留未来扩展其他 Provider 的边界。

### D4. Faux Provider 是 Agent Loop 的首要验证设施

- 日期：2026-07-15
- 决定：先完成可脚本化的 Faux Provider，再接真实模型。
- 原因：事件顺序、工具错误、取消和 transcript 必须能确定性复现，不能依赖付费 API 或模型随机性。

### D5. 当前交付 headless Agent Runtime，不做 SDK-first

- 日期：2026-07-15
- 决定：入口位于 `cmd/pi-go`，实现包位于 `internal/`；该命令只用于本地运行和验收，当前不向其他项目承诺调用协议。公共 Go SDK、gRPC、其他网络 RPC 和 IM 适配均推迟设计。
- 原因：核心 API 尚处于学习和校正阶段，过早公开会固化错误抽象。出现真实外部调用方后，再根据同进程嵌入或跨进程服务选择接口。

### D6. 不移植 TUI

- 日期：2026-07-15
- 决定：不实现主题、按键、命令提示、交互式选择器和 slash command UI。
- 原因：它们不决定 coding loop 是否能完成任务，会稀释当前课程对核心语义的关注。

### D7. 第一期是单目录、单任务，不建立 Agent 配置和 Session

- 日期：2026-07-15
- 决定：`cmd/pi-go` 每次只接收一个 workspace 和一条 task prompt，transcript 只在当前进程内存中存在。仓库级 Agent 配置、每任务 Session、跨任务上下文、多 active run 和多用户并发全部延后。
- 原因：第一期验收只需要一个真实 coding task。长期“Agent 按仓库、Session 按任务”的方向可以在二期重新验证，但不应提前生成配置或持久化抽象。

### D8. 第一期没有审批或 trust/yolo 策略系统

- 日期：2026-07-15
- 决定：模型选择的工具直接执行，不逐次请求批准，也不提供 trust/yolo 配置矩阵。`read`、`write`、`edit` 强制 workspace 文件边界；bash 只固定初始 cwd，不是 sandbox，可以访问当前用户有权访问的 workspace 外资源。bash 使用最小 allowlist 环境加显式非敏感配置；Provider 凭据不能进入子进程、argv、tool config、transcript、事件、日志或错误。参数校验、超时、取消和输出截断始终生效。
- 原因：逐工具审批会阻断长任务；第一期也没有足够场景证明需要策略抽象。不审批不等于隐藏权限或取消安全不变量。

### D9. 课程与代码按同一个提交节奏推进

- 日期：2026-07-15
- 决定：每课维护讲义、代码、测试和讨论记录；只有学习者明确说“开始第 NN 课”后才进入学习状态，理解确认前不进入下一课，只有学习者明确要求时才 commit。第 00 课是只建立 module、文档和验证证据的无 Runtime 代码例外。
- 原因：仓库要能还原完整理解过程，而不只是最终代码；占位 package 不能替代已经理解的行为实现。

### D10. 冻结上游参考基线

- 日期：2026-07-15
- 决定：第一轮课程固定参考 Pi commit `dcfe36c79702ec240b146c45f167ab75ecddd205`、agent package version `0.80.7`。
- 原因：固定源码坐标后，课程中的行为结论才能追溯到同一份实现；上游变化需要显式重新阅读和决策。

### D11. Provider stream 使用 pull boundary，Agent event 使用串行 awaited sink

- 日期：2026-07-15
- 决定：Provider 对 Agent 暴露可取消的 pull/receive 流抽象；Agent 通过等待完成的 event sink 通知观察者。并行工具 worker 只把 outcome 交给当前 Run 的 coordinator，由 coordinator 串行调用 sink，不能并发或重入 handler。
- 原因：pull boundary 把结束、错误和取消放在一个读取协议中；awaited sink 对齐 Pi 的 settlement 语义；单一 coordinator 同时保护事件顺序和 handler 并发边界。

### D12. 工具 schema 与 Go 参数校验分离

- 日期：2026-07-15
- 决定：Tool 向模型提供 JSON Schema，同时拥有 Go 侧参数解码和校验逻辑；初始版本不实现通用 JSON Schema 执行器。
- 原因：模型提示 schema 和运行时安全校验是两个责任。四个核心工具可以用强类型输入完成校验，不需要提前引入完整 schema framework。

### D13. 第一期只保留内存 transcript

- 日期：2026-07-15
- 决定：Run 入口接收一次原始 task 并将它转换成 transcript 的首条 user message；每次 Provider 调用收到包含 workspace/cwd 说明的稳定 system prompt、完整有序 transcript 和当前 tool schemas，不增加独立 `Task` 或 `WorkspaceContext` AI 协议字段。第一期不持久化 transcript，不实现 checkpoint、恢复、自动 compaction、摘要或跨 Session 上下文。
- 原因：基础多轮上下文已经足以验证 coding loop；持久化和压缩会引入版本、恢复和敏感数据存储契约，必须由二期真实需求驱动。

### D14. 第一轮 bash 进程树支持 macOS 和 Linux

- 日期：2026-07-15
- 决定：初始 bash executor 在 macOS 和 Linux 上实现进程组取消与测试；Windows 运行时支持在核心课程后单独设计。
- 原因：进程树控制是操作系统相关行为。当前开发与验收环境是 macOS，提前声称未经验证的跨平台等价会隐藏取消缺陷。

### D15. Agent Runtime 与 Agent Manager 分层

- 日期：2026-07-15
- 决定：第一期只构建单目录、单任务的本地 Runtime。后续 Agent Manager 才负责用户、仓库目录、Session、并发、worktree、GitHub 和 IM 路由；这些职责不能进入通用 Agent Loop。
- 原因：headless 只描述无 UI 的运行形态，不等于当前已经提供外部服务或多租户调度。

### D16. 学习过程以证据驱动，不以对话共识驱动

- 日期：2026-07-15
- 决定：导师持续质疑学习者的设计假设，同时复查自己的既有判断。讨论必须区分学习者假设、已验证的 Pi 契约、候选 Go 机制和已确定的 pi-go 决策；冻结 Pi 的源码、文档、测试和可复现实验优先于对话中的赞同。每个新概念必须先完成术语、源码路径和例子讲解，再进行理解检查。
- 原因：未经挑战的共识会把遗漏固化进 Go 实现，降低长期可靠性。

### D17. 课程语义使用 RunStart 和 RunEnd

- 日期：2026-07-15
- 决定：课程和 Runtime Go 命名使用 `run_start/run_end` 描述一次 Run 的边界；引用 Pi 源码和原始事件轨迹时保留 `agent_start/agent_end`。这不预先确定未来 gRPC 或 SDK 的公开命名。
- 原因：结束的是一次 task Run，不是长期存在的 Agent 实例；引用源码时也不能篡改上游事件名。

### D18. 冻结基线不进入 Runtime package

- 日期：2026-07-15
- 决定：Pi commit 和 npm package version 只记录在课程文档，不生成 `internal/contract` 或其他课程专用 Runtime package。
- 原因：冻结上游是源码学习责任，不是 Agent Runtime 的产品能力。

### D19. 工具调用使用屏障式分段调度

- 日期：2026-07-15
- 决定：按模型源顺序线性扫描 tool calls；连续且显式 `CanRunParallel` 的调用组成一个并行阶段，其他调用各自形成串行屏障。能力默认 false，第一期只有 `read` 为 true，`write`、`edit`、`bash` 和未知工具均为 false。事件可按真实完成顺序出现，写入 transcript 的 tool results 必须恢复模型源顺序。
- 原因：保留无副作用 read 的简单并行能力，同时避免把整个混合批次并行造成读写和命令竞态。该策略是 pi-go 对 Pi 整批调度的有意差异。

### D20. 普通工具错误形成结果并继续

- 日期：2026-07-15
- 决定：未知工具、非法参数、执行错误、单工具 timeout 和非零 bash exit 都形成 call-local error tool result；同批后续阶段继续执行，下一轮模型收到全部有序结果。单工具 timeout 只取消自己的 child context；只有 Run context 取消才停止未开始工作。
- 原因：调度器不知道模型调用之间的语义依赖，不能通过一个局部失败替模型猜测是否跳过后续调用；把错误交回模型更利于恢复。

### D21. Run 取消必须等待所有已启动工作收敛

- 日期：2026-07-15
- 决定：Run 取消停止新的 Provider 调用和工具阶段，取消所有已启动 child contexts，等待 stream、tool workers、awaited event sink 和 bash process group 收敛，然后只发出一次最终 canceled Run 事件并返回非成功结果。已启动调用保留 completed 或 canceled outcome；未启动调用不制造 synthetic result。
- 原因：取消请求不是完成信号。过早返回会遗留 goroutine、进程或晚到事件，并允许新旧副作用重叠。

### D22. 文件工具使用 os.Root；bash 不宣称 workspace containment

- 日期：2026-07-15
- 决定：Run 开始时用 `os.Root` 打开固定 workspace；`read`、`write`、`edit` 只执行 root-relative I/O 并拒绝非 regular-file 目标，不使用“先检查绝对路径再普通 open”的易竞态模式。bash 只从 workspace 启动，仍可访问当前用户资源。
- 原因：文件工具需要抵抗路径穿越和 symlink TOCTOU；cwd 不是命令 sandbox，不能把 fixture diff 或 canary 误写成主机级隔离。

### D23. 最终验收使用固定 bug-fix fixture，不在仓库内实现 Pi 比较

- 日期：2026-07-15
- 决定：最终验收使用 checked-in prompt 和一个初始测试失败的固定小型 Go fixture。Agent 必须自主读取、修改并运行测试；独立 harness 检查测试成功、不可修改文件哈希、生产文件 allowlist、相邻 canary 和遗留进程。Pi 与 pi-go 的效果比较由学习者在仓库外手动执行，不创建 benchmark、eval package、进程比较协议或评分工具。
- 原因：固定 fixture 同时提供可重复的 Runtime 验收和人工对比材料，而不会把一次主观效果比较污染成产品代码。

### D24. DeepSeek 数据外发必须明确，默认事件必须有界

- 日期：2026-07-15
- 决定：运行前明确提示 system prompt、task、模型选择的文件内容、命令和 tool output 会发送给 DeepSeek；操作者自行选择可披露 workspace，Runtime 不增加确认或审批交互。read/bash 内容在进入 transcript、Provider request 或 event preview 前完成大小限制；默认事件只呈现 metadata、状态和有界 preview。第一期没有通用 secret detector 或 redactor。
- 原因：操作者必须理解真实数据边界；减少重复输出能降低额外泄漏，但不能被描述为阻止模型看到完成任务所需的 tool results。

### D25. 第一阶段 AssistantMessage 只保留 Loop 可观察字段

- 日期：2026-07-16
- 决定：`AssistantMessage` 第一阶段只保留有序 content blocks、token usage、stop reason 和 error message。暂不加入 api/provider/model、response ID/model、diagnostics、timestamp、cost 或未核验的 cache/reasoning usage 细分；stream delta 不重复携带完整 partial message，terminal event 携带权威 final/aborted message。
- 原因：当前 Agent Loop 只观察内容、用量和终态；其余字段属于 Pi 的多 Provider、路由、Session/UI、诊断或费用生态。第 02 课若出现真实 partial snapshot 消费者，或第 04 课核对 DeepSeek 后出现新的 usage/trace 需求，再用证据扩展内部协议。

## 变更记录

- 2026-07-15：建立初始课程和架构决策，并补充 stream、tool validation、Session storage、平台范围与 Runtime/Manager 边界。
- 2026-07-15：增加证据驱动的学习原则、先讲后问顺序和 `run_start/run_end` 课程术语。
- 2026-07-15：移除课程专用 `internal/contract`；冻结基线仅保存在文档。
- 2026-07-15：取消当前阶段的外部调用承诺；`cmd/pi-go` 仅用于本地运行与验收。
- 2026-07-15：第一期收敛为单目录、单任务的最小 coding loop；Goal Runtime、Session、完整 lifecycle/subscription、Manager 和外部集成移至二期。
- 2026-07-15：确定屏障式工具调度、错误继续、取消 settlement、`os.Root` 文件边界、DeepSeek 数据外发和固定 bug-fix 验收。
- 2026-07-16：根据冻结 Pi `Context` 与 coding-agent system prompt 证据，明确 workspace/cwd 说明进入稳定 system prompt，不建立独立 `WorkspaceContext` AI 协议字段。
- 2026-07-16：学习者确认原始 task 只作为 transcript 首条 user message，不在 Provider Request 中建立第二份 `Task`；第一阶段完整 transcript 是单一事实源。
- 2026-07-16：学习者确认第一阶段 `AssistantMessage` 的最小字段与删减范围；partial snapshot 和 Provider/模型诊断元数据按真实消费者延后。
