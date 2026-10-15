---
id: 123
created: 2026-10-15
status: active
---
# Stop runtime output feedback and redundant writes

## Context

Task 123 follows [[audits/2026-10-15-runtime-redundant-work]] R1/R4: a failed-only order retains the full flush path; replacing orders.json wakes the same loop again. In the isolated experiment it ran 1,209 cycles in 1.2 seconds. Other stable outputs amplify server notifications. The prior empty-control fix remains effective.

## Scope and intended behavior

Remove backend-owned orders.json from runtime wake inputs. Keep orders-next.json and control.ndjson wakeups and the existing one-second ticker. Completions and merge results currently progress through that ticker rather than an explicit wake signal; preserve that bound instead of silently promising instantaneous wakeups.

Make only orders.json, state.json, pending-review.json and nonzero last-applied-seq writes content-idempotent. Equal bytes leave inode, mtime and mode untouched. Changed/missing/corrupted outputs are atomically replaced with the existing 0644 behavior. Preserve serialization, flush order, error propagation and command replay barriers.

Out of scope: historical metadata/tickets scans, adapter cadence, repeated projection computation, UI protocol, scheduler eligibility, canonical checkpoint deduplication, global atomic-writer behavior changes. These are separate backlog items. No polling delay increase or special-case skip of failed orders.

## Design and alternatives

Following [[principles/fix-root-causes]] and [[principles/subtract-before-you-add]], remove the incorrect output-to-input edge first. From first principles, backend projections are views, while commands and orders-next are inputs. Startup still reads legacy orders when canonical recovery needs them; running cycles already use in-memory/canonical state.

Following [[principles/make-operations-idempotent]], add an opt-in filex.WriteFileAtomicIfChanged helper and use it only at the four selected output boundaries. Read actual disk bytes and delegate to existing WriteFileAtomic on difference or read failure. A failed read is not a new error condition: the previous writer could replace an unreadable file in a writable parent. Actual replacement failures still propagate. No cache: restart, deletion and external corruption repair remain natural. Equal bytes intentionally do not normalize externally changed mode bits; avoiding unchanged metadata operations is part of this contract.

Alternatives: (1) debounce or slower polling only masks feedback and delays real inputs; rejected. (2) remove the output wake only stops storms but leaves stable output writes each second; incomplete. (3) disk-byte comparison plus ownership-correct wake filtering is chosen; a hash/mtime cache adds invalidation complexity without avoiding generation work in this task. Existing atomic writer remains unchanged for other callers. Atomic rename is existing visibility behavior, not a new fsync durability guarantee.

[[principles/serialize-shared-state-mutations]]: use existing single backend writer/flush sequencing; read-compare-write is not a general multi-writer transaction. [[principles/prove-it-works]]: commit failing regression tests before fixes, then validate real process CPU and file stability, not only paused state.

## Applicable skills

plan, adversarial-review, debugging, go-best-practices, testing, execute, worktree, commit, quality, reflect. Planning exploration is delegated; independent review uses Claude CLI (opposite provider), not Codex subagents. User explicitly authorized plan → separate review → implementation → CPU verification in this request; no additional human pause is needed between those stages.

## Phases

- [ ] [[plans/123-runtime-output-idempotence/phase-01-input-ownership]]
- [ ] [[plans/123-runtime-output-idempotence/phase-02-idempotent-writer]]
- [ ] [[plans/123-runtime-output-idempotence/phase-03-projection]]
- [ ] [[plans/123-runtime-output-idempotence/phase-04-pending-review]]
- [ ] [[plans/123-runtime-output-idempotence/phase-05-sequence]]
- [ ] [[plans/123-runtime-output-idempotence/phase-06-verification]]

Phases 2–5 use one helper contract and execute sequentially. Regression tests through existing output entry points are committed together before the production fix, so they compile on the red baseline. New-helper unit tests are added with its implementation; no production scaffolding is introduced merely to make a test compile. No unrelated task implementation is included.

## Verification

Run focused regression tests with -race for filex/projection/loop, including sequence replay, flush barriers, orders-next crash safety and real fsnotify. Run pnpm check, UI unit suite and pnpm test:smoke with the repo-compatible Node/pnpm/Git toolchain. Build with pnpm build, not bare go build. Independent code quality review follows implementation.

Capture a 15-second CPU-time delta and file-change baseline from the currently running failed-only playground process. After checks, install a version-labelled build at the existing local noodle target, gracefully restart only that verified process (no active provider work), and repeat the same measurement with the same failed order/history/config. Verify PID executable, version, loop state and unchanged order payload. If workload changes, report the comparison limit and use a controlled equivalent fixture. Retain old binary for rollback until verification succeeds. In addition to lower CPU, the four target outputs must have zero changes after startup settles. Remaining metadata/tickets/adapter work is expected and must be reported separately rather than widening scope.
