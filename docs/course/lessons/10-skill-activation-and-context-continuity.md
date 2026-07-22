# 第 10 课：按需 Skill Activation Tool

## 状态

本课已完成，正在通过 feature branch/PR 交付。2026-07-21 完成开课源码校准、横向实现比较和 oversized Skill 校准，并按 D68 选定 Grok Build 风格的无状态按需 activation；D69 固定首版 50 KiB final-result、full-or-error 与可恢复 call-local failure，阈值保留后续证据驱动调整。2026-07-22 继续确认 catalog lookup 在 Conversation 内冻结、正文每次调用读取当前位置的最新版本、不做 per-call frontmatter-name equality check，也不支持 activation dedupe。package split、catalog-selected entries、plain-string schema、parallel-safe tool、structured result envelope 与 D72 的 mid-Run aggregate context 归类均已落地；`make check` 与 `go test -race ./...` 全部通过。学习者随后确认理解并明确要求 commit、push 和创建 PR。真实模型效果验证仍是后续证据义务，不被离线契约测试替代。

## 解锁的能力

把 Lesson 09 的 catalog → 普通 `read` 基础路径升级为 coding-owned 的专用 `skill(name)` tool：模型按稳定 catalog name 选择 Skill，tool 读取当前 `SKILL.md` 并返回有界、结构化的完整 instructions。Skill result 仍是普通 tool result，沿用现有 Conversation History、Working Context 与 compaction 语义。

这不是一个长期保持“active”的状态机。一次 activation 只表示一次 `skill` tool invocation。

## 开课源码校准（2026-07-21）

本课核对的固定或当前 checkout：

- 冻结 Pi：`dcfe36c79702ec240b146c45f167ab75ecddd205`
- OpenCode：`cb562b2c6289c2eee707078f9ab644cbe1d3d8a9`
- Codex open-source checkout：`0fb559f0f6e231a88ac02ea002d3ecd248e2b515`
- Grok Build：`98c3b2438aa922fbbe6178a5c0a4c48f85edc8ce`

### 参考实现事实

| 实现 | 完整正文如何进入模型输入 | 重复调用 | 完整 compaction 以后 |
|---|---|---|---|
| 冻结 Pi | model-driven 路径使用普通、可分页的 `read`；显式 `/skill:name` 再读文件并展开正文 | 没有 Conversation activation dedupe | Skill 正文没有专用 protected state；进入旧 prefix 后由普通 summary 表达 |
| OpenCode | dedicated `skill` tool 从 instance Skill state 构造 `<skill_content>`；最终仍经过通用 tool-output truncation | 再次调用再次返回正文 | `skill` 只免于较早的 tool-output pruning；full compaction 仍把它作为被总结的旧消息处理 |
| Codex open source | 显式 mention 在当前 turn 读取 `SKILL.md` 并注入 contextual user fragment | 只对同一次 submitted input 内的重复 mention 去重 | available-Skills catalog 会重建；先前完整 `<skill>` body 不作为 durable activation state 原样重投影 |
| Grok Build | slash expansion 或 dedicated `skill` tool 每次读取当前 `SKILL.md` 并返回完整 block | catalog discovery/announcement 有 dedupe，Skill body invocation 没有 | session 保留并重建 available-Skills listing；完整 body 仍由普通 summary 处理 |

这里最容易误读的是 OpenCode 的 `PRUNE_PROTECTED_TOOLS = ["skill"]`：它只阻止较早的 output-pruning 阶段清空 Skill result，不代表 full compaction 后永久保留完整正文。Grok Build 的 `announced_names` 也只去重 catalog announcement，不是正文 residency 或 activation dedupe。

四个实现均未证明“曾加载过的 Skill 正文应永久留在 model context”。共同模式是 metadata catalog 与完整 instructions 分层，正文需要时进入，旧正文按普通历史参与 compaction。

### 当前 Pia 接入点

