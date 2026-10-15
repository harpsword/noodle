# Phase 6: Verify integrated behavior, review code and measure CPU

Back to [[plans/123-runtime-output-idempotence/overview]]

## Goal
Verify integrated behavior, review code and measure CPU.

## Changes
- loop/runtime_progress_test.go — exercise actual Run completion/merge progress without file input.
- brain/plans/123-runtime-output-idempotence/verification.md — record command results, review disposition and reproducible before/after measurements.

## Data structures
Process CPU-time deltas, file bytes/identity, loop status, order state; separate subprocess/browser costs.

## Routing and principles
Implementation: Codex, current configured model; independent review: separate current-environment agents, as explicitly requested by the user. Apply [[principles/fix-root-causes]], [[principles/make-operations-idempotent]] and [[principles/prove-it-works]].

## Verification
Static: gofmt, scoped tests and vet; full pnpm check at integrated verification. Runtime: Run full checks and real-fsnotify tests, independently review implementation, then version-label/install/restart and measure the actual failed-only loop. Require zero stable-output changes and a material CPU reduction; disclose residual once-per-second work. Complete and archive the plan only after verification succeeds.
