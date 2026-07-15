# 课程与架构决策

这个文件记录跨课程有效的决定。新证据推翻旧决定时，直接更新当前决定，并在“变更记录”中说明原因；Git 历史保存旧版本。

## 当前决定

### D1. 做语义移植，不做逐文件翻译

- 日期：2026-07-15
- 决定：Go 实现对齐 Pi 的可观察行为与设计约束，允许采用 Go 原生接口、goroutine、channel 和 `context.Context`。
- 原因：TypeScript 的类、Promise 和异步迭代器不是产品契约；机械复制会掩盖 Go 的并发与取消语义。

### D2. 先移植低层 Agent，再增加 Goal Runtime

- 日期：2026-07-15
- 决定：`internal/agent` 保持通用，负责 turn、stream、tool、queue 和状态；`internal/goal` 在其上负责 plan、progress、replan、done 与 blocked。
- 原因：目标规划是一种上层执行策略，不应改变低层工具循环的确定性语义。

### D3. DeepSeek-first，但 Provider 边界可替换

- 日期：2026-07-15
- 决定：第一个真实 Provider 只实现 DeepSeek；模型 ID、base URL 和密钥来自配置。非本地 base URL 默认必须使用 HTTPS，测试服务器需要显式开发覆盖。
- 原因：先减少兼容矩阵，同时保留未来扩展到其他 Provider 的边界。当前默认候选是 `deepseek-v4-pro`，课程开始实现 Provider 时重新核对官方文档，避免模型下线造成隐式漂移。

### D4. Faux Provider 是 Agent Loop 的首要验证设施

- 日期：2026-07-15
- 决定：先完成可脚本化的 Faux Provider，再接真实模型。
- 原因：事件顺序、工具错误、取消和队列行为必须能确定性复现，不能依赖付费 API 或模型随机性。

### D5. 当前交付 headless Agent Runtime，不做 SDK-first

- 日期：2026-07-15
- 决定：入口位于 `cmd/pi-go`，实现包优先位于 `internal/`；该命令只用于本地运行和验收，当前不向其他项目承诺调用协议。公共 Go SDK、gRPC、其他网络 RPC 和 IM 适配均推迟设计。
- 原因：核心 API 尚处于学习和校正阶段，过早公开会固化错误抽象。出现真实外部调用方后，再根据同进程嵌入或跨进程服务的需求选择公共 SDK、gRPC 或两者组合。

### D6. 不移植 TUI

- 日期：2026-07-15
- 决定：不实现主题、按键、命令提示、交互式选择器和 slash command UI。
- 原因：它们不决定 coding loop 是否能完成目标，会稀释当前课程对核心语义的关注。

### D7. Agent 按仓库配置，Session 按真实任务创建

- 日期：2026-07-15
- 决定：仓库级 Agent 配置提供模型、prompt、工具和执行策略；每个独立任务创建 Session，后续允许多用户并行。
- 原因：长期能力和短期任务状态的生命周期不同。

### D8. 执行策略可配置，初始默认 trusted

- 日期：2026-07-15
- 决定：headless 长任务不逐次请求批准。初始默认使用类似 YOLO 的 `trusted` 策略。read、write 和 edit 强制 workspace 路径边界；bash 只固定初始 cwd，不宣称是 sandbox，因此可以访问当前用户有权访问的 workspace 外资源。bash 使用最小继承环境加 Agent 配置显式环境变量，不直接复制父进程全部环境；Provider 凭据禁止进入工具子进程、transcript 或日志。参数校验、超时、取消和输出截断始终生效。
- 原因：逐工具审批会阻断长任务，但“不审批”不等于隐藏真实权限或取消运行时安全不变量。需要真正隔离时，应使用后续的受限策略或外部 sandbox。

### D9. 课程与代码按同一个提交节奏推进

- 日期：2026-07-15
- 决定：每课同时维护文档、代码和测试；只有学习者明确说“开始第 NN 课”后才进入学习状态，课程设计确认不算开课；理解确认前不进入下一课，只有学习者明确要求时才 commit。
- 原因：仓库本身要能还原完整的理解和构建过程，而不只是最终代码。

### D10. 冻结上游参考基线

- 日期：2026-07-15
- 决定：第一轮课程固定参考 Pi commit `dcfe36c79702ec240b146c45f167ab75ecddd205`、package version `0.80.7`。
- 原因：固定源码坐标后，课程中的行为结论才能追溯到同一份实现；上游变化需要显式重新阅读和决策。

### D11. Provider stream 使用 pull boundary，Agent event 使用 awaited sink

