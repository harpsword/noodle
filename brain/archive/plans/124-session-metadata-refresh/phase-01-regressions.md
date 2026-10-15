# Phase 1: regressions

Back to [[archive/plans/124-session-metadata-refresh/overview]]

## Goal

Reproduce repeated work before changing production code.

## Changes

monitor/claims_cache_test.go; monitor/metadata_refresh_test.go; monitor/derive_test.go. Follow [[principles/prove-it-works]] and [[principles/boundary-discipline]].

## Data structures

Use existing reader/monitor types and test helpers.

## Routing

Current-environment Codex implementation; independent local reviewers for review gates. No external CLI delegation.

## Verification

Static: gofmt, scoped tests and relevant cross-platform compilation; final pnpm check. Runtime: Stable scan count, stable historical metadata, terminal clocks, source correction and liveness scenarios. Keep failing tests in a separate commit.
