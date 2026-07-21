# Pia

Pia 是面向学习、验证和长期演进的 Go coding agent。当前仓库名与 Go module path 仍保留 `pi-go`，以冻结的 [Pi](https://github.com/earendil-works/pi) coding agent 为语义基线，但产品和后续课程讨论统一使用 **Pia**。

项目以课程驱动：先阅读冻结的 Pi 源码并提炼可观察契约，再完成对应 Go 实现、测试、讨论记录和一次由学习者批准的提交。

第一阶段只构建最小 coding loop；Lesson 07 进一步明确其内存所有权：Coding Agent 的 Conversation Owner 保存同一对话的完整有序 Conversation History，Core Agent 保存可替换的 Working Context，每次 DeepSeek 请求使用后者的独立 snapshot。Lesson 08 已加入两个 Runs 之间的 Working Context compaction；Agent 可以多轮调用 `read`、`write`、`edit`、`bash` 并追加 tool results，直到模型停止调用工具。Lesson 06 用本地 `pia` 命令把这些能力组装为当前目录中的 one-shot coding agent，并通过本地、被忽略的 Go bug-fix 项目验证真实闭环。

第一阶段不实现 Goal Runtime、Session 持久化、TUI、公共 SDK、RPC/IM、多用户、多仓库、worktree/GitHub 管理、权限策略矩阵或 Pi 对比工具。bash 不是 sandbox；具体安全边界和验收约束记录在完整实施计划中。

长期目标是在迁移 coding-relevant Pi 能力后，让 Pi parity 成为能力下限，并通过同模型、同任务、多次独立运行的稳定评测追求可证明的持续超越。Skills 是 Pia 的核心能力；当前先实现 project-local Pia Skill v1 的最小可靠闭环，完整 Agent Skills、Claude Code/Codex community roots 与 vendor runtime compatibility 分阶段补充，而不是一次性扩建 Skill engine。Pi 是语义基线和主要对照组；其他优秀开源 coding agent 以及 Codex、Grok 的可获得证据用于发现候选机制，而不是直接复制。长期产品将通过 Orchestrator、Gateway 和 IM 驱动多个可持久化、可恢复且相互隔离的 Sessions；完整方向、指标和投入领域见[产品策略](STRATEGY.md)。

- [产品策略](STRATEGY.md)
- [课程总纲](docs/course/README.md)
- [设计决策](docs/course/decisions.md)
- [共享术语](CONCEPTS.md)
- [第一阶段基础实施计划](docs/plans/2026-07-15-001-pi-core-go-learning-port-plan.md)
- [Lesson 06：`pia` one-shot 实施计划](docs/plans/2026-07-19-001-feature-pia-one-shot-coding-agent-plan.md)
- [第 0 课：学习契约与基线](docs/course/lessons/00-learning-contract-and-baseline.md)
- [第 1 课：AI 协议与 Faux Provider](docs/course/lessons/01-ai-protocol-and-faux-provider.md)
- [第 2 课：单次 Provider Turn 与 transcript](docs/course/lessons/02-agent-loop-and-transcript.md)
- [第 3 课：多轮 Tool Loop 与屏障式调度](docs/course/lessons/03-tool-loop-and-staged-scheduling.md)
- [第 4 课：OpenAI-Compatible DeepSeek Provider](docs/course/lessons/04-deepseek-provider.md)
- [第 5 课：Coding Tools 与 Workspace 边界](docs/course/lessons/05-coding-tools.md)
- [第 6 课：Headless one-shot Coding Task](docs/course/lessons/06-headless-coding-task.md)
- [第 7 课：Conversation History、Working Context 与 Request Snapshot](docs/course/lessons/07-conversation-history-and-active-context.md)
- [第 8 课：Context budget 与 compaction 核心](docs/course/lessons/08-context-budget-and-compaction.md)
- [第 9 课：Pia Project Skills v1 的发现与基础披露](docs/course/lessons/09-skill-discovery-and-bounded-catalog.md)
- [第 10 课：受管理的 Skill 激活与 Context Continuity](docs/course/lessons/10-skill-activation-and-context-continuity.md)

## 当前 one-shot 命令

`pia` 是产品名，也是当前本地入口名；当前 CLI 的参数、输出协议和公共 SDK 承诺仍不稳定。它从启动进程继承 `DEEPSEEK_API_KEY`，把当前工作目录作为 workspace，并把唯一的位置参数作为任务：

```bash
go run ./cmd/pia "Inspect this Go project, fix the bug, add meaningful tests, and verify the result."
```

产品 profile 固定使用 `deepseek-v4-pro`、thinking 和 high reasoning effort。成功时 stdout 只包含最终 assistant 文本；配置、Provider、运行或可选 trace 写入失败时，错误写入 stderr 并返回非零状态。`PIA_TRACE_PATH` 可在 Run 结束后创建一个新的 `0600` 调试 trace；该文件的 `transcript` 字段保存完整 Conversation History，并同时包含 prompt、任务、tool arguments/results 和错误，可能保存源码、命令输出与敏感信息，使用者负责安全保留或删除。

Pia 在 Conversation 启动前对 `<workspace>/.pia/skills/<direct-child>/SKILL.md` 做一次 project-local snapshot。initial request 只自动包含有界的 `name`、`description` 和 workspace-relative location；模型判断匹配后，才通过普通 `read` 获取完整 `SKILL.md`。当前不扫描 `.agents/skills`、`.claude/skills`、ancestor、nested 或 global roots，也不赋予 `scripts/`、`references/`、`assets/` 等 supporting files 任何 Skill 语义。单个无效 Skill 不阻塞 coding task；成功运行时 warning 写入 stderr，并随可选 trace 保存。

当前没有独立的 Skill trust UI：操作者选择 workspace 就表示允许 Pia 把其中有效 Skill metadata 暴露给模型，并允许模型按需读取相应 `SKILL.md`。这不是对 Skill 内容的安全认证；Skill instructions 与其他 project instructions 一样可能引导模型调用具有当前用户权限的 tools，因此只应在信任 workspace 内容和该执行权限时运行。

`write` 与 `edit` 通过 `os.Root` 限制在 workspace 内；`read` 既接受 workspace-relative path，也接受当前用户有权读取的 absolute host path，后者可以位于 workspace 外。`bash` 也不是 sandbox：它继承启动 `pia` 的完整环境，以当前用户的主机和网络权限执行模型生成的命令，能够读取 workspace 外资源并产生不可逆副作用。命令不会显示启动警告或逐次审批；操作者必须接受模型选择的 workspace 文件、absolute-read 文件与 tool results 可能发送给 DeepSeek，并只在信任该执行权限时运行。第一版也不设置自动 wall-clock 或模型轮次预算，使用进程 signal/cancellation 停止运行。

真实验收材料只保存在被 `.gitignore` 忽略的 `tmp/` 中，不提交 fixture、harness、真实 trace 或模型修改后的项目。

## 本地开发检查

项目使用 Go 1.26 和 golangci-lint v2。日常改动完成后运行：

```bash
make check
```

该命令依次格式化 Go 代码，并运行 `go vet ./...`、`go test ./...` 和 `golangci-lint run ./...`。涉及并发、取消或子进程管理时，额外运行：

```bash
make race
```

当前进度：第 00 至 09 课已完成并提交；Lesson 09 已实现 project-local Pia Skill v1 discovery、bounded catalog、普通 `read` 基础使用闭环、operator diagnostics 和完整验证。managed activation/compaction continuity 已拆为尚未开始的 Lesson 10。详细信息见[课程总纲](docs/course/README.md#后续课程的滚动式大纲)。
