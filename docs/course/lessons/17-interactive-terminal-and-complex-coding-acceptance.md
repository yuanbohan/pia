# 第 17 课：简化交互终端与复杂 coding task 验收

## 当前状态

学习者于 2026-07-30 明确开始本课，并在完整讨论 Host/Session/Engine 的输入
ownership、Steering safe point、Cancel 后队列策略和真实验收目标后要求直接
实现。本课于 2026-07-31 完成实现、离线/race 验证、真实 PTY 与固定复杂任务
验收；状态为**已完成，尚未提交**，规模为 **Large**。

本课是第三阶段收尾课。它提供一个 direct-Session、本地、non-alternate-screen
的行式交互终端，并用固定的多 package Go dependency-planner 任务验证同一个
长期 Session 能否通过 initial input、current-execution Steering 和后续
Advance 持续完成复杂 coding 工作。正式 TUI、Daemon、common Client Protocol、
journal 与 resume 均不在本课实现。

详细 product/implementation/acceptance contract 见
[Phase 3 Interactive Terminal Acceptance Plan](../../plans/2026-07-30-002-feature-phase-3-interactive-terminal-acceptance-plan.md)。

## 开课源码校准

### 固定证据

- 冻结 Pi `dcfe36c79702ec240b146c45f167ab75ecddd205` 的 interactive
  session 把提交后的多条 pending message 作为一个 ordered batch 交给 Agent；
  restore 操作会取回全部 pending input，并以空行连接回 composer。
- Codex CLI `0fb559f0f6e231a88ac02ea002d3ecd248e2b515` 提供 active-turn
  input 与 future queued input 分层，以及编辑最近 queue item 的对照；Pia
  只采纳 ownership 分层，不复制它的单项编辑策略。
- OpenCode `cb562b2c6289c2eee707078f9ab644cbe1d3d8a9` 的 direct
  interactive runtime 同样把 prompt queue 放在 durable Session state 之外；
  其 message identity/edit protocol 不是本地验收终端的必要前提。
- 当前 Pia `cmd/pia` 只有 one-shot 入口；`Session.TrySteer` 已经能在
  start、post-tool 和 would-stop safe points 向同一 Engine Run 注入输入，
  但 Lesson 16 的 one-input `Advance` 和 abnormal hand-back 会迫使 Terminal
  重新解释已经被 Session 接受的 ownership。

### 被确认、细化和推翻的假设

- **确认：** Terminal 只拥有 draft、Session 尚未接受的 FIFO Submissions、
  control intent 和 presentation projection；Session 继续独占 History、
  projection、active Advance、accepted Steering 和 lifetime。
- **细化：** 一个 Advance 仍是一次 synchronous external operation，但它可以
  接受一个 ordered non-empty Submission batch；每个 batch element 是独立
  user Message，并在第一次 Provider request 前全部可见。
- **推翻：** Lesson 16 的“一个 Advance 只由一个 initial submission 启动”
  不是长期必要边界。真实 Host 需要原子转移完整 FIFO batch，拆成第一次 input
  加 synthetic Steering 会让首个 Provider request 看不到完整初始约束。
- **推翻：** `UnconsumedSteering` 会让 `TrySteer(true)` 的 ownership
  acknowledgement 在 Cancel/error 后反悔。accepted-but-unseen Steering 现在
  由 Session 在 Engine terminal delta 后直接提交 History；只有
  `TrySteer(false)` 的整批输入继续属于 Host。
- **确认：** 本地终端采用 Bubble Tea v2 与 Bubbles textarea，不使用 alternate
  screen。Bubble Tea 负责 raw terminal input、modifier-aware keys 和串行
  Update；本课不自建 escape-sequence parser。

## 已确认的行为契约

### 输入与 ownership

- 每次非空 Enter 先产生一个 Host-owned Submission。
- idle 时，Host 把完整 pending FIFO 作为一个 `Advance` batch 转移；active
  时，Host 用完整 batch 调用 `TrySteer`。
