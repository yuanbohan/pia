# Concepts

Shared domain vocabulary for this project — entities, named processes, and status concepts with project-specific meaning. Seeded with core domain vocabulary, then accretes as ce-compound and ce-compound-refresh process learnings; direct edits are fine. Glossary only, not a spec or catch-all.

## Relationships

The main runtime terms are different kinds of things rather than peer controllers:

- **Session** is the long-lived lifecycle object.
- **Conversation** is the interaction state owned inside a Session.
- **Agent Execution Engine** is the run-local model/tool loop component a Session calls.
- **User advance** is one transient operation performed by a Session, not another long-lived object.

```text
Pia                 ── product-assembles ──> Coding Agent
Pia                 ── owns as a core capability ──> Skills
Coding Agent        ── hosts ──> Session

Session                         long-lived lifecycle authority
├── owns ─────────────────────> Conversation state
├── owns ─────────────────────> Workspace and resources
├── derives ──────────────────> Working Context
├── calls ────────────────────> Agent Execution Engine
├── performs ─────────────────> one user advance at a time
└── records committed facts ──> Session Journal

Conversation state
├── contains ─────────────────> Conversation History
└── determines ───────────────> current model-view projection

Agent Execution Engine ── runs ─────> Agent Loop
Run                    ── contains ─> one or more Turns
Run                    ── settles with ─> Run Message Delta
Session                ── commits ──> Run Message Delta
Working Context        ── copied into ─> Provider Request Snapshot
```

The ownership arrows describe semantic authority, not a required Go struct layout. In particular, a user advance does not require a durable struct or package, Conversation state is not an independent active object, and “Conversation Owner” is a role fulfilled by Session. Working Context is a derived model view rather than a second long-lived mutable store. The Agent Execution Engine may hold immutable Provider/tool dependencies, but all invocation-specific messages and control are run-local.

## Agent System

### Pia

The product and repository name for this Go coding agent. Its Go module and import path is `github.com/yuanbohan/pia`.

Pia product-assembles the Coding Agent and treats Skills as a built-in coding capability. A plugin or extension may distribute Skills later, but does not own or enable the core Skills lifecycle.

### Agent

An umbrella term for a model-directed system that can choose and execute multiple steps; it is not, by itself, a precise component boundary in Pia.

Use a qualified term such as Agent Execution Engine or Coding Agent whenever ownership or behavior matters. “Agent” alone must not imply ownership of complete history, persistence, workspace policy, or user interface state. “Core Agent” is a retired Pia architecture term retained only when discussing older lessons or external source vocabulary.

### Agent Loop

The repeated execution process that sends the current model input to a Provider, accepts one terminal assistant message, executes any valid tool calls, appends their results, and continues until completion or failure.

The Agent Loop defines ordering, settlement, cancellation, and tool-execution behavior. It does not define how a coding workspace is selected, how complete history is persisted, or how progress is rendered to a user.

### Run

One accepted Agent Execution Engine invocation ending only after the Agent Loop stops producing messages and all work started by that invocation has settled. A Run may be input-started, accepting one new user message, or input-free, continuing from an existing user or paired tool-result tail without appending input.

A Run may contain multiple Turns. A non-nil Run error describes its outcome but does not erase Messages already accepted during the Run.

### Turn

One Provider-produced terminal assistant response together with the tool calls and ordered tool-result Messages caused by that response.

A Turn is smaller than a Run: tool results can require another Provider call, producing another Turn in the same Run.

### Run Message Delta

The complete ordered batch of Messages newly accepted during one Run, returned as an ownership-independent value at Run settlement for Session to append to Conversation History.

An input-started Run delta includes its initiating input. An input-free continuation delta starts after the pre-existing Working Context tail and therefore contains only new assistant and tool-result Messages. A request rejected before Run acceptance has an empty delta; an accepted Run that ends in failure or cancellation still returns its error or aborted assistant and any required tool-call settlement results. The delta is transferred as an ownership-independent snapshot rather than through a channel or live event subscription.

### Agent Execution Engine

The reusable runtime component that executes the Agent Loop for one invocation. It receives an ownership-independent Working Context snapshot plus Provider, system prompt, and tool dependencies, maintains only invocation-local messages, and returns the resulting Run Message Delta.

