# Phase 1: Remove output-triggered runtime cycles

Back to [[plans/123-runtime-output-idempotence/overview]]

## Goal
Remove output-triggered runtime cycles.

## Changes
- loop/loop.go — accept only actual runtime inputs in shouldTriggerRuntimeCycle; retain ticker and drains.
- loop/loop_run_test.go — update allowlist/coalescing tests for orders-next and ignored orders.
- loop/runtime_output_idle_test.go — exercise real fsnotify and full cycle flush with deterministic fake business dependencies.

## Data structures
fsnotify.Event, OrdersFile, Loop canonical state; no schema change.

## Routing and principles
Implementation: Codex, current configured model; independent review: Claude CLI. Apply [[principles/fix-root-causes]], [[principles/make-operations-idempotent]] and [[principles/prove-it-works]].

## Verification
Static: gofmt, scoped tests and vet; full pnpm check at integrated verification. Runtime: Assert output replacement does not wake; control/orders-next create/write/rename do wake; no cycle storm across idle, paused, failed-only, review-waiting and active-without-output. Use generous bounded-cycle assertions rather than exact timing counts. Existing completion/merge and input-processing tests protect the one-second progress bound.
