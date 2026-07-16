# pi-go Development Rules

## Project Purpose

pi-go is a learning-driven Go semantic port of Pi's core coding loop. Preserve observable behavior rather than TypeScript file or API shape.

Phase 1 is a DeepSeek-first, headless coding agent for one task in one workspace. It includes the AI protocol, Faux and DeepSeek providers, the generic model/tool loop, coding tools, and a local acceptance command. Goal Runtime, Session persistence, TUI, public SDK, network RPC, IM adapters, multi-tenant Agent Manager, worktree/GitHub management, and additional providers are deferred.

## Course Workflow

- Treat `docs/course/README.md` as the course index and progress record.
- Treat `docs/plans/2026-07-15-001-pi-core-go-learning-port-plan.md` as the current product and implementation contract. Align older course documents before implementing conflicting behavior.
- Only enter a lesson when the learner explicitly asks to start that lesson.
- For each implementation lesson, proceed through explanation, discussion, implementation, tests, and documentation. Lesson 00 is the documented module/baseline exception and intentionally has no Runtime package.
- Teach each concept before asking the learner to predict or answer: define the terms, trace the relevant Pi path, and work through at least one concrete example before an understanding check.
- Keep every lesson explanation-led and progressive. Exercises and questions are secondary checks, not substitutes for teaching the material clearly.
- Connect explanations to the frozen Pi source and, once implementation begins, to the actual pi-go code being written; explain what each relevant type, function, and test is responsible for as it is introduced.
- Pause on genuinely uncertain or difficult points, discuss the evidence and trade-offs with the learner, and confirm understanding before settling the design or continuing implementation. Do not turn the course into a sequence of quizzes.
- Teach course material directly in the conversation and keep only necessary course records, code, and tests in the repository. Do not generate separate HTML lesson artifacts unless the learner explicitly asks for one.
- Actively challenge learner proposals and the instructor's own prior proposals against the frozen Pi source, documentation, tests, and explicit project constraints. Agreement is not evidence.
- Keep learner hypotheses, verified Pi contracts, candidate Go mechanisms, and settled pi-go decisions explicitly separated. Do not promote a suggestion into architecture merely because it sounds reasonable.
- When new evidence invalidates an earlier course conclusion, state the correction directly and update the durable record before implementation.
- Record lesson-specific discussion in its lesson document and durable architecture decisions in `docs/course/decisions.md`.
- Do not advance to the next lesson until the learner confirms understanding.
- Do not commit unless the learner explicitly asks for a commit.
- A lesson commit may contain only the files belonging to the confirmed lesson.

## Architecture Constraints

- Keep unstable implementation packages under `internal/`; do not create a public SDK without a new decision.
- Keep `internal/ai` independent; concrete providers depend on `ai`; `internal/agent` may depend on `ai`; coding tools implement contracts owned by `agent`; the local coding composition root assembles Agent, DeepSeek, and tools.
- Keep Agent Runtime separate from the future Agent Manager and IM adapters.
- Keep Phase 1 to the model/tool loop. Do not add Goal Runtime, Session state, compaction, steering, follow-up, or subscription lifecycle merely to prepare for later phases.
- Treat `cmd/pi-go` as a local execution and acceptance entrypoint, not a stable external integration contract. Defer gRPC and public SDK design until real external callers are in scope.
- Use a Faux Provider for deterministic Agent tests before real DeepSeek calls.
- Send every Provider call the stable system prompt, ordered in-memory transcript, and current tool schemas. Build workspace/cwd context into the stable system prompt, and represent the original task once as the transcript's initial user message; do not add separate Provider request fields for workspace context or task. Do not persist or compact transcript in Phase 1.
- Schedule consecutive explicitly parallel-safe tools as one parallel stage and every other tool as a serial barrier. Only `read` is parallel-safe in Phase 1; the default is false.
- Serialize awaited event delivery through the Run-owned coordinator even when tool workers run concurrently. Preserve completion order for events and model source order for transcript results.
- Use `os.Root` for file-tool workspace containment. Bash starts in the workspace with a minimal environment but is not sandboxed and may access other resources available to the current user.
- Do not propagate Provider credentials to tool environments, argv, tool configuration, transcript, events, logs, or errors.
- Make DeepSeek data egress explicit: operators must choose a workspace whose selected contents and tool results may be sent to the Provider. Phase 1 shows a warning but has no approval flow.
- Do not add TUI behavior to core packages.

## Go Quality

- Prefer idiomatic Go behavior over mechanical TypeScript translation.
- Pass `context.Context` through cancellable model, tool, process, and persistence operations.
- Avoid speculative interfaces and helpers; introduce abstractions only when the current lesson establishes their responsibility.
- Keep default tests offline and independent of API keys or paid services.
- Use explicit opt-in integration tests for real providers.
- After Go changes, run `gofmt` on changed files, `go test ./...`, and `go vet ./...`.
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
