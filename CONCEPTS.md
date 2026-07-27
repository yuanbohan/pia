# Concepts

Shared domain vocabulary for this project — entities, named processes, and status concepts with project-specific meaning. Seeded with core domain vocabulary, then accretes as ce-compound and ce-compound-refresh process learnings; direct edits are fine. Glossary only, not a spec or catch-all.

## Relationships

The main runtime terms are different kinds of things rather than peer controllers:

- **Session** is the long-lived lifecycle object.
- **Conversation** is the interaction state owned inside a Session.
- **Core Agent** is the execution engine a Session calls.
- **User advance** is one transient operation performed by a Session, not another long-lived object.

```text
Pia                 ── product-assembles ──> Coding Agent
Pia                 ── owns as a core capability ──> Skills
Coding Agent        ── hosts ──> Session

Session                         long-lived lifecycle authority
├── owns ─────────────────────> Conversation state
├── calls ────────────────────> Core Agent
├── performs ─────────────────> one user advance at a time
└── records committed facts ──> Session Journal

Conversation state
├── contains ─────────────────> Conversation History
└── determines ───────────────> current model-view projection

Core Agent          ── owns today ──> Working Context
Core Agent          ── runs ────────> Agent Loop
Run                 ── contains ────> one or more Turns
Run                 ── settles with ─> Run Message Delta
Current Conversation role,
later Session       ── commits ─────> Run Message Delta
Working Context     ── copied into ─> Provider Request Snapshot
```

The ownership arrows describe semantic authority, not a required Go struct layout. In particular, a user advance does not require a durable struct or package, Conversation state need not become an independent active object, and “Conversation Owner” remains a role. Before formal Session support exists, the current coding-owned role temporarily coordinates complete History, projection, compaction, recovery, and the outer active guard. A formal Session must absorb overlapping outer lifecycle responsibility rather than wrap that role with a second `busy` state.

## Agent System

### Pia

The product and repository name for this Go coding agent. Its Go module and import path is `github.com/yuanbohan/pia`.

Pia product-assembles the Coding Agent and treats Skills as a built-in coding capability. A plugin or extension may distribute Skills later, but does not own or enable the core Skills lifecycle.

### Agent

An umbrella term for a model-directed system that can choose and execute multiple steps; it is not, by itself, a precise component boundary in Pia.

Use a qualified term such as Core Agent or Coding Agent whenever ownership or behavior matters. “Agent” alone must not imply ownership of complete history, persistence, workspace policy, or user interface state.

### Agent Loop

The repeated execution process that sends the current model input to a Provider, accepts one terminal assistant message, executes any valid tool calls, appends their results, and continues until completion or failure.

The Agent Loop defines ordering, settlement, cancellation, and tool-execution behavior. It does not define how a coding workspace is selected, how complete history is persisted, or how progress is rendered to a user.

### Run

One accepted Core Agent execution ending only after the Agent Loop stops producing messages and all work started by that execution has settled. A Run can start through `Run(ctx, userInput)`, which appends one new user message at acceptance, or through `Continue(ctx)`, which resumes from an existing user or paired tool-result tail without appending input.

A Run may contain multiple Turns. A non-nil Run error describes its outcome but does not erase Messages already accepted during the Run.

### Turn

One Provider-produced terminal assistant response together with the tool calls and ordered tool-result Messages caused by that response.

A Turn is smaller than a Run: tool results can require another Provider call, producing another Turn in the same Run.

### Run Message Delta

The complete ordered batch of Messages newly accepted during one Run, returned as an ownership-independent value at Run settlement for the Conversation Owner to append to Conversation History.

An input-started Run delta includes its initiating input. An input-free continuation delta starts after the pre-existing Working Context tail and therefore contains only new assistant and tool-result Messages. A request rejected before Run acceptance has an empty delta; an accepted Run that ends in failure or cancellation still returns its error or aborted assistant and any required tool-call settlement results. The delta is transferred as an ownership-independent snapshot rather than through a channel or live event subscription.

### Core Agent

The reusable runtime component that executes the Agent Loop, owns the replaceable Working Context for one Conversation, and allows at most one active run to mutate that context at a time.

The simplest mental model is an execution engine: a Session invokes it one or more times while handling a user advance. The Core Agent receives a Provider, system prompt, and tool definitions as dependencies, but it does not construct coding-specific prompts, choose workspace policy, persist Sessions, own the complete Conversation History, or implement a TUI. “Core” describes this lower-level responsibility boundary; it does not mean a second agent process, model, or user-visible lifecycle controller.

The current stateful Core owns Working Context and its narrow active-execution guard. When formal Session lifecycle is designed, this boundary must be re-evaluated against real restore and safe-point requirements: either the narrow invariant remains independently justified, or Session assumes more persistent context ownership. A new Session-level `busy` flag alone is not evidence for retaining duplicate lifecycle state.

### Coding Agent