Session may invoke the engine more than once while handling one user advance, such as an input-started Run followed by bounded overflow recovery. The engine does not own long-lived Working Context, complete Conversation History, Session lifecycle, workspace policy, persistence, input queues, or UI state, and it has no second per-Conversation active guard. Its exact Go type name is an implementation choice; the vocabulary fixes the responsibility boundary, not a required identifier.

### Coding Agent

The complete application specialized for software-engineering tasks by composing a Session, Agent Execution Engine, coding workspace, coding system prompt, coding tools, and model configuration.

A Coding Agent is not another model and does not inherit from the execution engine. It is the product-level assembly that gives the generic model/tool runtime its coding behavior; a CLI, future TUI, or future Orchestrator hosts this application through Session.

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

The runtime responsibility that owns a Conversation’s complete ordered History and current model-view projection. In Pia, Session fulfills this role.

Conversation Owner names a role, not a separate controller, type, or package beside Session. Session accepts at most one active user advance for its Conversation and rejects concurrent attempts rather than silently queueing them. One accepted Advance may consume explicitly admitted Follow-up inputs at settled Run boundaries as additional input-started Runs under the same guard; these are not concurrent Advance attempts. An overflow recovery may likewise coordinate one input-started Run and one later input-free Run under that guard. None of this prevents different Sessions or parallel-safe tools within one Run from executing concurrently. Conversation state does not independently acquire queue, cancel, close, or persistence lifecycle.

### Message

One ordered semantic unit in the model/tool dialogue: an accepted user input, a terminal assistant response, or a tool result associated with an assistant tool call.

An assistant tool call is content inside an assistant message rather than a separate message. Partial streaming events, system prompts, tool schemas, traces, and display-only events are not authoritative Messages.

### Conversation History

The authoritative, complete, ordered record of accepted Messages in a Conversation, preserved even when older model input is summarized or excluded from the Working Context.

It retains successful, error, and aborted terminal assistant messages and the tool results that close accepted tool calls, including explicit not-executed results. It excludes partial stream formation, Provider Request Snapshots, system prompts, tool schemas, logs, and UI-only events.

Session commits each Run Message Delta after that engine invocation settles. One accepted coding user advance may commit multiple input-started deltas while consuming Follow-up inputs, or an initial failed delta and a later input-free continuation delta when bounded overflow recovery succeeds. Until future persistence creates a stronger requirement, Conversation History represents settled Runs rather than partially formed active-Run state.

*Avoid:* using “transcript” without qualification when it is unclear whether the complete Conversation History or the current derived Working Context is meant.

### Working Context

The ordered message view derived by Session for the next Agent Execution Engine invocation. It is not an independently owned mutable store and is not the authoritative historical record.

Before compaction it may equal the complete Conversation History. After compaction it may contain a synthetic summary, a retained recent suffix, and messages added afterward while omitting older raw messages represented by the summary. Overflow recovery may also omit an explicitly classified error assistant by its absolute Conversation History position while preserving that message as a historical fact. It must preserve model protocol integrity, including valid assistant-tool-call and tool-result relationships. The stable system prompt, tool schemas, Provider options, Session metadata, and display-only entries are outside the Working Context.

Session derives an ownership-independent Working Context snapshot from authoritative History and the committed projection at an execution boundary. The execution engine may extend its local copy during the Run, but it does not publish that copy back as a second long-lived context owner.

### Provider Request Snapshot

An ownership-independent, request-local copy of the complete model-visible inputs for one Provider call: the system prompt, current Working Context, and tool schemas. Provider configuration, model identity, workspace objects, and the original task as a separate field are outside this snapshot in current Pia.

It is disposable after that call and never becomes an authoritative history source. Provider-side mutation or protocol conversion must not modify the Working Context or Conversation History.

### Compaction

The process of publishing a model-view projection in which older Conversation History content is represented by a summary plus a retained suffix, so future model calls use less context without deleting the complete History. It may run lazily before a new input-started Run crosses the configured threshold or be forced after an eligible context-overflow terminal before an input-free continuation.

Compaction is not arbitrary truncation: the resulting Working Context must remain sufficient to continue the task and must preserve message and tool-call protocol integrity. Internal summary requests and their terminals are not Conversation Messages.

### Semantic Event

An ordered, ephemeral observation that a meaningful runtime fact occurred, such as Run acceptance, a terminal assistant message, tool execution, compaction, or final settlement. Cancellation-specific request and settlement observations are deferred until Session control and an `Esc` interaction exist.

