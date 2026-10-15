# Scheduler Self-Retrigger After Empty Orders

Status: open; tracked by backlog item 120. Recorded on 2026-10-14.

## Observed behavior

In the local `noodle_playground` project (Noodle v0.1.5, macOS, process runtime, Codex / gpt-6.1-sol, supervised mode), schedule sessions repeatedly started while the only execution order was active. Each scheduler found no additional actionable work, wrote `{"orders":[]}`, and exited. New sessions started approximately every minute instead of respecting the five-minute empty-result cooldown.

Evidence from `.noodle/loop-events.ndjson`: schedule completion events occurred at 23:55:43, 23:56:35, 23:57:28, 23:58:19, and 23:58:58 on 2026-10-13 (Asia/Shanghai). These were distinct sessions, not one stuck agent or duplicate dashboard rendering. Their session logs reported empty orders and avoidance of duplicate dispatch.

After the execution order merged and the backlog became empty, the final scheduler exited and the loop entered `idle`. This issue concerns unnecessary scheduling while execution remains active, not a claim that scheduling continues indefinitely after all work finishes.

## Source-level diagnosis

The observed behavior is consistent with a feedback loop in the current implementation:

1. `mise/builder.go: Builder.Build` compares the entire brief except `GeneratedAt`. Active summaries, resources, recent history, and recent events participate in `miseChanged`; this is broader than backlog changes.
2. Scheduler start/completion changes those fields. `loop/cook_completion.go: handleScheduleCompletion` emits `schedule.completed` and removes the schedule order.
3. `loop/loop_cycle_pipeline.go: handlePromotionResult` sets a five-minute cooldown after an empty promotion.
4. `prepareOrdersForCycle` clears that cooldown whenever `miseChanged`, despite its comment describing backlog changes.
5. While execution is active, `ensureScheduleIfNeeded` skips the no-active-work bootstrap/idle path. Its separate `miseChanged` branch injects a new schedule order whenever no scheduler or schedule order exists, without consulting the cooldown.

Together, scheduler lifecycle changes can request another scheduler even though no actionable scheduling input changed. This diagnosis is based on logs and source inspection; a deterministic regression test is still needed.

## Reproduction scenario

1. Configure a project with backlog, schedule, and execute skills and enough concurrency for scheduling alongside execution.
2. Add one task and keep its execute stage active long enough to observe several cycles.
3. Let the scheduler run while there is no other work to dispatch; have it atomically write `{"orders":[]}` and exit.
4. Observe whether its own completion/activity changes trigger another schedule session before the cooldown expires, without a backlog update or execution outcome.

Prefer a controlled fake runtime for the regression test; repeated real model calls are unnecessary.

## Proposed fix and acceptance

Separate meaningful scheduling triggers from full brief/projection changes. Scheduler lifecycle and its own history must not independently trigger another scheduling run. Backlog changes, execution outcomes, explicit operator requests, and other actionable inputs should remain capable of waking the scheduler. Define and test capacity-change handling explicitly.

Preserve empty-result cooldown unless meaningful scheduling input changes. Apply the same eligibility rules to both bootstrap and mise-change injection paths. While an execution is ongoing and no new work is available, wait for its outcome instead of repeatedly invoking a model.

- Empty orders followed only by scheduler lifecycle changes do not repeatedly spawn sessions during cooldown.
- A new actionable backlog item can wake scheduling promptly.
- Execution completion/failure and explicit operator requests trigger the necessary scheduling evaluation.
- No duplicate scheduler is dispatched while one is active.
- With no pending work or execution, the loop becomes idle.
- Test cooldown expiry and deliberate handling of blocked or already-running backlog items.

## Related merge blocker

The observed execution had also failed repeatedly because `worktree/app.go: assertRootClean` rejects any staged or unstaged tracked changes in the integration checkout. The dirty files were `.noodle.toml` and `todos.md`, including the normal addition of the task itself. Committing these files locally removed the blocker; execution subsequently merged, and the adapter marked the task done, making `todos.md` dirty again.

This is a separate issue from scheduler self-retriggering. A follow-up design should preserve local edits, distinguish overlapping merge changes from unrelated edits, and retain completed deliverables for merge retry rather than rerunning execution. Do not resolve it by blindly stashing or discarding user changes.

See also [[schedule-bootstrap-hot-loop]], [[worktree-gotchas]], [[../principles/make-operations-idempotent]], [[../principles/fix-root-causes]].
