# 第 16 课：Daemon-ready Session input 与 control 边界收敛

## 当前状态

学习者于 2026-07-30 明确开始本课。冻结 Pi
`dcfe36c79702ec240b146c45f167ab75ecddd205`、Codex CLI
`0fb559f0f6e231a88ac02ea002d3ecd248e2b515`、OpenCode
`cb562b2c6289c2eee707078f9ab644cbe1d3d8a9`，以及当前 Pia 的 Session、
Steering、Follow-up、Agent Execution Engine 与 one-shot composition 路径已经
完成开课校准。

本课状态为**已提交**，规模为 **Medium**。职责、
产品方向、运行时代码、测试与课程文档已经收敛。本课完成了删除式边界修正：
删除 Session-owned Follow-up 和 public Wait，把 Steering admission 收敛为
明确的 ownership-transfer 尝试；没有实现 Daemon、Client Protocol、Gateway、
后台 worker 或持久化 submission inbox。

D108 在本课完成后修正了第三阶段路线：D107 的 Session/future Submission
ownership 与严格 C/S 长期方向保持不变，但第三阶段不再继续 journal/safe
resume，而是以一个 direct-Session 的本地行式交互终端完成复杂 coding task
验收。该终端是开发与验收例外，不恢复被本课删除的 Session-owned future
queue。

> **Lesson 17 修正（2026-07-31）：** 真实 Terminal Host 证明 atomic routing
> 需要 ordered non-empty input batch，而不是一个 Advance 只接收一条 initial
> submission。D109 因此把 `Advance(ctx, string)`/`TrySteer(string)` 改为 batch
> surface；每个 element 仍是独立 user Message。`TrySteer(true)` 现在是永久
> ownership transfer，abnormal settlement 把 accepted-but-unseen Steering
> 直接提交 History，删除 `UnconsumedSteering`。本课下文的单 input 与 hand-back
> 描述保留为 Lesson 16 的历史实现记录，不代表当前 Runtime。

详细 implementation-ready 契约与实施单元见
[Daemon-ready Session Boundary Plan](../../plans/2026-07-30-001-refactor-daemon-ready-session-boundary-plan.md)。

## 解锁能力

Lessons 13–15 逐步证明了 Session lifecycle、Follow-up 和 Steering 的局部语义，
但当前 `activeAdvance` 同时保存：

- active execution cancellation 与 completion；
- current-execution Steering window 和 queue；
- future Follow-up window 和 queue；
- 两类输入的异常 hand-back。

这种形状适合探索单进程 terminal，但不适合作为长期多客户端服务边界。未来
Terminal、GUI、Mobile 和 IM Gateway 都应以 Client 身份通过同一协议接入 Pia
Daemon。Server 必须能确认接收普通 Submission，然后自行决定启动 Advance、
尝试 Steering，或在当前执行之后启动下一次 Advance。把 future submission
queue 留在 Session，会让 Terminal/IM policy、server acknowledgement 和
Conversation lifecycle 混在一个对象里。

本课完成后，心智模型收敛为：

```text
Client
  owns draft + unacknowledged input
        |
        | future common protocol
        v
Pia Daemon
  owns acknowledged not-yet-started submissions
        |
        | Advance / TrySteer / Cancel / Close
        v
Session
  owns lifetime + History/projection + active Advance
  + Steering accepted for the current Engine invocation
        |
        v
Engine
  owns one run-local Provider/tool loop
```

Lesson 16 只把 Session 修正到该边界。Daemon 一层仍只作为产品方向记录，不在
当前 runtime 中创建 speculative `Coordinator`、`SessionHost` 或 service package。

## 开课源码校准

### 已确认的大纲假设

- 冻结 Pi 的 stateful core `Agent` 同时保存 Steering/Follow-up queues，inner
  loop 在 tool settlement 后拉 Steering，outer loop 在 would-stop 后拉
  Follow-up。这证明两种输入语义不同，但不证明 Pia 的长期 Session 必须照搬
  两类 queue ownership。
- Codex core 把能加入 active turn 的 input 交给 runtime，而 TUI 保存尚未开始
  的 queued submissions，并在当前 turn 结束后再提交。这提供了
  current-execution input 与 future submission 分离的证据。
- OpenCode 的 direct interactive runtime 在 Session 外串行保存 prompt queue；
  durable Session 与 execution registry 也不是一个对象。这同样说明 future
  delivery policy 不必进入 Conversation Session。
- 当前 Pia Engine 已经拥有完整 Provider/tool loop、terminal settlement 与
  Run ordering。把 Engine 缩成单次 Provider call 会把工具阶段和 terminal
  closure 拆回 Session，不能简化责任。
- 当前 `cmd/pia` 的生产路径只调用 `Info`、`Advance` 与 `Close`；没有生产
  caller 使用 `FollowUp`、`Steer`、`Cancel` 或 `Wait`。因此可以删除错误的
  application surface，而无须保留兼容 shim。

### 细化后的认识

- “Server 决定 Steer 还是后续 Advance”不是按输入是否为 Run 的第一条消息
  静态分类，而是 delivery routing：若存在可接入的 current execution，
  ordinary submission 可尝试 Steering；否则由 server 保留，并在 later
  Advance 中成为 initial input。