- `internal/coding/skills/` 在 Conversation 创建前生成 metadata-only catalog snapshot，并把实际进入 catalog 的 entries 一起交给 composition；`internal/coding/skills.go` 只保留 application facade 与 diagnostic alias。
- `internal/coding/runtime.go` 先 discovery，再组装 `read`、`bash`、`edit`、`write`，有 catalog-visible entry 时追加 `skill`；Skill policy 没有下沉到 `internal/ai` 或 Provider。
- `internal/coding/conversation.go` 保存完整 Conversation History；`internal/coding/compaction.go` 只把 Working Context 投影为 summary + retained suffix。普通 Skill tool result 已能直接使用这条路径，不需要增加 Skill-specific projection。

### 对开课前大纲的校准

- **确认：** metadata catalog 与完整 instructions 必须渐进披露；dedicated tool 能按 catalog name 选择 Skill、隐藏路径读取细节，并产生可识别的结构化 result。
- **细化：** 本课的 stable identity 是 Lesson 09 启动 snapshot 中的 Skill name 与 location，不是一次 activation 后建立的长期 active identity。
- **推翻：** dedicated activation 不需要自动推出 Conversation-scoped frozen snapshot、resident/dormant 状态、receipt、重复调用短路或 Skill-specific compaction protection。D68 supersedes D62 中的 durable projection 要求和 D67。
- **缩小：** 本课从 Large 收敛为 Medium；复杂 context lifecycle 是先前候选设计制造出来的责任，不再为了课程完整感保留。

## 选定基线：Grok Build 风格的无状态按需 activation

选择 Grok Build 的可观察 activation 语义，因为它最贴合 Pia 当前 headless model-driven tool loop 与 Lesson 09 metadata-only discovery：

1. Conversation 启动时仍只生成稳定、有界的 Skill catalog；正文不预载进 model context。
2. `skill` tool 只接受 catalog 中的 Skill name，不接受任意 path。
3. 每次调用都按启动 snapshot 解析 name/location，再安全读取调用时的当前 `SKILL.md`，去掉 discovery frontmatter，返回有界、结构化的完整 instructions。
4. 成功 result 是普通 tool result，并正常进入 Working Context 和完整 Conversation History。
5. 重复调用重新读取并再次返回正文；不维护 `already active`、resident/dormant 或 hidden activation registry。
6. Compaction 不特殊保护 Skill body：落在 retained suffix 就保留原文，落入旧 prefix 就由普通 summary 表达。以后仍需要时，模型根据稳定 catalog 再调用 `skill`。

Pia 不机械复制 Grok Build 的 package、session manager、slash syntax、dynamic discovery 或多 harness 结构；只采用上面的可观察行为，并继续遵守 Pia 已有的 project-local root、symlink、regular-file、UTF-8、输入/输出有界和错误保因约束。

## Compaction 后究竟留下什么

```text
Conversation History（权威、完整）
  ... assistant calls skill(review-go)
  ... tool result contains full review-go instructions

Working Context before compaction
  ... same call + full tool result ...

Working Context after the result falls into compacted prefix
  synthetic summary
  + retained protocol-valid suffix

Stable system prompt after compaction
  same bounded available-Skills catalog

Not present
  no protected Skill body
  no dormant receipt
  no frozen activation snapshot
  no active-Skill set
```

因此，Skill “加载过”只是一条历史事实，不会创建 LLM 看不见却持续生效的状态。模型只能使用当前 Provider input 中存在的 instructions；旧正文被压缩后，如需精确正文就再次调用 `skill`。

## Oversized Skill 校准

“一个很大的 Skill 怎么办”至少有三个不同层次，不能把它们混成一个数字：

1. **作者组织建议**决定主 `SKILL.md` 应该保持多小，以及何时拆出 references。
2. **activation result 上限**决定一次 tool call 最多能给模型多少正文，以及超限是否仍算成功。
3. **Provider context capacity**是整个请求的硬容量，不能反过来证明单个 Skill 可以无界占用 Working Context。

### 四个 checkout 的实际行为

