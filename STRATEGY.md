---
name: pi-go
last_updated: 2026-07-19
---

# pi-go Strategy

## Target problem

对希望在 Go 中长期构建、理解并演进 coding agent 的工程者来说，关键能力和设计证据分散在 Pi 及其他 coding agent 中。机械移植只能复制某个时点的基线，零散拼接机制又容易破坏状态、工具和生命周期的一致性；同时，模型随机性使单次演示无法证明新 agent 真的不弱于或强于基线。

## Our approach

以冻结的 Pi coding agent 作为语义基线、能力下限和公平对照，用 Go 原生方式建立结构一致、可解释、可测试的核心系统。持续研究其他优秀开源 coding agent 以及 Codex、Grok 等可获得的工程证据；只有当候选机制能融入既有责任边界，并通过受控、重复评测证明收益时才吸收。

## Who it's for

**Primary:** 正在研究和构建 coding agent 的软件工程师——他们使用 pi-go，把不同 agent 中分散的设计证据转化成结构一致、可理解、可测试、可持续演进的 Go coding agent，并客观判断每次演进是否真正提升了 coding 能力。

## Key metrics

- **相对 Pi 的 coding resolve rate**——在相同模型、任务、初始仓库和资源约束下重复运行；通过未来的受控对照评测体系测量，Pi parity 是下限，稳定超过是目标。
- **IM 任务端到端完成率**——从 IM 创建任务，到 Session 返回通过独立验证的 coding 结果且无需人工修复运行状态；通过 Gateway、Orchestrator 和 Session telemetry 测量。
- **Session 连续性与恢复率**——连接中断、进程重启、暂停或长任务恢复后，Session 能从权威状态继续且不污染其他 Session；通过恢复测试和运行 telemetry 测量。
- **长上下文任务完成率**——需要多轮工具调用、context compaction 和多次推进的任务最终通过验证的比例；通过长任务评测集测量。
- **每个成功任务的成本**——每个通过验证的任务所消耗的 token、Provider 成本、turn 数和 wall time；通过 Provider usage 与 Session outcome 联合测量。

## Tracks

### Coding Capability Core

建设 Go-native 的模型循环、工具、上下文管理和 coding 能力，使 Pi parity 成为能力下限，并为吸收更有效的 agent 机制提供一致核心。

_Why it serves the approach:_ 没有可靠且边界清楚的核心，外部机制无法安全组合，也无法公平比较。

### Session and Runtime Reliability

建设完整 Conversation 与 Session 生命周期，包括 context compaction、持久化、恢复、并行 Session 隔离、取消和故障收敛。

_Why it serves the approach:_ 长任务和多任务运行要求完整事实、模型工作上下文及执行生命周期能够独立演进并可靠恢复。

### Orchestration and Access

建设 Gateway、Orchestrator、任务生命周期和 IM 接入，使用户能通过聊天创建、推进、暂停、恢复和查看 coding task。

_Why it serves the approach:_ Coding capability 需要成为可持续驱动的服务，而不是永远停留在一次性本地命令。

### Evaluation and Agent Research

建设 Pi 对照评测与候选机制实验体系，持续研究其他开源 coding agent，并用重复、可比较的结果决定是否吸收其方案。

_Why it serves the approach:_ 评测把“看起来更强”转化为可验证的能力进步，并防止项目退化成功能堆砌。

## Not working on

- 不机械复制 Pi 或其他 agent 的文件结构、API 形状和完整功能清单。
- 不因为某个机制流行或单次演示成功，就在缺少责任边界和对照证据时加入产品。
- 不从单个 fixture、单次模型运行或主观体验中宣称 pi-go 已经超过 Pi。
