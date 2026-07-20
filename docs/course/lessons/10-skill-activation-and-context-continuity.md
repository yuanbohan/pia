# 第 10 课：受管理的 Skill 激活与 Context Continuity

## 状态

未开始。只有学习者在 Lesson 09 完成并确认理解后明确要求开始，才进入本课。

## 解锁的能力

把 Lesson 09 的 catalog → 普通 `read` 基础使用路径升级为受管理的 activation lifecycle：完整 instructions 以稳定身份进入 Working Context，重复激活不会重复注入，context compaction 后已激活的 durable instructions 仍然有效。

## 证据与大致方向

- 冻结 Pi：`system-prompt.ts` 暴露 location，模型用普通 `read` 加载正文；`agent-session.ts` 另有显式 `/skill:name` 展开。
- Agent Skills client guide：允许 file-read 或 dedicated activation tool；dedicated tool 可结构化标记内容、列出但不预载 resources、追踪/去重激活，并让 compaction 识别 durable instructions。
- 当前 Pia：Lesson 08 已有 coding-owned compaction projection；Lesson 09 将提供 project-local Pia Skill v1 catalog 和普通 `read` 使用路径。

开课时必须重新阅读对应冻结 Pi 路径、Agent Skills context-management 指南和 Lesson 09 最终代码，再决定具体 Go API 与 projection 表达。

## 当前边界与非目标

- 当前候选是 coding-owned dedicated activation，而不是把 Skill 永久当作无法识别的普通 read result；这只是开课前方向，不是已确定 API。
- 本课包含 structured activation、bounded result、stable identity、dedupe 与 compaction continuity，因为它们共同构成“激活后持续有效”的一个闭环。
- 不包含 `.agents`/`.claude` 或 global discovery、完整 supporting-resource engine、Skill installer/registry、plugins、MCP、vendor runtime、TUI 或 public SDK。
- user-explicit `$skill`/slash invocation 是否并入本课，必须在开课源码校准后再判断；不得为了兼容语法扩大成完整交互系统。

## 完成信号

Faux Provider 先看到 Lesson 09 catalog；只在模型选择 Skill 后看到一次结构化 instructions。重复 activation 不重复注入；人为缩小 compaction threshold 后，激活指令在 replacement 后仍有明确、可测试的 model-visible 表达，而完整 Conversation History 保持权威原始记录。

依赖：Lesson 08、Lesson 09。规模：Large。
