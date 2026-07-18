# pi-go Development Rules

## Project Purpose

pi-go is a learning-driven Go semantic port of Pi's core coding loop. Preserve observable behavior rather than TypeScript file or API shape.

Phase 1 is a DeepSeek-first, headless coding agent for one workspace and one active task at a time. The Agent keeps the complete ordered in-memory transcript across sequential user inputs; the local acceptance command exercises one initial coding task. Phase 1 includes the AI protocol, Faux and DeepSeek providers, the generic model/tool loop, coding tools, and a local acceptance command. Goal Runtime, Session persistence, TUI, public SDK, network RPC, IM adapters, multi-tenant Agent Manager, worktree/GitHub management, and additional providers are deferred.

## Course Workflow

- Treat `docs/course/README.md` as the course index and progress record.
- Treat `docs/plans/2026-07-15-001-pi-core-go-learning-port-plan.md` as the current product and implementation contract. Align older course documents before implementing conflicting behavior.
- Only enter a lesson when the learner explicitly asks to start that lesson.
- For each implementation lesson, proceed through explanation, discussion, implementation, tests, and documentation. Lesson 00 is the documented module/baseline exception and intentionally has no Runtime package.
- Teach each concept before asking the learner to predict or answer: define the terms, trace the relevant Pi path, and work through at least one concrete example before an understanding check.
- Keep every lesson explanation-led and progressive. Exercises and questions are secondary checks, not substitutes for teaching the material clearly.
- Keep teaching concise and focused on key semantics, source evidence, and unresolved design choices; do not repeat concepts the learner has already confirmed merely to fill out a lesson. Faster teaching must not weaken the implementation: production code and tests still cover the full settled design, including error, cancellation, concurrency, ownership, and ordering contracts.
- Connect explanations to the frozen Pi source and, once implementation begins, to the actual pi-go code being written; explain what each relevant type, function, and test is responsible for as it is introduced.
- Pause on genuinely uncertain or difficult points, discuss the evidence and trade-offs with the learner, and confirm understanding before settling the design or continuing implementation. Do not turn the course into a sequence of quizzes.
- Teach course material directly in the conversation and keep only necessary course records, code, and tests in the repository. Do not generate separate HTML lesson artifacts unless the learner explicitly asks for one.
- Actively challenge learner proposals and the instructor's own prior proposals against the frozen Pi source, documentation, tests, and explicit project constraints. Agreement is not evidence.
- Keep learner hypotheses, verified Pi contracts, candidate Go mechanisms, and settled pi-go decisions explicitly separated. Do not promote a suggestion into architecture merely because it sounds reasonable.
- Keep lesson scope small without weakening the core technical design. A phase may postpone providers, fields, loop branches, persistence, or UI, but must not choose a disposable ownership or API model that contradicts the intended Agent architecture merely because the current lesson exercises fewer cases.
- When new evidence invalidates an earlier course conclusion, state the correction directly and update the durable record before implementation.
- Record lesson-specific discussion in its lesson document and durable architecture decisions in `docs/course/decisions.md`.
- Do not advance to the next lesson until the learner confirms understanding.
- Do not commit unless the learner explicitly asks for a commit.
- A lesson commit may contain only the files belonging to the confirmed lesson.

## Architecture Constraints

- Keep unstable implementation packages under `internal/`; do not create a public SDK without a new decision.
- Keep `internal/ai` independent and keep the consumer-owned `ai.Provider` interface beside `Request` and `Stream`. Group concrete implementations under `internal/ai/provider/`; `provider/faux`, `provider/openaicompatible`, and `provider/deepseek` depend inward on `ai`. Do not turn the organizational `provider/` directory into a Pi-style registry or move the small Provider port out of `ai`. `internal/agent` may depend on `ai`; coding tools implement contracts owned by `agent`; the local coding composition root assembles Agent, DeepSeek, and tools.
- Keep Agent Runtime separate from the future Agent Manager and IM adapters.
- Keep Phase 1 to the model/tool loop. Do not add Goal Runtime, Session persistence/recovery, compaction, steering, follow-up, or subscription lifecycle merely to prepare for later phases. The Agent-owned ordered in-memory transcript is core loop state, not deferred Session infrastructure.
- Treat `cmd/pi-go` as a local execution and acceptance entrypoint, not a stable external integration contract. Defer gRPC and public SDK design until real external callers are in scope.
- Use a Faux Provider for deterministic Agent tests before real DeepSeek calls.
- Let the Agent own one ordered in-memory transcript across sequential `Run` calls. Each new user input is appended; every Provider call receives the stable system prompt, the complete transcript so far, and current tool schemas; assistant messages and tool results are appended rather than replacing prior context. Build workspace/cwd context into the stable system prompt, and do not add separate Provider request fields for workspace context or task. Starting a new conversation requires a new Agent or an explicit reset boundary; never clear history merely because another user message arrived. Do not persist or compact transcript in Phase 1.
- Return a snapshot of the Agent's complete transcript together with an idiomatic Go error: completed loop returns nil error, Provider/stream failure returns a non-nil error, and cancellation preserves the context cause. Keep the terminal final/error/aborted assistant message in the Agent transcript; do not add a duplicate Run outcome field in Phase 1.
- Append each Provider turn's terminal assistant exactly once. If an error/aborted terminal contains completed tool calls, do not execute them; append same-ID not-executed tool results before returning the Turn error so the authoritative transcript has no orphaned calls and can be reused directly by the next Run.
- Schedule consecutive explicitly parallel-safe tools as one parallel stage and every other tool as a serial barrier. Only `read` is parallel-safe in Phase 1; the default is false.
- Preserve model source order for transcript results even when tool workers run concurrently. Defer the Agent event sink and terminal/TUI streaming display contract until the local headless entrypoint needs observable progress; do not add it to the basic transcript loop merely as future preparation.
- Use `os.Root` for file-tool workspace containment. Bash starts in the workspace with a minimal environment but is not sandboxed and may access other resources available to the current user.
- Do not propagate Provider credentials to tool environments, argv, tool configuration, transcript, events, logs, or errors.
- Make DeepSeek data egress explicit: operators must choose a workspace whose selected contents and tool results may be sent to the Provider. Phase 1 shows a warning but has no approval flow.
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
- Make resource ownership and lifecycle explicit. Constructors validate required dependencies; the creator closes owned resources; borrowed resources are not closed by consumers; every goroutine, stream, file, timer, and child process has a deterministic settlement path.
- Design mutations so failure and cancellation have explicit observable semantics. Avoid publishing partial state, clean up uncommitted temporary resources, preserve committed results after the commit point, and document any intentionally retained side effects.
- Keep model-visible and operator-visible outputs bounded and free of unnecessary secrets or host details. Wrap errors with useful operation context while preserving their cause for `errors.Is` and `context.Cause` checks.
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