The complete application specialized for software-engineering tasks by composing a Core Agent with a coding workspace, coding system prompt, coding tools, model configuration, and conversation orchestration.

A Coding Agent is not another model and does not inherit from the Core Agent. It is the product-level assembly that gives the generic model/tool runtime its coding behavior; a CLI or future TUI hosts this application, while future Session support extends its conversation lifecycle.

### Skill

A reusable unit of task-specific coding instructions that Pia can disclose progressively instead of placing every instruction in every model request. Agent Skills is the long-term portability target for this concept, not a claim that the current Pia runtime implements its complete specification or resource model.

Pia progressively discloses a Skill: bounded name and description metadata may be present in the initial model context, while full instructions enter context only after the model selects its catalog name through the dedicated `skill` tool. Each invocation rereads the current project-local file and produces one ordinary, bounded tool result; activation does not create an active set, body cache, dedupe record, or compaction-protected state. Skills are a core Pia capability, not synonymous with plugins, extensions, MCP servers, project instructions, arbitrary tool implementations, or ordinary project files that tools happen to access.

### Pia Skill v1

The current minimal project-local Skill subset: one direct child directory under `<workspace>/.pia/skills/`, with a `SKILL.md` containing required `name` and `description` discovery metadata plus Markdown instructions. Only `SKILL.md` has Skill semantics in v1; sibling scripts, references, assets, and other files are not discovered, indexed, injected, resolved, or executed by the Skill engine. Pia Skill v1 does not imply `.agents`/`.claude` discovery, global scopes, recursive discovery, symlink sources, vendor runtime fields, managed supporting-resource semantics, or complete Agent Skills compatibility; those remain staged future capabilities.

## Conversation and Context

### Conversation

One logical, continuable interaction lineage between a user and the Coding Agent, spanning sequential user inputs and all accepted assistant and tool-result messages produced from them.

Conversation is primarily a state/data concept inside a Session, not a peer lifecycle controller. Compaction does not create a new Conversation. Provider credentials, workspace file contents, tool implementations, request-local copies, traces, queues, cancellation controls, and UI rendering state are not Conversation contents merely because they participate in running it.

### Conversation Owner

The runtime responsibility that owns a Conversation’s complete ordered history and coordinates it with the Core Agent’s replaceable Working Context.

Conversation Owner names an ownership role, not a requirement for a type or package with that exact name and not a permanent controller beside Session. The current minimal owner accepts at most one active user advance per Conversation and rejects concurrent attempts rather than silently queueing them. An overflow recovery may coordinate one input-started Core Run and one later input-free Core continuation while retaining that same outer guard; this does not prevent different Conversations or parallel-safe tools within one Run from executing concurrently.

Once a formal Session exclusively owns one Conversation and all user inputs enter through that Session, Session is the intended outer lifecycle authority. It should absorb the current owner’s overlapping active/busy responsibility; Conversation state should not independently acquire queue, cancel, close, or persistence lifecycle. The minimal in-memory owner does not itself imply Session identity, branches, event subscriptions, or a public API.

### Message

One ordered semantic unit in the model/tool dialogue: an accepted user input, a terminal assistant response, or a tool result associated with an assistant tool call.

An assistant tool call is content inside an assistant message rather than a separate message. Partial streaming events, system prompts, tool schemas, traces, and display-only events are not authoritative Messages.

### Conversation History

The authoritative, complete, ordered record of accepted Messages in a Conversation, preserved even when older model input is summarized or excluded from the Working Context.

It retains successful, error, and aborted terminal assistant messages and the tool results that close accepted tool calls, including explicit not-executed results. It excludes partial stream formation, Provider Request Snapshots, system prompts, tool schemas, logs, and UI-only events.

The minimal in-memory Conversation Owner commits each Run Message Delta after that Core execution settles. One accepted coding user advance may commit an initial failed delta and a later continuation delta when bounded overflow recovery succeeds. Until future persistence or live observation creates a stronger requirement, Conversation History represents settled Core executions rather than partially formed active-Run state.

*Avoid:* using “transcript” without qualification when it is unclear whether the complete Conversation History or the Core Agent’s current Working Context is meant.

### Working Context

The ordered message view that the Core Agent can use to continue the Conversation on the next model call; it is replaceable while the Core Agent is idle and is not the authoritative historical record.

Before compaction it may equal the complete Conversation History. After compaction it may contain a synthetic summary, a retained recent suffix, and messages added afterward while omitting older raw messages represented by the summary. Overflow recovery may also omit an explicitly classified error assistant by its absolute Conversation History position while preserving that message as a historical fact. It must preserve model protocol integrity, including valid assistant-tool-call and tool-result relationships. The stable system prompt, tool schemas, Provider options, Session metadata, and display-only entries are outside the Working Context.

### Provider Request Snapshot