- `TrySteer(true, nil)` 永久转移整批 ownership；`false, nil` 不转移任何
  element，Host 保留原 batch。
- Session/Engine 保留每个 batch element 的独立 user Message 边界，不把文本
  join 成一个 prompt。
- Cancel/Close/Provider error 发生在 safe-point drain 前时，Session 在
  Engine terminal delta 后按 admission order 提交 accepted pending Steering，
  不执行、不 hand back、不重复。

### 本地交互策略

- 正常 Advance settlement 自动启动仍由 Host 持有的 queue；Cancel 或 error
  settlement 保持 idle pending，等待操作者确认。
- idle pending 时，空 Enter 原样执行，非空 Enter 追加后整体执行。
- `Alt+Up` 一次取回全部 Host pending，以 `\n\n` 按 FIFO 顺序连接在当前 draft
  之前，并清空 queue；再次 Enter 后它成为一条新的 Submission。
- ESC 只在 active 时 idempotently 请求 `Cancel()`；不修改 draft、Host queue
  或 History。Ctrl+C/Ctrl+D 本课不支持。
- 精确的 trimmed `/exit` 调用 `Close`、忽略 Host pending，并在 clean
  settlement 后退出。

### 输出与 trace

- 终端实时显示 bounded semantic progress：Advance、Run、Turn、tool、
  compaction、Steering admission、queue hold 和 cancel intent。
- terminal assistant text 只在 Advance settlement 后完整显示；raw reasoning、
  token delta、Provider events 和 Bash byte stream 不进入 live UI。
- 所有持久输出通过 Terminal Update loop 的小型 pending-output queue 顺序交给
  inline renderer，Terminal 不接管历史 scrollback。
- `/exit` 后，`PIA_TRACE_PATH` 继续保存完整 History 和本次 Session 的
  settlement evidence。已经由操作者恢复的 Advance error 进入 trace，但不让
  最终 clean exit 失败。

## 实现与发现记录

此节记录会影响产品边界、验收可信度或后续演进的事实；普通机械调用点迁移不逐项
罗列。被忽略的每次真实 attempt 保存更完整的 terminal log、trace、workspace
diff 与 verifier output。