- 日期：2026-07-15
- 决定：Provider 对 Agent 暴露可取消的 pull/receive 流抽象；Agent 通过等待完成的 event sink 通知观察者。Provider 内部可以使用 goroutine 和 channel，但不把 channel 暴露为跨层契约。
- 原因：pull boundary 能把结束、错误和取消放在一个读取协议中；awaited sink 对齐 Pi 在运行完成前等待 listener settlement 的语义。

### D12. 工具 schema 与 Go 参数校验分离

- 日期：2026-07-15
- 决定：Tool 向模型提供 JSON Schema，同时拥有 Go 侧参数解码和校验逻辑；初始版本不实现通用 JSON Schema 执行器。
- 原因：模型提示 schema 和运行时安全校验是两个责任。四个核心工具可以用强类型输入完成校验，不需要为单一实现提前引入完整 schema framework。

### D13. Agent 配置与 Session 状态分开存放

- 日期：2026-07-15
- 决定：仓库可在 `.pi-go/agent.json` 持有可审阅的 Agent 配置；Session transcript、checkpoint 和原始事件默认写入可配置的操作系统用户状态目录，并用 repo identity 与 session ID 定位。目录和文件使用仅当前用户可读写的权限；初始版本不提供静态加密。
- 原因：Agent 配置适合版本管理，任务对话和运行状态可能包含代码、工具输出或敏感信息，不应默认进入目标仓库。

### D14. 第一轮 bash 进程树支持 macOS 和 Linux

- 日期：2026-07-15
- 决定：初始 bash executor 在 macOS 和 Linux 上实现进程组取消与测试；Windows 运行时支持在核心课程后单独设计。
- 原因：进程树控制是操作系统相关行为。当前开发与验收环境是 macOS，提前声称未经验证的跨平台等价会隐藏取消缺陷。

### D15. Agent Runtime 与 Agent Manager 分层

- 日期：2026-07-15
- 决定：当前课程构建 headless Agent Runtime。一个仓库拥有一份 Agent 配置，一个真实任务拥有一个可恢复 Session，同一 Session 只有一个 active run。初期不同 Session 可通过独立进程和 worktree 并行；多用户、仓库目录、调度和 IM 路由由后续 Agent Manager 负责。
- 原因：Headless 只描述无 UI 的运行形态，不等于当前已经提供外部调用协议，也不应把多租户服务职责混入 Agent Loop。

### D16. 学习过程以证据驱动，不以对话共识驱动

- 日期：2026-07-15
- 决定：导师持续质疑学习者的设计假设，同时复查自己的既有判断。讨论必须区分学习者假设、已验证的 Pi 契约、候选 Go 机制和已确定的 pi-go 决策；冻结 Pi 的源码、文档、测试和可复现实验优先于对话中的赞同。每个新概念必须先完成术语、源码路径和例子讲解，再进行理解检查。
- 原因：学习者尚在掌握 Pi，导师也可能过早接受局部合理但证据不足的方案。未经挑战的共识会把遗漏固化进 Go 实现，降低长期可靠性。

### D17. 课程语义使用 RunStart 和 RunEnd

- 日期：2026-07-15
- 决定：后续课程讲解使用 `run_start/run_end` 描述一次 Run 的边界；引用 Pi 源码和原始事件轨迹时保留其字面名称 `agent_start/agent_end`。这项术语决定只约束课程和 Runtime Go 命名，不预先确定未来 gRPC 或 SDK 的公开命名。
- 原因：结束的是一次 `prompt()` 或 `continue()` 启动的 Run，不是长期存在的 Agent 实例；引用源码时也不能篡改上游事件名。

### D18. 冻结基线不进入 Runtime package

- 日期：2026-07-15
- 决定：Pi commit 和 npm package version 只记录在课程文档，不生成 `internal/contract` 或其他课程专用 Runtime package。根 Go module 只实现 pi-go Runtime。
- 原因：冻结上游是源码学习责任，不是 Agent Runtime 的产品能力。把课程元数据放进核心 package 会污染依赖边界并制造没有运行时价值的抽象。

## 变更记录

- 2026-07-15：建立初始课程和架构决策，并补充 stream、tool validation、Session storage、平台范围与 Runtime/Manager 边界。
- 2026-07-15：增加证据驱动的学习原则，要求持续挑战学习者假设和导师既有判断。
- 2026-07-15：增加先讲解后检查的教学顺序，并统一课程中的 RunStart/RunEnd 语义术语。
- 2026-07-15：移除课程专用 `internal/contract`，冻结基线仅保存在文档。
- 2026-07-15：取消当前阶段的外部调用承诺；`cmd/pi-go` 仅用于本地运行与验收，gRPC 和公共 SDK 留待真实调用方出现后设计。
