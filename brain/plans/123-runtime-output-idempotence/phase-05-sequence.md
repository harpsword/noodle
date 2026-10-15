# Phase 5: Keep unchanged applied command sequence stable

Back to [[plans/123-runtime-output-idempotence/overview]]

## Goal
Keep unchanged applied command sequence stable.

## Changes
- loop/control.go — use the opt-in writer for nonzero applied seq.
- loop/control_test.go — extend sequence persistence stability coverage.

## Data structures
lastAppliedSeq and its decimal-plus-newline on-disk representation.

## Routing and principles
Implementation: Codex, current configured model; independent review: Claude CLI. Apply [[principles/fix-root-causes]], [[principles/make-operations-idempotent]] and [[principles/prove-it-works]].

## Verification
Static: gofmt, scoped tests and vet; full pnpm check at integrated verification. Runtime: Preserve zero-seq no-file behavior, stable nonzero file identity, advance persisted seq on new commands, restore deleted output, and keep replay suppression/flush error ordering tests green.
