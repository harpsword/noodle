---
id: 124
created: 2026-10-15
status: active
---
# Refresh session metadata only when source state changes

## Context

After [[archive/plans/123-runtime-output-idempotence/overview]], the running playground process still consumes 9.0% CPU over a fresh 15.006-second sample (PID 53523, v0.1.5+local.3c51ca9). All 58 historical meta.json files change once per second: 870 observed replacements. The loop remains running/supervised with its existing failed schedule order. Source baseline is main 0aaef41. See [[audits/2026-10-15-runtime-redundant-work]].

Production constructs one Monitor and invokes RunOnce each maintenance cycle. Every call reparses every canonical log and rewrites metadata, including time-only changes for terminal sessions. Standalone Monitor.Run debounce does not control this path.

## Scope

Cache canonical-derived claims in CanonicalClaimsReader, keep current observation on every pass, and persist metadata only when derived values change. Preserve fresh spawn metadata, process/heartbeat checks, terminal process repair, scheduler exceptions, external metadata corrections and restart reconstruction. Ticket replay/writes (#125), watcher cadence, server snapshots and scheduling behavior are excluded.

## Constraints and design

Following [[principles/subtract-before-you-add]] and [[principles/boundary-discipline]], separate static log interpretation from live observation. Keep the ClaimsReader interface and fixtures unchanged. Cache only canonical-derived claims; read and merge the small spawn.json on every call. Missing canonical continues returning empty claims without spawn enrichment. Malformed spawn remains ignored. Never cache a failed canonical parse.

Three approaches were considered: skipping terminal sessions is cheap but misses corrected input and liveness; caching full metadata needs a multi-source dependency graph and masks external writers; caching canonical parsing alone removes the expensive work while preserving existing ownership. Choose the third, with independent idempotent metadata persistence per [[principles/make-operations-idempotent]]. Full reparsing on changes avoids incremental offset/replay complexity.

A cached generation includes file identity, size, mtime, mode and change time. Use os.SameFile plus Darwin/Linux ctime; other platforms safely read without caching until a trustworthy change-time implementation exists. This preserves Windows correctness and compilation, without claiming a Windows read optimization. Same-size replacement with preserved mtime invalidates by identity; in-place corrections with restored mtime invalidate when the OS reports a changed ctime. This is an OS-metadata change detector, not a cryptographically unique generation: filesystem timestamp collisions or undetectable rewrites are outside this guarantee. No content hash or periodic full scan is introduced, because stable histories must avoid full reads. Compare the opened file generation before/after scanning and the current path before publishing a cache entry; concurrent mutation may retain existing snapshot semantics but cannot mark a partial generation reusable. Cache hits remain point-in-time observations and changes after stat are detected next cycle.

Serialize reader cache mutation and Monitor.RunOnce. Prune canonical cache entries against current session IDs through a concrete reader method, without widening the public ClaimsReader contract. Memory is bounded by extant session count; restarts reconstruct from disk. Observer and prior metadata reads continue each pass, so PID death, heartbeat expiry, runtime corrections, removed metadata and retry-count changes remain visible. A failed repair is retried on following passes.

Terminal IdleSeconds becomes zero (idle age describes live sessions); terminal duration ends at the final event using effective alive state. Running duration and idle age continue advancing. UpdatedAt becomes the time of a substantive derived change, not the last poll. Compare derived metadata with the disk value excluding UpdatedAt, comparing time values semantically across JSON round-trips; unchanged values retain the timestamp and cause no write. Existing corrupt metadata errors remain visible. The current UI derives live duration independently from LoopState; neither UpdatedAt nor IdleSeconds has a production consumer.

## Applicable skills

Repo plan, worktree, debugging, go-best-practices, testing, execute, commit and adversarial-review; tech-doc-writing for this document. The user explicitly authorizes independent review only within the current environment, overriding the external opposite-model CLI requirement. Use isolated-context read-only reviewers with fork_turns none; no external review CLI or payload transmission. Implementation stays in the Noodle-managed codex/124-session-metadata worktree; brain changes stay on primary. Preserve unrelated #121 backlog edits.

## Phases

- [ ] [[plans/124-session-metadata-refresh/phase-01-regressions]]
- [ ] [[plans/124-session-metadata-refresh/phase-02-file-generation]]
- [ ] [[plans/124-session-metadata-refresh/phase-03-canonical-cache]]
- [ ] [[plans/124-session-metadata-refresh/phase-04-terminal-time]]
- [ ] [[plans/124-session-metadata-refresh/phase-05-metadata-persistence]]
- [ ] [[plans/124-session-metadata-refresh/phase-06-liveness-verification]]
- [ ] [[plans/124-session-metadata-refresh/phase-07-runtime-verification]]

## Verification

Commit red regressions before production changes. Assert actual canonical scan counts and unchanged meta bytes/inode/mtime, rather than merely equal JSON. Include deterministic replacement during scanning and a non-UTC timestamp round-trip. Verify append/truncate/replacement/in-place corrections, missing/create, parse-error recovery, fresh spawn, external metadata, cold restart and active liveness without new logs. Preserve scheduler terminal-repair exemption and loop deadline tests. Run race tests for monitor/loop, pnpm check, and local TestSmokeStartOnce e2e only; model-backed smoke is excluded by the no-external-review/data boundary. Cross-compile monitor tests for Linux and Windows.

After independent code review and merge, build with pnpm build and source-identified local version, atomically replace the authorized local executable and gracefully restart only the existing idle playground process. Measure cumulative process CPU and all metadata file generations over the same 15-second window with unchanged session/order/mode state. Record residual tickets churn separately; do not expand into #125. Archive the plan and complete the backlog only after this evidence is recorded, per [[principles/prove-it-works]].