An ownership-independent, request-local copy of the complete model-visible inputs for one Provider call: the system prompt, current Working Context, and tool schemas. Provider configuration, model identity, workspace objects, and the original task as a separate field are outside this snapshot in current Pia.

It is disposable after that call and never becomes an authoritative history source. Provider-side mutation or protocol conversion must not modify the Working Context or Conversation History.

### Compaction

The process of replacing older Working Context content with a summary plus a retained suffix so future model calls use less context without deleting the complete Conversation History. It may run lazily before a new input-started Run crosses the configured threshold or be forced after an eligible context-overflow terminal before an input-free continuation.

Compaction is not arbitrary truncation: the resulting Working Context must remain sufficient to continue the task and must preserve message and tool-call protocol integrity. Internal summary requests and their terminals are not Conversation Messages.

### Semantic Event

An ordered, ephemeral observation that a meaningful runtime fact occurred, such as Run acceptance, a terminal assistant message, tool execution, compaction, or final settlement. Cancellation-specific request and settlement observations are deferred until Session control and an `Esc` interaction exist.

Semantic Events let a terminal host or another observer follow work while it is happening. They are not authoritative Conversation Messages or durable Session records, and an observer does not gain ownership of Agent Loop, Conversation History, Working Context, or lifecycle state by consuming them.

The current internal event payload carries only bounded semantic state such as fixed modes/outcomes, message role, and a tool call's turn-local source index plus a tool-owned bounded safe summary. That summary may identify the operator-visible target action, such as a path or bounded command, but the event does not copy message text, raw tool arguments/results, raw errors, model-generated call IDs, or mutable authoritative objects. A future consumer may justify an additional bounded projection, but live events are not a second History or trace.

### User Advance

One transient Session operation that accepts a user input and coordinates everything required to settle that submission: optional pre-Run compaction, one input-started Core Run, bounded overflow recovery with an optional input-free continuation, Conversation History commits, and the final result snapshot.

“Advance” is a precise operation boundary used to distinguish the complete user request from an internal Core Run. It is not a fourth long-lived runtime object, does not require its own owner or package, and does not by itself add another lock or persisted state machine. Before formal Session support exists, the coding-owned Conversation role performs this operation; afterward it belongs to Session.

### Steering

User input accepted while a Session execution is active and intended to join that same ongoing advance at a defined safe boundary after current tool work settles.

Steering does not start a concurrent Core Run, preempt an in-flight Provider or tool call, or mean cancellation. Its exact safe-boundary behavior remains subject to source calibration when the corresponding course starts.

### Follow-up

User input accepted while a Session execution is active but reserved for a later sequential user advance after the current execution reaches an eligible settlement boundary.

A pending Follow-up is distinct from Steering and keeps the Session from reporting true idle until it is either consumed or explicitly discarded by a closing control operation.

### Session Journal

A versioned durable record of committed Session facts used to validate and rebuild supported Session state across process restarts.

The Session Journal may contain Session identity and workspace metadata, settled Conversation changes, lifecycle facts, and committed compaction records. It is not the live Semantic Event stream, a copy of internal Provider exchanges, or ordinary Conversation History with control records disguised as Messages.

### Session

A lifecycle and persistence envelope around a Conversation, and the intended single outer controller for user operations on that Conversation. It may add durable identity, workspace binding, stored entries, model settings, compaction records, branches, timestamps, queue/cancel/close controls, and resume behavior.

A Session is broader than Conversation History and is not synonymous with Working Context. It owns Conversation state, invokes the Core execution engine, and performs user advances; those three terms are not peer lifecycle controllers. Pia can establish the Conversation ownership boundary without implementing a persistent Session.

When Session becomes the exclusive entrypoint for one Conversation, it must become the only outer `busy`/queue/cancel/close authority. It must not merely wrap a Conversation owner and Core Agent while all three retain semantically overlapping lifecycle state. A narrow Core execution invariant may remain only when its separate owner, acceptance/release points, and failure semantics are independently justified.

A workspace binding, when owned by a Session, is durable Session metadata rather than a Conversation Message or Working Context content. The host process's launch directory is not implicitly part of the Conversation and must not silently replace that binding during resume.

A future durable compaction record belongs to the Session Journal as control/checkpoint state, alongside but distinct from message entries. It is not a Conversation Message or a live event: the latest committed record can rebuild the Working Context projection, while failed or canceled settled records remain observable without changing that model view.

## Flagged Ambiguities

- “Agent” had been used for both the generic execution runtime and the complete coding application; use Core Agent and Coding Agent when the distinction matters.
- “Transcript” had been used for both complete history and current model-visible messages; use Conversation History and Working Context for those distinct responsibilities.
- “Session” had been used as shorthand for a message list; a Session is the broader lifecycle and persistence envelope, not the list sent to a model.
- Session, Conversation, Core Agent, and User Advance had been presented as if they were peer components. Treat them respectively as the long-lived lifecycle object, its interaction data, its execution engine, and one transient operation.