| 日期 | 阶段 | 事实或症状 | 分析与修复 | 验证 |
|---|---|---|---|---|
| 2026-07-31 | Runtime batch | 旧 API 只能把一个字符串作为 initial input，且 Provider failure 会返回 `UnconsumedSteering` | `Engine.Run`、pre-run compaction、`Session.Advance` 与 `TrySteer` 改为 ordered non-empty batch；删除 hand-back。abnormal settlement 在 Engine delta 后提交 accepted pending Steering，并发 drain/seal/cancel 仍共用 Session mutex | batch message boundaries、whole-batch validation/clone、Provider failure、caller/Cancel/Close cancellation、final-seal race 和 overflow recovery tests；focused race tests 通过 |
| 2026-07-31 | Terminal focus | 真实 PTY 能渲染 composer，terminal reader trace 也显示已解析 `hello`，但 textarea 始终不显示输入 | `terminalModel.Init` 是值接收者；在那里调用 pointer `Focus()` 只修改临时副本。把 focus commit 移到 constructor，使进入 Bubble Tea loop 前的 authoritative model 已 focused，并增加 printable Unicode regression test | 失败测试先复现空 composer；修复后测试通过。tmux PTY 输入 `/exit` 显示 `closing`，pane 以 status 0 退出 |
| 2026-07-31 | Close/result ordering | active 状态输入 `/exit` 时，`Close` 与 `Advance` commands 会并发返回；若 `CloseDone` 先被 Update 处理，旧实现会在保存当前 `AdvanceResult` 前退出并写出不完整 trace | clean Close 不再直接 quit；Terminal 等待 active Advance settlement 和持久输出排空。Close grace expiry 单独标记 force-quit，避免不配合取消的 Provider 把有界 host grace 重新变成无限等待 | 增加 Close-first、final-output-before-quit 和 grace-expiry tests；最新 PTY clean `/exit` 仍以 0 退出并生成 `0600` trace |
| 2026-07-31 | Terminal output hygiene | Provider/transport error 与 terminal assistant text 可能包含控制字符；错误文本还可能无界增长。output queue 完成后也会保留旧 backing slice | 所有持久文本按行转义控制字符；错误投影限制为 2 KiB 并保持 UTF-8；queue 排空时释放 backing slice。长时间运行 commands 只 capture Session/context/batch，不 capture 整个 model | 增加单行、byte bound、UTF-8 tests；`make check` 的 vet/lint 全部通过 |
| 2026-07-31 | Observation settlement | accepted-but-unseen Steering 直接提交 History 后仍发出 user `Message` observation，但旧注释只允许“accepted into Working Context”；关闭 source channel 还会让 wait command 重复读取零值 | 把 `Message` 定义细化为 run-local Working Context 或 settlement History commit；closed channel 映射为一次 `terminalObservationStopped` | observation package tests 与 closed-source Terminal test 通过 |
| 2026-07-31 | Verifier fidelity | 第一版外部 verifier 只要求 unknown dependency/cycle 返回非零，通用无信息错误也会误通过，且没有外部覆盖 unsupported format | 在真实 attempt 前冻结更强 contract：unknown dependency/target 必须命名缺失项，cycle 必须可识别，unsupported `yaml` 必须返回识别 format/value 的错误 | baseline 自身 `go test ./...` 与 `go vet ./...` 通过；外部 verifier 注入 immutability test 后按预期失败于未实现 target planning |
| 2026-07-31 | Attempt 001 | 普通 `tmux paste-buffer` 没有 bracketed-paste framing；multiline initial task 的每个 newline 被解释为 Enter，产生 1 个 initial input 和 9 个错误的 Steering submissions | attempt 判为 transport-invalid 并完整保留；改用 `paste-buffer -p`，在发送 Enter 前先确认全部 multiline text 仍位于 composer | trace 明确显示 10 个被拆分 user messages；未把该次模型结果计入成功证据 |
| 2026-07-31 | Attempt 002 | fresh baseline 通过真实 bracketed paste 接收 exact initial、active Steering 与 later input | 不再修改产品、task、baseline 或 verifier；clean `/exit` 后用 trace byte comparison、workspace gates 和外部 verifier 独立检查 | exactly 2 Advances、1 accepted Steering、3 exact user messages；workspace tests/vet 与 external verifier 全部 PASS |

## 固定 depplan 验收项目

验收项目位于 ignored `tmp/pia-terminal-acceptance/`，不进入 Pia 产品代码：

```text
protocol.md
baseline/
bin/
attempts/<NNN>/workspace/
verifier/
evidence/
```

Baseline 是一个自身 tests 已通过、但尚不具备目标功能的 Go module，包含：

- `cmd/depplan`：CLI composition；
- `internal/manifest`：读取并验证 package manifest；
- `internal/planner`：计算 dependency plan；
- `internal/render`：text/JSON presentation。

固定交互分三段：

1. initial Advance：实现 `--target` dependency closure、shared dependency
   dedupe、unknown dependency/cycle error 和相应 tests；
2. active Steering：同层依赖必须保持 manifest source order，planner 不得
   修改 caller-owned manifest/input；
3. later Advance：增加 `--format=json`，同时保持默认 text output 和已有 tests。

外部 verifier 位于 Agent workspace 之外，并在 verification copy 中检查 shared
dependency、stable order、unknown dependency、cycle、input immutability、
default text compatibility 与 ordered JSON。一次成功必须同时具备固定 binary、
baseline、task、Steering、follow-up、trace、workspace tests、vet 和 external
verifier 证据；任何产品、prompt、routing、task、baseline 或 verifier 变化都会
使旧成功失效。

