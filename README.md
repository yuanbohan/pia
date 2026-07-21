# Pia

Pia 是面向学习、验证和长期演进的 Go coding agent。仓库为 [`yuanbohan/pia`](https://github.com/yuanbohan/pia)，产品名为 **Pia**，Go module path 为 `github.com/yuanbohan/pia`。项目以冻结的 [Pi](https://github.com/earendil-works/pi) coding agent 为语义基线。

项目以课程驱动：先阅读冻结的 Pi 源码并提炼可观察契约，再完成对应 Go 实现、测试、讨论记录和一次由学习者批准的提交。

课程按阶段推进。第一阶段包含 Lessons 00–06，交付最小 headless coding loop 和本地 `pia` one-shot coding agent；第二阶段从 Lesson 07 开始扩展 Conversation/Working Context、compaction 与 Skills 等 coding-relevant 能力。目前 Lessons 00–09 已完成，Lesson 10 尚未开始。每个阶段的目标、实施文档、课程表和当前状态统一记录在[课程阶段与实施计划](docs/course/README.md)。

当前实现仍是单 workspace、单 active Run、进程内状态的本地入口，尚未进入 Goal Runtime、Session 持久化、TUI、公共 SDK、RPC/IM、多用户、多仓库或 worktree/GitHub 管理。bash 不是 sandbox；具体安全边界和验收约束记录在课程与实施文档中。

长期目标是在迁移 coding-relevant Pi 能力后，让 Pi parity 成为能力下限，并通过同模型、同任务、多次独立运行的稳定评测追求可证明的持续超越。Skills 是 Pia 的核心能力；当前先实现 project-local Pia Skill v1 的最小可靠闭环，完整 Agent Skills、Claude Code/Codex community roots 与 vendor runtime compatibility 分阶段补充，而不是一次性扩建 Skill engine。Pi 是语义基线和主要对照组；其他优秀开源 coding agent 以及 Codex、Grok 的可获得证据用于发现候选机制，而不是直接复制。长期产品将通过 Orchestrator、Gateway 和 IM 驱动多个可持久化、可恢复且相互隔离的 Sessions；完整方向、指标和投入领域见[产品策略](STRATEGY.md)。

## 文档导航

- [产品策略](STRATEGY.md)：长期方向、指标与投入领域。
- [课程阶段与实施计划](docs/course/README.md)：阶段边界、实施文档、逐课目录与当前进度。
- [设计决策](docs/course/decisions.md)：已确定的课程与架构决策。
- [共享术语](CONCEPTS.md)：项目内稳定使用的概念边界。

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
