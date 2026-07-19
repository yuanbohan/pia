# pi-go Development Rules

## Project Purpose

pi-go is a learning-driven Go semantic port of Pi's core coding loop and an evidence-driven path toward a stronger Go coding agent. Preserve observable behavior rather than TypeScript file or API shape.

Phase 1 delivered a DeepSeek-first, headless coding agent for one workspace and one active task at a time. It includes the AI protocol, Faux and DeepSeek providers, the generic model/tool loop, coding tools, and a local acceptance command that exercises one initial coding task. Lesson 07, the first Phase 2 lesson, added a coding-owned Conversation Owner for the complete ordered in-memory Conversation History while the Core Agent owns the replaceable Working Context.

The long-term product is an orchestrator-driven coding-agent service: users create and advance coding tasks through IM, a Gateway exposes the service boundary, and multiple durable Sessions may run concurrently with isolation and recovery. Goal Runtime, Session persistence, Gateway/network RPC, IM adapters, multi-Session orchestration, TUI, public SDK, worktree/GitHub management, and additional providers are deferred from the current implementation scope, not rejected as product directions.

Treat `STRATEGY.md` as the canonical product-direction anchor. Pi parity is the capability floor, and stable evidence-backed performance beyond the pinned Pi coding-agent baseline is the improvement goal. Other open-source coding agents plus available Codex and Grok evidence are sources for candidate mechanisms, not contracts to copy. Do not claim superiority from a single fixture, model run, or subjective review.

## Course Workflow

- Keep `STRATEGY.md` short and durable: update it when the target problem, approach, primary user, key metrics, or investment tracks change. Do not put lesson status, implementation plans, API designs, or file-level details there.
- Treat `README.md` as the operator-facing overview and current-state entrypoint; link to canonical strategy, vocabulary, course, decisions, and plans rather than duplicating their full contents.
- Treat `docs/course/README.md` as the course index and progress record.
- Treat `docs/plans/2026-07-15-001-pi-core-go-learning-port-plan.md` as the Phase 1 base contract. For Lesson 06 command, prompt, output, trace, and acceptance behavior, `docs/plans/2026-07-19-001-feature-pia-one-shot-coding-agent-plan.md` is authoritative where the plans conflict. Align older course documents before implementing conflicting behavior.
- Maintain the future course map as a rolling outline. Give stable lesson numbers only to the nearest lessons whose dependency and responsibility boundaries are understood; keep later phases as coarse directions until earlier implementation evidence justifies a split and order.
- For each numbered future lesson, record the capability it unlocks, Pi's approximate approach and source area, the current pi-go boundary and non-goals, a concise completion signal, dependencies, relative size, and status. These fields add decision context; they must not pre-design APIs, package layouts, algorithms, or exhaustive test cases before the lesson starts.
- Size lessons as Small, Medium, Large, or XLarge by responsibility and architectural reach, not estimated human time. XLarge is not an enterable lesson size: discuss and split it before the learner can start it. Keep every resulting lesson centered on one independently explainable and testable capability.
- At the start of every lesson, before teaching its detailed design or settling its implementation, re-read the corresponding frozen Pi source and relevant tests and trace the current pi-go path that the lesson would change. Explicitly state which outline assumptions were confirmed, refined, or overturned by that evidence.
- Treat the roadmap as a revisable hypothesis. After the start-of-lesson source review, refine the lesson record into one accurate, closed capability with concrete source paths, current non-goals, and a proportionate implementation and verification direction. Update the roadmap and durable decisions before implementation when the evidence changes the boundary, ordering, size, or architecture. Accuracy and alignment with the long-term product goal take precedence over preserving an early outline.
- Only enter a lesson when the learner explicitly asks to start that lesson.
- For each implementation lesson, proceed through explanation, discussion, implementation, tests, and documentation. Lesson 00 is the documented module/baseline exception and intentionally has no Runtime package.
- Teach each concept before asking the learner to predict or answer: define the terms, trace the relevant Pi path, and work through at least one concrete example before an understanding check.
- Keep every lesson explanation-led and progressive. Exercises and questions are secondary checks, not substitutes for teaching the material clearly.
- Keep teaching concise and focused on key semantics, source evidence, and unresolved design choices; do not repeat concepts the learner has already confirmed merely to fill out a lesson. Faster teaching must not weaken the implementation: production code and tests still cover the full settled design, including error, cancellation, concurrency, ownership, and ordering contracts.
- Connect explanations to the frozen Pi source and, once implementation begins, to the actual pi-go code being written; explain what each relevant type, function, and test is responsible for as it is introduced.
- Pause on genuinely uncertain or difficult points, discuss the evidence and trade-offs with the learner, and confirm understanding before settling the design or continuing implementation. Do not turn the course into a sequence of quizzes.
- Teach course material directly and interactively in the current conversation or terminal UI, including when using an explanation workflow or skill. Keep only necessary Markdown course records, code, and tests in the repository; do not generate HTML explainers or lesson artifacts unless the learner explicitly asks for HTML.
- Actively challenge learner proposals and the instructor's own prior proposals against the frozen Pi source, documentation, tests, and explicit project constraints. Agreement is not evidence.
- Keep learner hypotheses, verified Pi contracts, candidate Go mechanisms, and settled pi-go decisions explicitly separated. Do not promote a suggestion into architecture merely because it sounds reasonable.
- Keep lesson scope small without weakening the core technical design. A phase may postpone providers, fields, loop branches, persistence, or UI, but must not choose a disposable ownership or API model that contradicts the intended Agent architecture merely because the current lesson exercises fewer cases.
- When new evidence invalidates an earlier course conclusion, state the correction directly and update the durable record before implementation.
- Record lesson-specific discussion in its lesson document and durable architecture decisions in `docs/course/decisions.md`.
- Treat `CONCEPTS.md` as the shared project vocabulary. Read and reuse its bounded terms when teaching or designing, and refine an entry only when source evidence or a settled decision changes its meaning, ownership, or exclusions. Keep plans, implementation status, and file-specific details out of the glossary.
- Do not advance to the next lesson until the learner confirms understanding.
- Do not commit unless the learner explicitly asks for a commit.
- A lesson commit may contain only the files belonging to the confirmed lesson.

