# pi-go

面向学习与验证的 [Pi](https://github.com/earendil-works/pi) 核心能力 Go 语义移植。

项目以课程驱动：先阅读冻结的 Pi 源码并提炼可观察契约，再完成对应 Go 实现、测试、讨论记录和一次由学习者批准的提交。

第一阶段只构建最小 coding loop：Agent 在当前进程中保留同一对话的完整有序 transcript，DeepSeek 每轮接收全部历史，Agent 多轮调用 `read`、`write`、`edit`、`bash` 并把 tool results 追加回 transcript，直到模型停止调用工具。最终用一个固定的小型 Go bug-fix fixture 验证它能在单一目录中完成真实编程任务。

第一阶段不实现 Goal Runtime、Session 持久化、TUI、公共 SDK、RPC/IM、多用户、多仓库、worktree/GitHub 管理、权限策略矩阵或 Pi 对比工具。bash 不是 sandbox；具体安全边界和验收约束记录在完整实施计划中。

- [课程总纲](docs/course/README.md)
- [设计决策](docs/course/decisions.md)
- [完整实施计划](docs/plans/2026-07-15-001-pi-core-go-learning-port-plan.md)
- [第 0 课：学习契约与基线](docs/course/lessons/00-learning-contract-and-baseline.md)
- [第 1 课：AI 协议与 Faux Provider](docs/course/lessons/01-ai-protocol-and-faux-provider.md)
- [第 2 课：单次 Provider Turn 与 transcript](docs/course/lessons/02-agent-loop-and-transcript.md)
- [第 3 课：多轮 Tool Loop 与屏障式调度](docs/course/lessons/03-tool-loop-and-staged-scheduling.md)
- [第 4 课：OpenAI-Compatible DeepSeek Provider](docs/course/lessons/04-deepseek-provider.md)
- [第 5 课：Coding Tools 与 Workspace 边界](docs/course/lessons/05-coding-tools.md)

当前进度：第 00 至 04 课已提交；第 05 课“Coding Tools 与 Workspace 边界”实现中，`read` 子阶段已经完成并经学习者确认，下一步进入 `write`。
