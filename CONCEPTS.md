# Concepts

Shared domain vocabulary for this project — entities, named processes, and status concepts with project-specific meaning. Seeded with core domain vocabulary, then accretes as ce-compound and ce-compound-refresh process learnings; direct edits are fine. Glossary only, not a spec or catch-all.

## Relationships

```text
Coding Agent
├── configures and hosts the Core Agent
├── provides coding-specific workspace, prompt, tools, and model choices
└── coordinates a Conversation Owner

Conversation Owner ── owns ──> Conversation History
Core Agent          ── owns ──> Working Context
Core Agent          ── runs ──> Agent Loop
Run                 ── contains ──> one or more Turns
Run                 ── settles with ──> Run Message Delta
Conversation Owner  ── commits ──> Run Message Delta
Working Context     ── copied into ──> Provider Request Snapshot
Session             ── may persist and extend ──> Conversation
```

The ownership arrows describe semantic authority, not a required Go struct layout. A later implementation may compose these responsibilities without giving every concept its own package or exported type.

## Agent System

### Agent

An umbrella term for a model-directed system that can choose and execute multiple steps; it is not, by itself, a precise component boundary in pi-go.

Use a qualified term such as Core Agent or Coding Agent whenever ownership or behavior matters. “Agent” alone must not imply ownership of complete history, persistence, workspace policy, or user interface state.

### Agent Loop

The repeated execution process that sends the current model input to a Provider, accepts one terminal assistant message, executes any valid tool calls, appends their results, and continues until completion or failure.

The Agent Loop defines ordering, settlement, cancellation, and tool-execution behavior. It does not define how a coding workspace is selected, how complete history is persisted, or how progress is rendered to a user.

### Run

One accepted Core Agent execution initiated by new input or an explicit continuation and ending only after the Agent Loop stops producing messages and all work started by that execution has settled.

A Run may contain multiple Turns. A non-nil Run error describes its outcome but does not erase Messages already accepted during the Run.

### Turn

One Provider-produced terminal assistant response together with the tool calls and ordered tool-result Messages caused by that response.

A Turn is smaller than a Run: tool results can require another Provider call, producing another Turn in the same Run.

### Run Message Delta

The complete ordered batch of Messages newly accepted during one Run, returned as an ownership-independent value at Run settlement for the Conversation Owner to append to Conversation History.

It includes initiating input, terminal assistant Messages, and tool-result Messages when those entries occurred during the Run. A request rejected before Run acceptance has an empty delta; an accepted Run that ends in failure or cancellation still returns its error or aborted assistant and any required tool-call settlement results. The delta is transferred as an ownership-independent snapshot rather than through a channel or live event subscription.

### Core Agent

The reusable runtime component that executes the Agent Loop, owns the replaceable Working Context for one Conversation, and allows at most one active run to mutate that context at a time.

The Core Agent receives a Provider, system prompt, and tool definitions as dependencies, but it does not construct coding-specific prompts, choose workspace policy, persist Sessions, own the complete Conversation History, or implement a TUI. “Core” describes this lower-level responsibility boundary; it does not mean a second agent process or model.

### Coding Agent

The complete application specialized for software-engineering tasks by composing a Core Agent with a coding workspace, coding system prompt, coding tools, model configuration, and conversation orchestration.

A Coding Agent is not another model and does not inherit from the Core Agent. It is the product-level assembly that gives the generic model/tool runtime its coding behavior; a CLI or future TUI hosts this application, while future Session support extends its conversation lifecycle.

## Conversation and Context

### Conversation

One logical, continuable interaction lineage between a user and the Coding Agent, spanning sequential user inputs and all accepted assistant and tool-result messages produced from them.

Compaction does not create a new Conversation. Provider credentials, workspace file contents, tool implementations, request-local copies, traces, and UI rendering state are not Conversation contents merely because they participate in running it.

### Conversation Owner

The runtime responsibility that owns a Conversation’s complete ordered history and coordinates it with the Core Agent’s replaceable Working Context.

Conversation Owner names an ownership role, not a requirement for a type or package with that exact name. The current minimal owner accepts at most one active Run per Conversation and rejects concurrent attempts rather than silently queueing them; this does not prevent different Conversations or parallel-safe tools within one Run from executing concurrently. Its minimal in-memory form does not imply Session persistence, branches, event subscriptions, or a public API.

### Message

One ordered semantic unit in the model/tool dialogue: an accepted user input, a terminal assistant response, or a tool result associated with an assistant tool call.

An assistant tool call is content inside an assistant message rather than a separate message. Partial streaming events, system prompts, tool schemas, traces, and display-only events are not authoritative Messages.

### Conversation History

The authoritative, complete, ordered record of accepted Messages in a Conversation, preserved even when older model input is summarized or excluded from the Working Context.

It retains successful, error, and aborted terminal assistant messages and the tool results that close accepted tool calls, including explicit not-executed results. It excludes partial stream formation, Provider Request Snapshots, system prompts, tool schemas, logs, and UI-only events.

The minimal in-memory Conversation Owner commits each Run Message Delta after that Run settles. Until future persistence or live observation creates a stronger requirement, Conversation History represents settled Runs rather than partially formed active-Run state.

*Avoid:* using “transcript” without qualification when it is unclear whether the complete Conversation History or the Core Agent’s current Working Context is meant.

### Working Context

The ordered message view that the Core Agent can use to continue the Conversation on the next model call; it is replaceable while the Core Agent is idle and is not the authoritative historical record.

Before compaction it may equal the complete Conversation History. After compaction it may contain a synthetic summary, a retained recent suffix, and messages added afterward while omitting older raw messages represented by the summary. It must preserve model protocol integrity, including valid assistant-tool-call and tool-result relationships. The stable system prompt, tool schemas, Provider options, Session metadata, and display-only entries are outside the Working Context.

### Provider Request Snapshot

An ownership-independent, request-local copy of the complete model-visible inputs for one Provider call: the system prompt, current Working Context, tool schemas, and any request-scoped options.

It is disposable after that call and never becomes an authoritative history source. Provider-side mutation or protocol conversion must not modify the Working Context or Conversation History.

### Compaction

The process of replacing older Working Context content with a summary plus a retained suffix so future model calls use less context without deleting the complete Conversation History.

Compaction is not arbitrary truncation: the resulting Working Context must remain sufficient to continue the task and must preserve message and tool-call protocol integrity.

### Session

A lifecycle and persistence envelope around a Conversation that may add durable identity, stored entries, model settings, compaction records, branches, timestamps, and resume behavior.

A Session is broader than Conversation History and is not synonymous with Working Context. pi-go can establish the Conversation ownership boundary without implementing a persistent Session.

## Flagged Ambiguities

- “Agent” had been used for both the generic execution runtime and the complete coding application; use Core Agent and Coding Agent when the distinction matters.
- “Transcript” had been used for both complete history and current model-visible messages; use Conversation History and Working Context for those distinct responsibilities.
- “Session” had been used as shorthand for a message list; a Session is the broader lifecycle and persistence envelope, not the list sent to a model.
