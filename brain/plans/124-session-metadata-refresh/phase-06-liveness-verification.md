# Phase 6: liveness verification

Back to [[plans/124-session-metadata-refresh/overview]]

## Goal

Prove cached claims do not suppress observation.

## Changes

monitor/metadata_liveness_test.go. Follow [[principles/prove-it-works]] and [[principles/boundary-discipline]].

## Data structures

Real PID observer and fake-clock heartbeat observer use the normal Monitor path.

## Routing

Current-environment Codex implementation; independent local reviewers for review gates. No external CLI delegation.

## Verification

Static: gofmt, scoped tests and relevant cross-platform compilation; final pnpm check. Runtime: Process exits without log append; heartbeat expires without file change; corrected spawn switches observer/scheduler policy; race tests pass.
