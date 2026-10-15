# Phase 2: Introduce opt-in content-idempotent atomic output

Back to [[plans/123-runtime-output-idempotence/overview]]

## Goal
Introduce opt-in content-idempotent atomic output.

## Changes
- internal/filex/filex.go — add WriteFileAtomicIfChanged; existing WriteFileAtomic stays unchanged.
- internal/filex/filex_test.go — cover file stability, replacement, repair and errors.

## Data structures
Disk bytes and file identity; no long-lived cache.

## Routing and principles
Implementation: Codex, current configured model; independent review: separate current-environment agents, as explicitly requested by the user. Apply [[principles/fix-root-causes]], [[principles/make-operations-idempotent]] and [[principles/prove-it-works]].

## Verification
Static: gofmt, scoped tests and vet; full pnpm check at integrated verification. Runtime: Cover equal bytes preserving identity/mtime, changed content atomically visible, missing/deleted/corrupt output repair, and replacement errors/temp cleanup. Set old timestamps instead of sleeping. Document unchanged-mode behavior and no new fsync promise.