Semantic Events let a terminal host or another observer follow work while it is happening. They are not authoritative Conversation Messages or durable Session records, and an observer does not gain ownership of Agent Loop, Conversation History, Working Context, or lifecycle state by consuming them.

The current internal event payload carries only bounded semantic state such as fixed modes/outcomes, message role, and a tool call's turn-local source index plus a tool-owned bounded safe summary. That summary may identify the operator-visible target action, such as a path or bounded command, but the event does not copy message text, raw tool arguments/results, raw errors, model-generated call IDs, or mutable authoritative objects. A future consumer may justify an additional bounded projection, but live events are not a second History or trace.

### User Advance

One transient Session operation that accepts an initial user input and coordinates everything required to reach quiescence from that submission: optional pre-Run compaction, its input-started Run, any explicitly admitted Follow-up inputs consumed as later input-started Runs, bounded overflow recovery with an optional input-free continuation for each eligible Run, Conversation History commits, and the final result snapshot.

“Advance” is a precise operation boundary used to distinguish the complete user request from an internal Run. It is not a fourth long-lived runtime object, does not require its own owner or package, and does not add another lock or persisted state machine. Session performs it.

### Steering

User input accepted while a Session execution is active and intended to join that same ongoing advance at a defined safe boundary after current tool work settles.

Steering does not start a concurrent Run, preempt an in-flight Provider or tool call, or mean cancellation. Its exact safe-boundary behavior remains subject to source calibration when the corresponding course starts.

### Follow-up

User input accepted while a Session Advance is active but reserved for a later sequential input-started Run under that same Advance, after the current Run reaches an eligible settlement boundary and commits its Message Delta.

Successful Follow-up admission transfers responsibility for that input from the caller to Session; it acknowledges acceptance, not execution completion or a per-input result. A pending Follow-up is not a Conversation Message until its Run accepts it. It is distinct from Steering and keeps the Session from reporting true quiescence until it is consumed or an abnormal Advance settlement explicitly hands it back to the host.

### Session Journal

A versioned durable record of committed Session facts used to validate and rebuild supported Session state across process restarts.

The Session Journal may contain Session identity and workspace metadata, settled Conversation changes, lifecycle facts, and committed compaction records. It is not the live Semantic Event stream, a copy of internal Provider exchanges, or ordinary Conversation History with control records disguised as Messages.

### Session

A lifecycle and persistence envelope around a Conversation, and the intended single outer controller for user operations on that Conversation. It may add durable identity, workspace binding, stored entries, model settings, compaction records, branches, timestamps, queue/cancel/close controls, and resume behavior.

A Session is broader than Conversation History and is not synonymous with Working Context. It owns Conversation state and Workspace resources, derives Working Context, invokes the Agent Execution Engine, and performs user advances; those terms are not peer lifecycle controllers.

Session is the exclusive entrypoint and the only `busy`/queue/cancel/wait/close authority for one Conversation. It does not wrap a second Conversation controller or stateful Core Agent with overlapping lifecycle or Working Context ownership. Persistence, queues, branches, and multi-Session management may extend Session in later lessons without changing this single-owner boundary.

A Session is **closing** after Close has permanently stopped admission but before active work and owned resources have fully settled. Close immediately requests cancellation, but cancel request and clean closure are different facts. A caller may stop waiting at its context boundary while the Session remains closing; timeout never reopens the Session or turns an incomplete shutdown into a clean close. A process host may subsequently terminate the process, leaving an unclean Session for checkpoint-based recovery.

A workspace binding, when owned by a Session, is durable Session metadata rather than a Conversation Message or Working Context content. The host process's launch directory is not implicitly part of the Conversation and must not silently replace that binding during resume.

A future durable compaction record belongs to the Session Journal as control/checkpoint state, alongside but distinct from message entries. It is not a Conversation Message or a live event: the latest committed record can rebuild the Working Context projection, while failed or canceled settled records remain observable without changing that model view.

## Flagged Ambiguities

- “Agent” had been used for both the generic execution runtime and the complete coding application; use Agent Execution Engine and Coding Agent when the distinction matters.
- “Transcript” had been used for both complete history and current model-visible messages; use Conversation History and Working Context for those distinct responsibilities.
- “Session” had been used as shorthand for a message list; a Session is the broader lifecycle and persistence envelope, not the list sent to a model.
- Session, Conversation, Agent Execution Engine, and User Advance had been presented as if they were peer controllers. Treat them respectively as the sole long-lived owner, its interaction data, a run-local component it calls, and one transient operation.
