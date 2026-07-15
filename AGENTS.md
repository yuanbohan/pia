# pi-go Development Rules

## Project Purpose

pi-go is a learning-driven Go semantic port of Pi's core Agent Runtime. Preserve observable behavior rather than TypeScript file or API shape.

The initial scope is DeepSeek-first and headless. The root Go module includes AI protocol, Agent Loop, coding tools, Goal Runtime, and Session persistence. TUI, public SDK, network RPC, IM adapters, multi-tenant Agent Manager, and additional providers are outside this repository's current scope.

## Course Workflow

- Treat `docs/course/README.md` as the course index and progress record.
- Only enter a lesson when the learner explicitly asks to start that lesson.
- For each lesson, proceed through explanation, discussion, implementation, tests, and documentation.
- Teach each concept before asking the learner to predict or answer: define the terms, trace the relevant Pi path, and work through at least one concrete example before an understanding check.
- Actively challenge learner proposals and the instructor's own prior proposals against the frozen Pi source, documentation, tests, and explicit project constraints. Agreement is not evidence.
- Keep learner hypotheses, verified Pi contracts, candidate Go mechanisms, and settled pi-go decisions explicitly separated. Do not promote a suggestion into architecture merely because it sounds reasonable.
- When new evidence invalidates an earlier course conclusion, state the correction directly and update the durable record before implementation.
- Record lesson-specific discussion in its lesson document and durable architecture decisions in `docs/course/decisions.md`.
- Do not advance to the next lesson until the learner confirms understanding.
- Do not commit unless the learner explicitly asks for a commit.
- A lesson commit may contain only the files belonging to the confirmed lesson.

## Architecture Constraints

- Keep unstable implementation packages under `internal/`; do not create a public SDK without a new decision.
- Keep `internal/ai` independent; `internal/agent` may depend on `ai`; `coding` may depend on `agent`; `goal` may depend on `coding`.
- Keep Agent Runtime separate from the future Agent Manager and IM adapters.
- Keep Goal Runtime above the generic Agent Loop.
- Treat `cmd/pi-go` as a local execution and acceptance entrypoint, not a stable external integration contract. Defer gRPC and public SDK design until real external callers are in scope.
- Use a Faux Provider for deterministic Agent tests before real DeepSeek calls.
- Do not add TUI behavior to core packages.

## Go Quality

- Prefer idiomatic Go behavior over mechanical TypeScript translation.
- Pass `context.Context` through cancellable model, tool, process, and persistence operations.
- Avoid speculative interfaces and helpers; introduce abstractions only when the current lesson establishes their responsibility.
- Keep default tests offline and independent of API keys or paid services.
- Use explicit opt-in integration tests for real providers.
- After Go changes, run `gofmt` on changed files, `go test ./...`, and `go vet ./...`.
- Run `go test -race ./...` after concurrency, cancellation, Session, or process-management changes.

## Git Safety

- Never commit, push, or open a PR unless the learner explicitly requests it.
- Stage explicit lesson paths only; never use `git add .` or `git add -A`.
- Preserve unrelated user changes.
- Never use destructive reset, checkout, clean, or stash commands to remove work.

## Frozen Pi Baseline

- Pi commit: `dcfe36c79702ec240b146c45f167ab75ecddd205`
- Pi Agent package version: `0.80.7`

Changing this baseline requires a recorded decision and updated source-reading notes.