## 最终验证

- Runtime batch 与 permanent accepted-Steering ownership：已完成 focused
  offline tests 和 focused race tests。
- Terminal Host state machine：已完成 Enter、whole-batch Steering、
  unavailable retention、normal auto-drive、Cancel/error hold、restore-all、
  ESC、Ctrl+C/D no-op、`/exit` 和 observation projection tests。
- 真实 PTY `/exit` smoke：修复 focus bug 后通过。
- 全仓 `make check` 与 `make race`：通过；Session final-seal/cancellation
  Steering 与 Terminal Close/auto-drive 关键用例在 race detector 下各重复
  20 轮通过。
- 固定 depplan baseline/verifier：已建立；baseline 自测为绿、外部 verifier
  按预期为红，三段输入已拆为独立冻结文件。
- 真实 Provider complex-task acceptance：Attempt 002 通过。trace 为 `0600`、
  settlement error 为空、共有 61 条 protocol messages，其中恰好 3 条 user
  messages 与 frozen initial/Steering/later files byte-for-byte 相同。
- 交互 transcript 中恰好有 2 次 `advance started`、2 次
  `advance settled (success)` 和 1 次 `steering accepted: 1 submission(s)`。
  trace 中 Steering 位于完整 tool result 后、下一 assistant turn 前；later
  input 位于第一次 terminal assistant 后。
- Attempt 002 workspace `go test ./...`、`go vet ./...` 与 workspace 外 external
  verifier 全部通过。verifier 另外 build CLI，并注入 deep immutability test。

冻结输入摘要：

| Artifact | SHA-256 |
|---|---|
| Pia binary | `2745f44b902979d0792c29f4d9a772edfec05943f6d9887a163ad2fdc63cba0d` |
| baseline tree | `c3c2ac8ae0dbfc308142456f319c71abe25ff45ae7b39a1e62b68b94e461ebf8` |
| verifier tree | `b0112484417a0869b1ee9fd3d0525c3f10d37e22caaf4a74164803e0d5871995` |
| input tree | `2dbb7f0716d7d338d7530f060ca39097ec369088f69fb07007ad12ee1d3ec1ee` |
| protocol | `8481d4bbbb2ec0a453b9a81c70f706f10e3b155da66e7c5059120e89a634965f` |

成功输出摘要：

| Artifact | SHA-256 |
|---|---|
| final workspace tree | `6f7965f265691d9c7a2d2db186e628de3d94eba039e4115cba5e719114334d67` |
| baseline diff | `c8339092daa80f9883c873665aeda87739e50792a24ec9eee314ae927ee9498f` |
| trace | `2bf0c2414273fcf772eb8667150db50a0e43a89c7f84f94f203894dbc9576ca8` |
| terminal transcript | `ac3a4048c337edc92b9cefea4038baa130ee204eed69611a4d880eef1ce5286e` |

## 非目标

- 不实现 Daemon、common Client Protocol、public SDK 或 client adapter。
- 不实现 journal、resume、hard-kill checkpoint 或 multi-Session orchestration。
- 不实现 full-screen panes、theme、selector、历史 scrollback owner、raw stream
  rendering 或 accepted-Steering retract/edit。
- 不把一次模型成功解释为正式 benchmark，也不强制在真实 attempt 中触发 Cancel
  或 compaction；这些边界由确定性 tests 独立验证。

## 本课文件

课程完成时更新本节；当前主要范围为：

- `internal/agent/loop.go` 与 tests；
- `internal/coding/{session,steering,compaction}.go` 与 tests；
- `cmd/pia/{main,interactive,terminal}.go` 与 tests；
- `internal/observation/event.go` 的 settlement observation 语义说明；
- `go.mod`、`go.sum`；
- `AGENTS.md`、`README.md`、本课、课程索引、决策、概念与
  implementation-ready plan。

## 提交信息

尚未提交。学习者尚未要求提交本课。
