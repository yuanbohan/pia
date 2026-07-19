# pi-go

面向学习与验证的 [Pi](https://github.com/earendil-works/pi) 核心能力 Go 语义移植。

项目以课程驱动：先阅读冻结的 Pi 源码并提炼可观察契约，再完成对应 Go 实现、测试、讨论记录和一次由学习者批准的提交。

第一阶段只构建最小 coding loop：Agent 在当前进程中保留同一对话的完整有序 transcript，DeepSeek 每轮接收全部历史，Agent 多轮调用 `read`、`write`、`edit`、`bash` 并把 tool results 追加回 transcript，直到模型停止调用工具。Lesson 06 用临时命令名 `pia` 把这些能力组装为当前目录中的 one-shot coding agent，并通过本地、被忽略的 Go bug-fix 项目验证真实闭环。

第一阶段不实现 Goal Runtime、Session 持久化、TUI、公共 SDK、RPC/IM、多用户、多仓库、worktree/GitHub 管理、权限策略矩阵或 Pi 对比工具。bash 不是 sandbox；具体安全边界和验收约束记录在完整实施计划中。

- [课程总纲](docs/course/README.md)
- [设计决策](docs/course/decisions.md)
- [第一阶段基础实施计划](docs/plans/2026-07-15-001-pi-core-go-learning-port-plan.md)
- [Lesson 06：`pia` one-shot 实施计划](docs/plans/2026-07-19-001-feature-pia-one-shot-coding-agent-plan.md)
- [第 0 课：学习契约与基线](docs/course/lessons/00-learning-contract-and-baseline.md)
- [第 1 课：AI 协议与 Faux Provider](docs/course/lessons/01-ai-protocol-and-faux-provider.md)
- [第 2 课：单次 Provider Turn 与 transcript](docs/course/lessons/02-agent-loop-and-transcript.md)
- [第 3 课：多轮 Tool Loop 与屏障式调度](docs/course/lessons/03-tool-loop-and-staged-scheduling.md)
- [第 4 课：OpenAI-Compatible DeepSeek Provider](docs/course/lessons/04-deepseek-provider.md)
- [第 5 课：Coding Tools 与 Workspace 边界](docs/course/lessons/05-coding-tools.md)
- [第 6 课：Headless one-shot Coding Task](docs/course/lessons/06-headless-coding-task.md)

## 临时 one-shot 命令

`pia` 是 Lesson 06 的临时命令名，不承诺稳定名称、参数或公共 SDK。它从启动进程继承 `DEEPSEEK_API_KEY`，把当前工作目录作为 workspace，并把唯一的位置参数作为任务：

```bash
go run ./cmd/pia "Inspect this Go project, fix the bug, add meaningful tests, and verify the result."
```

产品 profile 固定使用 `deepseek-v4-pro`、thinking 和 high reasoning effort。成功时 stdout 只包含最终 assistant 文本；配置、Provider、运行或可选 trace 写入失败时，错误写入 stderr 并返回非零状态。`PIA_TRACE_PATH` 可在 Run 结束后创建一个新的 `0600` 调试 trace；该文件包含完整 prompt、任务、transcript、tool arguments/results 和错误，可能保存源码、命令输出与敏感信息，使用者负责安全保留或删除。

文件工具通过 `os.Root` 限制在 workspace 内；`bash` 则不是 sandbox。它继承启动 `pia` 的完整环境，以当前用户的主机和网络权限执行模型生成的命令，能够读取 workspace 外资源并产生不可逆副作用。命令不会显示启动警告或逐次审批；操作者必须只在愿意将所选内容和 tool results 发送给 DeepSeek、且信任模型执行权限的目录中运行。第一版也不设置自动 wall-clock 或模型轮次预算，使用进程 signal/cancellation 停止运行。

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

当前进度：第 00 至 06 课已完成并提交；第 06 课“Headless one-shot Coding Task”已通过两次连续真实 DeepSeek 验收和学习者的独立手动运行复核。