- Steering queue 仍属于 Session，因为 acceptance、drain、would-stop seal、
  Cancel 和 Close 必须在同一 mutex 下线性化。外层 server 只拥有尚未被
  Session 接受的 Submission。
- Engine 负责识别 safe point；Session 负责回答“当前安全点有哪些 accepted
  Steering”以及“若为空能否同时封口”。这是同步 boundary handshake，不要求
  Session 驱动 Engine 的每一步。
- `SteeringSource`、`Drain`、`DrainOrSeal` 是当前 Go mechanism，不是耐久产品
  术语。实现时可以保留或收敛其形状，但不能把同步原子 handshake 改成第二套
  goroutine/channel lifecycle。
- 一个 Advance 只处理一个 server submission，但仍可能包含同一 Run 的
  Steering、pre-Run compaction，以及 eligible overflow 后的 input-free
  continuation；“一个 submission”不等于“只调用一次 Provider”。

### 被推翻的先前结论

- Lesson 14 的 Session-owned Follow-up queue、同一 Advance 排空多个 future
  inputs 与 `UnconsumedFollowUps` 被本课完全取代。Follow-up 以后只可能是
  client-facing delivery intent；是否进入 common protocol 等待真实 client
  需求决定。
- D99/D100 的 public `Wait(ctx)` 被本课取代。Session 内部仍需 completion
  signal 让 Close 和 active settlement 收敛，但当前没有独立 application
  consumer 需要 Wait。
- D106 的 `Steer(input) error` 加 `ErrSteerUnavailable` 被
  `TrySteer(input) (bool, error)` 取代。“当前不能加入 execution”是正常
  routing miss，不是业务错误。
- 直接由最小 Terminal host 调用 Session 并拥有 future queue 的第三阶段路线
  被严格 C/S 方向取代。第三阶段到 Daemon-ready、可安全恢复的单 Session
  Runtime 为止；Client Protocol、Daemon 和各 client 在后续阶段拆分。

## 已确认的职责与 scope

### Client

- 拥有本地 draft 与尚未收到 server acknowledgement 的输入。
- 选择自己的 UI intent，例如 Terminal 未来可以提供显式 Follow-up action；
  IM 初期可以只有 ordinary message。
- 不直接判断 Session safe point，也不直接拥有 acknowledged future submission。

### Future Daemon service layer

- 未来拥有已确认但尚未开始的 Submission。
- 把 ordinary submission 路由为新 Advance、`TrySteer`，或 settlement 后的下一
  Advance。
- 未来拥有 submission identity、acknowledgement、result delivery、durable
  inbox 与 multi-Session routing；这些都不在本课设计。

### Session

- 拥有 Workspace/resources、History、projection、lifetime 与唯一 active
  Advance。
- 拥有当前 Engine invocation 已接受的 Steering，及其 open/sealed/drained
  状态。
- 保持 one-active-Advance defensive guard；不提供 future submission queue。
- 负责 History/projection commit、overflow continuation、Cancel 和 Close。

### Agent Execution Engine

- 拥有一个 invocation 的 Provider/tool loop、tool stage ordering、terminal
  settlement、cancellation closure 与 run-local delta。
- 在 start、post-tool 和 would-stop 识别 Steering safe points。
- 不拥有 long-lived Conversation state、future submissions、Session lifetime
  或第二个 active guard。

## 本课 application surface

目标 surface：

```go
NewSession(SessionConfig) (*Session, error)
(*Session).Info() SessionInfo
(*Session).Advance(ctx, input string) (AdvanceResult, error)
(*Session).TrySteer(input string) (bool, error)
(*Session).Cancel()
(*Session).Close(ctx) error
```

其中：

- `TrySteer` 返回 `true, nil` 才表示文本 ownership 转给 Session。
- `false, nil` 表示当前没有可接入的 invocation，caller 继续拥有文本。
- blank input、closing/closed 等真实失败返回 error。
- `AdvanceResult` 保留完整 `History` 与
  `UnconsumedSteering`，删除 `UnconsumedFollowUps`。
- `ErrSessionBusy` 继续防御错误的并发 Advance 调用，不承担 queue routing。

## Steering 在 loop 中的驱动

Steering 的安全点保持 Lesson 15 已验证的三处：

1. Engine invocation 已建立、第一次 Provider request 前；
2. 一条 assistant message 的所有 tool calls 都取得 terminal result 后；
3. assistant 无 tool calls、Engine 原本将停止的 would-stop 边界。

前两处 drain 当前已接受的完整 batch；第三处必须原子
drain-or-seal。Engine 识别“现在到了边界”，Session 用同一 mutex 决定“返回
哪些输入”或“确认空并永久封口”。Provider call、tool batch 中途，以及
error/cancellation settlement 已开始后都不读取 Steering。

Cancel/Close 与 drain 继续遵守 exactly-one：

- drain 先在线性化点取得 batch，输入进入 Engine/History，不再 hand back；
- Cancel/Close 先封口，pending batch 不进入 Engine，随 Advance result 唯一
  hand back。