## Architecture Constraints

- Keep unstable implementation packages under `internal/`; do not create a public SDK without a new decision.
- Keep `internal/ai` independent and keep the consumer-owned `ai.Provider` interface beside `Request` and `Stream`. Group concrete implementations under `internal/ai/provider/`; `provider/faux`, `provider/openaicompatible`, and `provider/deepseek` depend inward on `ai`. Do not turn the organizational `provider/` directory into a Pi-style registry or move the small Provider port out of `ai`. `internal/agent` may depend on `ai`; coding tools implement contracts owned by `agent`; the local coding composition root assembles Agent, DeepSeek, and tools.
- Keep Agent Runtime separate from the future Orchestrator or Agent Manager, Gateway, and IM adapters.
- Keep Lesson 07 to the in-memory ownership boundary. Do not add Goal Runtime, Session persistence/recovery, compaction, steering, follow-up, or subscription lifecycle merely to prepare for later phases. Complete in-memory Conversation History is Coding Agent application state, not deferred Session infrastructure; the Core Agent owns only its replaceable Working Context.
- Treat the temporary `cmd/pia` command as a local execution and acceptance entrypoint, not a stable name or external integration contract. Defer gRPC and public SDK design until real external callers are in scope.
- Use a Faux Provider for deterministic Agent tests before real DeepSeek calls.
- Let the coding-owned Conversation Owner preserve one complete ordered in-memory Conversation History across sequential Runs, and let the Core Agent preserve the replaceable Working Context used by Provider calls. In the current coding composition, before any explicit Working Context replacement, their contents remain equal, but they are different owners. Each Provider call receives the stable system prompt, a deep-cloned snapshot of the current Working Context, and current tool schemas. Build workspace/cwd context into the stable system prompt, and do not add separate Provider request fields for workspace context or task. Starting a new conversation requires a new Conversation Owner and Core Agent; never clear History merely because another user message arrived.
- Return the Core Agent's ownership-independent run-local `NewMessages` together with an idiomatic Go error. The Conversation Owner commits that delta even when the accepted Run returns an error, then returns a deep-cloned complete History snapshot as the coding `RunResult.Transcript`. Completed loops return nil error, Provider/stream failures return non-nil errors, and cancellation preserves the context cause. Keep terminal final/error/aborted assistant messages in both the Working Context and committed History; do not add a duplicate Run outcome field.
- Append each Provider turn's terminal assistant exactly once. If an error/aborted terminal contains completed tool calls, do not execute them; append same-ID not-executed tool results before returning the Turn error so both the Working Context and authoritative Conversation History have no orphaned calls and can be reused directly.
- Allow at most one active Run per Conversation. The Conversation Owner rejects concurrent Runs without queueing and keeps its guard active through Core Agent settlement, History commit, and the returned History snapshot. The Core Agent retains its own guard and permits `ReplaceWorkingContext` only while idle; replacement deep-clones its input and never mutates Conversation History.
- Schedule consecutive explicitly parallel-safe tools as one parallel stage and every other tool as a serial barrier. Only `read` is parallel-safe in Phase 1; the default is false.
- Preserve model source order for run-local and committed tool results even when tool workers run concurrently. The Phase 1 one-shot CLI prints only the terminal assistant text after the Run settles. Bash still drains output incrementally to avoid pipe deadlocks and to construct its bounded result, but no Agent event sink or Tool progress callback is needed until a future interactive UI establishes a real consumer.
- Use `os.Root` for file-tool workspace containment. Bash starts in the workspace, inherits the complete parent process environment like frozen Pi, and is not sandboxed; Provider-generated commands have the invoking user's host and network authority and may create irreversible side effects outside the workspace.
- Do not inject Provider credentials held only in Provider configuration into tool argv, tool configuration, Conversation History, Working Context, trace metadata, events, logs, or errors. Bash deliberately inherits the parent environment, so any Provider credential already present there is visible to commands; do not describe this behavior as credential isolation.
- Make DeepSeek data egress explicit in operator documentation: operators must choose a workspace whose selected contents and tool results may be sent to the Provider. The Phase 1 one-shot command has no startup warning or approval flow.
- Keep real-model acceptance artifacts under the ignored `tmp/` directory. Do not commit a benchmark harness, task fixture, prompt, trace, or model-modified workspace for the Lesson 06 acceptance loop.
- Do not add an automatic wall-clock or model-turn budget in Phase 1. Process signals and caller cancellation are the execution boundary; revisit budgets only with evidence from real runs.
- Do not add TUI behavior to core packages.