| 实现 | 大正文行为 | 结论 |
|---|---|---|
| 冻结 Pi | 普通 `read` 每页最多 2000 行或 50 KiB，并给 continuation offset；显式 `/skill:name` 路径则整文件展开，没有 Skill-specific 上限 | 同一产品内已有“分页”和“整份注入”两种语义，不存在统一 Skill 上限 |
| OpenCode | discovery 把完整正文存入进程；`skill` 构造完整 result 后，通用 tool wrapper 把模型可见输出限制为默认 2000 行或 50 KiB。完整 result 写到临时文件，并把路径与继续读取提示交给模型 | 超限时是有标记的 preview + disk offload，不是完整正文成功进入当前 result |
| Codex open source | 当前 host/executor Skill 读取路径整份注入，未发现 Skill-specific body cap；orchestrator resource 读取另有 1 MiB 单资源上限 | catalog 有独立预算，不代表被选中的正文也使用同一预算；不同 provider 已有不同防线 |
| Grok Build | dedicated `skill` 与普通 `read_file(SKILL.md)` 都刻意返回完整正文，并测试约 220 KiB 的 Skill 不被截断；另一条 oversized user/slash prompt 路径会把完整内容落盘，只保留约 4 KiB Skill head/tail 在 25,000-byte prompt 中 | 最接近 D68 的 activation 路径选择“完整优先”；只有 prompt assembly 的另一层提供 offload 恢复 |

关键源码区域：冻结 Pi 的 `packages/coding-agent/src/core/tools/{read,truncate}.ts` 与 `core/agent-session.ts`，OpenCode 的 `packages/opencode/src/tool/{skill,tool,truncate}.ts`，Codex 的 `codex-rs/{core-skills,ext/skills}/src/`，Grok Build 的 `crates/codegen/xai-grok-tools/src/implementations/{opencode/skill,grok_build/read_file}/` 与 `xai-grok-shell/src/session/acp_session_impl/prompt_build.rs`。

### Agent Skills 官方边界