Session 不判断 caller 应该自动重投、恢复 composer、显示 warning 还是忽略。

## Cancel、Close、Wait 与 client controls

- `Cancel()` 只 nonblocking、idempotent 地取消当前 active Advance；idle 时
  no-op，不关闭 Session，也不处理 future server inbox。
- `Close(ctx)` 是 Session lifetime/resource operation：永久停止新的
  Advance/TrySteer admission，立即取消 active work，并有界等待 settlement。
  timeout 不重新开放、不强关借用中的 Workspace、不冒充 clean close。
- Public `Wait(ctx)` 删除。Close 和 settlement 内部继续使用 private completion
  signal。
- Client disconnect、Terminal `/exit`、Daemon shutdown 与
  `CloseSession` 不是同一动作。严格 C/S 下，一个 client 断开不能默认关闭
  server-owned Session；具体 protocol command 与 authorization 留待后续课程。
- `Esc`、Tab、Enter 只是 client UI action，不进入 Session Runtime 词汇。未来
  Terminal 可以把它们映射为 Cancel、ordinary submission 或显式 delivery
  intent；IM 可以选择不同 UI，而共用 server protocol。

## 实施方向

本课实现只允许围绕既有 `internal/coding` 与 `internal/agent` 做窄删除和重构：

1. 删除 Follow-up error、API、window、queue、drain、hand-back 与专用测试。
2. 删除 public Wait 及其专用 API tests，保留 Close 所需 private completion。
3. 把 `Steer`/`ErrSteerUnavailable` 改为 `TrySteer` 的 bool transfer result。
4. 简化 `activeAdvance`，只保留 execution control 与 current Steering state。
5. 保留 Engine safe-boundary handshake；只有真实代码证据证明现有
   `SteeringSource` 名称或方法增加不必要层次时，才做同课内收敛。
6. 更新 one-shot test seams、课程与开发约束，不创建新的 server abstraction。

## 验证方向

- 一个 Advance 只由一条 initial input 启动，不自动排空第二条 future input。
- `TrySteer` 覆盖 blank、idle、active-open、paused/sealed、closing/closed。
- start、post-tool、would-stop batch 与 Provider request ordering 保持。
- final seal 与并发 `TrySteer` 只有 accepted 或 caller-retained 两种结果。
- drain/cancel、drain/close 竞态没有双消费、丢失或重复 hand-back。
- overflow recovery 继续把 accepted Steering 转移到 continuation；最终异常才
  hand back。
- Close 继续立即 cancel、有界等待、最终只关闭 resources 一次。
- one-shot observable behavior 不回归。
- 完成 Go 变更后运行 `make check` 与 `go test -race ./...`。

## 实现结果

- `internal/coding/session.go` 删除了 Follow-up errors、result field、admission
  state、queue、dequeue/hand-back loop、public `Wait` 和无人消费的
  per-Advance `done` channel。`Advance` 现在只执行 initial input，并继续允许
  accepted Steering 与 overflow continuation。
- `internal/coding/steering.go` 用
  `TrySteer(input) (bool, error)` 取代 `Steer(input) error` 和
  `ErrSteerUnavailable`。idle、paused 与 sealed 返回 `false, nil`；只有
  accepted input 才转移 ownership。现有 `SteeringSource`、`Drain` 和
  `DrainOrSeal` 保持不变。
- 删除 `internal/coding/follow_up_test.go`。其中仍属于 current-execution
  Steering 的 cancel/close hand-back 证据迁移到 Steering tests，并补充成功
  terminal 已提交、Cancel 先封口且 pending Steering 必须 hand back 的回归
  场景。
- `internal/agent` 无代码改动。Engine 仍完整驱动 Provider/tool loop，
  Session 只在既有 safe-boundary handshake 中提供 drain/seal。
- `make check` 通过，包括格式化、`go vet ./...`、`go test ./...` 与
  `golangci-lint`；`go test -race ./...` 通过。final-seal、caller
  cancellation、Cancel、Close 与 terminal-wins 专项 race tests 另以
  `-count=20` 重复通过。

## 非目标

- 不实现 Daemon、Gateway、Client Protocol、RPC、IM/TUI/GUI/Mobile。
- 不命名或创建 `Coordinator`、`SessionHost`、`SessionService` 等未来 owner。
- 不设计 submission ID、ack/result protocol、durable inbox 或 multi-Session
  scheduler。
- 不实现 Session journal、resume 或 in-place interrupted recovery。
- 不增加 generic command interface、public SDK、state polling 或 async Advance
  handle。

## 完成信号

- Session 不再拥有 Follow-up/future submission state，public Wait 已删除。
- `TrySteer` 清晰表达 ownership transfer，safe-boundary 与 cancellation
  线性化行为保持。
- 当前 Session code/state/tests 明显减少，one-shot、History、overflow、Cancel
  与 Close 契约不回归。
- 课程、决策、策略、词汇和开发约束一致表达严格 C/S 与 Daemon-ready Session
  边界。
- 默认检查与 race tests 全部通过，学习者已确认理解。