## Architecture and Maintainability

- Do not over-design. Do not add abstractions, configuration, extension points, compatibility layers, or error branches for hypothetical consumers or failure modes that have not been established by the current lesson, verified source behavior, tests, or an explicit project decision.
- Prefer the smallest design that fully satisfies the settled behavior, but do not use “small scope” as a reason to weaken ownership, cancellation, security, or dependency-direction contracts that are already known.
- Before adding a new module, review the current project structure as a whole: dependency direction, package ownership, filenames, file cohesion, and how the new responsibility will interact with later confirmed work. Do not assume the layout chosen for an earlier, smaller implementation is still the clearest layout.
- If that review indicates a package move, package split, directory restructuring, meaningful file rename, or other structural refactor, present the evidence, intended boundaries, migration impact, and proposed layout to the learner before making the structural change.
- Keep each package centered on one cohesive responsibility and keep dependencies pointing inward according to the project's established architecture. Do not use a package as a dumping ground for unrelated helpers merely because it already exists.
- Apply Clean Architecture as a project-specific dependency-direction and ownership discipline. Do not impose fixed `entities`, `usecases`, or `adapters` directory layers when the current design does not need them.
- Keep each file focused on a coherent part of its package responsibility. Split a file when independent protocol, orchestration, platform, persistence, or utility concerns have accumulated; do not split files solely to meet an arbitrary line-count target.
- Keep non-test Go source files at or below 1,000 lines. Test files may exceed this ceiling when they remain cohesive. Treat the limit as a review backstop: split by responsibility rather than mechanically slicing a file.
- Put shared code in a common package only after current, concrete consumers establish the shared responsibility. A generic name such as `utils` does not justify moving unrelated code together; keep feature-specific behavior beside its feature until reuse and ownership are proven.
- Prefer consumer-owned, narrow interfaces and concrete types. Introduce an interface only when a current boundary needs substitution, testing, or dependency inversion; do not create speculative interfaces for possible future implementations.
- Make resource ownership and lifecycle explicit. Constructors validate required dependencies; the creator closes owned resources; borrowed resources are not closed by consumers; every goroutine, stream, file, timer, and active child process has a deterministic settlement path. Bash processes intentionally left running after a normally completed background command are the recorded exception, not an accidental leak.
- Design mutations so failure and cancellation have explicit observable semantics. Avoid publishing partial state, clean up uncommitted temporary resources, preserve committed results after the commit point, and document any intentionally retained side effects.
- Keep model-visible and operator-visible outputs bounded and free of unnecessary secrets or host details. Frozen Pi-compatible bash full-output temp files are the explicit exception to output storage bounds: the tool result and Working Context remain bounded, while the complete command output may continue growing on disk and is not automatically deleted. Wrap errors with useful operation context while preserving their cause for `errors.Is` and `context.Cause` checks.
- Treat concurrency as an explicit contract, not an implementation accident. Avoid package-level mutable state, state whether instances are safe for concurrent use, and add race/cancellation coverage whenever shared state or goroutines are introduced.
- During review, check both local clarity and architectural fit: package placement, dependency direction, API surface, file responsibility, error/cleanup paths, cancellation, concurrency, test seams, and whether a simpler evidence-backed design can replace newly added machinery.