[Agent Skills specification](https://agentskills.io/specification) 没有把正文长度写成有效性 hard limit；激活时读取完整 `SKILL.md`，并建议长内容拆分。[作者最佳实践](https://agentskills.io/skill-creation/best-practices) 给的是非规范性目标：主 `SKILL.md` 保持在 500 行以内、instructions 少于约 5000 tokens，把只在特定条件下需要的细节移到 references，并在主文件中明确说明何时读取哪个 reference。references 是按需加载，不是随主文件自动全部注入。

[Client implementation guide](https://agentskills.io/client-implementation/adding-skills-support) 也把正文完整加载与 resources 按需读取分开。它另行建议 compaction 时保护 Skill 内容并考虑 dedupe；这是 client guidance，不是 specification 的互操作性要求，而且与上面四个 checkout 的实际 lifecycle 并不一致。Pia 因而应把 D68 记为一个需要真实效果验证的明确 client choice，而不是声称官方标准要求无状态 reactivation。

### Pia v1 已确认边界

1. **作者指导，不作为 validity rule：** 主 `SKILL.md` 目标为少于约 5000 estimated tokens 且不超过 500 行；更长的条件性材料拆到同目录的普通 reference files。Lesson 10 不建立 supporting-resource engine；tool result 提供稳定的 Skill location/base，模型只能通过已有 `read` 按普通项目文件读取明确引用的路径。
2. **运行时硬边界：** 一次成功的、最终 model-visible `skill` result 整体最多 50 KiB。该数值不是 Agent Skills 标准，而是 Pia v1 safety ceiling：它沿用现有 `read`/tool-output 量级，又明显宽于推荐的核心 instructions 目标。
3. **完整或失败：** result 放不下完整 instructions 时返回有界的 call-local error，包含实际大小、上限、稳定 location 和拆分/读取提示；不把截断正文包装成 activation success，也不回退到旧 cache。普通 `read` 仍可显式分页查看原文件。
4. **不增加行数 hard limit：** 500 行是可读性指导，不是规范 validity 条件；50 KiB 的最终 result ceiling 已经约束一次模型输入。
5. **不复制临时文件：** OpenCode 的 disk offload 对任意 tool output 有价值；Pia 的原始 `SKILL.md` 已经在 workspace 中并可由 `read` 分页，因此 Lesson 10 没有证据再制造和清理一份临时副本。

这个边界与 Grok Build 的“每次读取当前正文”保持一致，但不会复制其 `SKILL.md` 无上限例外；Pia 已经决定所有 model-visible instructions 必须进入 projected Provider input 预算。它也借鉴 OpenCode 的 50 KiB 量级与可恢复提示，但保留 Agent Skills 所说的“完整 instructions”成功语义。

### 超限失败的实际作用域

这里的 “full-or-error” 不是终止 Conversation、结束 Run 或永久禁用 Skill。它沿用 D20 的普通 tool-error 语义：

```text
assistant calls skill("review-go")
  -> tool detects that the complete result exceeds 50 KiB
  -> append an IsError=true tool result saying "not activated"
  -> execute later calls in the same batch
  -> make the next Provider call with that error result
```

下一轮模型仍然看到稳定 catalog、Skill location 和有界诊断，可以选择：

- 用现有 `read` 按 offset 分页查看原始 `SKILL.md`；
- 要求作者把核心 instructions 留在主文件、条件性细节移到 references；
- 文件被缩小或重构后再次调用 `skill(name)`；D68 每次读取当前文件，没有永久 blacklist 或失败 cache；
- 如果任务不需要该 Skill，则解释失败或继续使用其他 tools。

因此，超限期间不能成功使用 dedicated `skill(name)` 一次性取得完整正文，但 Skill 文件并非彻底不可访问，Conversation 和其他 tools 也不会失效。分页 fallback 是降级路径，不应被描述成完整 activation：模型若只读了部分页面，就不能假装已经得到全部 instructions。若真实使用证明这种降级频繁发生，优先检查 Skill 是否应该按 Agent Skills 的 progressive-disclosure 方式拆分；之后才考虑增加 OpenCode 风格的 managed offload 或 paged activation，而不是取消所有输入边界。

## Frozen identity 与 current content

Lesson 09 已经建立安全的 discovery handle chain，但当时的 discovery result 只把 rendered catalog 和 diagnostics 交给 composition，没有保留 dedicated tool 所需的 records。Lesson 10 现在为每个实际进入 catalog 的 winner 保留启动 snapshot，包含 frontmatter name、direct Skill directory name 和稳定 workspace-relative location。这里冻结的是 lookup identity，不是正文 bytes 或长期打开的文件描述符。

每次 `skill(name)` 都重新打开当前位置，才能同时满足 D68 的 freshness 与 Lesson 09 的 source policy：

```text
frozen name -> frozen direct directory/location
                 |
                 v
open current .pia/skills source
  -> OpenDirectoryAt(source handle, frozen direct directory)
  -> OpenRegularFileAt(skill-directory handle, "SKILL.md")
  -> validate/read the one opened file handle
  -> close all call-owned handles
```

Darwin/Linux 上的两个 handle-relative opens 已经使用 `openat + O_NOFOLLOW + O_NONBLOCK`，并对打开后的对象类型做验证。这保证 direct Skill directory 与最终 `SKILL.md` 不能在调用时变成 symlink，FIFO 也不能在类型检查前无限等待；文件在 open 后再次被 path replacement，不会让同一次调用“验证 A、读取 B”。下一次调用重新走整条链，因此会观察下一版本。

不应把 discovery 时的 source/Skill/file handles 保持到整个 Conversation 结束。那样即使原目录已经 rename、删除并由新目录替换，后续 activation 仍会沿旧 handle 读取旧 inode，实际上形成隐藏 body snapshot，与 D68 的 current-file read 相反。也不能只用普通 workspace path 一步 reopen：那会丢失 Lesson 09 对 direct directory 与 final file 的逐层 no-follow 约束。

### 文件变化矩阵

| 调用时状态 | 已选可观察结果 |
|---|---|
| 同一 location 的正文变化 | 成功，返回调用时读到的新正文；不要求 current frontmatter name 等于 frozen name |
| `SKILL.md` 被另一个 regular file 原子替换 | 成功；本次调用从自己打开的 file handle 读取，下一次调用重新打开并可见再后续版本 |
| current frontmatter 的 name/description 变化 | 当前 Conversation 的 catalog 不变，activation 仍按 frozen name → directory/location 读取正文；新 Conversation 重新 discovery 后才刷新 metadata mapping |
| 文件或 direct Skill directory 被删除/rename | call-local failure，不回退旧正文 |
| source、direct Skill directory 或最终文件变成不允许的类型；direct directory/final file 变成 symlink 或 FIFO | call-local failure；supported platforms 不在验证前阻塞 FIFO |
| current file 缺少可识别的 frontmatter delimiter、正文不是 UTF-8，或完整 structured result 超限 | call-local failure，不返回截断正文或旧 cache |

此前候选的 identity-drift failure 过严，现已撤回。冻结 catalog 的目的，是让一次 Conversation 中模型看到的 lookup surface 稳定；它不是拿 current frontmatter 再做一次文件身份认证。Agent Skills client guide 明确允许 activation 时读取正文以观察两次 activation 之间的本地修改；冻结 Pi、当前 Codex 和 Grok Build 的 path-based reread 也没有普遍要求 current name 等于 discovery name。额外加入该检查会让 Pia 在没有横向证据的情况下拒绝用户希望立即生效的同路径编辑。

Activation 只需要再次识别有界 opening/closing delimiter，从当前文件中准确分离正文；它不重新解析 name、description 或其他 YAML fields。结果是：一个暂时写坏 YAML、但 delimiter 与正文仍可读的文件，在当前 Conversation 中仍可按 frozen mapping 加载；新 Conversation 的 discovery 则会按 Lesson 09 规则重新校验并可能排除它。这两个时点承担不同责任。

Handle chain 能固定 path lookup 得到的 inode，抵抗 source、directory 和 final entry 的 symlink/path-replacement 竞态；它不把普通文件内容变成事务性 snapshot。作者使用原子替换时，一次调用会读旧文件或新文件之一。若另一个进程同时对同一 inode 原地改写 bytes，Pia 不承诺跨整次读取的一致版本；Lesson 10 不为这种外部写法增加锁或文件版本协议。

## Go 实现

### 1. Skill lifecycle 收在一个内聚 package

实现前的 `internal/coding/skills.go` 有 592 行生产代码和 440 行测试，同时承担 source verification、discovery、frontmatter parsing、winner selection、catalog budgeting 与 diagnostics。若 activation 继续留在 `coding`，该文件还会混入 model-facing tool schema、current-document loading 与 result limits；若只新建 tool package，则必须复制 source handle chain，或者让 tool 反向依赖 `coding` 而形成 import cycle。

实现采用下面的有边界 package split：

```text
internal/coding/skills/
  discovery.go   metadata snapshot、winner 与 diagnostics
  catalog.go     bounded catalog，并返回实际进入 catalog 的 entries
  source.go      source/direct-directory/SKILL.md handle chain
  document.go    activation-time frontmatter removal 与 bounded body read

internal/coding/tools/skill/
  tool.go        JSON schema、name lookup、structured result 与 tool errors
```

`coding` composition 同时依赖这两个内层 package；`tools/skill` 依赖 `skills` 的 frozen entry 与 current-document loader；`skills` 不依赖 tool 或 runtime，因此没有 cycle。现有 `coding.SkillDiagnostic` 通过 `skills.Diagnostic` type alias 保持，使 `RunResult`、trace 与 `cmd/pia` 没有因内部归档产生无关 churn。这个 split 不建立通用 resource loader，也不把 Skills 移出 coding ownership。

### 2. Discovery 必须同时返回 catalog text 与准确的 activation entries

Lesson 09 的 `buildPiaSkillCatalog` 可能为了 4096 estimated-token ceiling 缩短 descriptions，最后还可能省略 lexical tail；当前函数只返回 rendered string。Lesson 10 不能把所有解析成功的 candidates 都放进 tool lookup，否则模型猜到一个 catalog 未披露的 name 仍能激活它。

discovery result 因而包含三项：

```text
Catalog       model-visible bounded text
Entries       only the winners actually present in Catalog
Diagnostics   bounded operator warnings
```

每个 entry 冻结 `name`、direct directory name 和 workspace-relative `location`。directory name 必须单独保留，不能从 frontmatter name 推导，因为 Lesson 09 明确允许二者不一致。`Entries` 与 `Catalog` 由同一次 selection 产生并保持同一顺序；tool constructor 深复制后建立只读 `name -> entry` map。

### 3. Composition 顺序改为 discovery → tools → prompt

runtime composition 采用的新顺序是：

```text
open workspace
  -> discover Skills once
  -> create read/bash/edit/write
  -> if catalog Entries is non-empty, append skill tool
  -> derive tool schemas
  -> build stable prompt from the same Catalog
  -> create Agent and Conversation
```

没有可披露 entry 时不注册空的 `skill` tool，四工具 baseline 保持原样。有 entry 时把 `skill` 追加在现有 `read, bash, edit, write` 后面，保留冻结 Pi 四工具的相对顺序。schema 只要求一个 bounded string `name`，不再用 enum 复制整份 catalog names；runtime map 负责 exact lookup，未知或被 catalog budget 省略的 name 形成 call-local error。

`skill` 没有可变 call state，每次调用只借用并发安全的 workspace root、创建自己的 handles，因此可以与 `read` 一样声明 parallel-safe。写工具仍是 serial barrier，所以同一个 assistant batch 中位于 write/edit 两侧的 activation 不会越过 mutation barrier；并发完成的 tool results 继续按模型 source order 提交。

### 4. 每次 Execute 重新走安全 handle chain，不重跑 discovery

一次 `skill(name)` 的执行路径是：

```text
strictly decode {"name": "review-go"}
  -> lookup frozen entry
  -> reopen and verify current .pia/skills source
  -> OpenDirectoryAt(source, frozen direct directory)
  -> OpenRegularFileAt(directory, "SKILL.md")
  -> locate bounded frontmatter delimiters
  -> read current body through this handle
  -> validate body UTF-8
  -> build and size-check final result
  -> close every call-owned handle, preserving read/close causes
```

这里没有 body cache、failure cache、blacklist、per-call directory rescan 或 current metadata equality check。source/path 被原子替换后，下一次调用自然观察新对象；删除或类型改变则只让该次调用失败。

### 5. 50 KiB 限制作用于最终 structured result

候选 envelope 保持简单，并只暴露 workspace-relative 信息：

```text
<skill_content name="review-go" location=".pia/skills/review-go/SKILL.md">
Base directory: .pia/skills/review-go
The following is the complete Skill instructions after frontmatter:
...current body bytes...
</skill_content>
```

name、location 与 base metadata 做 XML escaping，正文原样保留；这个 block 是模型 framing，不被当作需要再次解析的 XML document。tool 先计算固定 envelope overhead，再把剩余 byte budget 交给 document loader。loader 只读取有界 frontmatter prefix 和能进入结果的 body；稳定 regular file 的 handle size 可用于报告 projected final byte size，仍会在实际组装后再次检查 `len(result) <= 50*1024`，避免竞态或计算差错。

超限时 `Execute` 返回有界 error，不返回 preview；Agent 按 D20 把它转换为 `IsError=true` tool result。error 明确包含未激活、observed final size、51200-byte ceiling、稳定 location，以及可用 `read` 分页或缩小主文件的恢复提示。文件缩小后再次调用会重新读取并可成功。

### 6. Prompt 只改变选择动作，不改变 compaction

catalog guidance 从“用 `read` 读取 `SKILL.md`”改成“用 `skill` 按 catalog name 加载完整 instructions”。`codingToolPromptMetadata` 增加 `skill` 的简短说明；`read` 仍负责 instructions 明确引用的普通 reference files，以及 oversized activation 的显式分页降级。

Conversation History、Working Context、Agent tool-error settlement 和 compaction 不增加任何 Skill fields。成功或失败 result 都是普通 tool result；这也是实现上能保证没有 hidden active state、protected block 或 dedupe registry 的最直接方式。

### Activation dedupe 的明确支持边界

当前只保留 Lesson 09 的 discovery-level duplicate-name winner selection；它解决的是多个文件声明同一 catalog name 时哪个 location 可见，不是 activation dedupe。

Lesson 10 不支持 Conversation-scoped activation dedupe。同一 assistant batch 或不同 Provider turns 中的两次 `skill("review-go")` 都是两次真实调用：各自重新打开当前文件，并各自产生一份完整或失败的普通 tool result。没有 `already active` 短路、content hash、body cache、active set、失败 cache 或 transcript projection dedupe。通用 compaction 以后可以把旧重复结果纳入 summary，但这只是普通 context 回收，不能称为 activation dedupe。

这项取舍不是把已承诺能力漏掉。此前 dedupe 之所以是同课硬要求，是因为当时还候选永久 protected body；重复 protected copies 无法由普通 compaction 回收。D68 取消 protected projection 后，该有界状态不变量已经消失。现在保留 dedupe 反而必须回答 current body 是否已变化、旧 body 是否还在 Working Context、compaction 后 `already active` 能否让模型拿回正文等新状态问题，会破坏当前无状态模型。

若真实 trace 以后证明模型在正文仍处于当前 Working Context 时频繁无意义地重复调用，并且输入成本显著影响任务，才单独设计和评测 dedupe。当前 package boundary 不阻止后续能力，但不能把“以后可以扩展”写成“现在已支持”。学习者已于 2026-07-22 确认当前不支持 activation dedupe 的方案足够简单且便于维护。

### 7. 验证从 package contract 到完整 Provider path 分层进行

- `skills` package：保留 Lesson 09 的 discovery/catalog/security tests，并新增 catalog text 与 selected entries 一致、current document path replacement、删除、symlink、FIFO、delimiter、UTF-8、bounded read 与 handle cleanup tests。
- `tools/skill` package：锁定 strict arguments、unknown name、structured envelope、exact 50 KiB boundary、超限不泄露 partial body、失败后文件缩小可重试、两次调用观察正文 V1/V2，以及 current frontmatter name 改变仍按 frozen mapping 返回最新正文。
- coding integration：Faux Provider 首次 request 只有 catalog + `skill` schema；tool call 后第二次 request 才有正文；无有效 Skill 时不注册 tool；catalog omitted tail 不能激活；普通 tool failure 后 Run 继续。
- compaction integration：人为降低 threshold，让旧 `skill` result 进入普通 summary，断言 Provider input 没有额外 protected projection，同时完整 Conversation History 保留原 call/result。
- verification：运行 `make check`；因为 `skill` 声明 parallel-safe 并复用 root，再运行 `go test -race ./...`，并在 Darwin/Linux 覆盖 handle-swap 与 FIFO nonblocking cases。

## 实现与验证结果

- `internal/coding/skills/` 已接管 discovery、catalog selection、安全 source opening、metadata parsing 与 current-document loading；所有非测试 Go 文件均按职责保持在 1000 行以内。`internal/coding/tools/skill/` 只负责 model-facing schema、exact-name lookup、structured result 和 call-local error。
- discovery 返回的 `Entries` 与实际 rendered catalog 使用同一次 selection；被 4096 estimated-token catalog budget 省略的 lexical tail 不会进入 tool lookup。没有有效 entry 时四工具 baseline 不变，有 entry 时 `skill` 追加在 `read, bash, edit, write` 后。
- document/tool tests 锁定 current body reread、frontmatter metadata 不二次验证、regular-file 和 source 原子替换、删除后不回退、symlink/FIFO/UTF-8/delimiter failure、exact 50 KiB final-result boundary、超限不泄露 partial body、缩小后恢复、strict arguments、unknown name 与并发调用。
- coding integration tests 锁定 initial request 的 catalog + schema、调用后才出现正文、重复调用从 V1 更新到 V2、catalog-omitted name 不可猜测激活、超限只产生 `IsError=true` tool result 且 Run 继续，以及普通 compaction 不产生 protected/dormant/receipt projection，而完整 History 保留原结果。
- 最终主线程审查没有留下 actionable finding。简化检查复用了已有 conversation serializer，避免测试自建不完整的 Message 遍历；`make check` 与 `go test -race ./...` 全部通过。

## Mid-Run aggregate context 的归类

package split、catalog-selected entries、plain-string schema、parallel-safe declaration、candidate result envelope 和不支持 activation dedupe 均已得到学习者确认。最终 error wording 与 byte-boundary assertions 已直接固化为可观察 contract，没有增加新的 error taxonomy。

仍需明确归类的是 **mid-Run aggregate context**：现有 Conversation 只在一次 accepted Run 开始前调用 `compactBeforeRun`；Core Agent 在同一 Run 内执行 tools 后会直接发起下一次 Provider call，不在两个 Provider turns 之间触发 compaction。因而 D69 的 50 KiB 是单个最终 `skill` result ceiling，不是同一 batch 或 Run 内全部 Skill results 的 aggregate ceiling。多个不同或重复 Skill results 仍会在下一次 Provider input 中相加，正如多个有界 `read`/`bash` results 也会相加。

D72 确认不在 Lesson 10 增加 Skill-specific aggregate counter、调用数量限制、半批跳过、mid-Run compaction 或 context-overflow retry：这些机制会改变通用 Agent loop、tool settlement 与 compaction ownership，而且问题并非 Skill 独有。集成测试证明每个 result 有界并正常进入 Provider request；aggregate overflow recovery 留给课程表中已有的 Runtime 韧性方向。这个非目标不影响本课正常单个/少量 Skill activation 的闭环，但必须避免把“单次 50 KiB”误述成“整个 Run 永不超 context”。

## 效果验证义务

本课先把两类验证分开，不能用前一类替代后一类：

1. **契约验证：** 离线测试证明 initial request 不含正文、按 catalog name 调用后才出现完整 instructions、重复调用读取当前文件、失败有界、普通 compaction 不产生 Skill-specific projection，并且完整 Conversation History 仍保存原始 call/result。这只能证明实现符合 D68。
2. **真实效果验证：** 实现稳定后，使用固定 workspace、任务、Skill 与模型配置，对比没有 dedicated activation tool 的基础路径和启用后的路径。相关任务、无关任务和跨 compaction 后再次需要同一 Skill 的任务都要覆盖；每组需要多次运行，不能从一次 fixture 或主观阅读宣称效果更好。

真实验证至少记录：相关 Skill 是否被正确选择、无关 Skill 是否被误用、关键 instructions 是否在可观察结果中得到遵守、任务是否完成、`skill` 调用次数、近似输入成本，以及正文被 compaction 后再次需要时模型是否会重新调用。若证据暴露问题，先定位是 catalog 描述、tool contract、compaction summary 还是模型选择问题；只有重复调用的实际成本成为主要问题时才重新评估 dedupe，不能倒推回永久 protected body。

## 当前边界与非目标

- 不实现 activation registry、Conversation-scoped body cache、frozen snapshot、receipt、resident/dormant/relevant 状态或跨调用 dedupe。
- 不实现 Skill-specific compaction、自动 reactivation、manual deactivate、refresh、watcher、版本管理或跨 Conversation cache。
- 不加入 user-explicit `$skill`/slash syntax；当前只有 model-driven `skill` tool。
- 不加入 `.agents`/`.claude` 或 global discovery、完整 supporting-resource engine、Skill installer/registry、plugins、MCP、vendor runtime、TUI 或 public SDK。

## 完成信号（已达到）

Faux Provider 的 initial request 只看到 Lesson 09 的 bounded catalog 和新增 `skill` schema；模型调用 catalog name 后才收到有界、结构化的当前 `SKILL.md` instructions。重复调用重新读取当前文件并产生普通 tool result。人为缩小 compaction threshold 后，旧 Skill result 与其他 tool result 一样进入 summary 或 retained suffix，Provider input 中不存在 protected Skill body、receipt 或 activation registry projection，完整 Conversation History 仍保留原始 call/result。

依赖：Lesson 08、Lesson 09。规模：Medium。
