# Phase 5: metadata persistence

Back to [[archive/plans/124-session-metadata-refresh/overview]]

## Goal

Publish only changed metadata and bound cache lifecycle.

## Changes

monitor/monitor.go; monitor/metadata_refresh_test.go. Follow [[principles/prove-it-works]] and [[principles/boundary-discipline]].

## Data structures

A RunOnce mutex serializes maintenance; existing reader exposes concrete cache pruning.

## Routing

Current-environment Codex implementation; independent local reviewers for review gates. No external CLI delegation.

## Verification

Static: gofmt, scoped tests and relevant cross-platform compilation; final pnpm check. Runtime: Stable bytes/inode/mtime and returned metadata agree; removed metadata repairs, external retry count survives, corrupt metadata errors, removed-session cache prunes; non-UTC timestamp round-trips do not cause repeated writes.