## Go Quality

- Prefer idiomatic Go behavior over mechanical TypeScript translation.
- Pass `context.Context` through cancellable model, tool, process, and persistence operations.
- Avoid speculative interfaces and helpers; introduce abstractions only when the current lesson establishes their responsibility.
- Write all code comments in English. Repository documentation may use whichever language best serves its readers.
- Add code comments when an evidence-backed, non-obvious adjustment would otherwise be hard for later readers to understand; explain why the behavior is necessary and its observable consequence. Do not turn guesses into comments: ground them in verified source behavior, protocol documentation, tests, or a recorded decision.
- Keep default tests offline and independent of API keys or paid services.
- Use explicit opt-in integration tests for real providers.
- Keep `.golangci.yml` limited to explicit, high-signal checks. Resolve every lint finding; use a narrowly scoped `//nolint` only when necessary and include an English explanation.
- After Go changes, run `make check`; it formats Go files, runs `go vet ./...`, runs `go test ./...`, and runs `golangci-lint`.
- Run `go test -race ./...` after concurrency, cancellation, or process-management changes.

## Git Safety

- Never commit, push, or open a PR unless the learner explicitly requests it.
- Stage explicit lesson paths only; never use `git add .` or `git add -A`.
- Preserve unrelated user changes.
- Never use destructive reset, checkout, clean, or stash commands to remove work.

## Frozen Pi Baseline

- Pi commit: `dcfe36c79702ec240b146c45f167ab75ecddd205`
- Pi Agent package version: `0.80.7`

Changing this baseline requires a recorded decision and updated source-reading notes.
