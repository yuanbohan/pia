# 第 09 课：Pia Project Skills v1 的发现与基础披露

## 当前状态

本课已完成并提交。冻结 Pi 与 Agent Skills 规范的源码/文档校准、Pia 产品命名和 absolute-read 前置改造均已完成。Pia Skill v1 discovery、bounded catalog、普通 `read` 使用路径、operator diagnostics、最终审查与全仓检查均已完成；学习者于 2026-07-21 确认理解。Lesson 10 仍未开始，只有学习者明确要求后才进入。

开课讨论先后暴露了两个过大的候选范围：一是把 discovery、activation、resources 与 compaction continuity 合为一课，二是同时兼容 Pia、Claude Code、Codex 以及 project/global 多种来源。学习者最终把首版收紧为 Pia 自己的 project-local Skills 和最小可用格式。Lesson 09 只建立这个基础闭环；managed activation 与 compaction continuity 留给尚未开始的 Lesson 10，跨生态与全局兼容留给后续未编号方向。

固定基线：Pi commit `dcfe36c79702ec240b146c45f167ab75ecddd205`，Pi Agent package version `0.80.7`。

## 本课解锁的闭环

Pia 在 Conversation 启动前只检查 selected workspace 根目录下的 `.pia/skills/`，从每个直接子目录的 `SKILL.md` 读取有界 metadata，并把有独立预算的 catalog 放入稳定 model input。完整 Markdown instructions 不进入 initial request。

本课推荐同时沿用冻结 Pi 的最小使用路径：catalog 暴露 workspace-relative `SKILL.md` location，模型在任务匹配时用现有 `read` 按需读取正文。这样 Lesson 09 结束时 Skill 已经实际可用，但普通 tool result 没有 Skill identity、去重或 compaction protection；Lesson 10 再把它升级为受管理的 activation lifecycle。

## Pia Skill v1 最小格式

首版只承诺下面的 project-owned 形状：

```text
<workspace>/.pia/skills/<skill-name>/SKILL.md
```

```markdown
---
name: review-go-change
description: Review a Go change for correctness, tests, and maintainability.
---

# Instructions

Read the changed Go code and report concrete findings.
```

最小语义只有：

- 一个直接子目录代表一个 Skill，入口固定为 `SKILL.md`；
- frontmatter 只要求 `name` 与 `description`，用于 discovery/catalog；
- closing frontmatter 后的 Markdown 是按需读取的 instructions；
- 其他 frontmatter 字段没有运行语义；
- `scripts/`、`references/`、`assets/` 或其他 supporting files 完全不属于 Pia Skill v1 contract：Skill engine 不发现、不解析、不列出、不解析相对引用，也不提供执行语义。Coding Agent 的通用 tools 仍可能被模型用于普通项目文件，但不得把这种既有能力记录或宣传成 Skill resource support。

这个形状有意靠近 Agent Skills 的基础目录和 metadata 约定，避免创造完全不兼容的私有文件格式；但首版只宣称 **Pia Skill v1**，不宣称完整 Agent Skills、Claude Code Skills 或 Codex Skills compatibility。

## 开课源码与规范校准

### 冻结 Pi 证据

- `packages/coding-agent/src/core/skills.ts` 启动时读取 frontmatter，保存 name、description、file path、base directory 与 source metadata；不会把 Skill 正文直接放进 system prompt。
- `packages/coding-agent/src/core/system-prompt.ts` 把 `<available_skills>` metadata index 加入稳定 system prompt，并告诉模型匹配后通过普通 `read` 加载 `SKILL.md`。
- `packages/coding-agent/test/skills.test.ts` 覆盖 metadata parsing、递归发现、名称碰撞、XML escaping 与 model-visible filtering。
- 完整 Pi sources 还组合了 user/project paths、settings、packages、trust、explicit invocation 和 resource diagnostics。Pia v1 不移植这套 ResourceLoader。

### Agent Skills、Claude Code 与 Codex 证据

