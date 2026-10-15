# Phase 7: runtime verification

Back to [[archive/plans/124-session-metadata-refresh/overview]]

## Goal

Verify the integrated executable and record measured effects.

## Changes

brain/plans/124-session-metadata-refresh/verification.md; brain/plans/124-session-metadata-refresh/quality-review.md. Follow [[principles/prove-it-works]] and [[principles/boundary-discipline]].

## Data structures

Verification evidence records version, process state, CPU seconds and metadata generations.

## Routing

Current-environment Codex implementation; independent local reviewers for review gates. No external CLI delegation.

## Verification

Static: gofmt, scoped tests and relevant cross-platform compilation; final pnpm check. Runtime: pnpm check, race tests, local smoke, cross-compilation, independent code review, real binary restart and comparable CPU samples. Archive via repository backlog workflow.
