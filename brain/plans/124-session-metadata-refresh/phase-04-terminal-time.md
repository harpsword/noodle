# Phase 4: terminal time

Back to [[plans/124-session-metadata-refresh/overview]]

## Goal

Stop terminal elapsed/idle clocks while preserving live timers.

## Changes

monitor/derive.go; monitor/derive_test.go. Follow [[principles/prove-it-works]] and [[principles/boundary-discipline]].

## Data structures

SessionMeta uses effective Alive to select terminal duration; terminal IdleSeconds is zero.

## Routing

Current-environment Codex implementation; independent local reviewers for review gates. No external CLI delegation.

## Verification

Static: gofmt, scoped tests and relevant cross-platform compilation; final pnpm check. Runtime: Completed-but-alive ordinary sessions stop timing, persistent scheduler remains running, live duration advances.