- [Agent Skills specification](https://agentskills.io/specification) 把 `SKILL.md`、name、description、Markdown instructions 与可选 supporting resources 定义为 portable 基础。
- [Agent Skills client guide](https://agentskills.io/client-implementation/adding-skills-support)、[Claude Code Skills](https://code.claude.com/docs/en/skills) 与 [Codex Skills](https://developers.openai.com/codex/skills/) 说明了更多来源、metadata、symlink、invocation、resource 与 context-management 机制。
- 这些证据继续作为长期兼容方向，但不再是 Lesson 09 的实现清单。后续必须逐项验证并分课，不能因为文件名都是 `SKILL.md` 就一次性宣称完整兼容。

## 已确认、收紧与推翻

### 已确认

- Skills 是 coding-owned 的 Pia 核心能力，不依赖 extension 或 plugin 才能启用。
- metadata-first、instructions-on-demand 仍是正确的渐进披露基础；未使用 Skill 的正文不应占据 initial context。
- catalog 必须有总预算，Skill source 也必须有数量和 metadata 大小限制。

### 收紧

- “project scope”在首版只表示 selected workspace；当前 one-shot command 直接把启动 CWD 选为 workspace。
- 首版只有 `.pia/skills/<direct-child>/SKILL.md`，不做递归 source families、ancestor repository roots、nested on-demand scopes 或 dynamic reload。
- 首版只解析 Pia Skill v1 的必要 metadata，并按需加载 `SKILL.md`；supporting files 是否以及如何成为 Skill 能力全部留给后续重新决定。

### 推翻

- Lesson 09 不再扫描 `.agents/skills/` 或 `.claude/skills/`，也不为 Claude/Codex metadata fallback 和 vendor runtime 建兼容层。
- Lesson 09 不再发现 user、admin、system 或其他 global Skills。
- 项目 Skill 不通过 symlink 把 workspace 外目录变成隐式 Skill source；absolute `read` 保持为独立通用能力，不扩大 discovery scope。

## 当前 Pia 路径

- `internal/coding/skills.go` 在 `internal/coding` composition boundary 内完成 project-local discovery、frontmatter validation、catalog budgeting 和 bounded diagnostics；没有为首版创建独立 Skill engine package。
- `internal/coding/runtime.go` 在创建 Core Agent 前取得一次 Skill snapshot，把 diagnostics 保存到 `RunResult`，并把 catalog 交给 stable system prompt。
- `internal/coding/prompt.go` 从真实 tools、canonical workspace、project instructions 与可选 Skill catalog 构建一次稳定 prompt；没有有效 Skill 时整个 catalog section 省略。
- `internal/coding/tools/read/tool.go` 已支持 workspace-relative 与 absolute host paths。Pia Skill v1 location 使用 workspace-relative path，不需要为本课暴露 host absolute path。
- `internal/ai/estimate.go` 的 `EstimateTextTokens` 为 catalog 复用 D58 的 `ceil(characters / 4)` 近似；精度仍受 D58 限制。
- `internal/coding/runtime_test.go` 用 Faux Provider 锁定 initial request 只有 catalog metadata，并验证后续普通 `read` 才获得完整 instructions；`internal/coding/trace.go` 与 `cmd/pia` 分别投影 trace diagnostics 和 success-time stderr warnings。

## 已确定的本课边界

- 只发现 `<workspace>/.pia/skills/<skill-name>/SKILL.md`；缺少 `.pia/skills` 不是错误。
- 只检查一层直接 Skill directories，不递归搜索任意 `SKILL.md`。
- discovery 通过 workspace `os.Root` 完成；workspace 外 symlink、特殊文件和不安全目标不成为 Skill。
- `name` 与 `description` 都必须存在；不为 `.claude` 风格缺失字段做 fallback。Agent Skills strict specification 要求 name 为 1–64 个小写字母/数字/连字符、不得首尾或连续使用连字符，并与父目录名一致；Pia v1 对 mismatch、超长和字符形式等 cosmetic violations 产生 warning 后继续加载。缺少 name、缺少 description 或 YAML 无法解析才跳过，不能把“规范无效”直接等同于“运行时必须拒绝”。Catalog 使用 frontmatter name；同名时按稳定 lexical path 选择一个并 warning。
- model-visible catalog 只含 name、description 与 workspace-relative location；body 和 supporting files 不进入 initial request。
- Skill catalog 在 Conversation 创建时形成一次 snapshot；本课没有 watcher 或 Run 中途 reload。

## 已确认的实现契约

### 基础使用路径与 snapshot

catalog 明确告诉模型：任务匹配时，用普通 `read` 读取 workspace-relative location 中的完整 `SKILL.md`，然后应用其中 instructions。这个路径不增加新 tool，能形成可用闭环；普通 tool result 仍没有稳定 Skill identity、去重和 compaction protection，这些保持为 Lesson 10 的责任。

discovery 只在 Conversation 创建 Core Agent 前执行一次。后续 Provider turns 共享同一个 system prompt；文件变化不会触发 watcher 或 Run 中途 reload。

### YAML、validation 与 supporting files

- frontmatter 最多从 16 KiB prefix 中提取，使用官方 YAML organization 维护的 `go.yaml.in/yaml/v3`；只消费 string `name` 与 `description`，unknown fields warning 后忽略。
- 缺少/空必需字段、非法 YAML、重复 mapping key、非 string 必需字段、超限 frontmatter、不安全 symlink 或 special-file target 会跳过该 Skill。
- portable name 规范仍是 1–64 个小写字母/数字/连字符、无首尾/连续连字符并与目录一致；Pia 对 65–256 characters、字符形式或目录 mismatch 只 warning 后加载，超过 256 才按 hard safety 跳过。
- description 超过 1024 characters 时，catalog 使用前 1024 characters 并 warning；完整文件仍可由后续普通 `read` 获取。
- discovery 不解析或验证 closing delimiter 后的正文，也不检查、索引、列出、解析或执行 supporting files。测试特意证明 frontmatter 后的正文 sentinel、无效 UTF-8 bytes 和 sibling `references/` 都不影响 initial catalog；后续普通 `read` 仍按自身 UTF-8/分页契约处理完整文件。

### 数量、catalog 与 diagnostics budget

- source 先以 supported-platform nonblocking policy 打开并通过 opened handle 验证 directory 类型；enumeration 最多读取 257 个 direct entries，超过 256 时整个可选 source 忽略并 warning，避免在 candidate ceiling 生效前无界物化目录。未超限时跳过无法经 Provider JSON 保真传递 location 的非 UTF-8 direct directory names，再按 lexical order 检查前 64 个直接、非 symlink Skill directories，多出的 lexical tail 不读取并 warning。
- catalog entries 按 frontmatter name 再按 location 确定性排序；同名时 workspace-relative lexical path 较小者获胜。所有 name、description 和 location 都做 XML escaping。
- 完整 catalog ceiling 为 4096 estimated tokens。先通过统一 character cap 尽量保留所有 descriptions；即使 description 为空仍超限时，再省略确定性 lexical tail entries，并只为真实发生的 shortening/omission 产生 warning。
- 最多返回 64 条有界 `SkillDiagnostic`，额外 warning 聚合为 omission summary。单个 Skill 或可选 `.pia/skills` source 的错误不阻塞普通 coding task；diagnostics 保存到内部 `RunResult` 与可选 trace，`cmd/pia` 只在 Run/trace 成功时写到 stderr，并在输出边界同时引用 path 与 message 以转义 untrusted control characters。

4096、64、16 KiB、256 与 1024 都是首版可测试 ceiling，不是长期最优值。大型真实项目、Skills 数量增长、模型变化或 Lesson 10 managed activation 都必须重新触发 D55/D57 的 context 分桶复评。

### Trust 边界

首版没有独立 trust UI、Skill approval 或 permission grant。操作者选择 workspace 就允许 Pia 自动把有效 Skill metadata 放进 model input，并允许模型按需 `read` 完整 `SKILL.md`；Skill instructions 还能引导已有 `read`、`bash`、`edit` 和 `write`。这与当前 project instructions 的 operator trust 假设一致，但不是内容认证，README 已明确披露 Provider egress 和 host authority。

## 实现与测试结果

- 新增 `internal/coding/skills.go`，责任保持在 coding-owned composition boundary；没有向 `internal/ai`、通用 Agent Loop 或工具 package 泄漏 Skill policy。
- `RunResult.SkillDiagnostics` 是 discovery snapshot 的内部诊断投影；trace 使用同一有界结构，CLI 只负责进程级 stderr 展示。
- discovery/catalog tests 覆盖 deterministic scope、missing root、cosmetic validation、required fields、malformed YAML、frontmatter/name/description limits、duplicate winner、XML escaping、external symlink、directory symlink、FIFO、catalog truncation/omission 和 diagnostic bounds。
- Faux Provider integration test 证明 initial system prompt 不含正文或 supporting files，模型执行普通 `read` 后下一次 request 才得到完整 `SKILL.md`。
- 最终审查补出并修复了重复 YAML mapping key 静默采用最后值的问题，同时把 unknown-field diagnostic 的完整文本收紧到固定上限；没有剩余 actionable finding。
- `make check` 与 `go test -race ./...` 全部通过。

## 当前非目标

- `.agents/skills`、`.claude/skills`、user/global roots 或完整 Agent Skills compatibility；
- recursive/nested discovery、external symlink Skills、installer、registry、watcher 或 dynamic reload；
- dedicated activation tool、重复激活去重、resource index/base lifecycle 或 compaction protection；
- Claude dynamic shell/subagent/hooks/tool grants、Codex plugins/connectors/UI metadata；
- `$skill`、slash command、TUI selector 或其他 explicit invocation；
- 对 supporting files 的专门扫描、验证、执行或权限模型。

managed activation 与 compaction continuity 属于尚未开始的 Lesson 10。跨生态、global scopes 与更完整 resources 属于 Lesson 10 之后重新校准、拆分和编号的能力方向。

## 完成信号

使用受控临时 workspace 和 Faux Provider 验证：

- `.pia/skills` 的直接 Pia Skill v1 可确定性发现，其他 project/global roots 不参与；
- malformed、oversized、escaping/special-file candidates 不进入 catalog，并遵循最终确定的 diagnostics policy；
- initial request 含有界 catalog，但每个 `SKILL.md` body sentinel 都不可见；
- 匹配任务可通过普通 `read` location 获得 `SKILL.md` instructions；Pia 不发现、列出或注入 supporting files；
- catalog 达到预算时 description truncation、entry omission 与 diagnostics 可重复；
- catalog 已进入 projected input 估算，D55/D57 的 Skills-enabled 真实项目复评义务保留；
- `make check` 与 `go test -race ./...` 通过，学习者确认理解后结束 Lesson 09。
