# Phase 3: Keep unchanged projection files stable

Back to [[plans/123-runtime-output-idempotence/overview]]

## Goal
Keep unchanged projection files stable.

## Changes
- internal/projection/projection.go — route orders/state output through opt-in writer.
- internal/projection/projection_test.go — prove per-file stability and repair.

## Data structures
projection.Bundle and its existing deterministic serialized outputs.

## Routing and principles
Implementation: Codex, current configured model; independent review: Claude CLI. Apply [[principles/fix-root-causes]], [[principles/make-operations-idempotent]] and [[principles/prove-it-works]].

## Verification
Static: gofmt, scoped tests and vet; full pnpm check at integrated verification. Runtime: Repeat identical bundle without replacing either file; changed order/state is emitted correctly; deleting/corrupting one output repairs it independently without touching the other. Retain valid JSON and no-temp-file tests.
