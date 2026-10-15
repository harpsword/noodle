# Phase 3: canonical cache

Back to [[plans/124-session-metadata-refresh/overview]]

## Goal

Reuse successful parsing while retaining fresh source metadata.

## Changes

monitor/claims.go; monitor/claims_cache.go; monitor/claims_cache_test.go. Follow [[principles/prove-it-works]] and [[principles/boundary-discipline]].

## Data structures

Canonical cache entry contains file generation and canonical-only SessionClaims; a mutex serializes access.

## Routing

Current-environment Codex implementation; independent local reviewers for review gates. No external CLI delegation.

## Verification

Static: gofmt, scoped tests and relevant cross-platform compilation; final pnpm check. Runtime: Count scans on stable reads; reparse append/truncate/replace/correction; retry errors; preserve spawn freshness and missing-log semantics. Deterministically replace a file during a scan and prove the result is not reused.
